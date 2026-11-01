#!/bin/bash
# shot.sh PAGE ANCHOR OUT.png: screenshot one section of a blueprint page at
# desktop width through Aside. Serve the pages first, for example
# python -m http.server 28473 --bind 127.0.0.1 in the folder that holds PAGE.
# A section taller than the window is taken in window-high tiles, shifted up
# under a clipped body, and stitched.
page=$1; anchor=$2; out=$3
js=$(cat <<JS
const pg = await openTab('http://127.0.0.1:28473/$page');
await pg.setViewportSize({width:1440,height:1000});
for (let i=0;i<60;i++){ if (await pg.evaluate(()=>!!document.getElementById('$anchor'))) break; await sleep(500);}
const total = await pg.evaluate(() => { const s = document.getElementById('$anchor'); const m = document.querySelector('main'); for (const c of [...m.children]) if (c !== s) c.remove(); s.style.display = 'block'; for (const c of [...document.body.children]) if (c !== m) c.remove(); window.scrollTo(0,0); return document.documentElement.scrollHeight; });
for (let y = 0, n = 0; y < total; y += 800, n++) {
  await pg.evaluate(yy => { const r = document.documentElement; r.style.height = '800px'; r.style.overflow = 'hidden'; document.body.style.height = '800px'; document.body.style.overflow = 'hidden'; document.querySelector('main').style.marginTop = (-yy) + 'px'; }, y);
  await sleep(400);
  await pg.screenshot({path:'./artifacts/s' + n + '.png', fullPage: true});
  console.log('PART=' + n + ' ' + y + ' ' + total);
}
console.log('DIR='+pwd); await closeTab(pg);
JS
)
for try in 1 2 3 4 5; do
  timeout 170 aside repl "$js" < /dev/null > shot.log 2>&1
  dir=$(sed -n 's/^DIR=//p' shot.log | tr -d '\r')
  total=$(sed -n 's/^PART=0 0 //p' shot.log | tr -d '\r')
  # every tile must be there; a run that stalls is taken again
  [ -n "$dir" ] && [ -n "$total" ] && [ "$(grep -c '^PART=' shot.log)" -eq $(( (total + 799) / 800 )) ] && break
  dir=""
done
[ -n "$dir" ] || { echo "FAILED $out"; grep -i error shot.log | head -2; exit 1; }
python - "$dir" "$out" <<'PY'
import sys
from PIL import Image
d, out = sys.argv[1], sys.argv[2]
parts = [l.split() for l in open('shot.log', encoding='utf8', errors='replace') if l.startswith('PART=')]
tiles = [(int(p[1]), Image.open(f"{d}/artifacts/s{p[0][5:]}.png")) for p in parts]
total = int(parts[0][2])
k = tiles[0][1].width / 1440
canvas = Image.new('RGB', (tiles[0][1].width, int(total * k)))
for y, im in tiles:
    canvas.paste(im.crop((0, 0, im.width, min(im.height, int(800 * k)))), (0, int(y * k)))
canvas.save(out)
print(out, canvas.size)
PY
