"""Compare how well judge passes agree under absolute scores and pairwise verdicts.

    python -I agree.py SCORES_TSV PAIRWISE_DIR

SCORES_TSV is judge-scores.tsv: one row per repo, variant@pass, with set means
c1..c6, mean and set of views. PAIRWISE_DIR holds pass-<p>.tsv (a judge's
two tables) and pass-<p>-key.json (pairs.py's key) for each pairwise pass.

A cell is one question a keep rule asks: for variants X and Y on one repo, on
one criterion, which is better. Criteria are c1..c6, all (the absolute mean,
or the sum of pairwise wins over all criteria and pages) and set (set of
views). A pass answers a cell with +1 (X), -1 (Y) or 0 (tie).

  absolute  sign of X's set score minus Y's, both from the same pass
  pairwise  sign of X's wins minus Y's over the set's page comparisons, or
            the set comparison for set

For each method it prints, over every pair of passes that answer a cell:
how often they give the same answer, how often they give opposite strict
answers (a flip), and the Pearson correlation of the effect sizes. Then how
many cells all passes answer the same strict way. Pairwise also reports its
null comparisons (one image against a copy of itself, or two byte-identical
images) and its A-side rate; absolute reports how far one image set's scores
move between passes.
"""
import itertools
import json
import math
import os
import statistics
import sys
from collections import defaultdict

CRIT = ["c1", "c2", "c3", "c4", "c5", "c6"]
PAIRS = [("base", "it5"), ("it5", "it6"), ("base", "it6"), ("it6", "it7"), ("base", "it7")]
sign = lambda v: (v > 1e-9) - (v < -1e-9)


def absolute(path):
    s = {}
    for ln in open(path, encoding="utf8").read().splitlines()[1:]:
        f = ln.split("\t")
        if "@" not in f[1] or f[1][-1].isdigit():
            continue
        v, p = f[1].split("@")
        s[(p, f[0], v)] = dict(zip(CRIT + ["all", "set"], map(float, f[2:10])))
    eff = defaultdict(dict)
    for (p, r, x) in s:
        for a, b in PAIRS:
            if x == a and (p, r, b) in s:
                for c in CRIT + ["all", "set"]:
                    eff[(a, b, r, c)][p] = s[(p, r, a)][c] - s[(p, r, b)][c]
    return s, eff


def pairwise(d):
    eff, nulls, sides = defaultdict(dict), [], []
    page = defaultdict(dict)
    for f in sorted(os.listdir(d)):
        if not (f.startswith("pass-") and f.endswith(".tsv")):
            continue
        p = f[5:-4]
        key = json.load(open(os.path.join(d, f"pass-{p}-key.json"), encoding="utf8"))
        net = defaultdict(lambda: defaultdict(int))
        for ln in open(os.path.join(d, f), encoding="utf8").read().splitlines():
            c = ln.split("\t")
            if c[0] in key["pairs"]:
                k = key["pairs"][c[0]]
                a, b = k["A"], k["B"]
                v = {"A": 1, "B": -1, "=": 0}
                ans = [v[x.strip()] for x in c[1:7]]
                sides += [x for x in ans if x]
                if a == b:
                    nulls.append(("copy", ans))
                    continue
                pair = (a, b) if (a, b) in PAIRS else (b, a)
                o = 1 if pair == (a, b) else -1
                ans = [o * x for x in ans]
                for crit, x in zip(CRIT, ans):
                    net[(pair, k["repo"])][crit] += x
                    net[(pair, k["repo"])]["all"] += x
                    page[(pair, k["repo"], k["slot"], crit)][p] = x
            elif c[0] in key["sets"]:
                k = key["sets"][c[0]]
                pair = (k["A"], k["B"]) if (k["A"], k["B"]) in PAIRS else (k["B"], k["A"])
                o = 1 if pair == (k["A"], k["B"]) else -1
                net[(pair, k["repo"])]["set"] = o * {"A": 1, "B": -1, "=": 0}[c[1].strip()]
        for (pair, r), m in net.items():
            for crit, x in m.items():
                eff[pair + (r, crit)][p] = x
    return eff, page, nulls, sides


def pearson(xs, ys):
    if len(xs) < 3 or statistics.pstdev(xs) == 0 or statistics.pstdev(ys) == 0:
        return None
    mx, my = statistics.mean(xs), statistics.mean(ys)
    return sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / math.sqrt(sum((x - mx) ** 2 for x in xs) * sum((y - my) ** 2 for y in ys))


