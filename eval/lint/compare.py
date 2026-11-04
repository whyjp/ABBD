"""Total the lint tables that ABBD_LINT_FIXTURES writes and list the views
that get worse from the first table to each later one.

    python compare.py base baseline.tsv try try.tsv

A view is worse when it has more hard breaches, more steps, or more of any
total that has a ceiling in internal/screen/lint_test.go.
"""
import csv
import sys

WORSE = ["hard", "steps", "P1", "P2", "H1", "H2", "O1", "O2", "F1", "phantoms", "paths_over2", "traced_bends", "back", "weighed"]


def load(path):
    with open(path, newline="", encoding="utf8") as f:
        return {r["view"]: r for r in csv.DictReader(f, delimiter="\t")}


names, tables = sys.argv[1::2], [load(p) for p in sys.argv[2::2]]
cols = [c for c in next(iter(tables[0].values())) if c != "view"]
print("table\tviews\t" + "\t".join(cols))
for name, t in zip(names, tables):
    print(f"{name}\t{len(t)}\t" + "\t".join(str(sum(int(r[c]) for r in t.values())) for c in cols))
base = tables[0]
for name, t in zip(names[1:], tables[1:]):
    worse = []
    for view, r in base.items():
        if view not in t:
            sys.exit(f"{name} lacks {view}")
        bits = [f"{c} {r[c]} -> {t[view][c]}" for c in WORSE if int(t[view][c]) > int(r[c])]
        if bits:
            worse.append(f"{view}\t" + ", ".join(bits))
    print(f"\n{len(worse)} views worse from {names[0]} to {name}:")
    print("\n".join(worse))
