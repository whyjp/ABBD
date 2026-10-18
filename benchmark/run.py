#!/usr/bin/env python3
"""ABBD appendix A pilot: run the same cumulative task sequence through several arms, each in
its own workspace and its own headless Claude Code session per task, then score every arm with
hidden tests the agents never see.

Arms
  pstack    Claude Code + the pstack plugin only
  abbd      Claude Code + the ABBD plugin only, abbd CLI on PATH, repo bootstrapped by abbd
  baseline  Claude Code with no plugin
  reference applies benchmark/testdata/reference/<task>.patch (no model); validates the tasks

Each session runs with --setting-sources project, so user-level plugins, hooks and settings do
not leak into an arm; the arm's plugin is loaded with --plugin-dir. Everything else (model,
prompt, budget, permission mode, base repository) is identical across arms.
"""
import argparse
import datetime as dt
import json
import os
import shutil
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
HIDDEN = os.path.join(HERE, "testdata", "hidden")
REFERENCE = os.path.join(HERE, "testdata", "reference")
SUFFIX = ("\n\nYou are working alone in this repository; nobody will answer questions. "
          "Complete the change, keep existing behavior working, and run the tests before you finish.")


def sh(cmd, cwd, env=None, timeout=None, check=True):
    r = subprocess.run(cmd, cwd=cwd, env=env, timeout=timeout, capture_output=True, text=True, encoding="utf-8", errors="replace")
    if check and r.returncode != 0:
        raise RuntimeError(f"{cmd} failed ({r.returncode}): {r.stderr[-2000:]}")
    return r


def git(repo, *args, check=True):
    return sh(["git", "-c", "user.email=bench@abbd", "-c", "user.name=bench", "-c", "core.autocrlf=false", *args], repo, check=check)


def prepare(arm, out, tasks_file, abbd_bin):
    work = os.path.join(out, arm)
    repo = os.path.join(work, "repo")
    shutil.rmtree(work, ignore_errors=True)
    shutil.copytree(os.path.join(REPO, tasks_file["base"]), repo)
    git(repo, "init", "-q")
    git(repo, "add", "-A")
    git(repo, "commit", "-qm", "base")
    env = dict(os.environ)
    if arm == "abbd":
        env["PATH"] = os.path.dirname(abbd_bin) + os.pathsep + env["PATH"]
        sh([abbd_bin, "init", "--mode", "code-only", "--verify-available"], repo, env=env)
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "abbd bootstrap")
    return repo, env


def plugin_args(arm, pstack_dir):
    if arm == "abbd":
        return ["--plugin-dir", os.path.join(REPO, "plugins", "abbd")]
    if arm == "pstack":
        return ["--plugin-dir", pstack_dir]
    return []


def run_session(arm, repo, env, task, args):
    if arm == "reference":
        patch = os.path.join(REFERENCE, task["id"] + ".patch")
        start = time.time()
        r = git(repo, "apply", "--whitespace=nowarn", patch, check=False)
        return {"is_error": r.returncode != 0, "error": r.stderr[-500:], "duration_ms": int((time.time() - start) * 1000), "total_cost_usd": 0, "num_turns": 0}
    cmd = ["claude", "-p", task["prompt"] + SUFFIX, "--setting-sources", "project", *plugin_args(arm, args.pstack_dir),
           "--permission-mode", "bypassPermissions", "--model", args.model, "--output-format", "json",
           "--max-budget-usd", str(args.budget)]
    start = time.time()
    try:
        r = sh(cmd, repo, env=env, timeout=args.timeout, check=False)
    except subprocess.TimeoutExpired:
        return {"is_error": True, "error": "timeout", "duration_ms": int((time.time() - start) * 1000)}
    try:
        out = json.loads(r.stdout)
    except json.JSONDecodeError:
        return {"is_error": True, "error": (r.stdout + r.stderr)[-1000:], "duration_ms": int((time.time() - start) * 1000)}
    keep = {k: out.get(k) for k in ("is_error", "subtype", "total_cost_usd", "num_turns", "duration_ms", "usage", "session_id")}
    keep["result_tail"] = (out.get("result") or "")[-600:]
    return keep


def hidden_files(task_id):
    base = os.path.join(HIDDEN, task_id)
    for root, _, files in os.walk(base):
        for f in files:
            src = os.path.join(root, f)
            yield src, os.path.relpath(src, base)


