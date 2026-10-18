# Track B. SWE-Milestone

Environment proven on 2026-10-19 with no model call. No score in this file comes from a model run.

Pins: harness DeepCommit-ai/SWE-Milestone @ `17a8f1593e172e26b36cea15e2b30fb9536c93f5` (v1.0.2), unedited.
Data: SWE-Milestone-data tag `v1.0.2` = `3500478e740d42583277c2658aa7729eb36e10f4`. Images by content digest
from the harness's `manifests/digests-v1.0.2.tsv`. Every run parameter is in `configs/swe_milestone.yaml`.

## How the harness works (what the adapter must respect)

- One persistent container per repo and trial, started from `<repo>__base-offline` (quarantine is auto-on).
  The harness turns `/testbed` into one baseline commit, deletes all tags and history, records that commit
  as the snapshot BASE, then locks outbound network to an allowlist (`api.anthropic.com` is on it).
- The agent is `claude --model M --output-format json --dangerously-skip-permissions --session-id S < prompt`,
  run as `fakeroot` in `/testbed`. Later turns are `--resume S` with a recover message. The container is
  never reset between milestones.
- The agent sees only `/e2e_workspace/TASK_QUEUE.md` and the SRS of unlocked milestones. A milestone
  unlocks when its dependencies are submitted, so future requirements are never shown.
- Submission is `git tag agent-impl-<id>`. After 120 s of tag stability the harness takes
  `git archive <tag> <repo_src_dirs> <build manifests>`, strips test files, and evaluates the tar in a fresh
  container of the milestone image. Score, Precision, Recall and Fully Resolved come from
  `harness.e2e.collect_results`, unchanged.
- On a usage or rate limit (`claude` exits non-zero with "usage limit", "hit your limit", 429 and the like)
  the harness sleeps until the parsed reset (default 60 min, cap 2 h per sleep), then resumes the same
  session. A limit never counts as no-progress. This only works when the agent name is `claude-code`,
  which is why the adapter keeps that name.

## Adapter

`adapters/for_my_harness/swe_milestone_agent.py` replaces the registry entry for `claude-code` with a
subclass that calls the stock method first and only appends. It is loaded through
`adapters/for_my_harness/swe_milestone_site/sitecustomize.py` (on `PYTHONPATH`, active only when
`ABBD_GROUP` is set), so the worker that `run_all.py` spawns picks it up and the harness checkout stays
clean. A failed import crashes the run; a group never falls back to stock silently.

| group | mounted read-only | `claude` gets | before the first turn |
|---|---|---|---|
| A | nothing | stock command | nothing |
| B | pstack 0.9.93 at `/opt/bench-plugin` | `--plugin-dir /opt/bench-plugin` | nothing |
| C | arms/C, `abbd` at `/usr/local/bin/abbd` | `--plugin-dir` | nothing |
| D | arms/D, `abbd` | `--plugin-dir` | `abbd init --mode code-only --verify-available`, once per repo |
| E | arms/E, `abbd` | `--plugin-dir` | same as D |

Every group, A included, gets the subscription token the same way: `-e CLAUDE_CODE_OAUTH_TOKEN` by name, so
the value never appears in argv or a log, and the stock mount of `~/.claude/.credentials.json` is dropped
so the token is the only auth source. `ANTHROPIC_API_KEY`, `UNIFIED_API_KEY` and both base URLs are
removed from the launcher's environment. The token value does sit in the container's config (visible to
`docker inspect` on this host), the same as the harness's own API-key path.

`abbd init` runs inside the first agent invocation, guarded by `[ -d .abbd ]`. That puts it after git
truncation and network lockdown, keeps it out of later milestones, resumes and session rotations, and
counts its time (about 100 s on go-zero, mostly `go test ./...`) in the agent's wall time. Its output
goes to `/tmp/abbd-init.log` in the container; one status line goes to `agent_stderr.txt`.

`default_agent_model: claude-sonnet-5-5` pins Claude Code's haiku, opus, fable and subagent slots to the
same model in every group, so background calls and pstack's model-routed subagents also run Sonnet 5.5.

### `.abbd/` does not reach the evaluator

The snapshot is `git archive` of `repo_src_dirs` plus build manifests (`go.mod`, `go.sum`, `go.work*`,
`pom.xml`, `Cargo.*`). For go-zero that is `core/ gateway/ internal/ rest/ zrpc/ go.mod go.sum`; for
Element Web `src/ packages/shared-components/src/ res/css/`. `.abbd/` is outside both. The one way in
would be a file named like a build manifest inside `.abbd/`, which the recursive manifest overlay picks
up; the go-zero init writes none.

