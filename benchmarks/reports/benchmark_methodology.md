# Benchmark methodology

Status: phase 1 (spec §12 steps 1 to 5). Nothing here has been run against a model. No score in this
directory comes from our runs until `results/normalized/` holds rows.

## 1. What is compared

The harness under test is ABBD (formerly "For My Harness"): the Go CLI `abbd` plus the Claude Code
plugin `plugins/abbd` (8 skills, MCP server `abbd mcp`, 5 hooks). The host agent in every group is
Claude Code, so the only variable between groups is what the harness adds. Groups and what each one
actually enables are defined in `configs/harnesses.yaml`. The capability verdict for each group is in
`feasibility_and_cost.md` §1. A group that cannot be built from shipped code is marked `not_runnable`
and is never reported with a score.

| group | definition (spec §7) | runnable today |
|---|---|---|
| A | Claude Code, no plugin | yes |
| B | A + pstack 0.9.93 | yes (pstack is installed on this host) |
| C | A + ABBD skills only | see `configs/harnesses.yaml` |
| D | C + Architecture SSOT + Feature Map | see `configs/harnesses.yaml` |
| E | D + Code/Module KG MCP | see `configs/harnesses.yaml` |
| F | E + Domain State Graph + full auto-update loop | not runnable: domain layer is reserved only |

Group E is the ABBD plugin exactly as shipped (all hooks, skills, MCP, CLI gates). Groups C and D are
partial ablations of it, and their caveats travel with every number they produce.

## 2. Fairness controls (spec §2)

Every run records these fields in `results/raw/<run_id>/run_meta.json`, and the normalizer copies
them into each row. A run missing any of them is invalid and is not reported.

| control | how it is fixed |
|---|---|
| model id and version | one exact id per comparison from `configs/models.yaml`; no alias such as `sonnet` in official runs |
| provider and inference settings | Anthropic API direct; default temperature; thinking/effort setting recorded verbatim |
| harness version and commit | ABBD git SHA, pstack version, Claude Code version (`claude --version`) |
| Docker image and dataset version | image digest per task; dataset git SHA or registry version from `manifests/frozen_task_manifest.json` |
| CPU and RAM | host and per-container limits as the benchmark's own task config sets them; never raised per group |
| time and token budget per task | the benchmark's official timeout; one USD cap per session (`--max-budget-usd`) identical across groups |
| network policy | whatever the benchmark specifies; the same for every group |
| human intervention | none; any manual action invalidates the run and is logged |
| repetitions and seeds | at least 3 reps per group in official runs; rep index recorded; benchmark seeds left at their defaults |

Same-model, same-task comparisons come first. External leaderboard numbers are shown only in their own
section with source URL, fetch date, harness, version and model, and with the conditions that differ
from ours (timeouts, hardware, model, number of trials) listed next to them.

Forbidden in every group: putting verifier code, reference solutions, hidden tests, or future
milestones into the agent's context; rewriting requirements; lowering difficulty; tuning ABBD on the
evaluated tasks. ABBD's own bootstrap (`abbd init --mode code-only`) is allowed because it reads only
the task's starting repository, and its time and cost count toward the run's totals.

## 3. Task selection

The selection rule is written in `manifests/frozen_task_manifest.json` before any task was chosen.
Pilot subsets are drawn by that rule. A subset score is always labelled as a subset and never shown as
a full-benchmark score. Official comparison on Terminal-Bench uses the full 2.0 set.

## 4. Metrics: official and in-house stay separate

Official metrics are taken from each benchmark's own evaluator output, unchanged:

| benchmark | official metric(s) |
|---|---|
| Terminal-Bench 2.0 | resolution rate (task passes all its tests), per the Harbor verifier |
| SWE-Milestone | Score, Precision, Recall, Fully Resolved, as the official evaluator defines them |
| SlopCodeBench | per-checkpoint pass results and the benchmark's own quality metrics |
| SWE-CI | Average Normalized Change (ANC), README definition (not run in phase 2) |
| SkillsBench | pass rate with and without skills, normalized gain g (optional, not scaffolded) |

In-house metrics are computed by our scripts, stored under `in_house` in each normalized row, and
never combined into an official score:

