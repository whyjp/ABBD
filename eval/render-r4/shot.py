"""Screenshot one section of a blueprint page at desktop width through Aside.

    python eval/render-r4/shot.py URL ANCHOR OUT.png

URL is the page over local http, for example http://127.0.0.1:28474/page.html
after python -m http.server 28474 --bind 127.0.0.1 in its folder. The page
keeps only the section ANCHOR and is taken in window-high tiles, each by its
own short Aside call, then stitched. A tile whose call times out or fails is
taken again, on a freshly opened tab if the old one is gone, so one stalled
call costs one tile instead of the whole shot.
"""
import os
import re
import secrets
import shutil
import subprocess
import sys
import tempfile

from PIL import Image

W, TILE, TRIES, CALL_TIMEOUT = 1440, 800, 5, 75

url, anchor, out = sys.argv[1], sys.argv[2], os.path.abspath(sys.argv[3])
token = secrets.token_hex(4)
page_url = f"{url}?shot={token}"

ISOLATE = f"""
const isolate = async (pg) => {{
  await pg.setViewportSize({{width:{W},height:{TILE}}});
  for (let i = 0; i < 60; i++) {{ if (await pg.evaluate(() => !!document.getElementById('{anchor}'))) break; await sleep(250); }}
  return await pg.evaluate(() => {{
    const s = document.getElementById('{anchor}'); const m = document.querySelector('main');
    for (const c of [...m.children]) if (c !== s) c.remove();
    s.style.display = 'block';
    for (const c of [...document.body.children]) if (c !== m) c.remove();
    m.style.marginTop = '0px';
    const r = document.documentElement; r.style.height = ''; r.style.overflow = ''; document.body.style.height = ''; document.body.style.overflow = '';
    window.scrollTo(0, 0);
    return document.documentElement.scrollHeight;
  }});
}};
const find = async () => {{
  const t = (await listBrowserTabs()).find(t => (t.url || '').includes('shot={token}'));
  return t ? await attachBrowserTab(t.targetId) : null;
}};
"""


def repl(js):
    try:
        r = subprocess.run(["aside", "repl", ISOLATE + js], stdin=subprocess.DEVNULL, capture_output=True,
                           text=True, encoding="utf8", errors="replace", timeout=CALL_TIMEOUT)
        return r.stdout
    except subprocess.TimeoutExpired:
        return ""


def value(text, key):
    m = re.search(rf"^{key}=(.*)$", text, re.M)
    return m.group(1).strip() if m else None


def total():
    for _ in range(TRIES):
        got = value(repl(f"""
let pg = await find();
if (!pg) pg = await openTab('{page_url}#{anchor}');
console.log('TOTAL=' + await isolate(pg));
"""), "TOTAL")
        if got:
            return int(got)
    sys.exit(f"FAILED {out}: the page did not open")


def tile(n, y, work):
    for _ in range(TRIES):
        got = value(repl(f"""
let pg = await find();
if (!pg) pg = await openTab('{page_url}#{anchor}');
await isolate(pg);
await pg.evaluate(yy => {{ const r = document.documentElement; r.style.height = '{TILE}px'; r.style.overflow = 'hidden'; document.body.style.height = '{TILE}px'; document.body.style.overflow = 'hidden'; document.querySelector('main').style.marginTop = (-yy) + 'px'; }}, {y});
await sleep(300);
const p = path.join(pwd, 'artifacts', 't{n}.png');
await pg.screenshot({{path: './artifacts/t{n}.png'}});
console.log('TILE=' + p);
"""), "TILE")
        if got and os.path.exists(got):
            dst = os.path.join(work, f"t{n}.png")
            shutil.copy(got, dst)
            return dst
    sys.exit(f"FAILED {out}: tile {n} at {y}")


def main():
    work = tempfile.mkdtemp(prefix="abbd-shot-")
    try:
        h = total()
        tiles = [(y, tile(n, y, work)) for n, y in enumerate(range(0, h, TILE))]
        first = Image.open(tiles[0][1])
        k = first.width / W
        canvas = Image.new("RGB", (first.width, int(h * k)))
        for y, p in tiles:
            im = Image.open(p)
            canvas.paste(im.crop((0, 0, im.width, min(im.height, int(TILE * k)))), (0, int(y * k)))
        canvas.save(out)
        print(out, canvas.size, len(tiles), "tiles")
    finally:
        repl("const pg = await find(); if (pg) await closeTab(pg);")
        shutil.rmtree(work, ignore_errors=True)


main()
