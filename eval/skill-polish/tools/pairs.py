"""Build one blind pairwise pack from screenshot sets.

    python -I pairs.py OUT KEY SEED NAME=DIR... [--pair X,Y]... [--null NAME] [--same-runs] [--changed-only]

Each DIR holds <repo>-overview.png and <repo>-<page>.png files (hover shots,
<repo>-<page>-hover-<n>.png, are left out). Per repo a set's views are its
overview and its pages in name order, and slot 1 of one set meets slot 1 of
the other, which on two different topologies compares two different modules.
With --same-runs (two drawings of the same runs, as after a re-render) a page
meets the page of the same ID instead; IDs mean nothing across runs.
--changed-only drops comparisons of byte-identical drawings, for a re-render
where most views do not change. Every (set, view) is copied once into OUT under
a random name; a view whose bytes another set shares still gets its own name,
so a natural null stays hidden. --null NAME adds, per repo, that set's
overview against a near copy of itself: the drawing and its hover shots moved
down one pixel (the bottom row drops) and encoded again, under fresh names,
so neither the bytes nor the names give it away.

OUT/pairs.tsv lists, per comparison, its id, repo, view kind and the files
shown as A and B, with A and B drawn at random, and, when the sets hold
hover shots from shoot.py, each side's hover shots under random names.
OUT/sets.tsv lists the set-level comparisons by the same letters, and
OUT/rubric.md is a copy of the rubric. KEY maps every file and comparison
back to its set and view, and <KEY stem>-prompt.txt is the judge prompt.

The pack is what a judge may open, so OUT must be new and the only entry of
its parent, and KEY and every DIR must lie outside that parent.
"""
import json
import os
import random
import shutil
import struct
import sys
import zlib

HERE = os.path.dirname(os.path.abspath(__file__))
CRIT = ["c1", "c2", "c3", "c4", "c5", "c6"]

args = sys.argv[1:]
out, keyf, seed = os.path.abspath(args[0]), os.path.abspath(args[1]), int(args[2])
sets, pairs, nulls, changed_only, same_runs = {}, [], [], False, False
i = 3
while i < len(args):
    if args[i] == "--pair":
        pairs.append(tuple(args[i + 1].split(",")))
        i += 2
    elif args[i] == "--same-runs":
        same_runs = True
        i += 1
    elif args[i] == "--changed-only":
        changed_only = True
        i += 1
    elif args[i] == "--null":
        nulls.append(args[i + 1])
        i += 2
    else:
        k, v = args[i].split("=", 1)
        sets[k] = os.path.abspath(v)
        i += 1

root = os.path.dirname(out)
under = lambda p: os.path.normcase(p).startswith(os.path.normcase(root) + os.sep)
if os.path.exists(out) or (os.path.isdir(root) and os.listdir(root)):
    sys.exit(f"{out} must be new, in a parent that holds nothing else")
for p in [keyf] + list(sets.values()):
    if under(p):
        sys.exit(f"{p} lies beside the pack; keep keys and source shots under another root")
os.makedirs(out)
rng = random.Random(seed)
used = set()


def fresh(repo):
    while True:
        n = f"{repo}-{rng.randrange(1000, 10000)}.png"
        if n not in used:
            used.add(n)
            return n


def same(a, b):
    return open(a, "rb").read() == open(b, "rb").read()


def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))


def shifted(src, dst):
    """Write src moved down one pixel row, the top row repeated, re-encoded."""
    b = open(src, "rb").read()
    pos, chunks, idat = 8, [], b""
    while pos < len(b):
        n, kind = struct.unpack(">I4s", b[pos:pos + 8])
        data = b[pos + 8:pos + 8 + n]
        pos += 12 + n
        if kind == b"IDAT":
            if not idat:
                chunks.append((b"IDAT", None))
            idat += data
        else:
            chunks.append((kind, data))
    w, h, depth, ctype, _, _, lace = struct.unpack(">IIBBBBB", dict(chunks)[b"IHDR"])
    assert depth == 8 and ctype in (2, 6) and lace == 0, src
    bpp = 3 if ctype == 2 else 4
    stride = 1 + w * bpp
    raw = zlib.decompress(idat)
    ftype, row = raw[0], bytearray(raw[1:stride])
    if ftype in (1, 3, 4):
        for x in range(bpp, len(row)):
            row[x] = (row[x] + (row[x - bpp] >> 1 if ftype == 3 else row[x - bpp])) & 0xFF
    top = b"\0" + bytes(row)
    body = zlib.compress(top + top + raw[stride:len(raw) - stride], 9)
    with open(dst, "wb") as fh:
        fh.write(b[:8] + b"".join(chunk(k, body if k == b"IDAT" else d) for k, d in chunks))


def views(d, repo):
    pages = sorted(f[len(repo) + 1:-4] for f in os.listdir(d) if f.startswith(repo + "-") and f.endswith(".png") and "-hover-" not in f and f != f"{repo}-overview.png")
    return ["overview"] + pages


