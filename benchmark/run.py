#!/usr/bin/env python3
"""ABBD appendix A benchmark: run the same cumulative task sequence through several arms, each in
its own workspace and its own headless Claude Code session per task, then score every arm with
hidden tests the agents never see.

Suites
  v1  benchmark/tasks.json, ten self-contained tasks (the pilot; every arm hit the ceiling)
  v2  benchmark/v2/tasks.json, twelve tasks where early tasks state project-wide rules once and
      later tasks add paths those rules silently apply to (long-horizon intent)

Arms
  baseline   Claude Code with no plugin
  abbd       Claude Code + the ABBD plugin, abbd CLI on PATH, repo bootstrapped by abbd init
  pstack     Claude Code + the pstack plugin (--pstack-dir)
  reference  applies <suite>/testdata/reference/<task>.patch (no model); validates the tasks
  NAME       any harness given as --harness NAME=PLUGIN_DIR

Each session runs with --setting-sources project, so user-level plugins, hooks and settings do
not leak into an arm; the arm's plugin is loaded with --plugin-dir. Everything else (model,
prompt, budget, permission mode, base repository) is identical across arms. Scoring is per hidden
test function, so a later task's rule checks (TestHiddenTnnRule<Name>) are attributed separately.
"""
import argparse
import datetime as dt
import json
import os
import re
import shutil
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
SUITES = {"v1": HERE, "v2": os.path.join(HERE, "v2")}
BUILTIN_ARMS = ("baseline", "abbd", "pstack", "reference")
SUFFIX = ("\n\nYou are working alone in this repository; nobody will answer questions. "
          "Complete the change, keep existing behavior working, and run the tests before you finish.")
TEST_FUNC = re.compile(r"^func (TestHidden\w+)\(", re.M)
TEST_RESULT = re.compile(r"^--- (PASS|FAIL|SKIP): (\S+)", re.M)


def sh(cmd, cwd, env=None, timeout=None, check=True):
    r = subprocess.run(cmd, cwd=cwd, env=env, timeout=timeout, capture_output=True, text=True, encoding="utf-8", errors="replace")
    if check and r.returncode != 0:
        raise RuntimeError(f"{cmd} failed ({r.returncode}): {r.stderr[-2000:]}")
    return r


def git(repo, *args, check=True):
    return sh(["git", "-c", "user.email=bench@abbd", "-c", "user.name=bench", "-c", "core.autocrlf=false", *args], repo, check=check)


def load_suite(name):
    d = SUITES[name]
    return {"name": name, "spec": json.load(open(os.path.join(d, "tasks.json"), encoding="utf-8")),
            "hidden": os.path.join(d, "testdata", "hidden"), "reference": os.path.join(d, "testdata", "reference")}


def prepare(arm, work, suite, abbd_bin):
    repo = os.path.join(work, "repo")
    shutil.rmtree(work, ignore_errors=True)
    shutil.copytree(os.path.join(REPO, suite["spec"]["base"]), repo)
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


def plugin_dir(arm, args):
    if arm in args.harness:
        return args.harness[arm]
    return {"abbd": os.path.join(REPO, "plugins", "abbd"), "pstack": args.pstack_dir}.get(arm)