def score(repo, upto, env):
    """Run each task's hidden tests separately so one task's compile error cannot hide another's."""
    results = {}
    for task_id in upto:
        placed = []
        for src, rel in hidden_files(task_id):
            dst = os.path.join(repo, rel)
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            shutil.copy(src, dst)
            placed.append(dst)
        pkgs = sorted({"./" + os.path.dirname(os.path.relpath(p, repo)).replace(os.sep, "/") for p in placed})
        r = sh(["go", "test", "-count=1", "-run", "TestHidden" + task_id, *pkgs], repo, env=env, check=False, timeout=300)
        results[task_id] = {"passed": r.returncode == 0, "tail": (r.stdout + r.stderr)[-800:]}
        for p in placed:
            os.remove(p)
            d = os.path.dirname(p)
            while d != repo and os.path.isdir(d) and not os.listdir(d):
                os.rmdir(d)
                d = os.path.dirname(d)
    own = sh(["go", "test", "-count=1", "./..."], repo, env=env, check=False, timeout=600)
    vet = sh(["go", "vet", "./..."], repo, env=env, check=False, timeout=600)
    return results, own.returncode == 0, vet.returncode == 0


def abbd_state(repo, env, abbd_bin):
    if not os.path.isdir(os.path.join(repo, ".abbd")):
        return None
    r = sh([abbd_bin, "--json", "status"], repo, env=env, check=False)
    try:
        st = json.loads(r.stdout)
    except json.JSONDecodeError:
        return {"error": r.stderr[-300:]}
    changes = [f for f in os.listdir(os.path.join(repo, ".abbd", "changes"))] if os.path.isdir(os.path.join(repo, ".abbd", "changes")) else []
    return {"freshness": st.get("freshness"), "active_turn": (st.get("active_turn") or {}).get("id"), "receipts": len(changes)}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--arms", default="pstack,abbd")
    ap.add_argument("--tasks", default="")
    ap.add_argument("--model", default="sonnet")
    ap.add_argument("--budget", type=float, default=4.0, help="max USD per task session")
    ap.add_argument("--timeout", type=int, default=1800, help="seconds per task session")
    ap.add_argument("--pstack-dir", default=os.path.expanduser("~/.claude/plugins/cache/pstack-claude/pstack/0.9.93"))
    ap.add_argument("--out", default="")
    args = ap.parse_args()

    tasks_file = json.load(open(os.path.join(HERE, "tasks.json"), encoding="utf-8"))
    tasks = tasks_file["tasks"]
    if args.tasks:
        keep = set(args.tasks.split(","))
        tasks = [t for t in tasks if t["id"] in keep]
    out = args.out or os.path.join(HERE, "results", dt.datetime.now().strftime("%Y%m%d-%H%M%S"))
    os.makedirs(out, exist_ok=True)
    abbd_bin = os.path.join(out, "bin", "abbd.exe" if os.name == "nt" else "abbd")
    sh(["go", "build", "-o", abbd_bin, "./cmd/abbd"], REPO)
    meta = {"started": dt.datetime.now().isoformat(), "model": args.model, "budget_per_task": args.budget, "arms": args.arms.split(","),
            "tasks": [t["id"] for t in tasks], "abbd_commit": sh(["git", "rev-parse", "--short", "HEAD"], REPO).stdout.strip()}
    json.dump(meta, open(os.path.join(out, "meta.json"), "w"), indent=2)

    for arm in args.arms.split(","):
        repo, env = prepare(arm, out, tasks_file, abbd_bin)
        log = []
        for i, task in enumerate(tasks):
            print(f"[{arm}] {task['id']} running", flush=True)
            session = run_session(arm, repo, env, task, args)
            git(repo, "add", "-A")
            git(repo, "commit", "-qm", f"{task['id']} ({arm})", "--allow-empty")
            hidden, own_ok, vet_ok = score(repo, [t["id"] for t in tasks[: i + 1]], env)
            row = {"task": task["id"], "session": session, "hidden": hidden, "own_tests_pass": own_ok, "vet_pass": vet_ok,
                   "abbd": abbd_state(repo, env, abbd_bin) if arm == "abbd" else None}
            log.append(row)
            json.dump(log, open(os.path.join(out, f"{arm}.json"), "w", encoding="utf-8"), indent=2, ensure_ascii=False)
            passed = sum(1 for h in hidden.values() if h["passed"])
            print(f"[{arm}] {task['id']} hidden {passed}/{i + 1} own-tests {own_ok} cost {session.get('total_cost_usd')}", flush=True)
    subprocess.run([sys.executable, os.path.join(HERE, "report.py"), out])
    print(out)


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
