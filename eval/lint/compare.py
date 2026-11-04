"""Total the lint tables that ABBD_LINT_FIXTURES writes, and hold each later
table to the gate of docs/blueprint-svg.md against the first.

    python compare.py base baseline.tsv try try.tsv

For each later table it says whether the weighed sum stays at or under the
first table's and the placement debt rises by at most 2% of it. It lists
the views whose hard breaches rose, which break the gate unless their debt
records them, and the views whose weighed sum rose by more than 8, which
need a look by eye. The traced bends and the paths that turn more than
twice are tie-breakers. The gate itself is checked per fixture by
TestLintFixtures and abbd lint.
"""
import csv
import sys

LOOK = 8
TIES = ["traced_bends", "paths_over2"]


def load(path):
    with open(path, newline="", encoding="utf8") as f:
        return {r["view"]: r for r in csv.DictReader(f, delimiter="\t")}


def total(t, c):
    return sum(int(r[c]) for r in t.values())


names, tables = sys.argv[1::2], [load(p) for p in sys.argv[2::2]]
cols = [c for c in next(iter(tables[0].values())) if c != "view"]
print("table\tviews\t" + "\t".join(cols))
for name, t in zip(names, tables):
    print(f"{name}\t{len(t)}\t" + "\t".join(str(total(t, c)) for c in cols))
base = tables[0]
for name, t in zip(names[1:], tables[1:]):
    missing = [v for v in base if v not in t]
    if missing:
        sys.exit(f"{name} lacks {', '.join(missing)}")
    w0, w1 = total(base, "weighed"), total(t, "weighed")
    p0, p1 = total(base, "placement"), total(t, "placement")
    print(f"\n{names[0]} to {name}:")
    print(f"weighed {w0} -> {w1}, {'kept' if w1 <= w0 else 'broken'}")
    print(f"placement {p0} -> {p1}, at most {p0 + p0 // 50}, {'kept' if p1 <= p0 + p0 // 50 else 'broken'}")
    print("tie-breakers " + ", ".join(f"{c} {total(base, c)} -> {total(t, c)}" for c in TIES))
    harder = [f"{v}\thard {r['hard']} -> {t[v]['hard']}" for v, r in base.items() if int(t[v]["hard"]) > int(r["hard"])]
    print(f"\n{len(harder)} views with more hard breaches:")
    print("\n".join(harder))
    look = [f"{v}\tweighed {r['weighed']} -> {t[v]['weighed']}" for v, r in base.items() if int(t[v]["weighed"]) - int(r["weighed"]) > LOOK]
    print(f"\n{len(look)} views need a visual look, their weighed sum up by more than {LOOK}:")
    print("\n".join(look))
