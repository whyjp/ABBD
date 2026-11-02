"""Fit the round-4 supplement to abbd v3's anchor rule.

  python -I eval/fixsupp5.py <script extraction.json> <supplement.json in> <supplement.json out>

A supplement unit that the script now emits (same key) becomes a description of that script unit: its evidence
goes and one of the script's anchors replaces its own. A unit the script does not emit keeps only the quotes that
sit off every anchor of a script unit of its kind. Each change is printed.
"""
import json
import sys

script = json.load(open(sys.argv[1], encoding="utf-8"))
supp = json.load(open(sys.argv[2], encoding="utf-8"))
by_key = {u["key"]: u for u in script["units"]}
anchors = {}
for u in script["units"]:
    for a in u["anchors"]:
        anchors.setdefault(u["kind"], []).append(a["path"])


def covered(kind, path):
    return any(path == p or path.startswith(p + "/") for p in anchors.get(kind, []))


for u in supp["units"]:
    if not u.get("evidence"):
        continue
    if u["key"] in by_key:
        s = by_key[u["key"]]
        u.pop("evidence")
        u["anchors"] = [s["anchors"][0]]
        u.pop("parent", None)
        print("now describes script unit", u["key"])
        continue
    keep = [e for e in u["evidence"] if not covered(u["kind"], e["path"])]
    if len(keep) != len(u["evidence"]):
        print("dropped %d quote(s) on script anchors from %s" % (len(u["evidence"]) - len(keep), u["key"]))
        u["evidence"] = keep
json.dump(supp, open(sys.argv[3], "w", encoding="utf-8"), indent=1, sort_keys=True, ensure_ascii=False)
