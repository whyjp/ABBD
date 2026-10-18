# Track C. SlopCodeBench

Not run. Commands are from SprocketLab/slop-code-bench @ `31ceea3add480edb33431e70475c4c70597e6b31` and its
README and docs, read 2026-10-18.

## Facts verified

- Problems live in **gabeorlanski/scb-problems** (@ `9cd9ca3a51c3d3e2a99d2488a25baf73a2204451`, 2026-10-07).
  `github.com/SprocketLab/scb-problems` does not exist; the runner README says the problems moved.
- 36 problems, 196 checkpoints (counted from each `config.yaml`; scbench.ai says the same for v1.0).
  `file_backup` has 4 checkpoints. `execution_server` has 6. Six problems have the maximum of 8:
  circuit_eval, forge, meshctl, mocked_http, recli, test_translator.
- Runner: Python 3.12+, `uv`, Docker. README recommends 8 GB+ RAM and 10 GB+ disk.
- Built-in `claude_code` agent with `extra_args` (appended to the `claude` command), `env`, `version`,
  `permission_mode`. No first-class MCP or plugin-dir option.
- Each checkpoint re-runs earlier checkpoints' tests (regression group).
- Metrics (docs/metrics-reference.md): `checkpoint_pass_rate` excludes regression tests; `pass_rate`
  includes them; a checkpoint is solved at `pass_rate == 1.0`. Quality metrics include loc, cyclomatic
  complexity, lint, `cloned_pct`, and the composite `verbosity` and `erosion` from the pinned `scb-check`
  release (no formula in the docs).

## Setup

```sh
git clone https://github.com/SprocketLab/slop-code-bench && cd slop-code-bench && git checkout 31ceea3
uv sync
# problems: follow the runner README to fetch gabeorlanski/scb-problems @ 9cd9ca3
# shared image for all groups (adapters/for_my_harness/scb/Dockerfile), from a staging dir:
docker build -t abbd-bench/scb-python3.12:<abbd commit> <staging dir>
```

## Runs

```sh
uv run slop-code run \
  --agent <cccv>/benchmarks/adapters/for_my_harness/scb/agent-group-<G>.yaml \
  --model anthropic/<model> \
  --environment <cccv>/benchmarks/adapters/for_my_harness/scb/environment-abbd-python3.12.yaml \
  --prompt configs/prompts/just-solve.jinja \
  --problem circuit_eval --problem forge --problem meshctl --problem mocked_http \
  --problem recli --problem test_translator --problem execution_server --problem file_backup \
  thinking=<same for all groups>
uv run slop-code eval <run-dir>
```

`thinking=` and the Claude Code `version` are the same for every group. Group A uses
`agent-group-A.yaml` and the same shared image.

## Outputs to keep

The run directory goes to `results/raw/<run_id>/`. Official per-checkpoint results come from
`slop-code eval` unchanged: `checkpoint_pass_rate`, `pass_rate`, solved flags, `problem_solved`. Quality
metrics (erosion, verbosity, cc, clones) are recorded as reported and treated as supporting evidence.
In-house: cumulative regression across checkpoints, churn per checkpoint, cost of checkpoint k+1, and
snapshots at 25/50/75/100% of each problem's checkpoints.

## Open items before the pilot

- Confirm the agent user's HOME and PATH handling for `env` (see agent-group-C/D/E.yaml).
- Confirm that the quality metrics ignore `.abbd/` in the workspace.
- `--agent` accepting a config file path is from the docs; not tried.
