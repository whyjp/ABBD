"""Check the scb-problems checkout against the frozen manifest.

Usage: python check_problems.py <scb-problems dir> <frozen_task_manifest.json>
Prints problem and checkpoint counts and exits 1 on any mismatch with the manifest.
Needs PyYAML (run it with the slop-code-bench venv: `uv run --project <runner> python ...`).
"""

import json
import sys
from pathlib import Path

import yaml


def main() -> int:
    problems_dir, manifest_path = Path(sys.argv[1]), Path(sys.argv[2])
    section = json.loads(manifest_path.read_text(encoding="utf-8"))["slop_code_bench"]
    counts = {}
    for config in sorted(problems_dir.glob("*/config.yaml")):
        data = yaml.safe_load(config.read_text(encoding="utf-8"))
        counts[data["name"]] = len(data["checkpoints"])
    total = sum(counts.values())
    print(f"problems={len(counts)} checkpoints={total}")
    errors = []
    if (len(counts), total) != (section["counts"]["problems"], section["counts"]["checkpoints"]):
        errors.append(f"manifest counts {section['counts']} != found {len(counts)}/{total}")
    for entry in section["pilot"]:
        found = counts.get(entry["problem"])
        has_solutions = (problems_dir / entry["problem"] / "solutions").is_dir()
        print(f"  {entry['problem']}: checkpoints={found} manifest={entry['checkpoints']} reference_solutions={has_solutions}")
        if found != entry["checkpoints"]:
            errors.append(f"{entry['problem']}: {found} != {entry['checkpoints']}")
    for error in errors:
        print("MISMATCH", error)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