- cumulative feature preservation: share of previously passing tests still passing after each step;
- new failures of previously passing tests (regressions), per step;
- new-feature success rate per step;
- impacted modules and interfaces count, and graph impact-analysis hit and miss against the
  ground-truth changed set (the ground truth is the diff and the failing tests, never ABBD's own graph);
- SSOT / Feature Map update accuracy against the ground-truth diff;
- cumulative cost growth, and cost of the next step;
- checkpoint-to-checkpoint code churn and static structure measures (supporting evidence only).

Progress snapshots at 25, 50, 75 and 100% of milestones or checkpoints are taken for SWE-Milestone
and SlopCodeBench.

## 5. Statistics

- Pilot: 1 rep per group. Pilot output is for environment and failure classification only. It is
  never used to claim a difference.
- Official: at least 3 reps per group per task.
- Per task, the group's value is the mean over reps. Each comparison is paired by task between two
  groups on the same model.
- 95% CI: percentile bootstrap over tasks of the paired per-task difference (10,000 resamples,
  `reports/build_comparison.py`). With binary outcomes on fewer than about 30 tasks the interval is
  wide. It is reported as computed, not narrowed.
- Counts reported with every comparison: improved, worsened, both solved, both failed, same partial.
- Efficiency: extra tasks solved per extra USD and per extra minute, against group A.
- Timeouts and environment errors are counted separately and are not silently dropped. A task that
  hit an environment error in one group is re-run for that group; if it still fails it is excluded
  from the pair for both groups and the exclusion is listed.
- Trade-offs: if a group is slower or more expensive and solves more, the cost per extra solve is
  reported. If it only costs more, the report says so.

## 6. Hypotheses and how each is tested

| hypothesis | primary evidence | track | what would refute it |
|---|---|---|---|
| H1 cumulative feature pass rate is higher | SWE-Milestone Score and Fully Resolved at 25/50/75/100%; SCB checkpoint pass rate by checkpoint index | B, C | paired difference vs A and vs B with CI covering 0 or negative |
| H2 architecture violations grow more slowly | regressions per step; SCB structure measures per checkpoint; ABBD contract-violation records checked against the real diff | B, C | slope of regressions or structure decay not lower than A |
| H3 impact analysis misses fewer affected sites | graph-predicted impact set vs ground truth (files and symbols touched by the reference change plus newly failing tests) | B (E vs D vs C) | recall not higher than plain search (group C) |
| H4 future change cost is lower | cost and turns of step k+1 given steps 1..k; SCB next-checkpoint cost | B, C | later-step cost not lower than A |

No difference is a result and is reported. Evidence that refutes a hypothesis is reported in the same
table as evidence that supports one.

## 7. What our in-house suite already measures

`benchmark/run.py` runs the same cumulative task sequence through isolated arms (`claude -p ...
--setting-sources project --plugin-dir <arm>`), scores with hidden Go tests the agent never sees, and
records cost, turns and duration per task.

- v1 (10 self-contained feature tasks, 1 rep): every arm 10/10.
- v2 (12 cumulative tasks where early tasks state project-wide rules, 2 reps, sonnet, abbd 86fe734):
  feature first-try 100% for every arm. Rule final 98% for baseline and pstack, 100% for
  superpowers, gstack and abbd. Regressions 0 everywhere. Mean cost per 12-task run $1.57 (baseline)
  to $2.03 (pstack); abbd $1.92 with the fewest turns (86) and the longest time (6.5 min).

The suite is saturated. All arms sit at or near the ceiling, and the only difference (one rule missed
once in two reps by baseline and pstack) is not significant at n=2. It does not separate harnesses,
which is why this public-benchmark plan exists. It also has two known confounds: arms share the host
`~/.claude/CLAUDE.md` (`--setting-sources project` does not remove it), and the model alias `sonnet`
was used instead of an exact id. Both are fixed in this plan. Public-benchmark runs happen inside
containers, which have no host `~/.claude`, and §2 requires an exact model id. `claude --bare` is not a fix
for host runs, because it also skips plugin hooks, which ABBD needs.

## 8. Knowledge-graph quality (spec §8)

KG quality is measured against a reference extracted by an independent tool, never ABBD's own
extractor. Candidate references: compiler-grade indexes (gopls / `go list -json` + `go/types` for Go,
SCIP indexers for TS/Java/Rust/Python). Measured: node and edge precision/recall, definition and
reference accuracy, call edges, module boundaries. Freshness: graph revision equals HEAD after each
step, renames and moves reflected, deleted symbols gone, full rebuild equals incremental result (ABBD
always rebuilds fully, so this check is trivially equal and is reported as such). Domain integrity
(valid transitions kept, invalid blocked, invariants) is not measurable because the domain layer is
not implemented.

## 9. Reporting

`reports/build_comparison.py` builds `harness_comparison.csv` and `harness_comparison.html` from
`results/normalized/*.jsonl` only. With no rows it writes no numbers. Every row carries run id, raw
log path, harness revision and dataset revision. A partially executed plan reports completed and
not-run cells separately.
