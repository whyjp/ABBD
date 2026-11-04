"""Replay ordinary code changes through abbd sync and count the blueprint churn.

usage: python replay.py ABBD_BIN SRC_TREE WORK_DIR OUT_TSV [PATCH_DIR]

SRC_TREE is a copy of the repository without .git, whose code and .abbd/
are the base state. ABBD_BIN is the renderer under test, built from any
checkout. Each case copies the tree, applies one change to code outside
internal/screen, runs `ABBD_BIN sync`, and diffs .abbd/blueprint/ against
the base sync. A real case is a patch in PATCH_DIR (`*.patch`, applied
reversed, so a past commit's own diff, `git diff C~1 C -- internal cmd`,
replays as its undo on today's code). Synthetic cases are below.

Each row is one rewritten file: the bytes of the tags and lines a diff does
not match, the lines a pull request shows changed, and those bytes charged
to the causes in CAUSES. ONLY=a,b runs some cases. REUSE=1 charges the
files a run kept in WORK_DIR again without syncing.
"""
import difflib
import json
import os
import re
import shutil
import subprocess
import sys

ABBD, SRC, WORK, OUT = sys.argv[1:5]
PATCHES = sys.argv[5] if len(sys.argv) > 5 else ""


def edit(path, fn):
    def apply(root):
        p = os.path.join(root, path)
        with open(p, encoding="utf-8", newline="") as f:
            s = f.read()
        t = fn(s)
        assert t != s, path
        with open(p, "w", encoding="utf-8", newline="") as f:
            f.write(t)
    return apply


def after_first(pat, text):
    return lambda s: re.sub(pat, lambda m: m.group(0) + text, s, count=1, flags=re.M)


SYNTH = {
    "add-func": edit("internal/gitx/gitx.go", after_first(r"^\)\n", "\nfunc unusedHelper(a, b int) int {\n\treturn a + b\n}\n")),
    "add-func-end": edit("internal/narrative/narrative.go", lambda s: s + "\nfunc trailing() string { return \"x\" }\n"),
    "doc-string": edit("internal/narrative/narrative.go", lambda s: s.replace("// ", "// Note: ", 1)),
    "evidence-lines": edit("internal/store/store.go", after_first(r"^package store\n", "\n\n\n\n")),
    "new-dep": edit("internal/narrative/narrative.go", lambda s: s.replace("import (", "import (\n\t\"github.com/whyjp/abbd/internal/gitx\"", 1) + "\nvar _ = gitx.Snapshot\n"),
    "add-module": lambda root: (os.makedirs(os.path.join(root, "internal/zzz"), exist_ok=True), open(os.path.join(root, "internal/zzz/zzz.go"), "w").write("package zzz\n\nimport \"github.com/whyjp/abbd/internal/model\"\n\nvar K = model.Feature\n")),
    "add-command": edit("cmd/abbd/main.go", lambda s: s.replace('\t"diff":  ', '\t"noop":            {flags: []string{"root"}, pos: []int{0}, run: func(e env, _ []string) error { return e.diff() }},\n\t"diff":  ', 1)),
    "add-file": lambda root:open(os.path.join(root, "internal/gitx/extra.go"), "w").write("package gitx\n\nfunc extra() {}\n"),
}


def run(cmd, cwd):
    r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, encoding="utf-8", errors="replace")
    if r.returncode not in (0, 1):
        raise SystemExit(f"{cmd} failed in {cwd}:\n{r.stdout}\n{r.stderr}")
    return r


def copy(src, dst):
    if os.path.exists(dst):
        shutil.rmtree(dst)
    shutil.copytree(src, dst, ignore=shutil.ignore_patterns(".git"))


def read_bp(root):
    d = os.path.join(root, ".abbd", "blueprint")
    out = {}
    for n in sorted(os.listdir(d)):
        with open(os.path.join(d, n), encoding="utf-8", newline="") as f:
            out[n] = f.read()
    return out


TOKEN = re.compile(r"(?=<)|\n")


def changed(ta, tb):
    """Sizes of the tokens on either side that a diff does not match."""
    i = 0
    while i < min(len(ta), len(tb)) and ta[i] == tb[i]:
        i += 1
    j = 0
    while j < min(len(ta), len(tb)) - i and ta[-1 - j] == tb[-1 - j]:
        j += 1
    ta, tb = ta[i:len(ta) - j], tb[i:len(tb) - j]
    out = 0
    n = 0
    for op, i1, i2, j1, j2 in difflib.SequenceMatcher(None, ta, tb, autojunk=False).get_opcodes():
        if op != "equal":
            out += sum(len(t.encode()) + 1 for t in ta[i1:i2]) + sum(len(t.encode()) + 1 for t in tb[j1:j2])
            n += (i2 - i1) + (j2 - j1)
    return out, n


def churn(a, b):
    """Bytes a reviewer reads changed, by tag and line, and lines a PR shows changed."""
    nbytes, _ = changed(TOKEN.split(a), TOKEN.split(b))
    _, lines = changed(a.split("\n"), b.split("\n"))
    return nbytes, lines


