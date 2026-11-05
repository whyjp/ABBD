"""Capture whole blueprint drawings and check what hovering them shows.

    python shoot.py OUT REPO=BLUEPRINT_DIR... [--pages N|all] [--hover-shots N]

For each repository the overview and its N module pages with the most boxes
(2 by default) are opened in the installed Chrome (or $CHROME), headless, dark
mode, from file URLs, driven over the DevTools protocol with no other
dependency. Each view's drawing is saved whole as OUT/<repo>-<view>.png at its
drawn size, so nothing below the fold or past the right edge is lost. To do
that the viewport is widened to the page's scroll width and grown to its
height, capped at 12000px; a drawing taller than that is cut at the cap, and
a page whose layout follows the viewport width may reflow when widened, so
the shot shows the widened layout.

Then every box and chip is hovered with the real mouse, and every line where
the mouse can reach it, and the page's own CSS decides what lights up. These
checks run on what it shows:

  cut       a lit line end with no lit box or chip and no other lit line
            within 12px: the lit route stops partway
  stray     a lit box or chip, other than the hovered one, that no lit line
            reaches; a chip that sits on a box counts as reached when its box
            is lit, and is stray when it stays lit on a dimmed box
  mixed     lit lines of two colours joined into one route; two lines that
            meet within 8px of a box only share its anchor and are not joined
  line_cut  hovering a line lights a set whose loose ends do not all reach a
            box or chip, so the reader cannot see both ends
  hidden    a line the mouse can reach nowhere, because others cover it
  all_lit   a box hover that lights every line of a view that has lines; it
            is checked like any other hover and listed, since it is also
            what a hover the page ignores would show

and, without hovering, one more:

  overlap   two lines of different colours that run together along one track
            for 12px or more without a break, so one paints over the other

OUT/<repo>-<view>-hover-<n>.png shows the hub's hover and the flagged hovers,
up to --hover-shots per view (6 by default). OUT/findings.json holds every
view's size, what a 1600x1100 shot at the top of the page would have cut, and
each finding with the hover that produced it. A summary table goes to stdout.
"""
import base64
import json
import math
import os
import shutil
import socket
import struct
import subprocess
import sys
import tempfile
import time
import urllib.request
from collections import defaultdict

TOUCH_BOX, TOUCH_LINE, STUB, STEP = 8.0, 3.0, 12.0, 2.0
OLD_W, OLD_H = 1600, 1100

GEOMETRY = r"""
(sel) => {
  const sec = document.querySelector(sel);
  const svgs = [...sec.querySelectorAll('svg')];
  const svg = svgs.reduce((a, b) => (b.getBoundingClientRect().width * b.getBoundingClientRect().height >
                                     a.getBoundingClientRect().width * a.getBoundingClientRect().height ? b : a));
  svg.setAttribute('data-shot', '1');
  const R = svg.getBoundingClientRect();
  const toPage = (m, x, y) => { const p = new DOMPoint(x, y).matrixTransform(m); return [p.x, p.y]; };
  const wires = [...svg.querySelectorAll('path.w, path.route, g.route path.way')].map((p, i) => {
    p.setAttribute('data-wi', i);
    const m = p.getScreenCTM(); const L = p.getTotalLength(); const pts = [];
    for (let s = 0; s <= L; s += %STEP%) { const q = p.getPointAtLength(s); pts.push(toPage(m, q.x, q.y)); }
    const q = p.getPointAtLength(L); pts.push(toPage(m, q.x, q.y));
    const cs = getComputedStyle(p);
    return {i, cls: p.getAttribute('class'), route: !p.classList.contains('w'), color: cs.stroke, width: parseFloat(cs.strokeWidth), pts};
  });
  const targets = [...svg.querySelectorAll('g.box, g.port')].map((g, i) => {
    g.setAttribute('data-ti', i);
    const r = (g.querySelector('rect') || g).getBoundingClientRect();
    const name = (g.querySelector('text.name') || g.querySelector('text') || {textContent: ''}).textContent.trim();
    const hov = (g.getAttribute('class').match(/\bp\d+\b/) || [null])[0];
    return {i, cls: g.getAttribute('class'), name, hov, rect: [r.left, r.top, r.right, r.bottom]};
  });
  return {rect: [R.left, R.top, R.width, R.height], wires, targets};
}
"""

