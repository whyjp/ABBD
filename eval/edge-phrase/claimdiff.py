"""Diff the stored claims of two revisions of .abbd, leaving out phrase notes.

usage: python claimdiff.py BASE HEAD   (run from the repository root)

Every unit file's frontmatter is compared without its `phrases:` and without
each evidence item's `lines`, which a sync moves when the code above a quote
grows. Anything else that differs is printed, and the exit code is 1.
"""
import subprocess
import sys

import yaml

BASE, HEAD = sys.argv[1:3]
DIRS = [".abbd/architecture", ".abbd/features"]


def git(*args):
    return subprocess.run(["git", *args], capture_output=True, text=True, encoding="utf-8", check=True).stdout


def files(rev):
    return set(git("ls-tree", "-r", "--name-only", rev, "--", *DIRS).split())


def strip(v):
    if isinstance(v, dict):
        return {k: strip(x) for k, x in v.items() if k != "lines"}
    if isinstance(v, list):
        return [strip(x) for x in v]
    return v


def claims(rev, path):
    text = git("show", f"{rev}:{path}").replace("\r\n", "\n")
    front = text.split("\n---\n", 1)[0].removeprefix("---\n")
    doc = yaml.safe_load(front) or {}
    doc.pop("phrases", None)
    return strip(doc)


def main():
    a, b = files(BASE), files(HEAD)
    bad = 0
    for p in sorted(a ^ b):
        print(f"only in {'BASE' if p in a else 'HEAD'}: {p}")
        bad += 1
    for p in sorted(a & b):
        x, y = claims(BASE, p), claims(HEAD, p)
        if x != y:
            bad += 1
            for k in sorted(set(x) | set(y)):
                if x.get(k) != y.get(k):
                    print(f"{p}: {k} differs")
    print(f"{len(a & b)} unit files compared, {bad} differ")
    sys.exit(1 if bad else 0)


main()
