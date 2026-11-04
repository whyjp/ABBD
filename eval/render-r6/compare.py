"""Total the path measures of ABBD_METRICS tables and list the views that get
worse from the first table to each later one.

    python compare.py main layer-order.tsv wide wide.tsv
"""
import csv
import sys

TOTALS = ["phantom", "jogs", "crossings", "bends", "back", "back_ink", "ink", "braid", "paths_over2"]
WORSE = ["phantom", "jogs", "paths_over2", "crossings", "bends", "back"]


def load(path):
    with open(path, newline="", encoding="utf8") as f:
        return {(r["fixture"], r["view"]): r for r in csv.DictReader(f, delimiter="\t")}


names, tables = sys.argv[1::2], [load(p) for p in sys.argv[2::2]]
print("table\tviews\t" + "\t".join(TOTALS))
for name, t in zip(names, tables):
    print(f"{name}\t{len(t)}\t" + "\t".join(str(sum(int(r[c]) for r in t.values())) for c in TOTALS))
base = tables[0]
for name, t in zip(names[1:], tables[1:]):
    worse = []
    for key, r in base.items():
        if key not in t:
            sys.exit(f"{name} lacks {key}")
        bits = [f"{c} {r[c]} -> {t[key][c]}" for c in WORSE if int(t[key][c]) > int(r[c])]
        if bits:
            worse.append(f"{key[0]}\t{key[1]}\t" + ", ".join(bits))
    print(f"\n{len(worse)} views worse from {names[0]} to {name}:")
    print("\n".join(worse))
