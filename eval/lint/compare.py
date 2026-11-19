"""Total the lint tables that ABBD_LINT_FIXTURES writes, and hold each later
table to the gate of docs/blueprint-svg.md ("What a change is weighed by")
against the first.

    python compare.py base baseline.tsv try try.tsv

For each later table it checks every ceiling of the gate whose column both
tables carry: each such total stays at or under the first table's, and the
placement debt rises by at most 2% of it. It lists the views whose hard
breaches rose, which break the gate unless their debt records them, and the
views whose weighed sum rose by more than 8, which need a look by eye. The
traced bends and the paths that turn more than twice are tie-breakers.

The tables carry no against_axis column and no page bounds, so this script
cannot check them. TestLintFixtures and abbd lint check the whole gate per
fixture against its own .abbd/lint-debt.yaml.
"""
import csv
import sys

LOOK = 8
TIES = ["traced_bends", "paths_over2"]
# The ceilings of internal/screen/gate.go, by table column, with the slack
# each may pass by as a divisor of the base total (0 for none). overflow is
# the views that break S, so its column is S.
CEILINGS = [
    ("weighed", "weighed", 0),
    ("tees", "tees", 0),
    ("excess_turns", "excess_turns", 0),
    ("placement", "placement", 50),
    ("detours", "detours", 0),
    ("two_lane_off", "two_lane_off", 0),
    ("opposite_shared", "opposite_shared", 0),
    ("overflow", "S", 0),
]


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
    print(f"\n{names[0]} to {name}:")
    have = set(next(iter(base.values()))) & set(next(iter(t.values())))
    for label, col, rise in CEILINGS:
        if col not in have:
            print(f"{label}: not in both tables, not checked")
            continue
        b0, b1 = total(base, col), total(t, col)
        most = b0 + (b0 // rise if rise else 0)
        print(f"{label} {b0} -> {b1}, at most {most}, {'kept' if b1 <= most else 'broken'}")
    print("tie-breakers " + ", ".join(f"{c} {total(base, c)} -> {total(t, c)}" for c in TIES))
    harder = [f"{v}\thard {r['hard']} -> {t[v]['hard']}" for v, r in base.items() if int(t[v]["hard"]) > int(r["hard"])]
    print(f"\n{len(harder)} views with more hard breaches:")
    print("\n".join(harder))
    look = [f"{v}\tweighed {r['weighed']} -> {t[v]['weighed']}" for v, r in base.items() if int(t[v]["weighed"]) - int(r["weighed"]) > LOOK]
    print(f"\n{len(look)} views need a visual look, their weighed sum up by more than {LOOK}:")
    print("\n".join(look))
