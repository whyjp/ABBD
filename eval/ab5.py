"""Round-5 implementation A/B on real open Orca issues.

  python -I eval/ab5.py prepare <orca clone> <worktrees dir>
  python -I eval/ab5.py run <orca clone> <worktrees dir> <out dir> <abbd plugin dir> <abbd bin dir> <run id> [<run id> ...]
  python -I eval/ab5.py measure <orca clone> <worktrees dir> <out dir> <abbd exe> <script extraction.json>

A run id is <issue>-<A|B>-r<rep>. A starts from branch abbd-base (.abbd/ and the CLAUDE.md block committed, the
abbd plugin loaded, abbd on PATH, the prompt names /abbd). B starts from eval-base (the same Orca commit without
any of that). Both share Orca's dependencies through a node_modules junction to the clone, installed with
--ignore-scripts. Each run is one fresh headless `claude -p` with no MCP servers.
"""
import json
import os
import re
import subprocess
import sys
import time

ISSUES = ("36652", "36697")
BASE = {"A": "abbd-base", "B": "eval-base"}
HERE = os.path.dirname(os.path.abspath(__file__))
ENV_NOTE = (
    "Environment: Orca's dependencies are installed (node_modules) without lifecycle or build scripts, so the "
    "workspace packages under src/packages are unbuilt and `tsc` already reports errors about them (for example "
    "@orca/process-host) before any change. Do not run any package-manager install, and do not push or open a PR."
)
PREFIX = {
    "A": "/pstack:poteto-mode Use the abbd skill (/abbd:abbd) for this repository's architecture before and after the change.\n\n",
    "B": "/pstack:poteto-mode\n\n",
}
ASK = "Implement this; do not open a PR, write the PR description to PR.md."
TOOLS = "Bash,Read,Edit,Write,Glob,Grep,Agent,Task,Skill,TodoWrite,NotebookEdit"


def sh(args, cwd=None, check=True):
    return subprocess.run(args, cwd=cwd, capture_output=True, text=True, encoding="utf-8", errors="replace", check=check).stdout


def runs():
    return ["%s-%s-r%d" % (i, c, r) for i in ISSUES for c in "AB" for r in (1, 2)]


def prompt(run_id):
    issue, cond, _ = run_id.split("-")
    text = open(os.path.join(HERE, "ab-orca", "issues", issue + ".md"), encoding="utf-8-sig").read().strip()
    return PREFIX[cond] + text + "\n\n" + ASK + "\n\n" + ENV_NOTE + "\n"


def prepare(clone, wts):
    os.makedirs(wts, exist_ok=True)
    for r in runs():
        path = os.path.join(wts, r)
        cond = r.split("-")[1]
        if not os.path.isdir(path):
            sh(["git", "-C", clone, "worktree", "add", "-q", "-b", "run/" + r, path, BASE[cond]])
            if cond == "A":
                sh(["git", "-C", path, "branch", "-q", "--set-upstream-to=" + BASE[cond]])
        nm = os.path.join(path, "node_modules")
        if not os.path.exists(nm):
            subprocess.run(["cmd", "/c", "mklink", "/J", nm, os.path.join(clone, "node_modules")], capture_output=True, check=True)
        print(r, sh(["git", "-C", path, "log", "-1", "--format=%h %s"]).strip(), sh(["git", "-C", path, "status", "--short"]).strip() or "clean")


def run_one(clone, wts, out, plugin, abbd_bin, r):
    cond = r.split("-")[1]
    path = os.path.join(wts, r)
    raw = os.path.join(out, "raw")
    os.makedirs(raw, exist_ok=True)
    env = dict(os.environ)
    env.pop("CLAUDECODE", None)
    cmd = ["claude", "-p", prompt(r), "--output-format", "stream-json", "--verbose", "--model", "opus",
           "--permission-mode", "acceptEdits", "--allowedTools", TOOLS, "--strict-mcp-config",
           "--no-session-persistence", "--max-budget-usd", os.environ.get("AB5_MAX_USD", "12")]
    if cond == "A":
        cmd += ["--plugin-dir", plugin]
        env["PATH"] = abbd_bin + os.pathsep + env["PATH"]
    t0 = time.time()
    with open(os.path.join(raw, r + ".jsonl"), "w", encoding="utf-8") as f:
        p = subprocess.run(cmd, cwd=path, stdout=f, stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace", env=env)
    meta = {"run": r, "exit": p.returncode, "wall_s": round(time.time() - t0, 1), "stderr": p.stderr[-2000:],
            "prompt": prompt(r)}
    json.dump(meta, open(os.path.join(raw, r + ".meta.json"), "w", encoding="utf-8"), indent=1)
    print(r, "exit", p.returncode, "wall", meta["wall_s"], flush=True)


def usage(raw_path):
    """Fresh tokens over the whole run, subagents included, from the result event's modelUsage."""
    res, tools, asks = None, {}, 0
    for line in open(raw_path, encoding="utf-8"):
        try:
            ev = json.loads(line)
        except ValueError:
            continue
        if ev.get("type") == "result":
            res = ev
        if ev.get("type") == "assistant":
            for b in ev.get("message", {}).get("content", []):
                if b.get("type") == "tool_use":
                    tools[b["name"]] = tools.get(b["name"], 0) + 1
                    if b["name"] == "AskUserQuestion":
                        asks += 1
    if not res:
        return {"result": None, "tools": tools, "asks": asks}
    fresh = read = 0
    for m in (res.get("modelUsage") or {}).values():
        fresh += m.get("inputTokens", 0) + m.get("cacheCreationInputTokens", 0) + m.get("outputTokens", 0)
        read += m.get("cacheReadInputTokens", 0)
    u = res.get("usage", {})
    main_fresh = u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0) + u.get("output_tokens", 0)
    text = res.get("result") or ""
    return {"fresh": fresh, "cache_read": read, "main_fresh": main_fresh, "usd": res.get("total_cost_usd"),
            "turns": res.get("num_turns"), "subtype": res.get("subtype"), "tools": tools, "asks": asks,
            "result_questions": len(re.findall(r"\?\s*$", text, re.M)), "result": text[-1500:]}


