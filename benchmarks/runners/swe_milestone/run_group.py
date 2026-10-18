"""Launch one SWE-Milestone trial for one ABBD study group and one repo, through the official run_all.py.

Run inside WSL with the harness venv:
  ~/bench/swe-milestone/SWE-Milestone/.venv/bin/python run_group.py E go-zero --wait
  ... run_group.py A go-zero --milestones 2 --dry-run --wait      # fake claude, no model call

Re-running the same command resumes the trial (run_all's default). --force restarts it from the base
commit, --new opens the next _NNN. Every run parameter comes from configs/swe_milestone.yaml.
"""
import argparse
import os
import re
import subprocess
import sys
import time
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
BENCHMARKS = HERE.parent.parent
CONFIG = BENCHMARKS / "configs" / "swe_milestone.yaml"
SITE_DIR = BENCHMARKS / "adapters" / "for_my_harness" / "swe_milestone_site"
SHIM = HERE / "fake_claude.sh"
GROUPS = "ABCDE"
SCRUBBED = ("ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "UNIFIED_API_KEY", "UNIFIED_BASE_URL")


def trial_name(group: str, model: str, rep: int, dry_run: bool) -> str:
    # "claude-code" in the name: harness/e2e/trial_metrics.py recalculates cost only for such trials.
    prefix = "dryrun_" if dry_run else ""
    return f"{prefix}claude-code_abbd-{group}_{model.removeprefix('claude-')}_r{rep}"


def read_env_value(path: Path, key: str) -> str:
    for raw in path.read_text().splitlines():
        line = raw.strip().removeprefix("export ")
        if line.startswith(f"{key}="):
            return line.split("=", 1)[1].strip().strip('"').strip("'")
    return ""


def build_env(cfg: dict, group: str, dry_run: bool) -> dict:
    paths = {k: Path(os.path.expanduser(v)) for k, v in cfg["paths"].items()}
    env = {k: v for k, v in os.environ.items() if k not in SCRUBBED}
    env.pop("CLAUDE_CODE_OAUTH_TOKEN", None)
    env.update(
        ABBD_GROUP=group,
        ABBD_INIT_ARGS=cfg["abbd_init_args"],
        PYTHONPATH=str(SITE_DIR),
        SWE_MILESTONE_DATA_ROOT=str(paths["data_root"]),
    )
    if group != "A":
        env["ABBD_PLUGIN"] = str(paths["assets"] / "plugins" / group)
    if group in "CDE":
        env["ABBD_BIN"] = str(paths["assets"] / "abbd-linux")
    if dry_run:
        env["ABBD_CLAUDE_SHIM"] = str(SHIM)
    else:
        token = read_env_value(paths["env_file"], "CLAUDE_CODE_OAUTH_TOKEN")
        if not token:
            sys.exit(f"CLAUDE_CODE_OAUTH_TOKEN missing from {paths['env_file']}")
        env["CLAUDE_CODE_OAUTH_TOKEN"] = token
    for key in ("ABBD_PLUGIN", "ABBD_BIN", "ABBD_CLAUDE_SHIM"):
        if key in env and not Path(env[key]).exists():
            sys.exit(f"{key}={env[key]} does not exist (run stage_assets.sh)")
    return env


def write_trial_config(cfg: dict, name: str, repo: str) -> Path:
    paths = {k: Path(os.path.expanduser(v)) for k, v in cfg["paths"].items()}
    trial = {
        "data_root": str(paths["data_root"]),
        "trial_name": name,
        "agent": "claude-code",
        "agent_version": cfg["agent_version"],
        "model": cfg["model"],
        "default_agent_model": cfg["default_agent_model"],
        "timeout": cfg["timeout"],
        "auto_compact_window": cfg["auto_compact_window"],
        "repos": [repo],
    }
    if cfg.get("reasoning_effort"):
        trial["reasoning_effort"] = cfg["reasoning_effort"]
    out = paths["bench"] / "trial_configs" / f"{name}__{repo}.yaml"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(yaml.safe_dump(trial, sort_keys=False))
    return out


def wait_for(pid: int) -> None:
    while True:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            return
        time.sleep(30)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("group", choices=list(GROUPS))
    ap.add_argument("repo", help="run_all --repos substring, e.g. go-zero")
    ap.add_argument("--rep", type=int, default=1)
    ap.add_argument("--milestones", help="dependency-closed prefix, passed to run_all")
    ap.add_argument("--dry-run", action="store_true", help="fake claude shim, no model call")
    ap.add_argument("--wait", action="store_true", help="block until the repo worker exits")
    mode = ap.add_mutually_exclusive_group()
    mode.add_argument("--force", action="store_true")
    mode.add_argument("--new", action="store_true")
    args = ap.parse_args()

    cfg = yaml.safe_load(CONFIG.read_text())
    harness = Path(os.path.expanduser(cfg["paths"]["harness"]))
    name = trial_name(args.group, cfg["model"], args.rep, args.dry_run)
    config_path = write_trial_config(cfg, name, args.repo)
    cmd = [str(harness / ".venv" / "bin" / "python"), "scripts/run_all.py", "--config", str(config_path),
           "--repos", args.repo]
    if args.milestones:
        cmd += ["--milestones", args.milestones]
    if args.force:
        cmd.append("--force")
    if args.new:
        cmd.append("--new")

    print(f"group={args.group} repo={args.repo} trial={name} config={config_path}", flush=True)
    out = subprocess.run(cmd, cwd=harness, env=build_env(cfg, args.group, args.dry_run),
                         capture_output=True, text=True)
    sys.stdout.write(out.stdout)
    sys.stderr.write(out.stderr)
    if out.returncode != 0:
        sys.exit(out.returncode)
    pids = [int(p) for p in re.findall(r"PID=(\d+)", out.stdout)]
    if args.wait:
        for pid in pids:
            wait_for(pid)
        subprocess.run([sys.executable, str(HERE / "trial_status.py"), "--trial", name, args.repo])


if __name__ == "__main__":
    main()