Checked in the group E dry run: the shim ran `git add -A`, so all 2130 `.abbd/` files were committed
under both submission tags, and both snapshots (444 entries each) contain 0 `.abbd` paths. Their
top-level entries are exactly `core gateway internal rest zrpc go.mod go.sum`.

## Setup (done on this machine; rerun only to rebuild)

```sh
# WSL Ubuntu-24.04, no sudo. uv and git-lfs went to ~/.local/bin.
curl -LsSf https://astral.sh/uv/install.sh | sh
cd ~/bench/swe-milestone
git clone https://github.com/DeepCommit-ai/SWE-Milestone && git -C SWE-Milestone checkout 17a8f15
(cd SWE-Milestone && uv sync --extra dev)       # dev extra: ast-grep, without it every worker exits at launch
git lfs install --skip-repo
git clone https://huggingface.co/datasets/DeepCommit-ai/SWE-Milestone-data
git -C SWE-Milestone-data checkout --detach refs/tags/v1.0.2
(cd SWE-Milestone && SWE_MILESTONE_DATA_ROOT=~/bench/swe-milestone/SWE-Milestone-data ./scripts/pull_data.sh)
(cd SWE-Milestone && ./scripts/pull_images.sh --repo go-zero && ./scripts/pull_images.sh --repo element-web)
sh /mnt/c/Users/cxx/orca/projects/cccv/benchmarks/runners/swe_milestone/stage_assets.sh
```

Measured (anonymous Docker Hub pulls worked; no `docker login`):

| step | time | disk |
|---|---|---|
| clone harness + `uv sync` | under 1 min | small |
| dataset clone (git-lfs) | 5 s | 190 MB |
| go-zero images (25) | 534 s pull | about 41 GB (`docker system df` images 11.2 to 52.5 GB) |
| Element Web images (20) | 936 s pull | about 63 GB (53.3 to 116.2 GB) |
| container setup per trial (go-zero) | about 10 min, once per repo | |
| `abbd init` (go-zero, D and E) | about 100 s, once per repo | `.abbd/` 20 MB, 2130 files |
| evaluator, one go-zero milestone | 111 s to 4.5 min | |
| evaluator, one Element Web milestone (Playwright) | 9710 s (initial state of `maintenance_ui_ux`) | |

The pulled images are the environment; the harness builds nothing locally (`--pull=never`).

## Environment check (no model)

Official evaluator, go-zero M001 (`python -m harness.e2e.evaluator ... --allow-legacy-snapshot`):

| snapshot | resolved | N2P | P2P | time |
|---|---|---|---|---|
| reference END state (`git archive milestone-M001-end` from the M001 image) | True | 134/134 | 2425/2425, 0 failed | 111 s |
| initial state, submitted unchanged by the dry run | False | 16/134 | 2414/2425, 0 failed, 11 missing | in-trial |

Element Web, first graded milestone in topological order with an image (`maintenance_ui_ux`, 29 F2P,
5237 P2P), same CLI on the initial state (`git archive milestone-maintenance_ui_ux-start`, 1965 entries):
not resolved, F2P 0/29, P2P 5234/5237 (3 failed), 5462 tests run, 9710 s. Most of that time is the
Playwright suite (`playwright/e2e/right-panel/`, headless Firefox, 120 s per-test timeout). The
reference END state was still evaluating at writing; results land in
`~/bench/swe-milestone/gold/ew-maintenance_ui_ux-end/`.
Consequence for the pilot: an Element Web milestone with a Playwright suite costs up to about 2.7 h of
evaluation on this host, and with early unblock several can run at once beside the agent container.
Run Element Web only after go-zero, and never next to the TB2 pilot.

Dry runs: `run_group.py <G> go-zero --milestones 2 --dry-run --wait --force`, which installs
`runners/swe_milestone/fake_claude.sh` over the in-container `claude` (the real 2.1.302 CLI stays at
`claude-real` and answers `--version`). The shim reports what the adapter set up, runs
`claude-real plugin validate` on the mounted plugin, and submits the first queued milestone unchanged.

| | group A | group E |
|---|---|---|
| wall time (launch to trial COMPLETED) | 708 s | 1258 s |
| container setup (once per repo) | 4.6 min | 10.1 min (first launch, cold caches) |
| first agent turn | 2 s | 101 s (`abbd init` exit 0, then the shim) |
| `claude` argv | stock | stock plus `--plugin-dir /opt/bench-plugin` |
| plugin / `abbd` / `.abbd/` seen by the agent | none / not on PATH / absent | arms/E, `plugin validate` passed / `/usr/local/bin/abbd` / 2130 files |
| milestone loop | M005 submitted on the first turn, M001 after unlock, 3 recover turns | same |
| M001 (unchanged code) | not resolved; N2P 16/134, P2P 2414/2425, 0 broken | identical |
| M005 (unchanged code) | not resolved; F2P 0/1, N2P 14/21, P2P 1942/2099, 0 broken | identical |
| collect_results | Score-1000 2.88%, Resolve 0/23, cost $0.00 | identical |
| `.abbd` entries in snapshots | 0 of 444, both milestones | 0 of 444, both milestones |
| trial_status.py | completed | completed |

