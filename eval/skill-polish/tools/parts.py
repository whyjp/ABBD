"""Score a run's modules against a repository's reference parts, with no judge.

    python -I parts.py REF_TSV RUN_DIR...

REF_TSV has rows `path<TAB>top<TAB>inner`: a directory (ending in /) or a
file, relative to the repository root, and the part an engineer would draw it
in; a file takes the row with its longest matching path, and inner is `-`
when the top part has no inner parts. RUN_DIR is a repository with a
`.abbd/architecture/` state.

A run puts each file in the deepest module that anchors it. Its top part is
that module's top ancestor, and its inner part the ancestor one level below,
or the top part again. Over the files both sides place, it prints B-cubed
precision, recall and F1 for the top level, and for the two levels together
(a file's label is its top and inner part), with the run's and the
reference's part counts and the share of reference files the run places.
Precision falls when a run merges what the reference keeps apart, recall
when it splits what the reference keeps together.
"""
import os
import re
import sys
from collections import defaultdict


def reference(path):
    rows = []
    for ln in open(path, encoding="utf8").read().splitlines()[1:]:
        f = ln.split("\t")
        if len(f) >= 2 and f[0].strip():
            rows.append((f[0].strip(), f[1].strip(), (f[2].strip() if len(f) > 2 else "-") or "-"))
    rows.sort(key=lambda r: -len(r[0]))

    def label(file):
        for p, top, inner in rows:
            if file == p or (p.endswith("/") and file.startswith(p)) or p in ("/", "./"):
                return top, (top if inner == "-" else inner)
        return None
    return label


def run_modules(run):
    d = os.path.join(run, ".abbd", "architecture")
    mods = {}
    for f in os.listdir(d):
        if not f.startswith("MOD-"):
            continue
        s = open(os.path.join(d, f), encoding="utf8").read().split("\n---\n")[0]
        name = re.search(r"^name:\n\s+value: (.*)$", s, re.M)
        anchors = re.findall(r"^\s+- path: (.*)$", s.split("\nparents:")[0].split("\nevidence:")[0].split("\nrelations:")[0], re.M)
        parent = re.search(r"^parents:\n\s+- to: (\S+)", s, re.M)
        mods[f[:-3]] = {"name": name.group(1).strip() if name else f[:-3], "anchors": [a.strip().strip('"') for a in anchors],
                        "parent": parent.group(1) if parent else None}
    return mods


def chain(mods, m):
    out = [m]
    while mods[out[-1]]["parent"] in mods and len(out) < 20:
        out.append(mods[out[-1]]["parent"])
    return out[::-1]


def bcubed(pred, gold):
    files = list(pred)
    by_p, by_g = defaultdict(set), defaultdict(set)
    for f in files:
        by_p[pred[f]].add(f)
        by_g[gold[f]].add(f)
    p = sum(len(by_p[pred[f]] & by_g[gold[f]]) / len(by_p[pred[f]]) for f in files) / len(files)
    r = sum(len(by_p[pred[f]] & by_g[gold[f]]) / len(by_g[gold[f]]) for f in files) / len(files)
    return p, r, 2 * p * r / (p + r) if p + r else 0.0


def score(ref, run):
    mods = run_modules(run)
    deepest = {}
    for m in mods:
        depth = len(chain(mods, m))
        for a in mods[m]["anchors"]:
            if os.path.isfile(os.path.join(run, a)) and (a not in deepest or depth > deepest[a][1]):
                deepest[a] = (m, depth)
    top, two = {}, {}
    gold_top, gold_two = {}, {}
    ref_files = 0
    for root, dirs, files in os.walk(run):
        dirs[:] = [x for x in dirs if not x.startswith(".") and x != "node_modules"]
        for fn in files:
            f = os.path.relpath(os.path.join(root, fn), run).replace("\\", "/")
            g = ref(f)
            if g is None:
                continue
            ref_files += 1
            if f not in deepest:
                continue
            c = chain(mods, deepest[f][0])
            top[f], two[f] = c[0], (c[0], c[1] if len(c) > 1 else c[0])
            gold_top[f], gold_two[f] = g[0], g
    if not top:
        return None
    return {"files": len(top), "coverage": len(top) / ref_files, "run_top": len(set(top.values())),
            "ref_top": len(set(gold_top.values())), "top": bcubed(top, gold_top), "two": bcubed(two, gold_two)}


def main():
    ref = reference(sys.argv[1])
    print("run\tfiles\tcoverage\trun_top\tref_top\ttop_P\ttop_R\ttop_F1\ttwo_P\ttwo_R\ttwo_F1")
    for run in sys.argv[2:]:
        s = score(ref, run)
        if s is None:
            print(f"{run}\t0")
            continue
        print(f"{run}\t{s['files']}\t{s['coverage']:.2f}\t{s['run_top']}\t{s['ref_top']}\t" + "\t".join(f"{x:.2f}" for x in s["top"] + s["two"]))


main()
