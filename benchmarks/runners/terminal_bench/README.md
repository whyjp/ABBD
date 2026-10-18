# Track A. Terminal-Bench 2.0 via Harbor

Not run. Commands below are from Harbor's README and source (harbor-framework/harbor @ 7331d8a, v0.24.0),
read 2026-10-18.

## Facts verified

- Tasks: harbor-framework/terminal-bench-2 @ `2fd12b88aafdd04a52c298e3940bcb189f9766d6`. 89 tasks: 55 medium,
  30 hard, 4 easy (from each `task.toml` `[metadata]`). Agent timeouts range from 600 s to 12000 s; 48 tasks use 900 s.
  No task needs a GPU. Max 4 CPUs and 8192 MB per task. Every task allows internet.
- Harbor: `uv tool install harbor` (PyPI `harbor` 0.24.0, Python >= 3.12). Docker is the default sandbox,
  not the only one (podman, daytona, modal, e2b and others). Windows host support is not documented.
- Claude Code is a built-in agent (`-a claude-code`). Harbor installs it in the container from
  downloads.claude.ai or npm and runs `claude --verbose --output-format=stream-json ... --print`.
  Skills: `--skill <dir>`. MCP: `--mcp-config <file>`. There is no `--plugin-dir` option; the adapter adds it.
- tbench.ai now headlines Terminal-Bench 4.0, and also lists 3.0 and 2.1. The spec asks for 2.0, and that is
  what this track runs. Dataset name: `terminal-bench@2.0`.

## Setup (on a Linux Docker host; see reports/feasibility_and_cost.md for this machine)

```sh
uv tool install harbor==0.24.0
export ANTHROPIC_API_KEY=...
cd <cccv repo>
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /abs/bench/abbd-linux ./cmd/abbd
python benchmarks/adapters/for_my_harness/build_arms.py . /abs/bench/arms
# oracle check: the reference solutions must pass on this host before any agent run
harbor run -d terminal-bench@2.0 -a oracle --n-concurrent 4
```

## Runs

`MODEL` is one exact id from `configs/models.yaml`. `TASKS` is the pilot list from
`manifests/frozen_task_manifest.json` (`terminal_bench_2.pilot`), passed as repeated
`-i <task>` (`--include-task-name`, cli/jobs.py line 1232). Omit `-i` for the full set.

```sh
export PYTHONPATH=<cccv repo>   # so Harbor can import benchmarks.adapters...
A: harbor run -d terminal-bench@2.0 -a claude-code -m anthropic/$MODEL --n-concurrent 4
B: harbor run -d terminal-bench@2.0 -a benchmarks.adapters.for_my_harness.harbor_agent:PstackClaudeCode -m anthropic/$MODEL \
     --ae ABBD_PLUGIN=$HOME/.claude/plugins/cache/pstack-claude/pstack/0.9.93 --n-concurrent 4
C/D/E: harbor run -d terminal-bench@2.0 -a benchmarks.adapters.for_my_harness.harbor_agent:AbbdClaudeCode -m anthropic/$MODEL \
     --ae ABBD_GROUP=E --ae ABBD_BIN=/abs/bench/abbd-linux --ae ABBD_PLUGIN=<cccv>/plugins/abbd --n-concurrent 4
```

For C and D, set `ABBD_GROUP` and point `ABBD_PLUGIN` at `/abs/bench/arms/C` or `/abs/bench/arms/D`.
Pin the Claude Code version for all groups with `--ak version=<x.y.z>` (`InstalledAgentOptions.version`,
agents/installed/base.py line 716). Use the same `--ak max_budget_usd=` and `--ak reasoning_effort=` for all
groups (both are `ClaudeCodeOptions` fields).

## Outputs to keep

Harbor's job directory per run goes to `results/raw/<run_id>/` unchanged. The normalizer reads the
per-trial reward (resolved 0/1), Harbor's timing, `claude-code.txt` stream-json (tokens, cost, tool calls)
and writes rows to `results/normalized/tb2.jsonl`. Official metric is the resolution rate on the full
89-task set. Pilot subset results are labelled `subset=pilot15` and never shown as a 2.0 score.

## Environment check (2026-10-18, WSL2 Ubuntu-24.04, Docker 29.4.1, Harbor 0.24.0)

`harbor run -d terminal-bench@2.0 -a oracle -i cancel-async-tasks -i sparql-university -n 2`: 2/2 trials,
reward 1.0, 0 exceptions, 44 s. Model calls: none.

Open before the pilot: Harbor's registry resolved `terminal-bench@2.0` to
`laude-institute/terminal-bench-2 @ 69671fbaac6d67a7ef0dfec016cc38a64ef7a77c`, not the
`harbor-framework/terminal-bench-2 @ 2fd12b8` recorded in the manifest. Pin one source for every group
(per-task `task_checksum` is in each trial's result.json) and record it in the manifest.

## Subscription auth (user decision 2026-10-18: no API billing)

Runs use the user's Claude subscription, not an API key. Harbor 0.24.0 supports this in
`agents/installed/claude_code.py` (lines ~1771-1811): with `CLAUDE_FORCE_OAUTH=1` it drops
`ANTHROPIC_API_KEY` and passes `CLAUDE_CODE_OAUTH_TOKEN` into the container, where Claude Code runs
`claude --print` (the same `-p` mode as the in-house runner). The token comes from `claude setup-token`
run by the user in WSL; it is stored only in `~/bench/.env` (mode 600) and never printed or committed.

```sh
set -a; . ~/bench/.env; set +a      # CLAUDE_CODE_OAUTH_TOKEN=...
export CLAUDE_FORCE_OAUTH=1
```

Consequence: the dollar figures in reports/feasibility_and_cost.md become usage-limit consumption.
Runs go in small chunks (`-n 1..2`), and a chunk stopped by the usage limit is recorded as
`limit_stopped`, then rerun after the reset. A limit stop is never scored as a task failure.