Both runs had no token in the environment by design, so the shim reported `CLAUDE_CODE_OAUTH_TOKEN unset`
and the stock credentials mount stayed in place. With a placeholder token set, a direct call of the adapter for each
of A to E returned `-e CLAUDE_CODE_OAUTH_TOKEN` with no value in any argument, dropped the credentials
mount, and built the commands above; without `ABBD_GROUP` the registry still returns the stock class.

## Pilot runs

Order: go-zero for A, E, B, C, D, then Element Web in the same order. One trial at a time (n concurrent 1):
`run_all.py` would launch all repos in parallel, so `run_group.py` always passes one `--repos`. Do not run a
group while the TB2 pilot holds the usage budget.

```sh
P=~/bench/swe-milestone/SWE-Milestone/.venv/bin/python
R=/mnt/c/Users/cxx/orca/projects/cccv/benchmarks/runners/swe_milestone
$P $R/run_group.py A go-zero --wait      # trial claude-code_abbd-A_sonnet-5-5_r1_001
$P $R/run_group.py E go-zero --wait
$P $R/run_group.py B go-zero --wait
$P $R/run_group.py C go-zero --wait
$P $R/run_group.py D go-zero --wait
# then the same five with element-web
```

Without `--wait` the command returns after launch and the worker keeps running detached. Watch it with
`./scripts/monitor.sh <trial>_001 --detail` from the harness dir, or
`$P $R/trial_status.py --trial claude-code_abbd-E_sonnet-5-5_r1 go-zero`.

### Usage limits and resume

- The harness itself waits out a usage limit and resumes the same session; `trial_status.py` shows
  `limit_waiting` while that happens. Nothing needs to be done.
- To stop a trial on purpose (a weekly cap, or the machine is needed), kill the worker: the PID is in
  `<data_root>/<repo>/e2e_trial/.locks/<trial>.info`. Never `docker rm` the container: `/testbed`, the git
  tags and the session live only there.
- A stopped trial whose last agent failure was a usage limit reports `limit_stopped`. Record it as
  `limit_stopped` in the normalized rows, not as a failure. Its milestones are not scored yet.
- Resume: rerun the exact same `run_group.py` command (no `--force`). `run_all.py` sees
  `trial_metadata.json` and starts `run_e2e --resume-trial`, which restarts the container, re-verifies the
  network lock and resumes the agent session.
- `--force` wipes the trial and its container and restarts from the base commit. Use it only for an
  infrastructure failure, and record why.

### Time per milestone

Measured harness overhead: about 10 min container setup once per repo, 120 s tag debounce per
milestone, and 2 to 5 min of evaluation per go-zero milestone, which runs in the background while the
agent continues. Agent time per milestone with Sonnet 5.5 is not measured; a guess is 10 to 40 min, which
puts go-zero (23 graded milestones) at roughly 5 to 15 h per group, across several usage windows.

## Outputs to keep

The trial directory `<data_root>/<repo>/e2e_trial/<trial>_001/` (trial_metadata.json, orchestrator.log,
log/agent_*.txt, evaluation/) goes to `results/raw/<run_id>/`. Collect official numbers with
`python -m harness.e2e.collect_results --workspace-root <data_root>/<repo> --trials <trial>_001 --trial-type e2e`.
In-house metrics stay under `in_house` and never merge into Score.

## Not yet verified

- A real model turn in this harness: subscription auth through `CLAUDE_CODE_OAUTH_TOKEN` inside the
  locked-down container, plugin loading and hook execution under `--resume`, and the MCP server in E.
  The dry run proves mounts, PATH, flags, `plugin validate` and the milestone loop, not a live session.
- That `claude` exits non-zero with a message the harness's limit patterns match when a subscription
  limit hits (the TB2 track saw this through Harbor, not here).
- Element Web through the full trial loop (only its images and evaluator were exercised).
- run_all's own "already completed" check reads `resume_state.completed_milestones`, which the dry run
  left empty, so rerunning a finished trial starts a no-op resume instead of skipping. Harmless; use
  `trial_status.py` to decide.
- ABBD's turn model: the first prompt is the only UserPromptSubmit with the system prompt; later
  milestones arrive as recover messages and file reads (see adapters README).
