"""List supplement units whose evidence sits on an anchor a script unit of the same kind holds.

  python -I eval/collide5.py <script extraction.json> <supplement.json>

abbd v3 setup refuses such a unit ("would merge into it"). An anchor that is a directory covers every file under it.
"""
import json
import sys

script = json.load(open(sys.argv[1], encoding="utf-8"))
supp = json.load(open(sys.argv[2], encoding="utf-8"))
anchors = {}
for u in script["units"]:
    for a in u["anchors"]:
        anchors.setdefault(u["kind"], []).append((a["path"], u["key"]))
hits = []
with_ev = [u for u in supp["units"] if u.get("evidence")]
for u in with_ev:
    for e in u["evidence"]:
        hit = next(((p, k) for p, k in anchors.get(u["kind"], []) if e["path"] == p or e["path"].startswith(p + "/")), None)
        if hit:
            hits.append((u["kind"], u["key"], e["path"], hit[1], hit[0]))
            break
for h in hits:
    print("%s %s | evidence %s | script %s anchor %s" % h)
print("colliding %d of %d supplement units with evidence" % (len(hits), len(with_ev)))