def run_session(arm, repo, env, task, args, suite):
    if arm == "reference":
        patch = os.path.join(suite["reference"], task["id"] + ".patch")
        start = time.time()
        r = git(repo, "apply", "--whitespace=nowarn", patch, check=False)
        return {"is_error": r.returncode != 0, "error": r.stderr[-500:], "duration_ms": int((time.time() - start) * 1000), "total_cost_usd": 0, "num_turns": 0}
    plugin = plugin_dir(arm, args)
    cmd = ["claude", "-p", task["prompt"] + SUFFIX, "--setting-sources", "project", *(["--plugin-dir", plugin] if plugin else []),
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


def hidden_files(hidden_dir, task_id):
    base = os.path.join(hidden_dir, task_id)
    for root, _, files in os.walk(base):
        for f in files:
            src = os.path.join(root, f)
            yield src, os.path.relpath(src, base)


def run_hidden(repo, hidden_dir, task_id, env, count=1):
    """Place one task's hidden tests, run them, remove them. Every declared test function gets a
    verdict; a compile error fails them all, so one task's breakage cannot hide another's."""
    placed, declared = [], []
    for src, rel in hidden_files(hidden_dir, task_id):
        dst = os.path.join(repo, rel)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copy(src, dst)
        placed.append(dst)
        declared += TEST_FUNC.findall(open(src, encoding="utf-8").read())
    pkgs = sorted({"./" + os.path.dirname(os.path.relpath(p, repo)).replace(os.sep, "/") for p in placed})
    try:
        r = sh(["go", "test", f"-count={count}", "-v", "-run", f"^TestHidden{task_id}", *pkgs], repo, env=env, check=False, timeout=300)
        out = r.stdout + r.stderr
        ok = r.returncode == 0
    except subprocess.TimeoutExpired:
        out, ok = "timeout", False
    finally:
        for p in placed:
            os.remove(p)
            d = os.path.dirname(p)
            while d != repo and os.path.isdir(d) and not os.listdir(d):
                os.rmdir(d)
                d = os.path.dirname(d)
    verdicts = TEST_RESULT.findall(out)
    tests = {name: any(n == name and v == "PASS" for v, n in verdicts) and not any(n == name and v != "PASS" for v, n in verdicts)
             for name in declared}
    return {"passed": ok and all(tests.values()), "tests": tests, "tail": out[-800:]}


def score(repo, hidden_dir, upto, env):
    results = {task_id: run_hidden(repo, hidden_dir, task_id, env) for task_id in upto}
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


def parse_harness(values):
    harness = {}
    for v in values:
        name, sep, path = v.partition("=")
        if not sep or not name or not path:
            raise SystemExit(f"--harness wants NAME=PLUGIN_DIR, got {v!r}")
        if name in BUILTIN_ARMS:
            raise SystemExit(f"--harness name {name!r} collides with a built-in arm")
        if not os.path.isdir(path):
            raise SystemExit(f"--harness {name}: {path} is not a directory")
        harness[name] = os.path.abspath(path)
    return harness


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--suite", choices=sorted(SUITES), default="v1")
    ap.add_argument("--arms", default="pstack,abbd", help="comma-separated; built-ins: " + ", ".join(BUILTIN_ARMS) + ", plus --harness names")
    ap.add_argument("--harness", action="append", default=[], metavar="NAME=PLUGIN_DIR", help="extra arm that loads PLUGIN_DIR (repeatable)")
    ap.add_argument("--reps", type=int, default=1, help="repeat each arm N times into <out>/<arm>-r<k>")
    ap.add_argument("--tasks", default="")
    ap.add_argument("--model", default="sonnet")
    ap.add_argument("--budget", type=float, default=4.0, help="max USD per task session")
    ap.add_argument("--timeout", type=int, default=1800, help="seconds per task session")
    ap.add_argument("--pstack-dir", default=os.path.expanduser("~/.claude/plugins/cache/pstack-claude/pstack/0.9.93"))
    ap.add_argument("--out", default="")
    args = ap.parse_args()
    args.harness = parse_harness(args.harness)
    arms = args.arms.split(",")
    unknown = [a for a in arms if a not in BUILTIN_ARMS and a not in args.harness]
    if unknown:
        raise SystemExit(f"unknown arms {unknown}; define them with --harness NAME=PLUGIN_DIR")

    suite = load_suite(args.suite)
    tasks = suite["spec"]["tasks"]
    if args.tasks:
        keep = set(args.tasks.split(","))
        tasks = [t for t in tasks if t["id"] in keep]
    out = args.out or os.path.join(HERE, "results", dt.datetime.now().strftime("%Y%m%d-%H%M%S"))
    os.makedirs(out, exist_ok=True)
    abbd_bin = os.path.join(out, "bin", "abbd.exe" if os.name == "nt" else "abbd")
    if "abbd" in arms:
        sh(["go", "build", "-o", abbd_bin, "./cmd/abbd"], REPO)
    meta = {"started": dt.datetime.now().isoformat(), "suite": args.suite, "model": args.model, "budget_per_task": args.budget,
            "arms": arms, "reps": args.reps, "harness": args.harness, "tasks": [t["id"] for t in tasks],
            "abbd_commit": sh(["git", "rev-parse", "--short", "HEAD"], REPO).stdout.strip()}
    json.dump(meta, open(os.path.join(out, "meta.json"), "w"), indent=2)

    for arm in arms:
        for k in range(1, args.reps + 1):
            run_name = f"{arm}-r{k}"
            repo, env = prepare(arm, os.path.join(out, run_name), suite, abbd_bin)
            log = []
            for i, task in enumerate(tasks):
                print(f"[{run_name}] {task['id']} running", flush=True)
                session = run_session(arm, repo, env, task, args, suite)
                git(repo, "add", "-A")
                git(repo, "commit", "-qm", f"{task['id']} ({run_name})", "--allow-empty")
                hidden, own_ok, vet_ok = score(repo, suite["hidden"], [t["id"] for t in tasks[: i + 1]], env)
                row = {"task": task["id"], "session": session, "hidden": hidden, "own_tests_pass": own_ok, "vet_pass": vet_ok,
                       "abbd": abbd_state(repo, env, abbd_bin) if arm == "abbd" else None}
                log.append(row)
                json.dump(log, open(os.path.join(out, f"{run_name}.json"), "w", encoding="utf-8"), indent=2, ensure_ascii=False)
                tests = [v for h in hidden.values() for v in h["tests"].values()]
                print(f"[{run_name}] {task['id']} hidden tests {sum(tests)}/{len(tests)} own-tests {own_ok} cost {session.get('total_cost_usd')}", flush=True)
    subprocess.run([sys.executable, os.path.join(HERE, "report.py"), out])
    print(out)


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
