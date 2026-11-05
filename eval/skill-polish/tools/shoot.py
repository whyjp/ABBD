"""Capture whole blueprint drawings and check what hovering them shows.

    python shoot.py OUT REPO=BLUEPRINT_DIR... [--pages N|all] [--hover-shots N]

For each repository the overview and its N module pages with the most boxes
(2 by default) are opened in headless Chrome, dark mode, from file URLs. Each
view's drawing is saved whole as OUT/<repo>-<view>.png at its drawn size, so
nothing below the fold or past the right edge is lost.

Then every box and chip is hovered with the real mouse, and every line where
the mouse can reach it, and the page's own CSS decides what lights up. Four
checks run on what it shows:

  cut       a lit line end with no lit box or chip and no other lit line
            within 12px: the lit route stops partway
  stray     a lit box or chip, other than the hovered one, that no lit line
            reaches
  mixed     lit lines of two colours joined into one route
  line_cut  hovering a line lights a set whose loose ends do not all reach a
            box or chip, so the reader cannot see both ends
  hidden    a line the mouse can reach nowhere, because others cover it

and, without hovering, one more:

  overlap   two lines of different colours that share a track for 12px or
            more, so one paints over the other

OUT/<repo>-<view>-hover-<n>.png shows the hub's hover and the flagged hovers,
up to --hover-shots per view (6 by default). OUT/findings.json holds every
view's size, what a 1600x1100 shot at the top of the page would have cut, and
each finding with the hover that produced it. A summary table goes to stdout.
"""
import json
import math
import os
import sys
from collections import defaultdict