def owners(extraction):
    units = json.load(open(extraction, encoding="utf-8"))["units"]
    roots = []
    for u in units:
        if u["kind"] != "MOD":
            continue
        for a in u["anchors"]:
            roots.append((a["path"], u["key"]))
    roots.sort(key=lambda x: -len(x[0]))

    def owner(path):
        for root, key in roots:
            if path == root or path.startswith(root + "/"):
                return key
        return "(none)"
    return owner


def tsc_errors(path, project):
    p = subprocess.run(["node", "node_modules/typescript/bin/tsc", "--noEmit", "-p", "config/tsconfig.%s.json" % project,
                        "--composite", "false"], cwd=path, capture_output=True, text=True, encoding="utf-8", errors="replace")
    return set(l.strip() for l in p.stdout.splitlines() if "error TS" in l)


def measure(clone, wts, out, abbd, extraction):
    owner = owners(extraction)
    base_err = {}
    for proj in ("node", "web", "cli"):
        base_err[proj] = tsc_errors(clone, proj)
    rows = []
    for r in runs():
        path = os.path.join(wts, r)
        raw = os.path.join(out, "raw", r + ".jsonl")
        if not os.path.exists(raw):
            continue
        cond = r.split("-")[1]
        sh(["git", "-C", path, "add", "-A"])
        names = [n for n in sh(["git", "-C", path, "diff", "--cached", "--name-only", BASE[cond]]).split("\n") if n]
        code = [n for n in names if not n.startswith(".abbd/") and n != "PR.md"]
        diff = sh(["git", "-C", path, "diff", "--cached", BASE[cond], "--", ".", ":!.abbd", ":!PR.md"])
        stat = sh(["git", "-C", path, "diff", "--cached", "--shortstat", BASE[cond], "--", ".", ":!.abbd", ":!PR.md"]).strip()
        sh(["git", "-C", path, "reset", "-q"])
        pack = os.path.join(out, "runs", r)
        os.makedirs(pack, exist_ok=True)
        open(os.path.join(pack, "change.diff"), "w", encoding="utf-8").write(diff)
        pr = os.path.join(path, "PR.md")
        pr_text = open(pr, encoding="utf-8", errors="replace").read() if os.path.exists(pr) else ""
        open(os.path.join(pack, "PR.md"), "w", encoding="utf-8").write(pr_text)
        new_err = {}
        for proj in ("node", "web", "cli"):
            new_err[proj] = sorted(tsc_errors(path, proj) - base_err[proj])
        open(os.path.join(pack, "tsc-new-errors.txt"), "w", encoding="utf-8").write(
            "\n".join("%s: %s" % (p, e) for p in new_err for e in new_err[p]) + "\n")
        diff_out = ""
        if cond == "A":
            diff_out = sh([abbd, "diff", "--base", BASE[cond]], cwd=path, check=False)
            open(os.path.join(pack, "abbd-diff.txt"), "w", encoding="utf-8").write(diff_out)
        u = usage(raw)
        meta = json.load(open(os.path.join(out, "raw", r + ".meta.json"), encoding="utf-8"))
        row = {"run": r, "exit": meta["exit"], "wall_s": meta["wall_s"], "files": len(code), "stat": stat,
               "modules": sorted({owner(n) for n in code}), "tsc_new": {p: len(v) for p, v in new_err.items()},
               "pr_md": bool(pr_text.strip()), "pr_mentions_abbd": bool(re.search(r"abbd|topolog", pr_text, re.I)),
               "abbd_committed": any(n.startswith(".abbd/") for n in names),
               **{k: v for k, v in u.items() if k != "result"}}
        open(os.path.join(pack, "final-reply.txt"), "w", encoding="utf-8").write(u.get("result") or "")
        rows.append(row)
        print(json.dumps(row, ensure_ascii=False))
    json.dump(rows, open(os.path.join(out, "measure.json"), "w", encoding="utf-8"), indent=1, ensure_ascii=False)


def main():
    cmd = sys.argv[1]
    if cmd == "prepare":
        prepare(*sys.argv[2:4])
    elif cmd == "run":
        clone, wts, out, plugin, abbd_bin = sys.argv[2:7]
        for r in sys.argv[7:]:
            run_one(clone, wts, out, plugin, abbd_bin, r)
    elif cmd == "measure":
        measure(*sys.argv[2:7])
    elif cmd == "prompt":
        print(prompt(sys.argv[2]))


main()
