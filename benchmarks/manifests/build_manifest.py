#!/usr/bin/env python3
"""Write manifests/frozen_task_manifest.json from manifests/sources/*.json.

The selection rules in RULES were written before this script was first run, and they read only
official metadata (difficulty, category, checkpoint counts, the spec's repo order). They never read
task content, solutions, ABBD's behavior or any result. Re-running the script on the same sources
gives the same selection.
"""
import hashlib
import json
import os
from collections import Counter

HERE = os.path.dirname(os.path.abspath(__file__))
SRC = os.path.join(HERE, "sources")
SALT = "abbd-public-bench-pilot-v1"

RULES = {
    "frozen_at": "2026-10-18",
    "salt": SALT,
    "terminal_bench_2": {
        "population": "all 89 tasks of harbor-framework/terminal-bench-2 at the recorded commit",
        "official_comparison": "the full 2.0 set (89 tasks). A pilot subset score is never shown as a 2.0 score.",
        "pilot_eligible": "metadata.difficulty == 'hard' in task.toml (official label). No other filter: no task "
                          "needs a GPU, max memory is 8192 MB, every task allows internet.",
        "pilot_size": 15,
        "pilot_draw": "stratify the hard tasks by metadata.category; allocate 15 slots proportionally by largest "
                      "remainder (ties broken by category name); within each category take the tasks with the "
                      "smallest sha256(salt + ':' + task_id).",
    },
    "swe_milestone": {
        "population": "the 7 repos of DeepCommit-ai/SWE-Milestone-data at the recorded tag",
        "official": "B1 go-zero, B2 Element Web, B3 Apache Dubbo, B4 Nushell (spec order); extension ripgrep, scikit-learn, Navidrome",
        "pilot": "the first two of the spec's B1..B4 order: go-zero (Go) and Element Web (TypeScript). This also gives one "
                 "repo where ABBD's graph is strongest (Go AST) and one where it is regex-only (TS).",
        "milestones": "all graded milestones of each selected repo, in the official DAG, with the official e2e_config",
    },
    "slop_code_bench": {
        "population": "all 36 problems of gabeorlanski/scb-problems at the recorded commit",
        "pilot": "every problem whose checkpoint count equals the maximum (8), plus the two problems the spec names as "
                 "examples (execution_server, file_backup), which are flagged spec_named",
    },
    "swe_ci": {"pilot": "none selected in phase 1. The official harness has no Claude Code adapter (see runners/swe_ci)."},
    "skillsbench": {"pilot": "none selected in phase 1. Optional track; SE subset is 16 tasks (category software-engineering)."},
}


def h(task_id):
    return hashlib.sha256(f"{SALT}:{task_id}".encode()).hexdigest()


def largest_remainder(counts, slots):
    total = sum(counts.values())
    quotas = {c: n * slots / total for c, n in counts.items()}
    alloc = {c: int(q) for c, q in quotas.items()}
    for c in sorted(quotas, key=lambda c: (-(quotas[c] - alloc[c]), c))[: slots - sum(alloc.values())]:
        alloc[c] += 1
    return alloc


def tb2(src):
    tasks = src["tasks"]
    hard = [t for t in tasks if t["difficulty"] == "hard"]
    by_cat = Counter(t["category"] for t in hard)
    alloc = largest_remainder(by_cat, RULES["terminal_bench_2"]["pilot_size"])
    pilot = []
    for cat in sorted(alloc):
        ranked = sorted((t for t in hard if t["category"] == cat), key=lambda t: h(t["id"]))
        pilot += [{"id": t["id"], "category": cat, "difficulty": t["difficulty"], "agent_timeout_sec": t["agent_timeout_sec"],
                   "cpus": t["cpus"], "memory_mb": t["memory_mb"]} for t in ranked[: alloc[cat]]]
    return {"source": src["source"], "commit": src["commit"], "fetched": src["fetched"], "harbor_dataset": "terminal-bench@2.0",
            "counts": {"total": len(tasks), "by_difficulty": dict(Counter(t["difficulty"] for t in tasks)), "hard_by_category": dict(by_cat)},
            "allocation": alloc, "pilot": pilot, "full": sorted(t["id"] for t in tasks)}


def swe_milestone(src):
    repos = {r["name"]: r for r in src["repos"]}
    pick = lambda names: [{k: repos[n][k] for k in ("name", "github", "language", "base_tag", "end_tag", "graded_milestones")} for n in names]
    return {"repo": src["repo"]["url"], "commit": src["repo"]["head_sha"], "benchmark_version": src["repo"]["benchmark_version"],
            "dataset": src["dataset"]["url"], "dataset_commit": src["dataset"]["head_sha"], "fetched": src["fetched"],
            "pilot": pick(["go-zero", "Element Web"]), "official": pick(["go-zero", "Element Web", "Apache Dubbo", "Nushell"]),
            "extension": pick(["ripgrep", "scikit-learn", "Navidrome"])}


