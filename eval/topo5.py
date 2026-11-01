"""Put every A/B run's code change on the same ABBD baseline and record the topology difference.

  python -I eval/topo5.py <topo worktree on abbd-base> <abbd exe> <ab-orca dir> <run worktrees dir>

B runs have no .abbd/, so this is the only way to read both conditions' changes through one lens. For each run the
worktree is reset to abbd-base, the files the run's change.diff names (code only, .abbd/ and PR.md excluded) are copied from the run's worktree, and
`abbd sync`, `abbd diff --base abbd-base` and `abbd diff --check` run. Output goes to runs/<run>/topology.txt.
"""
import glob
import os
import re
import shutil
import subprocess
import sys


def sh(args, cwd, check=True):
    p = subprocess.run(args, cwd=cwd, capture_output=True, text=True, encoding="utf-8", errors="replace")
    if check and p.returncode:
        raise SystemExit("%s: %s" % (" ".join(args), p.stderr))
    return p


def main():
    tree, abbd, ab, wts = sys.argv[1:5]
    for run_dir in sorted(glob.glob(os.path.join(ab, "runs", "*"))):
        diff = os.path.join(run_dir, "change.diff")
        if not os.path.exists(diff) or not os.path.getsize(diff):
            continue
        sh(["git", "reset", "-q", "--hard", "abbd-base"], tree)
        sh(["git", "clean", "-qfd", "-e", "node_modules"], tree)
        src = os.path.join(wts, os.path.basename(run_dir))
        names = re.findall(r"^diff --git a/(\S+) b/", open(diff, encoding="utf-8").read(), re.M)
        for n in names:
            a, b = os.path.join(src, n), os.path.join(tree, n)
            if os.path.exists(a):
                os.makedirs(os.path.dirname(b), exist_ok=True)
                shutil.copyfile(a, b)
            elif os.path.exists(b):
                os.remove(b)
        ap = subprocess.CompletedProcess([], 0, "", "copied %d files from the run worktree" % len(names))
        sync = sh([abbd, "sync"], tree, check=False)
        d = sh([abbd, "diff", "--base", "abbd-base"], tree, check=False)
        c = sh([abbd, "diff", "--base", "abbd-base", "--check"], tree, check=False)
        text = "apply exit %d %s\nsync exit %d %s\n--- abbd diff\n%s\n--- abbd diff --check exit %d\n" % (
            ap.returncode, ap.stderr.strip(), sync.returncode, sync.stderr.strip()[-500:], d.stdout + d.stderr, c.returncode)
        open(os.path.join(run_dir, "topology.txt"), "w", encoding="utf-8").write(text)
        print(os.path.basename(run_dir), "apply", ap.returncode, "check", c.returncode, d.stdout.strip().splitlines()[1:3])
    sh(["git", "reset", "-q", "--hard", "abbd-base"], tree)


main()

