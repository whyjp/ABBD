"""Classify one SWE-Milestone trial as completed, running, limit_waiting, limit_stopped or stopped.

  python trial_status.py --trial claude-code_abbd-E_sonnet-5-5_r1 go-zero

A trial whose worker is gone with milestones left, and whose last agent failure in orchestrator.log is
a usage or rate limit, is limit_stopped: rerun the same run_group.py command after the reset and it
resumes. It is never scored as a task failure. Reads only files the harness writes.
"""
import argparse
import json
import os
import re
from pathlib import Path

import yaml

CONFIG = Path(__file__).resolve().parent.parent.parent / "configs" / "swe_milestone.yaml"
LIMIT = re.compile(r"Rate limit hit|usage limit|hit your limit", re.IGNORECASE)
AGENT_OK = re.compile(r"Agent (first run|recover \d+) completed ✓")


def latest_trial_dir(data_root: Path, repo: str, trial: str) -> Path:
    repo_dirs = [p for p in data_root.iterdir() if p.is_dir() and repo in p.name]
    if len(repo_dirs) != 1:
        raise SystemExit(f"repo filter {repo!r} matched {[p.name for p in repo_dirs]}")
    root = repo_dirs[0] / "e2e_trial"
    exact = root / trial
    if re.search(r"_\d{3}$", trial):
        return exact
    found = sorted(root.glob(f"{trial}_[0-9][0-9][0-9]"))
    if not found:
        raise SystemExit(f"no trial {trial}_NNN under {root}")
    return found[-1]


def worker_alive(trial_dir: Path) -> bool:
    info = trial_dir.parent / ".locks" / f"{trial_dir.name}.info"
    try:
        pid = int(json.loads(info.read_text())["pid"])
        os.kill(pid, 0)
        return True
    except (OSError, ValueError, KeyError):
        return False


def last_agent_signal(log: Path) -> str:
    signal = ""
    if log.exists():
        for line in log.read_text(errors="replace").splitlines():
            if LIMIT.search(line):
                signal = "limit"
            elif AGENT_OK.search(line):
                signal = "ok"
    return signal


def classify(trial_dir: Path) -> dict:
    summary_path = trial_dir / "evaluation" / "summary.json"
    summary = json.loads(summary_path.read_text()) if summary_path.exists() else {}
    total = summary.get("total_milestones", 0)
    ms = summary.get("milestone_status", {})
    finished = set(ms.get("passed", [])) | set(ms.get("failed", [])) | set(ms.get("skipped", []))
    alive = worker_alive(trial_dir)
    limited = last_agent_signal(trial_dir / "orchestrator.log") == "limit"
    if total and len(finished) >= total and not ms.get("error"):
        status = "completed"
    elif alive:
        status = "limit_waiting" if limited else "running"
    else:
        status = "limit_stopped" if limited else "stopped"
    return {"trial_dir": str(trial_dir), "status": status, "evaluated": len(finished),
            "passed": len(ms.get("passed", [])), "eval_error": len(ms.get("error", [])), "total": total}


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--trial", required=True)
    ap.add_argument("repo")
    args = ap.parse_args()
    cfg = yaml.safe_load(CONFIG.read_text())
    data_root = Path(os.path.expanduser(cfg["paths"]["data_root"]))
    print(json.dumps(classify(latest_trial_dir(data_root, args.repo, args.trial))))


if __name__ == "__main__":
    main()