from playwright.sync_api import sync_playwright

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
  const wires = [...svg.querySelectorAll('path.w')].map((p, i) => {
    p.setAttribute('data-wi', i);
    const m = p.getScreenCTM(); const L = p.getTotalLength(); const pts = [];
    for (let s = 0; s <= L; s += %STEP%) { const q = p.getPointAtLength(s); pts.push(toPage(m, q.x, q.y)); }
    const q = p.getPointAtLength(L); pts.push(toPage(m, q.x, q.y));
    const cs = getComputedStyle(p);
    return {i, cls: p.getAttribute('class'), color: cs.stroke, width: parseFloat(cs.strokeWidth), pts};
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
  const wires = [...svg.querySelectorAll('path.w')].map(p => { const cs = getComputedStyle(p); return [eff(p) > 0.5, parseFloat(cs.strokeWidth), cs.stroke]; });
  const targets = [...svg.querySelectorAll('g.box, g.port')].map(g => eff(g) > 0.5);
  return {wires, targets};
}
"""

TOP_OF = r"""
([x, y]) => { const e = document.elementFromPoint(x, y); return e && e.hasAttribute('data-wi') ? parseInt(e.getAttribute('data-wi')) : -1; }
"""


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
        cx, cy, out = int(p[0] // self.cell), int(p[1] // self.cell), set()
        for i in range(cx - 1, cx + 2):
            for j in range(cy - 1, cy + 2):
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
            for o in hit_w:
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
    wires = geo["wires"]
    pts_of = {w["i"]: w["pts"] for w in wires}
    grid = Grid(wires)
    run = defaultdict(int)
    for w in wires:
        for p in w["pts"]:
            for o in grid.near(p, 1.5, pts_of):
                if o > w["i"] and wires[o]["color"] != w["color"]:
                    run[(w["i"], o)] += 1
    return [{"lines": [a, b], "colors": [wires[a]["color"], wires[b]["color"]], "px": round(n * STEP)}
            for (a, b), n in sorted(run.items()) if n * STEP >= 12]


def inside(a, b):
    return b[0] <= a[0] and b[1] <= a[1] and a[2] <= b[2] and a[3] <= b[3]


def center(r):
    return ((r[0] + r[2]) / 2, (r[1] + r[3]) / 2)


def shot(pg, geo, path):
    x, y, w, h = geo["rect"]
    pg.screenshot(path=path, clip={"x": x, "y": y, "width": w, "height": h})


def view(pg, url, sel, out, stem, hover_shots):
    pg.set_viewport_size({"width": OLD_W, "height": OLD_H})
    pg.goto(url)
    pg.wait_for_selector(sel + " svg")
    r = pg.evaluate("(s) => { const e = [...document.querySelector(s).querySelectorAll('svg')].map(v => v.getBoundingClientRect()); const b = e.reduce((a, c) => c.width * c.height > a.width * a.height ? c : a); return [b.left, b.top, b.width, b.height]; }", sel)
    seen_w = max(0, min(r[0] + r[2], OLD_W) - max(r[0], 0))
    seen_h = max(0, min(r[1] + r[3], OLD_H) - max(r[1], 0))
    cut_old = 1 - (seen_w * seen_h) / (r[2] * r[3])
    full = pg.evaluate("() => [document.documentElement.scrollWidth, document.documentElement.scrollHeight]")
    pg.set_viewport_size({"width": max(OLD_W, int(full[0])), "height": min(int(full[1]) + 20, 12000)})
    pg.add_style_tag(content="*, *::before, *::after { transition: none !important; animation: none !important; }")
    pg.evaluate("() => window.scrollTo(0, 0)")
    geo = pg.evaluate(GEOMETRY.replace("%STEP%", str(STEP)), sel)
    dims = {t["i"] for t in geo["targets"] if ("box" in t["cls"].split() or any(c[0] == "n" and c[1:].isdigit() for c in t["cls"].split()))
            and not any(o is not t and inside(t["rect"], o["rect"]) for o in geo["targets"])}
    shot(pg, geo, os.path.join(out, stem + ".png"))
    base = pg.evaluate(LIT)
    colors = {i: c for i, (_, _, c) in enumerate(base["wires"])}
    rec = {"view": stem, "width": round(geo["rect"][2]), "height": round(geo["rect"][3]), "old_cut": round(cut_old, 3),
           "boxes": sum(1 for t in geo["targets"] if "box" in t["cls"].split()), "lines": len(geo["wires"]),
           "cut": [], "stray": [], "mixed": [], "line_cut": [], "hidden": [], "overlap": overlaps(geo), "shots": []}
    flagged = []
    hub, hub_n = None, -1
    for t in geo["targets"]:
        if not t["hov"]:
            continue
        pg.mouse.move(*center(t["rect"]))
        lit = pg.evaluate(LIT)
        lit_w = [i for i, (on, _, _) in enumerate(lit["wires"]) if on]
        lit_t = [i for i, on in enumerate(lit["targets"]) if on]
        if len(lit_w) == len(lit["wires"]):
            continue
        loose, reached, mixed = route_check(lit_w, lit_t, geo, {i: c for i, (_, _, c) in enumerate(lit["wires"])})
        stray = [geo["targets"][i]["name"] for i in lit_t if i in dims and i != t["i"] and i not in reached]
        who = t["name"] or t["hov"]
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
    for w in geo["wires"]:
        spot = None
        for p in w["pts"][len(w["pts"]) // 2:] + w["pts"][:len(w["pts"]) // 2]:
            if pg.evaluate(TOP_OF, p) == w["i"]:
                spot = p
                break
        if spot is None:
            rec["hidden"].append(w["i"])
            continue
        pg.mouse.move(*spot)
        lit = pg.evaluate(LIT)
        on = [i for i, (_, sw, _) in enumerate(lit["wires"]) if sw > base["wires"][i][1] + 0.1]
        dim = [i for i, (o, _, _) in enumerate(lit["wires"]) if o]
        if dim and len(dim) < len(lit["wires"]):
            on = sorted(set(on) | set(dim))
        if not on:
            continue
        loose, reached, _ = route_check(on, list(range(len(geo["targets"]))), geo, colors)
        if loose or len(reached) < 2:
            rec["line_cut"].append({"line": w["i"], "lit": on, "loose": len(loose), "reaches": len(reached)})
            flagged.append(("line", spot, f"line {w['i']}"))
    picks = ([("box", hub, "hub " + (hub["name"] or hub["hov"]))] if hub else []) + [f for f in flagged if f[1] is not hub]
    for n, (kind, what, label) in enumerate(picks[:hover_shots]):
        pg.mouse.move(*(center(what["rect"]) if kind == "box" else what))
        p = os.path.join(out, f"{stem}-hover-{n}.png")
        shot(pg, geo, p)
        rec["shots"].append({"file": os.path.basename(p), "hover": label})
    pg.mouse.move(0, 0)
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
    with sync_playwright() as p:
        b = p.chromium.launch(channel="chrome")
        pg = b.new_page(color_scheme="dark", viewport={"width": OLD_W, "height": OLD_H})
        for repo, bp in repos:
            mods = pages_by_boxes(pg, bp)
            mods = mods if pages == "all" else mods[:int(pages)]
            todo = [("overview", os.path.join(bp, "index.html"), "section#overview")]
            todo += [(m, os.path.join(bp, m + ".html"), "section.page") for m in mods]
            for name, path, sel in todo:
                r = view(pg, file_url(path), sel, out, f"{repo}-{name}", hover_shots)
                r["repo"], r["page"] = repo, name
                recs.append(r)
        b.close()
    with open(os.path.join(out, "findings.json"), "w", encoding="utf8") as fh:
        json.dump(recs, fh, indent=1, ensure_ascii=False)
    print("view\tsize\told_cut\tboxes\tlines\tcut\tstray\tmixed\tline_cut\thidden\toverlap")
    for r in recs:
        print(f"{r['view']}\t{r['width']}x{r['height']}\t{r['old_cut']:.2f}\t{r['boxes']}\t{r['lines']}\t{len(r['cut'])}\t{len(r['stray'])}\t"
              f"{len(r['mixed'])}\t{len(r['line_cut'])}\t{len(r['hidden'])}\t{len(r['overlap'])}")


main()
