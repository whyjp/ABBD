"""Apply one round's keep rule to its pairwise passes.

    python -I verdict.py ROUND_DIR KEPT CAND --tier renderer|proxy|fresh [--target c4,c3]

ROUND_DIR holds pass-<p>.tsv (a judge's final answer, both tables) and
pass-<p>-key.json (pairs.py's key) for each pass. KEPT and CAND are the set
names given to pairs.py. Every rule below is the one README.md states.

  view verdict   per comparison and criterion, the side more than half the
                 passes pick, else a tie
  flip rate      over every comparison but the near-nulls, every criterion
                 the pack asks (c1..c6, c7 with hover shots) and every pair of
                 passes, the share of pass pairs with opposite strict answers;
                 above 0.10 the round is rerun with new judges
  near-null      a pass that answers a near-null pair with A or B on any
                 criterion is rerun with a new judge
  renderer       kept if every targeted criterion, each on its own, wins more
                 views than it loses, c6 wins at least as many views as it
                 loses, and no overview loses c6
  proxy          goes to fresh runs if the overview loses c6 in no more
                 repositories than it wins; never kept
  fresh          kept if the overview wins c6 unanimously (every pass) in at
                 least 2 of 3 repositories and loses it in none, and the
                 pages win c6 at least as often as they lose; a 2-of-3 win
                 that is not unanimous needs a fourth repository or three
                 more judges first
"""
import itertools
import json
import os
import sys

args = sys.argv[1:]
d, kept, cand = args[0], args[1], args[2]
tier = args[args.index("--tier") + 1]
target = args[args.index("--target") + 1].split(",") if "--target" in args else []
if tier == "renderer" and not target:
    sys.exit("a renderer round names its targeted criteria (--target) before the judges run")

answers, info, crits = {}, {}, None
for f in sorted(os.listdir(d)):
    if not (f.startswith("pass-") and f.endswith(".tsv")):
        continue
    p = f[5:-4]
    key = json.load(open(os.path.join(d, f"pass-{p}-key.json"), encoding="utf8"))
    for ln in open(os.path.join(d, f), encoding="utf8").read().splitlines():
        c = [x.strip() for x in ln.split("\t")]
        if c[0] == "id" and c[1] == "c1":
            crits = [x for x in c[1:] if x.startswith("c") and x[1:].isdigit()]
        elif c[0] in key["pairs"] and crits:
            k = key["pairs"][c[0]]
            if k["A"] == k["B"]:
                answers[("null", c[0], p)] = c[1:1 + len(crits)]
                continue
            if {k["A"], k["B"]} != {kept, cand}:
                continue
            o = {"A": 1 if k["A"] == cand else -1, "B": 1 if k["B"] == cand else -1}
            cid = (k["repo"], k["slot"])
            info[cid] = "overview" if k["slot"] == 0 else "page"
            answers[(cid, p)] = {cr: o.get(x, 0) for cr, x in zip(crits, c[1:1 + len(crits)])}
passes = sorted({k[-1] for k in answers})
views = sorted(info)

bad = sorted({k[2] for k, v in answers.items() if k[0] == "null" and any(x in "AB" for x in v)})
n = flips = 0
for cid in views:
    for cr in crits:
        for p, q in itertools.combinations(passes, 2):
            a, b = answers.get((cid, p), {}).get(cr), answers.get((cid, q), {}).get(cr)
            if a is not None and b is not None:
                n += 1
                flips += a * b == -1
rate = flips / n if n else 0.0
print(f"passes {len(passes)}, comparisons {len(views)}, criteria {' '.join(crits)}")
print(f"flip rate {flips}/{n} = {rate:.3f}" + ("  RERUN: above 0.10" if rate > 0.10 else ""))
print("near-nulls: " + (f"RERUN passes {' '.join(bad)}" if bad else "every answer ="))


def verdict(cid, cr):
    xs = [answers[(cid, p)][cr] for p in passes if (cid, p) in answers]
    for side in (1, -1):
        if sum(1 for x in xs if x == side) * 2 > len(xs):
            return side, all(x == side for x in xs)
    return 0, False


def tally(kind, cr):
    vs = [verdict(cid, cr)[0] for cid in views if kind in (None, info[cid])]
    return sum(1 for v in vs if v > 0), sum(1 for v in vs if v < 0)


print("criterion\twins\tlosses\t(over all views, candidate's side)")
for cr in crits:
    w, l = tally(None, cr)
    print(f"{cr}\t{w}\t{l}")
ov = {cid[0]: verdict(cid, "c6") for cid in views if info[cid] == "overview"}
print("overview c6 by repo: " + ", ".join(f"{r} {'win' if v > 0 else 'loss' if v < 0 else 'tie'}{' (unanimous)' if u else ''}" for r, (v, u) in sorted(ov.items())))
if tier == "renderer":
    ok_t = all(tally(None, cr)[0] > tally(None, cr)[1] for cr in target)
    w6, l6 = tally(None, "c6")
    keep = ok_t and w6 >= l6 and not any(v < 0 for v, _ in ov.values())
    print(f"renderer rule: targeted {','.join(target)} {'each win more than they lose' if ok_t else 'do not each win more than they lose'}; "
          f"c6 {w6}-{l6}; overview losses {sum(1 for v, _ in ov.values() if v < 0)} -> {'KEEP' if keep else 'REJECT'}")
elif tier == "proxy":
    w, l = sum(1 for v, _ in ov.values() if v > 0), sum(1 for v, _ in ov.values() if v < 0)
    print(f"proxy rule: overview c6 {w} wins, {l} losses -> {'GO TO FRESH RUNS' if l <= w else 'STOP'}")
else:
    unan = sum(1 for v, u in ov.values() if v > 0 and u)
    wins = sum(1 for v, _ in ov.values() if v > 0)
    loss = sum(1 for v, _ in ov.values() if v < 0)
    pw, pl = tally("page", "c6")
    keep = unan >= 2 and loss == 0 and pw >= pl
    more = not keep and wins >= 2 and loss == 0 and pw >= pl
    print(f"fresh rule: overview c6 unanimous wins {unan}, wins {wins}, losses {loss}; pages c6 {pw}-{pl} -> "
          + ("KEEP" if keep else "ADD A REPOSITORY OR THREE JUDGES" if more else "REJECT"))