def sections(s, keep):
    """s with every <section> whose opening tag matches keep blanked."""
    return re.sub(r'<section[^>]*>.*?</section>', lambda m: "<section/>" if re.match(keep, m.group(0)) else m.group(0), s, flags=re.S)


def graph_data(s):
    try:
        g = json.loads(s)
    except ValueError:
        return s
    g.pop("overview", None)
    for m in g.get("modules") or []:
        m.pop("Drawing", None)
    return json.dumps(g, indent=2, ensure_ascii=False)


# Each collateral cause is a rewrite that blanks what it names. A file's
# bytes are charged to a cause by how much the diff shrinks once it is
# blanked, in this order, and what is left is charged to the content.
CAUSES = [
    ("quote-lines", lambda s: re.sub(r'([\w./-]+\.\w+):\d+(?:-\d+)?(?= )', r"\1", s)),
    ("hue", lambda s: re.sub(r' data-h="[^"]*"', "", s)),
    ("mask-id", lambda s: re.sub(r'<mask id="[^"]*"', "<mask", re.sub(r' mask="url\(#[^)]*\)"', ' mask=""', s))),
    ("nav", lambda s: re.sub(r'<nav class="site".*?</nav>', "<nav/>", s, flags=re.S)),
    ("header-counts", lambda s: re.sub(r'(단위 목록|계약 목록) <span class="muted"[^>]*>[^<]*', r"\1", s)),
    ("cross-page-href", lambda s: re.sub(r'href="[^"#]*\.html(#[^"]*)?"', 'href=""', s)),
    ("index-figures", lambda s: sections(s, r'<section class="inside (flow|stack|matrix)"')),
    ("index-overview-focus", lambda s: sections(s, r'<section class="inside" id="f-overview--')),
    ("graph-geometry", graph_data),
]


def attribute(name, a, b):
    out = {}
    base = churn(a, b)[0]
    for cause, fn in CAUSES:
        if cause.startswith("index-") and name != "index.html":
            continue
        if cause.startswith("graph-") != (name == "graph.json") and cause != "quote-lines":
            continue
        a, b = fn(a), fn(b)
        left = churn(a, b)[0]
        if base - left:
            out[cause] = base - left
        base = left
    if base:
        out["graph-data" if name == "graph.json" else "content"] = base
    return out


def summary(rows):
    cases, causes = {}, {}
    for r in rows:
        case, f, nb, _, c = r.split("\t")
        if f.startswith("TOTAL"):
            cases[case] = r
            continue
        for k, v in json.loads(c).items():
            causes[k] = causes.get(k, 0) + v
    out = ["", "cause\tbytes"]
    for k, v in sorted(causes.items(), key=lambda kv: -kv[1]):
        out.append(f"{k}\t{v}")
    return out


def main():
    os.makedirs(WORK, exist_ok=True)
    base = os.path.join(WORK, "base")
    reuse = os.environ.get("REUSE")
    if not reuse:
        copy(SRC, base)
        run([ABBD, "sync"], base)
    before = read_bp(base)
    cases = list(SYNTH.items())
    if PATCHES:
        for n in sorted(os.listdir(PATCHES)):
            if n.endswith(".patch"):
                p = os.path.join(PATCHES, n)
                dry = subprocess.run(["patch", "-p1", "-R", "--dry-run", "-s", "-i", p], cwd=base, capture_output=True)
                if dry.returncode:
                    print(f"{n}: does not reverse onto this tree, skipped", flush=True)
                    continue
                cases.append((n[:-6], lambda root, p=p: run(["patch", "-p1", "-R", "-s", "-i", p], root)))
    rows = ["case\tfile\tbytes\tlines\tcauses"]
    only = [c for c in os.environ.get("ONLY", "").split(",") if c]
    for name, change in cases:
        if only and name not in only:
            continue
        if reuse:
            d = os.path.join(WORK, "after", name)
            after = dict(before)
            for f in os.listdir(d) if os.path.isdir(d) else []:
                with open(os.path.join(d, f), encoding="utf-8", newline="") as r:
                    after[f] = r.read()
        else:
            case = os.path.join(WORK, "case")
            copy(base, case)
            change(case)
            run([ABBD, "sync"], case)
            after = read_bp(case)
        files = sorted(set(before) | set(after))
        total_b = total_l = n = 0
        for f in files:
            a, b = before.get(f, ""), after.get(f, "")
            if a == b:
                continue
            nb, nl = churn(a, b)
            if not reuse:
                keep = os.path.join(WORK, "after", name)
                os.makedirs(keep, exist_ok=True)
                with open(os.path.join(keep, f), "w", encoding="utf-8", newline="") as w:
                    w.write(b)
            n += 1
            total_b += nb
            total_l += nl
            rows.append(f"{name}\t{f}\t{nb}\t{nl}\t{json.dumps(attribute(f, a, b))}")
        rows.append(f"{name}\tTOTAL {n} files\t{total_b}\t{total_l}\t")
        print(rows[-1], flush=True)
    rows += summary(rows[1:])
    with open(OUT, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(rows) + "\n")


main()
