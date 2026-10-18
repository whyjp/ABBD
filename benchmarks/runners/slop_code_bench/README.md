# Track C. SlopCodeBench

Environment checked on 2026-10-19 in WSL2 Ubuntu-24.04 (Docker 29.4.1, uv 0.11.6) with zero model calls.
No agent run with a model has happened, so nothing here is a score.

Pins: runner SprocketLab/slop-code-bench @ `31ceea3add480edb33431e70475c4c70597e6b31`, problems
gabeorlanski/scb-problems @ `9cd9ca3a51c3d3e2a99d2488a25baf73a2204451`, Claude Code 2.1.302 (same as the TB2
smoke), model `claude-sonnet-5-5`, `thinking=none` (Claude Code's default effort), prompt `just-solve`,
pass policy `any` (runner default), `--max-turns 100` (stock agent config). The manifest records the same
under `slop_code_bench.run_pin`.

## Files

| file | role |
|---|---|
| `setup.sh` | clone and pin both repos, `uv sync`, add the sonnet-5.5 model file, check problem counts, build the shared image, render the environment config |
| `check_problems.py` | recounts problems and checkpoints from each `config.yaml` against the manifest; exits 1 on a mismatch |
| `refcheck.sh` | no-model evaluator check: reference solution of every checkpoint, plus an empty submission |
| `abbd_scoring_check.sh` | scores one reference solution with and without a populated `.abbd/` and diffs every output |
| `run_group.sh` | the pilot runner: one group, problems in order, concurrency 1, resume and `limit_stopped` handling |
| `classify_run.py` | status of one (group, problem) run dir: `not_started`, `partial`, `complete`, `limit_stopped`, `agent_error` |
| `dryrun.sh` | runs `run_group.sh` end to end with a fake `claude` that writes reference solutions |
| `../../adapters/for_my_harness/scb/` | agent configs A to E, the shared Dockerfile, the `claude-abbd` launcher, the environment template, the model file, the dry-run layer |

## How the adapter plugs in

The runner's `claude_code` agent runs `<binary> --output-format stream-json --verbose --model M --max-turns 100
--permission-mode bypassPermissions <extra_args> --print -- <prompt>` in the problem workspace, with
`<env>` added to the process environment and to `~/.claude/settings.json`. Each checkpoint is a fresh
`claude -p` call in the same workspace, so checkpoint k+1 starts from the code checkpoint k left.
The agent configs differ only in `binary`, `extra_args` and `env`:

| group | binary | extra_args | env |
|---|---|---|---|
| A | `claude` | none | none |
| B | `claude` | `--plugin-dir /opt/bench-plugins/pstack` | none |
| C | `/opt/abbd/launch/claude-abbd` | `--plugin-dir /opt/bench-plugins/C` | none |
| D | `/opt/abbd/launch/claude-abbd` | `--plugin-dir /opt/bench-plugins/D` | `ABBD_INIT=1` |
| E | `/opt/abbd/launch/claude-abbd` | `--plugin-dir /opt/bench-plugins/E` | `ABBD_INIT=1` |

`claude-abbd` puts `/opt/abbd/bin` on PATH (so `abbd`, the hooks and E's `abbd mcp` server find it), runs
`abbd init` when `ABBD_INIT=1` and the workspace has no `.abbd/` yet, then execs `claude`. Because `.abbd/`
stays in the workspace, init runs once per problem, at checkpoint 1. Init output goes to the checkpoint's
`agent/stderr.log`. Every group uses the same image (`abbd-bench/scb-python3.12:<hash>`), so `abbd` and all
plugin dirs exist in A's and B's container too, but off PATH. The image tag is a hash of the staged context
(binary, launcher, plugins); slop-code reuses derived images by environment name, so a changed binary or plugin
gets a new name instead of a stale image.

Auth: `--model claude_code_oauth/sonnet-5.5` makes the runner read `CLAUDE_CODE_OAUTH_TOKEN` (its built-in
`claude_code_oauth` provider) and pass it into the container as `CLAUDE_CODE_OAUTH_TOKEN`; no API key is set.
The dry run showed `CLAUDE_CODE_OAUTH_TOKEN` set and `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` unset inside
the agent process. `run_group.sh` reads the token from `~/bench/.env` and never prints it.

## `.abbd/` and scoring

`.abbd/` is kept in every checkpoint snapshot on purpose. `--resume` restores the workspace from the last good
snapshot, and D/E need `.abbd/` back after a usage-limit stop. Nothing official reads it:

- Correctness runs the problem's pytest suite against the entry file; `.abbd/` is never imported.
- The per-snapshot quality metrics (`measure_snapshot_quality`) only measure files with a supported language,
  and Python is the only one at this commit. `.abbd/` holds md, yaml, json, html and one `.ignore` (27 files
  after `abbd init` plus one `abbd run`), no `.py`.
- Churn (`lines_added`/`lines_removed`) is summed only over files that have file metrics, so the same filter
  applies.
- `scb-check 0.1.3 check --report --include-all` (erosion, verbosity, clones) gave the identical report.

Measured by `abbd_scoring_check.sh file_backup 1`: every output (correctness report, `evaluation.json`,
`files.jsonl`, `symbols.jsonl`, `overall_quality.json`, scb-check report) is identical with and without
`.abbd/`, apart from a `created` timestamp and the filesystem order of `source_files`, which the script
ignores. End to end, the dry runs of group A (no `.abbd/`) and group E (`.abbd/` in every snapshot) on the same
reference code gave identical per-checkpoint tests passed, loc, verbosity, erosion, cloned_pct and lines_added,
and the same run summary (pass rate 1.000, lint:loc 0.1228, verbosity 0.2453, erosion 0.6141). No exclusion
was needed, so none was added. If a later ABBD
version writes `.py` files under `.abbd/`, this stops holding; rerun `abbd_scoring_check.sh` after an ABBD
upgrade.

## Setup (once per ABBD build)

```sh
export CCCV=/mnt/c/Users/cxx/orca/projects/cccv
export ABBD_BIN=/mnt/c/Users/cxx/orca/projects/abbd-bench-pub/abbd-linux
export ARMS=/mnt/c/Users/cxx/orca/projects/abbd-bench-pub/arms
export PSTACK=/mnt/c/Users/cxx/.claude/plugins/cache/pstack-claude/pstack/0.9.93
bash $CCCV/benchmarks/runners/slop_code_bench/setup.sh        # 26 s when the repos are cached
bash $CCCV/benchmarks/runners/slop_code_bench/refcheck.sh file_backup execution_server
```

Scripts on the Windows checkout may carry CRLF; run them as `bash <(sed 's/\r$//' <script>)` from WSL.
The first evaluation builds slop-code's base image on top of ours (apt, node, rust, then a `chown -R` of the
agent home): 6.0 GB and about 33 min on this host while other benchmark agents shared Docker. The first agent
run builds the Claude Code layer on top (about 15 min). Both are reused afterwards.

## Pilot commands

One command per group. Each runs the 8 pilot problems one after another (58 checkpoints), one problem at a
time, and is safe to rerun: finished problems are skipped, `limit_stopped` and `partial` ones are resumed,
`agent_error` ones are kept.

```sh
P="file_backup execution_server forge circuit_eval meshctl mocked_http recli test_translator"
bash $CCCV/benchmarks/runners/slop_code_bench/run_group.sh A pilot-<abbd commit> $P
bash $CCCV/benchmarks/runners/slop_code_bench/run_group.sh B pilot-<abbd commit> $P
bash $CCCV/benchmarks/runners/slop_code_bench/run_group.sh C pilot-<abbd commit> $P
bash $CCCV/benchmarks/runners/slop_code_bench/run_group.sh D pilot-<abbd commit> $P
bash $CCCV/benchmarks/runners/slop_code_bench/run_group.sh E pilot-<abbd commit> $P
```

Output: `~/bench/scb/outputs/<label>/group-<G>/<problem>/` (the runner's own layout: per checkpoint
`snapshot/`, `agent/stdout.jsonl`, `evaluation.json`, `quality_analysis/`, plus `checkpoint_results.jsonl`
and `result.json` per run), and one status line per problem in `~/bench/scb/outputs/<label>/status.jsonl`.
Copy the label dir to `results/raw/<run_id>/` unchanged.

## Usage-limit-aware chunking

- Concurrency is 1 everywhere: `--num-workers 1`, one problem per `slop-code run`, one group at a time. The
  runner adds one evaluation container per checkpoint after the solve, never overlapping it
  (`--concurrent-evaluation` stays off).
- A chunk is one (group, problem). Run the groups interleaved per problem, not group after group: all five
  groups on `file_backup`, then all five on `execution_server`, and so on (`run_group.sh <G> <label> <one
  problem>` five times). A limit window then hits every group at the same point of the pilot, and every
  finished problem is a complete five-group comparison.
- Order: `file_backup` (4 checkpoints) and `execution_server` (6) first, as the cheap smoke; then the six
  8-checkpoint problems.
- When Claude Code stops on the usage limit, the runner retries the checkpoint once with `--continue`; that
  retry hits the limit too, and the checkpoint is saved with `had_error`. `classify_run.py` reads the
  `is_error` result payloads in `agent/stdout.jsonl` and marks the problem `limit_stopped` when one mentions a
  usage or rate limit; `run_group.sh` appends that line to `status.jsonl` and exits 3. Rerun the same command
  after the reset. The resume restores the workspace and `.abbd/` from the last good checkpoint and reruns the
  stopped checkpoint from its original prompt. Each checkpoint is a fresh `claude -p`, so nothing but the
  partial work of the stopped checkpoint is lost.
- A `limit_stopped` checkpoint is never scored. The normalizer takes each checkpoint's final attempt; the
  status lines show which checkpoints were resumed.
- Runner quirk, measured in the dry run: the first checkpoint after a `--resume` gets its diff against an empty
  workspace, so its `lines_added` equals the whole loc (1876 instead of 268 for `file_backup` checkpoint 3).
  Correctness and the snapshot quality metrics are unaffected. The normalizer must recompute churn for resumed
  checkpoints from snapshot k-1 and snapshot k.

## Expected time

Measured with the fake agent: evaluation takes about 25 to 60 s per checkpoint (submission image build plus
pytest; `file_backup` pytest runs 30 s), and the agent-side overhead (container exec, snapshot) is about 1 s.
Not measured: real agent time. Guess, from the TB2 smoke (3 to 4 min per one-shot task) and the size of SCB
specs: 5 to 20 min per checkpoint, so roughly 5 to 15 h per group for the 58 pilot checkpoints and 1 to 3 days
for all five groups at concurrency 1, before limit waits.

## What ran (2026-10-19)

| check | result |
|---|---|
| `check_problems.py` | 36 problems, 196 checkpoints; every pilot problem matches the manifest (six with 8, execution_server 6, file_backup 4); every pilot problem ships `solutions/checkpoint_N` |
| `refcheck.sh file_backup` | reference solutions pass every test: 32/32, 50/50, 68/68, 89/89 (core, functionality, regression, error groups all full). 2098 s, of which about 33 min was the first base-image build |
| `refcheck.sh execution_server` | all 6 reference checkpoints pass every test (45, 58, 101, 149, 185, 70 tests). 310 s |
| empty submission, checkpoint 1 | `file_backup` 4/32 (only the 4 error-case tests, which expect a failing command); `execution_server` 0/45. 22 to 24 s each |
| `abbd_scoring_check.sh file_backup 1` | no difference in any official output (see above). 51 s |
| `claude plugin validate` in the image | pstack, C, D, E pass (D warns about an unquoted `${CLAUDE_PLUGIN_ROOT}`; the path has no spaces) |
| dry run, group A, `file_backup` | 4/4 checkpoints, pass rate 1.000, no `--plugin-dir`, `abbd` not on PATH, no `.abbd/`. 158 s |
| dry run, group E, `file_backup` | 4/4; per checkpoint the same tests passed and the same loc, verbosity, erosion, cloned_pct and lines_added as A; `abbd init` ran once, before checkpoint 1 (its output is in that checkpoint's `stderr.log` only); `.abbd/` was in every snapshot and present at the start of checkpoints 2 to 4; `abbd run` and `abbd status` worked inside the agent process. 1816 s, of which about 30 min was building the dry-run images |
| dry run, group E, simulated limit at checkpoint 3 | first command: checkpoints 1 and 2 done, checkpoint 3 failed twice (the runner's `--continue` retry), `status.jsonl` got `limit_stopped at checkpoint_3`, exit 3, 110 s. Same command after the simulated reset: `--resume`, checkpoint 3 started with `.abbd/` restored and no second `abbd init`, checkpoints 3 and 4 passed every test, status `complete`, 104 s |

## Not verified

- Any real model call: the agent's behavior, time and token use, and whether the ABBD skills, hooks and the
  E MCP server actually load and fire inside `claude -p` 2.1.302 (the plugins validate, and their `abbd`
  calls work from the launcher's PATH, but the fake agent does not execute hooks).
- The exact text Claude Code 2.1.302 prints when the subscription limit is hit. `classify_run.py` matches
  `usage limit|hit your limit|limit reached|rate limit|429|overloaded` in `is_error` result payloads. Check the
  first real `limit_stopped` (or a misfiled `agent_error`) by hand and widen the pattern if needed.
- Groups B, C and D were not dry-run end to end. They differ from A and E only in the three config fields above,
  and their plugin dirs validate.
- `abbd init` at checkpoint 1 sees the problem's static assets (`file_backup` copies a `files/` dir into the
  workspace), so it reports `mode: code-only`, not `greenfield`. The earlier design assumed an empty workspace.
- `--max-turns 100` is the stock config for every group. ABBD groups spend turns on `abbd` commands; if many
  checkpoints end on the turn cap only in C/D/E, that is a finding about overhead, not a setup bug.
- The E plugin's Stop hook (`stop-gate.sh`) can keep a `claude -p` turn going; its effect on time per
  checkpoint is unmeasured.
