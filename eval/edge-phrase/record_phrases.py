"""Record each phrase in a TSV as a reader's claim, citing the line that holds its quote.

usage: python record_phrases.py ABBD_BIN ROOT TSV

Each row is EDGE, VIA, PHRASE, PATH, QUOTE. The quote's line is looked up in
the working tree, so the evidence names where the code says it now.
"""
import os
import subprocess
import sys

ABBD, ROOT, TSV = sys.argv[1:4]


def squash(s):
    return "".join(s.split())


def line_of(path, quote):
    with open(os.path.join(ROOT, path), encoding="utf-8") as f:
        for i, l in enumerate(f, 1):
            if squash(quote) in squash(l):
                return i
    return None


def main():
    ok = bad = 0
    with open(TSV, encoding="utf-8") as f:
        rows = [l.rstrip("\n").split("\t") for l in f if l.strip()]
    for edge, via, phrase, path, quote in rows:
        n = line_of(path, quote)
        if n is None:
            print(f"SKIP {edge}: {path} does not hold {quote!r}", flush=True)
            bad += 1
            continue
        fr, kind, to = edge.split(".")
        r = subprocess.run([ABBD, "record", "relation", fr, kind, to, "--via", via, "--phrase", phrase,
                            "--evidence", f"{path}:{n}:{quote}"], cwd=ROOT, capture_output=True, text=True, encoding="utf-8")
        out = (r.stdout + r.stderr).strip().replace("\n", " | ")
        print(f"{'OK  ' if r.returncode == 0 else 'FAIL'} {edge} {phrase!r}: {out[:200]}", flush=True)
        ok += r.returncode == 0
        bad += r.returncode != 0
    print(f"{ok} recorded, {bad} not", flush=True)


main()
