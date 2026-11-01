"""Compare round 4's before and after path metrics.

    python eval/render-r4/summarize.py

reads before.tsv, after.tsv and their .store.txt (Orca's real store) next
to this file, written by eval/render-r1/measure.py.
"""
import csv
import os
import re
import statistics

here = os.path.dirname(os.path.abspath(__file__))


def load(tag):
    rows = list(csv.DictReader(open(os.path.join(here, tag + ".tsv"), encoding="utf8"), delimiter="\t"))
    pat = r"(.*?): relations (\d+), lost (\d+), phantom (\d+), bends median ([\d.]+) max (\d+), path crossings median ([\d.]+) max (\d+), unmarked T (\d+)"
    for line in open(os.path.join(here, tag + ".tsv.store.txt"), encoding="utf8"):
        m = re.match(pat, line)
        if m:
            rows.append(dict(fixture="orca-store", view=m[1], lost=m[3], phantom=m[4], bends_med=m[5], cross_med=m[7], cross_max=m[8], unmarked_t=m[9]))
    return rows


def line(label, rs):
    f = lambda k: [float(r[k]) for r in rs]
    return (f"{label}: {len(rs)} views, lost {int(sum(f('lost')))}, phantom {int(sum(f('phantom')))}, unmarked T {int(sum(f('unmarked_t')))}, "
            f"mean bends median {statistics.mean(f('bends_med')):.2f}, views over 2 bends {sum(b > 2 for b in f('bends_med'))}, "
            f"mean crossing median {statistics.mean(f('cross_med')):.3f}")


groups = [
    ("fixture full views", lambda r: r["fixture"] != "orca-store" and " focus " not in r["view"]),
    ("fixture focus views", lambda r: r["fixture"] != "orca-store" and " focus " in r["view"]),
    ("orca store full views", lambda r: r["fixture"] == "orca-store" and " focus " not in r["view"]),
    ("orca store focus views", lambda r: r["fixture"] == "orca-store" and " focus " in r["view"]),
]
named = [("self", "overview"), ("orca", "MOD-0042 desktop main process"), ("orca", "MOD-0042 focus MOD-0052 desktop-main/ipc"),
         ("orca", "MOD-0042 focus MOD-0054 desktop-main/persistence"), ("pstack", "overview"), ("pstack", "overview focus MOD-0003 principles"),
         ("orca", "MOD-0056 focus DATA-0003 orca-runtime.json"), ("pstack", "MOD-0010 focus FEAT-0024 /maintain-formal-verification-skill"),
         ("pstack", "MOD-0010 focus FEAT-0059 /statechart")]
data = {t: load(t) for t in ("before", "after")}
for label, pred in groups:
    for t in data:
        print(t.ljust(6), line(label, [r for r in data[t] if pred(r)]))
print()
for fx, v in named:
    got = []
    for t in data:
        r = next(r for r in data[t] if r["fixture"] == fx and r["view"] == v)
        got.append(f"bends {r['bends_med']} crossings {r['cross_med']} (max {r['cross_max']})")
    print(f"{fx} {v}: " + " -> ".join(got))