def report(name, eff, crits):
    same = flip = n = 0
    by_pp = defaultdict(lambda: ([], []))
    unanimous = cells = 0
    for cell, by in eff.items():
        if cell[3] not in crits:
            continue
        ps = sorted(by)
        if len(ps) < 2:
            continue
        cells += 1
        sg = [sign(by[p]) for p in ps]
        unanimous += all(x == sg[0] for x in sg) and sg[0] != 0
        for p, q in itertools.combinations(ps, 2):
            n += 1
            same += sign(by[p]) == sign(by[q])
            flip += sign(by[p]) * sign(by[q]) == -1
            by_pp[(p, q)][0].append(by[p])
            by_pp[(p, q)][1].append(by[q])
    rs = [r for xs, ys in by_pp.values() if (r := pearson(xs, ys)) is not None]
    r = f"{statistics.mean(rs):.2f}" if rs else "-"
    print(f"{name}\tcells {cells}\tpass pairs {n}\tsame {same / n:.2f}\tflip {flip / n:.2f}\tr {r}\tunanimous strict {unanimous}/{cells} ({unanimous / cells:.2f})")


def main():
    scores, ab = absolute(sys.argv[1])
    pw, page, nulls, sides = pairwise(sys.argv[2])
    common = [c for c in ab if c in pw]
    ab_c = {c: ab[c] for c in common}
    pw_c = {c: pw[c] for c in common}
    print("== cells both methods answer (5 variant pairs x 3 repos x 8 criteria)")
    for label, crits in [("c1..c6", CRIT), ("all", ["all"]), ("set", ["set"])]:
        print(f"-- {label}")
        report("absolute, every pass pair", ab_c, crits)
        report("pairwise", pw_c, crits)
    print("\n== absolute, same round only (pass pairs inside one round)")
    within = defaultdict(dict)
    for c, by in ab_c.items():
        for rnd in "4567":
            sub = {p: v for p, v in by.items() if p[0] == rnd}
            if len(sub) >= 2:
                within[c + (rnd,)] = sub
    same_round = {(c[0], c[1], c[2] + "/" + c[4], c[3]): v for c, v in within.items()}
    for label, crits in [("c1..c6", CRIT), ("all", ["all"]), ("set", ["set"])]:
        report(f"absolute {label}", same_round, crits)
    print("\n== per criterion, absolute (same round) against pairwise")
    for crit in CRIT:
        report(f"absolute {crit}", same_round, [crit])
        report(f"pairwise {crit}", pw_c, [crit])
    print("\n== absolute, the same images scored in different passes (set means)")
    by_set = defaultdict(list)
    for (p, r, v), m in scores.items():
        by_set[(r, v)].append(m["all"])
    sds = []
    for (r, v), xs in sorted(by_set.items()):
        if len(xs) >= 3:
            sds.append(statistics.stdev(xs))
            print(f"{r}\t{v}\tpasses {len(xs)}\tmean {statistics.mean(xs):.2f}\tsd {statistics.stdev(xs):.2f}\trange {min(xs):.2f}-{max(xs):.2f}")
    print(f"median sd of one set's mean between passes: {statistics.median(sds):.2f}")
    margins = [abs(statistics.mean(by.values())) for c, by in ab_c.items() if c[3] == "all"]
    print(f"median |X-Y| on the mean over the decision cells: {statistics.median(margins):.2f}")
    print("\n== pairwise, image level (one view drawn twice), pass pairs agreeing")
    for kind, keep in [("overview", lambda s: s == 0), ("page slot", lambda s: s > 0)]:
        n = same = flip = 0
        for (pair, r, s, crit), by in page.items():
            if not keep(s):
                continue
            for p, q in itertools.combinations(sorted(by), 2):
                n += 1
                same += by[p] == by[q]
                flip += by[p] * by[q] == -1
        print(f"{kind}\tpass pairs {n}\tsame {same / n:.2f}\tflip {flip / n:.2f}")
    print("\n== pairwise null comparisons")
    zero = sum(1 for _, a in nulls for x in a if x == 0)
    print(f"image against its own copy: {zero}/{sum(len(a) for _, a in nulls)} answers =")
    ident = [(c, by) for c, by in page.items() if c[0] == ("it5", "it6") and (c[1], c[2]) in {("gin", 1), ("storefront", 1), ("storefront", 2)}]
    z = sum(1 for _, by in ident for x in by.values() if x == 0)
    print(f"byte-identical images in it5/it6 (gin page 1, storefront pages 1 and 2): {z}/{sum(len(by) for _, by in ident)} answers =")
    print(f"A side among strict answers: {sum(1 for x in sides if x == 1) / len(sides):.2f} of {len(sides)}")
    print("\n== per cell, criterion all: absolute passes | pairwise passes")
    for c in sorted(common):
        if c[3] == "all":
            print(f"{c[0]}/{c[1]}\t{c[2]}\t" + " ".join(f"{p}:{v:+.2f}" for p, v in sorted(ab_c[c].items())) + "\t| " + " ".join(f"{p}:{v:+d}" for p, v in sorted(pw_c[c].items())))


main()
