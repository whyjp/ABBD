#!/usr/bin/env python3
"""Stand-in for `claude` in the no-model dry run. Never calls a model.

Called by the runner exactly like the real CLI (stream-json, --print, prompt last). It records what the
real agent would have been given (flags, auth env, PATH, workspace, `.abbd/` state), validates any
--plugin-dir with the real Claude Code CLI, runs a few model-free `abbd` commands when abbd is on PATH,
then writes the reference solution of the checkpoint whose spec is in the prompt, so the official evaluator
has something real to score. FAKE_LIMIT_AT=N makes checkpoint N end the way a usage-limit stop does,
until the epoch FAKE_LIMIT_UNTIL if set (so a later --resume succeeds, like after a limit reset).
"""

import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path

SOLUTIONS = Path(os.environ.get("FAKE_SOLUTIONS", "/opt/fake/solutions"))


def emit(payload):
    print(json.dumps(payload), flush=True)


def run(cmd):
    try:
        done = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
        return {"cmd": " ".join(cmd), "exit": done.returncode, "out": (done.stdout + done.stderr)[-600:]}
    except (OSError, subprocess.TimeoutExpired) as error:
        return {"cmd": " ".join(cmd), "error": repr(error)}


def flag(argv, name):
    return argv[argv.index(name) + 1] if name in argv else None


def checkpoint_in(prompt):
    # Specs share boilerplate and may hold template placeholders, so match the most distinctive plain lines.
    best, best_hits = None, 0
    for spec in sorted(SOLUTIONS.glob("specs/checkpoint_*.md")):
        lines = [l.strip() for l in spec.read_text().splitlines() if len(l.strip()) > 30 and "{" not in l]
        hits = sum(1 for l in lines if l in prompt)
        if lines and hits / len(lines) > 0.8 and hits > best_hits:
            best, best_hits = int(spec.stem.split("_")[1]), hits
    return best


def main():
    argv = sys.argv[1:]
    prompt = argv[-1] if argv else ""
    cwd = Path.cwd()
    # The runner retries a failed checkpoint once with `--continue` and a generic prompt.
    last = Path(os.environ.get("HOME", "/tmp")) / ".claude" / "fake-last-checkpoint"
    ckpt = checkpoint_in(prompt)
    if ckpt is None and "--continue" in argv and last.exists():
        ckpt = int(last.read_text())
    if ckpt is not None:
        last.write_text(str(ckpt))
    limited = str(ckpt) == os.environ.get("FAKE_LIMIT_AT") and (
        not os.environ.get("FAKE_LIMIT_UNTIL") or time.time() < float(os.environ["FAKE_LIMIT_UNTIL"]))
    real_claude = shutil.which("claude")
    probe = {
        "argv_flags": [a for a in argv[:-1]],
        "prompt_chars": len(prompt),
        "checkpoint": ckpt,
        "cwd": str(cwd),
        "home": os.environ.get("HOME"),
        "oauth_token_set": bool(os.environ.get("CLAUDE_CODE_OAUTH_TOKEN")),
        "anthropic_api_key_set": bool(os.environ.get("ANTHROPIC_API_KEY")),
        "anthropic_auth_token_set": bool(os.environ.get("ANTHROPIC_AUTH_TOKEN")),
        "effort": os.environ.get("CLAUDE_CODE_EFFORT_LEVEL"),
        "abbd_on_path": shutil.which("abbd"),
        "real_claude": real_claude,
        "abbd_dir_before": (cwd / ".abbd").is_dir(),
        "workspace_before": sorted(p.name for p in cwd.iterdir()),
    }
    if real_claude:
        probe["claude_version"] = run([real_claude, "--version"])
        plugin_dir = flag(argv, "--plugin-dir")
        if plugin_dir:
            probe["plugin_validate"] = run([real_claude, "plugin", "validate", plugin_dir])
    if probe["abbd_on_path"] and probe["abbd_dir_before"]:
        probe["abbd_run"] = run(["abbd", "run", f"dry-run checkpoint {ckpt}"])
        probe["abbd_status"] = run(["abbd", "status"])
    emit({"type": "system", "subtype": "init", "session_id": "dryrun", "model": flag(argv, "--model")})
    usage = {"input_tokens": 0, "output_tokens": 0, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}
    if limited:
        emit({"type": "system", "subtype": "dryrun_probe", "probe": probe})
        emit({"type": "result", "subtype": "success", "is_error": True, "total_cost_usd": 0.0, "usage": usage,
              "result": "You've hit your limit · resets 3am (UTC) [dry-run simulated]"})
        return 1
    if ckpt is not None:
        shutil.copytree(SOLUTIONS / f"checkpoint_{ckpt}", cwd, dirs_exist_ok=True)
    probe["workspace_after"] = sorted(p.name for p in cwd.iterdir())
    emit({"type": "system", "subtype": "dryrun_probe", "probe": probe})
    emit({"type": "assistant", "message": {"id": f"dryrun-{ckpt}", "role": "assistant", "usage": usage,
          "content": [{"type": "text", "text": f"dry run wrote reference solution {ckpt}"}]}})
    emit({"type": "result", "subtype": "success", "is_error": False, "total_cost_usd": 0.0, "usage": usage,
          "result": f"dry run checkpoint {ckpt}"})
    return 0


if __name__ == "__main__":
    sys.exit(main())
