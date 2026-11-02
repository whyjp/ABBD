"""Blind pairwise judge for the round-5 A/B runs.

  python -I eval/judge5.py run <ab-orca dir> <judge tree> <judge work dir>
  python -I eval/judge5.py score <ab-orca dir>

For each issue and rep k, the A and B runs of rep k form a pair. Each pair is judged twice, once in each order, by a
fresh headless `claude -p` (opus) that may read the base Orca tree (no .abbd/, no CLAUDE.md block) but not edit it.
The judge sees the issue, the two code diffs (without .abbd/) and the two PR.md files as "X" and "Y". It is not told
what produced them. The order of the first judging is drawn from a hash of the pair, so it is fixed and recorded.
"""
import hashlib
import json
import os
import re
import subprocess
import sys

ISSUES = ("36652", "36697")
HERE = os.path.dirname(os.path.abspath(__file__))
INSTR = """You are reviewing two independent implementations of the same GitHub issue in the Orca repository (the
current directory is the repository before either change). Read the issue, both diffs and both PR descriptions in
{work}. Open repository files as needed to judge fit with the existing code. Do not edit anything.

Answer two questions.
1. Which implementation better fits the codebase's existing architecture: module boundaries, where settings and
   contracts already live, the patterns neighbouring code uses, and the places that must change together? Judge
   fit, not size.
2. Which PR description lets a reviewer judge the change faster and with more confidence?

End your reply with exactly one JSON object on its own line:
{{"architecture": "X" | "Y" | "tie", "architecture_why": "<two sentences>", "pr": "X" | "Y" | "tie", "pr_why": "<two sentences>", "concerns_X": "<one sentence>", "concerns_Y": "<one sentence>"}}
"""


def first_order(issue, k):
    return "AB" if int(hashlib.sha256(("%s-%d" % (issue, k)).encode()).hexdigest(), 16) % 2 == 0 else "BA"


def jobs():
    for issue in ISSUES:
        for k in (1, 2):
            o = first_order(issue, k)
            yield issue, k, o
            yield issue, k, o[::-1]


def run(ab, tree, work_root):
    raw_dir = os.path.join(ab, "judge", "raw")
    os.makedirs(raw_dir, exist_ok=True)
    issue_text = {i: open(os.path.join(HERE, "ab-orca", "issues", i + ".md"), encoding="utf-8-sig").read() for i in ISSUES}
    for issue, k, order in jobs():
        name = "%s-r%d-%s" % (issue, k, order)
        if os.path.exists(os.path.join(raw_dir, name + ".jsonl")):
            continue
        work = os.path.join(work_root, name)
        os.makedirs(work, exist_ok=True)
        open(os.path.join(work, "issue.md"), "w", encoding="utf-8").write(issue_text[issue])
        for label, cond in zip("XY", order):
            src = os.path.join(ab, "runs", "%s-%s-r%d" % (issue, cond, k))
            for f, out in (("change.diff", "%s.diff" % label), ("PR.md", "%s-PR.md" % label)):
                open(os.path.join(work, out), "w", encoding="utf-8").write(open(os.path.join(src, f), encoding="utf-8").read())
        env = dict(os.environ)
        env.pop("CLAUDECODE", None)
        cmd = ["claude", "-p", INSTR.format(work=work.replace("\\", "/")), "--output-format", "stream-json", "--verbose",
               "--model", "opus", "--allowedTools", "Read,Grep,Glob", "--disallowedTools", "Edit,Write,Bash,NotebookEdit,Agent,Task,WebFetch,WebSearch",
               "--add-dir", work, "--strict-mcp-config", "--no-session-persistence", "--max-budget-usd", "3"]
        with open(os.path.join(raw_dir, name + ".jsonl"), "w", encoding="utf-8") as f:
            p = subprocess.run(cmd, cwd=tree, stdout=f, stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace", env=env)
        print(name, "exit", p.returncode, flush=True)


def verdict(path):
    res = None
    for line in open(path, encoding="utf-8"):
        try:
            ev = json.loads(line)
        except ValueError:
            continue
        if ev.get("type") == "result":
            res = ev
    if not res:
        return None
    m = re.findall(r"\{[^{}]*\"architecture\"[^{}]*\}", res.get("result") or "", re.S)
    return json.loads(m[-1]) if m else None


def score(ab):
    raw_dir = os.path.join(ab, "judge", "raw")
    rows = []
    for issue, k, order in jobs():
        name = "%s-r%d-%s" % (issue, k, order)
        path = os.path.join(raw_dir, name + ".jsonl")
        if not os.path.exists(path):
            continue
        v = verdict(path)
        if v is None:
            rows.append({"job": name, "error": "no verdict"})
            continue
        unmask = {"X": order[0], "Y": order[1], "tie": "tie"}
        rows.append({"job": name, "architecture": unmask.get(v["architecture"], "?"), "pr": unmask.get(v["pr"], "?"),
                     "architecture_why": v.get("architecture_why"), "pr_why": v.get("pr_why"),
                     "concerns": {unmask["X"]: v.get("concerns_X"), unmask["Y"]: v.get("concerns_Y")}})
    json.dump(rows, open(os.path.join(ab, "judge", "verdicts.json"), "w", encoding="utf-8"), indent=1, ensure_ascii=False)
    for r in rows:
        print(r["job"], "arch:", r.get("architecture"), "pr:", r.get("pr"))


def main():
    if sys.argv[1] == "run":
        run(*sys.argv[2:5])
    else:
        score(sys.argv[2])


main()
