"""Status of one (group, problem) run dir, as one JSON line, for run_group.sh and the normalizer.

Usage: python classify_run.py <run dir> <problem>        (needs SCBENCH_PROBLEMS_PATH)
status is one of:
  not_started    no checkpoint has an inference result
  complete       every checkpoint ran without an agent error
  limit_stopped  the first errored checkpoint ended on a usage or rate limit; resume after the reset.
                 Never scored as a task failure.
  agent_error    the first errored checkpoint failed for any other reason; a real result, not resumed
  partial        no error, but some checkpoints have no result yet (an interrupted run); resume it
"""

import json
import os
import re
import sys
from pathlib import Path

LIMIT = re.compile(r"usage limit|hit your limit|limit reached|rate.?limit|\b429\b|overloaded", re.I)


def checkpoint_names(problem: str) -> list[str]:
    config = Path(os.environ["SCBENCH_PROBLEMS_PATH"]) / problem / "config.yaml"
    names = re.findall(r"^  (checkpoint_\d+):", config.read_text(encoding="utf-8"), re.M)
    return sorted(names, key=lambda n: int(n.split("_")[1]))


def result_errors(checkpoint_dir: Path) -> str:
    """is_error result payloads Claude Code printed; a usage-limit stop shows up only here."""
    texts = []
    for stdout in checkpoint_dir.rglob("stdout.jsonl"):
        for line in stdout.read_text(encoding="utf-8", errors="replace").splitlines():
            try:
                payload = json.loads(line)
            except ValueError:
                continue
            if isinstance(payload, dict) and payload.get("type") == "result" and payload.get("is_error"):
                texts.append(str(payload.get("result")))
    return " | ".join(t for t in texts if t)


def classify(run_dir: Path, problem: str) -> dict:
    names = checkpoint_names(problem)
    ran = []
    for name in names:
        path = run_dir / problem / name / "inference_result.json"
        if not path.exists():
            break
        result = json.loads(path.read_text(encoding="utf-8"))
        if result.get("had_error"):
            claude_errors = result_errors(path.parent)
            status = "limit_stopped" if LIMIT.search(claude_errors) else "agent_error"
            error = claude_errors or str(result.get("error_message") or "")
            return {"status": status, "at": name, "ran": ran, "of": len(names), "error": error[-300:]}
        ran.append(name)
    if not ran:
        return {"status": "not_started", "ran": [], "of": len(names)}
    return {"status": "complete" if len(ran) == len(names) else "partial", "ran": ran, "of": len(names)}


def main() -> None:
    run_dir, problem = Path(sys.argv[1]), sys.argv[2]
    print(json.dumps({"run_dir": str(run_dir), "problem": problem, **classify(run_dir, problem)}))


if __name__ == "__main__":
    main()
