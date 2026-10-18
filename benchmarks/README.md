# Public benchmarks for ABBD

This directory evaluates ABBD (formerly "For My Harness") on public benchmarks against the same Claude Code
agent without it, with pstack, and with partial ABBD configurations. The goal is to learn under which
conditions ABBD's structure helps, not to prove it is best.

Status: phase 1 of the mission spec (section 12, steps 1 to 5). No model has been run. No file here holds a
score from our runs. External scores are in `results/external_leaderboard/` with source and fetch date.

## Layout

| path | what it is |
|---|---|
| `configs/models.yaml` | candidate model ids and prices; the primary model is a user decision |
| `configs/harnesses.yaml` | groups A to F mapped to what ABBD can actually toggle, with verdicts |
| `configs/selected_tasks.yaml` | pilot and official task lists, generated from the manifest |
| `manifests/build_manifest.py` | the frozen selection rules and the script that applies them |
| `manifests/frozen_task_manifest.json` | rules, dataset commits, pilot and full lists |
| `manifests/sources/` | the official metadata the rules read (TB2 task.toml fields, SCB checkpoint counts, SWE-Milestone repos) |
| `adapters/for_my_harness/` | adapter design, arm builder, Harbor and SWE-Milestone agent sketches, SCB configs (all untested) |
| `runners/<track>/README.md` | exact setup and run commands per track |
| `results/raw/` | unmodified benchmark output per run id (empty) |
| `results/normalized/` | one JSON row per run and task, schema in `reports/build_comparison.py` (empty) |
| `results/external_leaderboard/` | verified public scores: `leaderboard.json` and the raw TB2 2.0 feed |
| `reports/benchmark_methodology.md` | fairness controls, statistics, hypotheses H1 to H4 |
| `reports/feasibility_and_cost.md` | capability inventory, blockers on this machine, cost table, user decisions |
| `reports/cost_model.py` | the cost estimate with every assumption in one place |
| `reports/build_comparison.py` | builds `harness_comparison.csv` and `.html` from normalized rows only |

## Tracks

| track | benchmark | state |
|---|---|---|
| A | Terminal-Bench 2.0 via Harbor 0.24.0 | adapter sketched; needs a Linux container runtime |
| B | SWE-Milestone v1.0.2 | adapter sketched; needs Docker on Linux; pilot go-zero and Element Web |
| C | SlopCodeBench (problems in gabeorlanski/scb-problems) | configs sketched; needs Docker |
| D | SWE-CI, SkillsBench | facts recorded; not adapted (no Claude Code adapter in SWE-CI; SkillsBench tests a different claim) |

## Rebuild the generated files

```sh
python -I manifests/build_manifest.py      # same sources give the same selection
python -I reports/cost_model.py            # prints the cost table
python -I reports/build_comparison.py      # writes no numbers until results/normalized has rows
```

Nothing in this directory is committed yet.