LIT = r"""
() => {
  const svg = document.querySelector('svg[data-shot]');
  const eff = (el) => { let o = 1; for (let e = el; e && e !== svg.parentNode; e = e.parentElement) o *= parseFloat(getComputedStyle(e).opacity); return o; };
  const wires = [...svg.querySelectorAll('path.w, path.route, g.route path.way')].map(p => { const cs = getComputedStyle(p); return [eff(p) > 0.5, parseFloat(cs.strokeWidth), cs.stroke]; });
  const targets = [...svg.querySelectorAll('g.box, g.port')].map(g => eff(g) > 0.5);
  return {wires, targets};
}
"""

TOP_OF = r"""
([x, y]) => { const e = document.elementFromPoint(x, y); return e && e.hasAttribute('data-wi') ? parseInt(e.getAttribute('data-wi')) : -1; }
"""


class Page:
    """One headless Chrome tab driven over the DevTools protocol."""

    FLAGS = ["--headless=new", "--remote-debugging-port=0", "--hide-scrollbars", "--mute-audio", "--force-color-profile=srgb",
             "--blink-settings=primaryHoverType=2,availableHoverTypes=2,primaryPointerType=4,availablePointerTypes=4",
             "--enable-features=CDPScreenshotNewSurface", "--disable-features=PaintHolding,RenderDocument,Translate",
             "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-background-networking",
             "--disable-component-update", "--disable-renderer-backgrounding", "--disable-background-timer-throttling"]

    def __init__(self, width, height):
        self.dir = tempfile.mkdtemp(prefix="shoot-")
        self.proc = subprocess.Popen([chrome()] + self.FLAGS + [f"--user-data-dir={self.dir}", "about:blank"],
                                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        port_file = os.path.join(self.dir, "DevToolsActivePort")
        words = []
        for _ in range(300):
            # Chrome holds the file open while it writes it, and on Windows a
            # read then fails with PermissionError, so the read is retried.
            try:
                with open(port_file) as f:
                    words = f.read().split()
            except (FileNotFoundError, PermissionError):
                words = []
            if len(words) >= 2:
                break
            time.sleep(0.05)
        port = int(words[0])
        tabs = json.load(urllib.request.urlopen(f"http://127.0.0.1:{port}/json/list"))
        path = next(t["webSocketDebuggerUrl"] for t in tabs if t["type"] == "page").split(str(port), 1)[1]
        self.sock = socket.create_connection(("127.0.0.1", port))
        key = base64.b64encode(os.urandom(16)).decode()
        self.sock.sendall(f"GET {path} HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
                          f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n".encode())
        head = b""
        while b"\r\n\r\n" not in head:
            head += self.sock.recv(1)
        assert b" 101 " in head.split(b"\r\n", 1)[0], head
        self.next_id, self.events = 0, []
        self.send("Page.enable")
        self.send("Runtime.enable")
        self.send("Emulation.setFocusEmulationEnabled", enabled=True)
        self.send("Emulation.setEmulatedMedia", media="", features=[{"name": "prefers-color-scheme", "value": "dark"}])
        self.viewport(width, height)

    def _recv(self, n):
        buf = b""
        while len(buf) < n:
            chunk = self.sock.recv(n - len(buf))
            if not chunk:
                raise ConnectionError("Chrome closed the DevTools connection")
            buf += chunk
        return buf

    def _read(self):
        data, done = b"", False
        while not done:
            b0, b1 = self._recv(2)
            n = b1 & 0x7F
            if n == 126:
                n = struct.unpack(">H", self._recv(2))[0]
            elif n == 127:
                n = struct.unpack(">Q", self._recv(8))[0]
            payload, op, done = self._recv(n), b0 & 0x0F, bool(b0 & 0x80)
            if op == 0x8:
                raise ConnectionError("Chrome closed the DevTools connection")
            if op == 0x9:
                self._write(payload, 0xA)
                done = False
                continue
            data += payload
        return json.loads(data)

    def _write(self, payload, op=0x1):
        n = len(payload)
        size = bytes([0x80 | n]) if n < 126 else bytes([0xFE]) + struct.pack(">H", n) if n < 65536 else bytes([0xFF]) + struct.pack(">Q", n)
        mask = os.urandom(4)
        self.sock.sendall(bytes([0x80 | op]) + size + mask + bytes(c ^ mask[i % 4] for i, c in enumerate(payload)))

    def send(self, method, **params):
        self.next_id += 1
        self._write(json.dumps({"id": self.next_id, "method": method, "params": params}).encode())
        while True:
            m = self._read()
            if m.get("id") == self.next_id:
                if "error" in m:
                    raise RuntimeError(f"{method}: {m['error']}")
                return m["result"]
            if "method" in m:
                self.events.append(m["method"])

    def wait_event(self, name):
        while name not in self.events:
            m = self._read()
            if "method" in m:
                self.events.append(m["method"])

    def viewport(self, width, height):
        self.size = (width, height)
        self.send("Emulation.setDeviceMetricsOverride", width=width, height=height, deviceScaleFactor=1, mobile=False,
                  screenWidth=width, screenHeight=height, screenOrientation={"angle": 0, "type": "landscapePrimary"})

    def goto(self, url):
        self.events = []
        self.send("Page.navigate", url=url)
        self.wait_event("Page.loadEventFired")

    def evaluate(self, fn, arg=None):
        r = self.send("Runtime.evaluate", expression=f"({fn})({json.dumps(arg)})", returnByValue=True, awaitPromise=True)
        if "exceptionDetails" in r:
            raise RuntimeError(r["exceptionDetails"])
        return r["result"].get("value")

    def wait_for(self, sel):
        while not self.evaluate("(s) => !!document.querySelector(s)", sel):
            time.sleep(0.05)

    def style(self, css):
        self.evaluate("(c) => { const s = document.createElement('style'); s.textContent = c; document.head.appendChild(s); }", css)

    def move(self, x, y):
        self.send("Input.dispatchMouseEvent", type="mouseMoved", x=x, y=y, button="none", buttons=0, modifiers=0, force=0)

    def screenshot(self, path, x, y, w, h):
        self.evaluate("() => document.fonts.ready.then(() => 0)")
        vw, vh = self.size
        x0, y0, x1, y1 = max(0, min(x, vw)), max(0, min(y, vh)), max(0, min(x + w, vw)), max(0, min(y + h, vh))
        vv = self.send("Page.getLayoutMetrics")["cssVisualViewport"]
        clip = {"x": vv["pageX"] + x0, "y": vv["pageY"] + y0, "width": math.floor((x1 - x0) / vv["scale"] + 1e-3),
                "height": math.floor((y1 - y0) / vv["scale"] + 1e-3), "scale": vv["scale"]}
        data = self.send("Page.captureScreenshot", format="png", clip=clip, captureBeyondViewport=False)["data"]
        with open(path, "wb") as fh:
            fh.write(base64.b64decode(data))

    def close(self):
        self.sock.close()
        self.proc.terminate()
        self.proc.wait()
        shutil.rmtree(self.dir, ignore_errors=True)


def chrome():
    spots = [os.path.join(os.environ.get(v, ""), "Google", "Chrome", "Application", "chrome.exe") for v in ("PROGRAMFILES", "PROGRAMFILES(X86)", "LOCALAPPDATA")]
    spots.append("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
    found = os.environ.get("CHROME") or next((p for p in spots if os.path.exists(p)), None) or shutil.which("google-chrome") or shutil.which("chromium")
    if not found:
        sys.exit("no Chrome found; set CHROME to its path")
    return found


def near_rect(p, r, tol):
    dx = max(r[0] - p[0], 0, p[0] - r[2])
    dy = max(r[1] - p[1], 0, p[1] - r[3])
    return math.hypot(dx, dy) <= tol


class Grid:
    def __init__(self, wires, cell=4.0):
        self.cell, self.g = cell, defaultdict(set)
        for w in wires:
            for x, y in w["pts"]:
                self.g[(int(x // cell), int(y // cell))].add(w["i"])

    def near(self, p, tol, pts_of):
        cx, cy, out, r = int(p[0] // self.cell), int(p[1] // self.cell), set(), math.ceil(tol / self.cell)
        for i in range(cx - r, cx + r + 1):
            for j in range(cy - r, cy + r + 1):
                for wi in self.g.get((i, j), ()):
                    if wi not in out and any(math.hypot(p[0] - x, p[1] - y) <= tol for x, y in pts_of[wi]):
                        out.add(wi)
        return out


def route_check(lit_w, lit_t, geo, colors):
    """Loose ends, unreached targets and colour-mixed components of a lit set."""
    wires, targets = geo["wires"], geo["targets"]
    sub = [wires[i] for i in lit_w]
    pts_of = {w["i"]: w["pts"] for w in sub}
    grid = Grid(sub)
    loose, reached, adj = [], set(), defaultdict(set)
    for w in sub:
        for end in (w["pts"][0], w["pts"][-1]):
            hit_t = [t for t in lit_t if near_rect(end, targets[t]["rect"], TOUCH_BOX)]
            reached.update(hit_t)
            hit_w = grid.near(end, TOUCH_LINE, pts_of) - {w["i"]}
            at_box = any(near_rect(end, t["rect"], TOUCH_BOX) for t in targets)
            for o in hit_w if not at_box else ():
                adj[w["i"]].add(o)
                adj[o].add(w["i"])
            if not hit_t and not hit_w and not grid.near(end, STUB, pts_of) - {w["i"]}:
                loose.append((w["i"], [round(end[0]), round(end[1])]))
    seen, mixed = set(), []
    for w in lit_w:
        if w in seen:
            continue
        comp, stack = set(), [w]
        while stack:
            x = stack.pop()
            if x in comp:
                continue
            comp.add(x)
            stack.extend(adj[x] - comp)
        seen |= comp
        cs = sorted({colors[x] for x in comp})
        if len(cs) > 1:
            mixed.append({"lines": sorted(comp), "colors": cs})
    return loose, reached, mixed


def overlaps(geo):
    wires = [w for w in geo["wires"] if not w["route"]]
    pts_of = {w["i"]: w["pts"] for w in wires}
    grid = Grid(wires)
    run = {}
    for w in wires:
        cur = defaultdict(int)
        for p in w["pts"]:
            here = {o for o in grid.near(p, 1.5, pts_of) if o > w["i"] and wires[o]["color"] != w["color"]}
            cur = defaultdict(int, {o: cur[o] + 1 for o in here})
            for o, n in cur.items():
                run[(w["i"], o)] = max(run.get((w["i"], o), 0), n)
    return [{"lines": [a, b], "colors": [wires[a]["color"], wires[b]["color"]], "px": round((n - 1) * STEP)}
            for (a, b), n in sorted(run.items()) if (n - 1) * STEP >= 12]


def inside(a, b):
    return b[0] <= a[0] and b[1] <= a[1] and a[2] <= b[2] and a[3] <= b[3]


def center(r):
    return ((r[0] + r[2]) / 2, (r[1] + r[3]) / 2)


def shot(pg, geo, path):
    pg.screenshot(path, *geo["rect"])


def view(pg, url, sel, out, stem, hover_shots):
    pg.viewport(OLD_W, OLD_H)
    pg.goto(url)
    pg.wait_for(sel + " svg")
    r = pg.evaluate("(s) => { const e = [...document.querySelector(s).querySelectorAll('svg')].map(v => v.getBoundingClientRect()); const b = e.reduce((a, c) => c.width * c.height > a.width * a.height ? c : a); return [b.left, b.top, b.width, b.height]; }", sel)
    seen_w = max(0, min(r[0] + r[2], OLD_W) - max(r[0], 0))
    seen_h = max(0, min(r[1] + r[3], OLD_H) - max(r[1], 0))
    cut_old = 1 - (seen_w * seen_h) / (r[2] * r[3])
    full = pg.evaluate("() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight]")
    pg.viewport(max(OLD_W, int(full[0])), min(int(full[1]) + 20, 12000))
    pg.style("*, *::before, *::after { transition: none !important; animation: none !important; }")
    pg.evaluate("() => window.scrollTo(0, 0)")
    geo = pg.evaluate(GEOMETRY.replace("%STEP%", str(STEP)), sel)
    on_box = {t["i"]: o["i"] for t in geo["targets"] for o in geo["targets"] if o is not t and inside(t["rect"], o["rect"])}
    shot(pg, geo, os.path.join(out, stem + ".png"))
    base = pg.evaluate(LIT)
    colors = {i: c for i, (_, _, c) in enumerate(base["wires"])}
    rec = {"view": stem, "width": round(geo["rect"][2]), "height": round(geo["rect"][3]), "old_cut": round(cut_old, 3),
           "boxes": sum(1 for t in geo["targets"] if "box" in t["cls"].split()), "lines": len(geo["wires"]),
           "cut": [], "stray": [], "mixed": [], "line_cut": [], "hidden": [], "all_lit": [], "overlap": overlaps(geo), "shots": []}
    flagged = []
    hub, hub_n = None, -1
    for t in geo["targets"]:
        if not t["hov"]:
            continue
        pg.move(*center(t["rect"]))
        lit = pg.evaluate(LIT)
        lit_w = [i for i, (on, _, _) in enumerate(lit["wires"]) if on]
        lit_t = [i for i, on in enumerate(lit["targets"]) if on]
        who = t["name"] or t["hov"]
        if lit_w and len(lit_w) == len(lit["wires"]):
            rec["all_lit"].append(who)
        loose, reached, mixed = route_check(lit_w, lit_t, geo, {i: c for i, (_, _, c) in enumerate(lit["wires"])})
        lit_set = set(lit_t) | {t["i"]}
        stray = [geo["targets"][i]["name"] for i in lit_t if i != t["i"]
                 and (on_box[i] not in lit_set if i in on_box else i not in reached)]
        if loose:
            rec["cut"].append({"hover": who, "ends": loose})
        if stray:
            rec["stray"].append({"hover": who, "targets": stray})
        if mixed:
            rec["mixed"].append({"hover": who, "routes": mixed})
        if loose or stray or mixed:
            flagged.append(("box", t, who))
        if len(lit_w) > hub_n:
            hub, hub_n = t, len(lit_w)
    routes = any(w["route"] for w in geo["wires"])
    for w in geo["wires"]:
        if w["route"]:
            continue
        spot = None
        for p in w["pts"][len(w["pts"]) // 2:] + w["pts"][:len(w["pts"]) // 2]:
            if pg.evaluate(TOP_OF, p) == w["i"]:
                spot = p
                break
        if spot is None:
            rec["hidden"].append(w["i"])
            continue
        pg.move(*spot)
        lit = pg.evaluate(LIT)
        on = [i for i, (_, sw, _) in enumerate(lit["wires"]) if sw > base["wires"][i][1] + 0.1]
        dim = [i for i, (o, _, _) in enumerate(lit["wires"]) if o]
        if routes:
            on = dim
        elif dim and len(dim) < len(lit["wires"]):
            on = sorted(set(on) | set(dim))
        if not on:
            continue
        loose, reached, _ = route_check(on, list(range(len(geo["targets"]))), geo, colors)
        if loose or len(reached) < 2:
            rec["line_cut"].append({"line": w["i"], "lit": on, "loose": len(loose), "reaches": len(reached)})
            flagged.append(("line", spot, f"line {w['i']}"))
    picks = ([("box", hub, "hub " + (hub["name"] or hub["hov"]))] if hub else []) + [f for f in flagged if f[1] is not hub]
    for n, (kind, what, label) in enumerate(picks[:hover_shots]):
        pg.move(*(center(what["rect"]) if kind == "box" else what))
        p = os.path.join(out, f"{stem}-hover-{n}.png")
        shot(pg, geo, p)
        rec["shots"].append({"file": os.path.basename(p), "hover": label})
    pg.move(0, 0)
    return rec


def pages_by_boxes(pg, bp):
    got = []
    for f in sorted(os.listdir(bp)):
        if f.startswith("MOD-") and f.endswith(".html"):
            pg.goto(file_url(os.path.join(bp, f)))
            n = pg.evaluate("() => { const s = document.querySelector('section.page'); return s ? s.querySelectorAll('g.box').length : -1; }")
            if n >= 0:
                got.append((-n, f[:-5]))
    return [v for _, v in sorted(got)]


def file_url(p):
    return "file:///" + os.path.abspath(p).replace("\\", "/")


def main():
    args = sys.argv[1:]
    out, pages, hover_shots, repos = args[0], "2", 6, []
    i = 1
    while i < len(args):
        if args[i] == "--pages":
            pages, i = args[i + 1], i + 2
        elif args[i] == "--hover-shots":
            hover_shots, i = int(args[i + 1]), i + 2
        else:
            repos.append(args[i].split("=", 1))
            i += 1
    os.makedirs(out, exist_ok=True)
    recs = []
    pg = Page(OLD_W, OLD_H)
    try:
        for repo, bp in repos:
            mods = pages_by_boxes(pg, bp)
            mods = mods if pages == "all" else mods[:int(pages)]
            todo = [("overview", os.path.join(bp, "index.html"), "section#overview")]
            todo += [(m, os.path.join(bp, m + ".html"), "section.page") for m in mods]
            for name, path, sel in todo:
                r = view(pg, file_url(path), sel, out, f"{repo}-{name}", hover_shots)
                r["repo"], r["page"] = repo, name
                recs.append(r)
    finally:
        pg.close()
    with open(os.path.join(out, "findings.json"), "w", encoding="utf8") as fh:
        json.dump(recs, fh, indent=1, ensure_ascii=False)
    print("view\tsize\told_cut\tboxes\tlines\tcut\tstray\tmixed\tline_cut\thidden\tall_lit\toverlap")
    for r in recs:
        print(f"{r['view']}\t{r['width']}x{r['height']}\t{r['old_cut']:.2f}\t{r['boxes']}\t{r['lines']}\t{len(r['cut'])}\t{len(r['stray'])}\t"
              f"{len(r['mixed'])}\t{len(r['line_cut'])}\t{len(r['hidden'])}\t{len(r['all_lit'])}\t{len(r['overlap'])}")


main()