def scb(src):
    top = max(p["checkpoints"] for p in src)
    named = {"execution_server", "file_backup"}
    pilot = [{"problem": p["problem"], "checkpoints": p["checkpoints"], "category": p["language/category if stated"],
              "difficulty": p.get("difficulty"), "spec_named": p["problem"] in named}
             for p in src if p["checkpoints"] == top or p["problem"] in named]
    return {"runner": "https://github.com/SprocketLab/slop-code-bench", "runner_commit": "31ceea3add480edb33431e70475c4c70597e6b31",
            "problems_repo": "https://github.com/gabeorlanski/scb-problems", "problems_commit": "9cd9ca3a51c3d3e2a99d2488a25baf73a2204451",
            "fetched": "2026-10-18", "counts": {"problems": len(src), "checkpoints": sum(p["checkpoints"] for p in src)},
            "pilot": sorted(pilot, key=lambda p: (-p["checkpoints"], p["problem"]))}


def selected_tasks_yaml(m):
    tb, sm, sc = m["terminal_bench_2"], m["swe_milestone"], m["slop_code_bench"]
    lines = ["# Generated by manifests/build_manifest.py from frozen_task_manifest.json. Do not edit by hand.",
             f"# Rules frozen {RULES['frozen_at']}; see the manifest's `rules` block.", "",
             "terminal_bench_2:", f"  dataset: {tb['harbor_dataset']}", f"  commit: {tb['commit']}",
             "  official: full   # all 89 tasks", "  pilot:"]
    lines += [f"    - {t['id']}   # {t['category']}, hard, timeout {t['agent_timeout_sec']}s" for t in tb["pilot"]]
    lines += ["", "swe_milestone:", f"  benchmark_version: {sm['benchmark_version']}", f"  repo_commit: {sm['commit']}",
              f"  dataset_commit: {sm['dataset_commit']}", "  pilot:"]
    lines += [f"    - {r['name']}   # {r['language']}, {r['graded_milestones']} graded milestones" for r in sm["pilot"]]
    lines += ["  official:"] + [f"    - {r['name']}" for r in sm["official"]]
    lines += ["  extension:"] + [f"    - {r['name']}" for r in sm["extension"]]
    lines += ["", "slop_code_bench:", f"  runner_commit: {sc['runner_commit']}", f"  problems_commit: {sc['problems_commit']}", "  pilot:"]
    lines += [f"    - {p['problem']}   # {p['checkpoints']} checkpoints{', spec-named example' if p['spec_named'] else ''}" for p in sc["pilot"]]
    lines += ["", "swe_ci: {pilot: []}   # no Claude Code adapter in the official harness", "skillsbench: {pilot: []}   # optional", ""]
    return "\n".join(lines)


def main():
    load = lambda n: json.load(open(os.path.join(SRC, n), encoding="utf-8"))
    manifest = {"rules": RULES, "terminal_bench_2": tb2(load("tb2_tasks.json")),
                "swe_milestone": swe_milestone(load("swe_milestone.json")), "slop_code_bench": scb(load("scb_problems.json"))}
    # Hand-maintained run pins are not derived from sources/; keep them across rebuilds.
    out = os.path.join(HERE, "frozen_task_manifest.json")
    if os.path.exists(out):
        old = json.load(open(out, encoding="utf-8"))
        if "run_pin" in old.get("terminal_bench_2", {}):
            manifest["terminal_bench_2"]["run_pin"] = old["terminal_bench_2"]["run_pin"]
        if "cross_harness_arms" in old:
            manifest["cross_harness_arms"] = old["cross_harness_arms"]
    json.dump(manifest, open(os.path.join(HERE, "frozen_task_manifest.json"), "w", encoding="utf-8"), indent=2)
    open(os.path.join(os.path.dirname(HERE), "configs", "selected_tasks.yaml"), "w", encoding="utf-8", newline="\n").write(selected_tasks_yaml(manifest))
    print(json.dumps({"tb2_pilot": [t["id"] for t in manifest["terminal_bench_2"]["pilot"]],
                      "tb2_alloc": manifest["terminal_bench_2"]["allocation"],
                      "scb_pilot": [p["problem"] for p in manifest["slop_code_bench"]["pilot"]]}, indent=1))


if __name__ == "__main__":
    main()