def hover_files(d, repo, view):
    return sorted(f for f in os.listdir(d) if f.startswith(f"{repo}-{view}-hover-") and f.endswith(".png"))


repos = sorted({f.split("-", 1)[0] for d in sets.values() for f in os.listdir(d) if f.endswith(".png")})
files, hovers, key, rows, setrows = {}, {}, {"files": {}, "pairs": {}, "sets": {}}, [], []
for repo in repos:
    for name, d in sets.items():
        for slot, view in enumerate(views(d, repo)):
            to = fresh(repo)
            shutil.copy(os.path.join(d, f"{repo}-{view}.png"), os.path.join(out, to))
            files[(name, repo, slot)] = to
            key["files"][to] = [name, repo, view, slot]
            for h in hover_files(d, repo, view):
                th = fresh(repo)
                shutil.copy(os.path.join(d, h), os.path.join(out, th))
                hovers.setdefault(to, []).append(th)
                key["files"][th] = [name, repo, h[len(repo) + 1:-4], slot]
    cmp = []
    for x, y in pairs:
        vx, vy = views(sets[x], repo), views(sets[y], repo)
        both = [v for v in vx if v in vy] if same_runs else ["overview"]
        rest = list(zip([v for v in vx if v not in both], [v for v in vy if v not in both]))
        for a, b in [(v, v) for v in both] + rest:
            sx, sy = vx.index(a), vy.index(b)
            fx, fy = files[(x, repo, sx)], files[(y, repo, sy)]
            if changed_only and same(os.path.join(out, fx), os.path.join(out, fy)):
                continue
            cmp.append((x, y, sx, fx, fy))
        setrows.append((repo, x, y))
    for name in nulls:
        to = fresh(repo)
        shifted(os.path.join(sets[name], f"{repo}-overview.png"), os.path.join(out, to))
        key["files"][to] = [name, repo, "overview-near-null", 0]
        for h in hover_files(sets[name], repo, "overview"):
            th = fresh(repo)
            shifted(os.path.join(sets[name], h), os.path.join(out, th))
            hovers.setdefault(to, []).append(th)
            key["files"][th] = [name, repo, h[len(repo) + 1:-4] + "-near-null", 0]
        cmp.append((name, name, 0, files[(name, repo, 0)], to))
    rng.shuffle(cmp)
    rows += [(repo,) + c for c in cmp]
has_hovers = any(hovers.values())
hcol = (lambda f: "\t" + (" ".join(hovers.get(f, [])) or "-")) if has_hovers else (lambda f: "")
lines = ["id\trepo\tview\tA\tB" + ("\tA hovers\tB hovers" if has_hovers else "")]
for n, (repo, x, y, s, fx, fy) in enumerate(rows, 1):
    flip = rng.random() < 0.5
    a, b, sa, sb = (fy, fx, y, x) if flip else (fx, fy, x, y)
    pid = f"c{n:02d}"
    lines.append(f"{pid}\t{repo}\t{'overview' if s == 0 else 'page'}\t{a}\t{b}{hcol(a)}{hcol(b)}")
    key["pairs"][pid] = {"repo": repo, "slot": s, "A": sa, "B": sb}
with open(os.path.join(out, "pairs.tsv"), "w", encoding="utf8") as fh:
    fh.write("\n".join(lines) + "\n")
lines = ["id\trepo\tA files\tB files"]
for n, (repo, x, y) in enumerate(setrows, 1):
    if rng.random() < 0.5:
        x, y = y, x
    sid = f"s{n:02d}"
    fa = [files[(x, repo, s)] for s in range(len(views(sets[x], repo)))]
    fb = [files[(y, repo, s)] for s in range(len(views(sets[y], repo)))]
    lines.append(f"{sid}\t{repo}\t{' '.join(fa)}\t{' '.join(fb)}")
    key["sets"][sid] = {"repo": repo, "A": x, "B": y}
with open(os.path.join(out, "sets.tsv"), "w", encoding="utf8") as fh:
    fh.write("\n".join(lines) + "\n")
shutil.copy(os.path.join(HERE, "..", "rubric.md"), os.path.join(out, "rubric.md"))
with open(keyf, "w", encoding="utf8") as fh:
    json.dump(key, fh, indent=1)
crit = CRIT + (["c7"] if has_hovers else [])
prompt = open(os.path.join(HERE, "judge-prompt.txt"), encoding="utf8").read().format(
    pack=out.replace("\\", "/") + "/", n_files=len(key["files"]), repos=", ".join(repos), n_pairs=len(key["pairs"]), n_sets=len(key["sets"]),
    hover_note=", each side with its hover shots listed beside it" if has_hovers else "", last_crit=len(crit),
    crit_cols="\t".join(crit), crit_example="\t".join(rng.choice("AB=") for _ in crit))
with open(os.path.splitext(keyf)[0] + "-prompt.txt", "w", encoding="utf8") as fh:
    fh.write(prompt)
print(len(key["files"]), "files,", len(key["pairs"]), "comparisons,", len(key["sets"]), "set comparisons")
