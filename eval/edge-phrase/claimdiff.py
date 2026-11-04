"""Diff the stored claims of two revisions of .abbd, leaving out phrase notes.

usage: python claimdiff.py BASE HEAD   (run from the repository root; "." is the working tree)

It compares what readers, the supplement and humans wrote: their relation
claims, shown and hidden, their parent claims, every mask, and the facts
they wrote. Each evidence item is compared without its `lines`, which a
sync moves when the code above a quote grows, and each claim without its
`digest`, which a sync stamps from the code around its quotes. The
script's and the group's claims are what sync re-extracts from the code,
so they are listed apart and do not fail the diff. Any other difference is
printed, and the exit code is 1.
"""
import os
import subprocess
import sys

import yaml

BASE, HEAD = sys.argv[1:3]
DIRS = [".abbd/architecture", ".abbd/features"]
CODE = {"script", "group"}
FACTS = ["name", "purpose", "responsibility", "signature"]


def git(*args):
    return subprocess.run(["git", *args], capture_output=True, text=True, encoding="utf-8", check=True).stdout


def files(rev):
    if rev == ".":
        return {f"{d}/{n}" for d in DIRS for n in os.listdir(d) if n.endswith(".md")}
    return set(git("ls-tree", "-r", "--name-only", rev, "--", *DIRS).split())


def strip(v):
    if isinstance(v, dict):
        return {k: strip(x) for k, x in v.items() if k not in ("lines", "digest")}
    if isinstance(v, list):
        return [strip(x) for x in v]
    return v


def read(rev, path):
    if rev == ".":
        with open(path, encoding="utf-8") as f:
            text = f.read()
    else:
        text = git("show", f"{rev}:{path}")
    front = text.replace("\r\n", "\n").split("\n---\n", 1)[0].removeprefix("---\n")
    return yaml.safe_load(front) or {}


def claims(doc, code):
    """The claims of one unit file, by whether the script or group wrote them."""
    keep = lambda c: (c.get("writer") in CODE) == code
    rels = sorted((strip(c) for c in (doc.get("relations") or []) + (doc.get("hidden") or []) if keep(c)),
                  key=lambda c: (c["to"], c["kind"], c["writer"]))
    out = {"relations": rels, "parents": [strip(c) for c in doc.get("parents") or [] if keep(c)]}
    if code:
        out.update({k: doc.get(k) for k in ["entries", "digest"]})
    else:
        out["masks"] = doc.get("masks")
        out["evidence"] = strip(doc.get("evidence"))
        out.update({k: strip(doc.get(k)) for k in FACTS if isinstance(doc.get(k), dict) and doc[k].get("writer") not in CODE})
    return out


def main():
    a, b = files(BASE), files(HEAD)
    bad = 0
    for p in sorted(a ^ b):
        print(f"only in {'BASE' if p in a else 'HEAD'}: {p}")
        bad += 1
    code = []
    for p in sorted(a & b):
        x, y = read(BASE, p), read(HEAD, p)
        cx, cy = claims(x, False), claims(y, False)
        if cx != cy:
            bad += 1
            for k in sorted(set(cx) | set(cy)):
                if cx.get(k) != cy.get(k):
                    print(f"{p}: {k} differs")
                    if k == "relations":
                        for r in cx[k]:
                            if r not in cy[k]:
                                print(f"  - {r['kind']}.{r['to']} by {r['writer']}: {r.get('evidence')}")
                        for r in cy[k]:
                            if r not in cx[k]:
                                print(f"  + {r['kind']}.{r['to']} by {r['writer']}: {r.get('evidence')}")
        if claims(x, True) != claims(y, True):
            code.append(p)
    print(f"{len(a & b)} unit files compared, {bad} differ in what readers, the supplement or humans wrote")
    print(f"{len(code)} differ in what the script re-extracted: {' '.join(os.path.basename(p) for p in code)}")
    sys.exit(1 if bad else 0)


main()
