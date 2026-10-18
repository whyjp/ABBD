# Feasibility and cost

Phase 1 (spec section 12, steps 1 to 5). No model session was run, no benchmark was started, and no
number below comes from a run. Sources were fetched on 2026-10-18.

## 1. What ABBD actually has (code read at 94e0495)

| spec component | verdict | evidence |
|---|---|---|
| Architecture SSOT | implemented | `.abbd/architecture/MOD-*.md` + frontmatter (`internal/model/entities.go`), written only by `knowledge.Publish`, checked by `abbd check`, hand edits detected |
| Feature Map | implemented, shallow | `knowledge.Seed` makes one FEAT per entry point with a placeholder purpose; meaning comes from `abbd intent` or user confirmation |
| Code/Module/Interface KG | implemented, limited | `generated/graph.json`, full rebuild on every publish, no DB, no incremental (comment says Phase 3). Node kinds module, contract, symbol, test, feature, requirement, change, decision. Go via go/ast; TS/JS/Python regex only; Java, Rust, shell none |
| Graph MCP | implemented | 20 tools in `internal/mcp/server.go`, same engine functions as the CLI. No skill names an MCP tool |
| Domain Entity/State/Transition graph | not implemented | `abbd domain` is a stub that exits 1; PRD v1.5 ch.06 reserves DOM/STATE/TRN/INV IDs for Phase 3 |
| Turn loop step 1, latest state | partly enforced | SessionStart hook; `abbd run` reconciles; nothing forces `run` before edits; Stop hook sends back once |
| step 2, intent | capture enforced, interpretation heuristic | UserPromptSubmit hook stores the prompt verbatim; `intent.Resolve` uses keywords and token overlap |
| step 3, explore features/modules/domain | prose | `read_first`, `abbd inspect/graph`, skills; no domain part |
| step 4, impact and invariants | contracts enforced, invariants prose | tier A/B/C from contract diffs blocks sync; invariants are typed-in claims never checked against code |
| step 5, modify and test | enforced | sync needs a PASSED verify on the current tree; failing-first for bug fixes |
| step 6, contract verification | partial | stored vs extracted contract diff; behavior is only what the tests check |
| step 7, update KG and maps | enforced | sync publishes md, BLUEPRINT, graph, HTML at one watermark and refuses completion if not CURRENT |
| step 8, reuse next turn | enforced by hooks | BLUEPRINT.md, `read_first`, standing rules, history guard (regex, not a security boundary) |

Groups (full mapping in `configs/harnesses.yaml`):

| group | verdict |
|---|---|
| A base agent | runnable |
| B + pstack 0.9.93 | runnable (installed on this host) |
| C + ABBD skills | runnable with caveat. The skills call `abbd run`, which builds the SSOT and graph, so C cannot be cleanly separated from D |
| D + SSOT + Feature Map | runnable with caveat. Graph queries hidden by an arm-only PreToolUse deny; graph.json is still built internally |
| E + KG MCP | runnable. This is the shipped plugin |
| F + Domain State Graph + full loop | not runnable. The domain layer does not exist |

The spec's Q5 (does a domain state graph reduce state and invariant regressions) cannot be answered with
this code. The KG quality checks for domain integrity in spec section 8 cannot be run either.

## 2. What blocks running on this machine

Host: Windows 11 Pro 10.0.26200, 16 cores, 32 GB RAM (34,277,101,568 bytes), 233 GB free on C:,
Python 3.13, uv 0.11.1, Go 1.26.3, Claude Code 2.1.302. `docker` is not installed. WSL2 Ubuntu-24.04
exists and is stopped.

| track | blocker | what unblocks it |
|---|---|---|
| Terminal-Bench 2.0 | No container runtime. All tasks are Linux images. Harbor's Windows host support is not documented | Docker Engine inside WSL2 Ubuntu and run Harbor from WSL (recommended), or Docker Desktop with the WSL2 backend, or a cloud sandbox (`-e daytona`, `-e modal`, needs that account) |
| SWE-Milestone | No Docker. Agent container uses iptables allowlisting, which needs a Linux Docker host. About 115 images; only one size is known (Navidrome base, 795 MB compressed). No RAM or disk requirement is documented | Docker Engine in WSL2. Pull the pilot repos' images only (go-zero, Element Web) and measure disk before pulling the rest |
| SlopCodeBench | No Docker. README asks 8 GB+ RAM, 10 GB+ disk | Docker in WSL2 |
| SWE-CI | No Claude Code adapter; README targets Linux with iFlow CLI; about 50 GB dataset; about 48 h on 32 cores and 64 GB | Out of scope for phase 2. Would need a new adapter |
| SkillsBench | Modal by default; Docker locally | Optional. Tests a different claim (task skills, not a dev workflow) |

The ABBD binary itself is not a blocker. `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/abbd`
gives a static ELF on this host. Hooks need `sh`, `grep`, `sed`, `git` in the sandbox.

Resource fit, inferred. TB2 tasks ask for at most 4 CPUs and 8 GB each, so about 4 concurrent tasks fit in
16 cores and 32 GB with headroom for WSL. SWE-Milestone's evaluator defaults to `--cpus 16`, so run one repo
at a time per group, or lower that flag.

## 3. Setup steps (after the user picks the runtime)

1. Enable a Linux container runtime (user decision; see section 6). With WSL2: start Ubuntu-24.04, install
   Docker Engine, add the user to the docker group, put Harbor, the SWE-Milestone repo and slop-code-bench
   inside the WSL filesystem, not /mnt/c. Cloning scb-problems and skillsbench on Windows hit
   "Filename too long" during phase 1.
2. `uv tool install harbor==0.24.0`. Clone SWE-Milestone @ 17a8f15 and slop-code-bench @ 31ceea3; `uv sync`.
3. Build `abbd-linux` at a pinned ABBD commit. Build arm dirs with `adapters/for_my_harness/build_arms.py`.
4. Oracle runs, zero model cost: `harbor run -d terminal-bench@2.0 -a oracle` on the 15 pilot tasks; the
   SWE-Milestone and SCB equivalents per their docs. Any task whose oracle fails on this host is logged as an
   environment failure and excluded for every group, before any model run.
5. Smoke run (smallest paid step): 2 TB2 pilot tasks x groups A and E x 1 rep, to prove the adapter
   end to end. Mid estimate about $11 on Sonnet 5.5 (2 x $2.5 x (1 + 1.22)).
6. Pilot (1 rep), then failure classification, then official runs (3 reps).

## 4. Cost and time

Measured anchor. In-house v2 (12 small Go tasks, 2 reps, alias `sonnet`): $0.131 per task for baseline,
$0.169 for pstack, $0.160 for abbd, about 27 s of session per task. The model id was not recorded; the token
mix (about 200k cache-read, 21k cache-write and 2.7k output tokens per task) fits Sonnet 5.5 pricing best
(inferred). Group overhead vs A from v2: pstack x1.29, abbd x1.22. C and D are assumed equal to abbd.

Everything else is a guess. Unit costs for group A on Sonnet 5.5 (low / mid / high): TB2 $1 / $2.5 / $6 per
task; SWE-Milestone $1.5 / $3 / $6 per milestone; SCB $0.5 / $1.5 / $3 per checkpoint. They scale v2's
~$0.29 per agent-minute to each benchmark's timeouts. Opus 5.5 is priced at 2x Sonnet 5.5 ($4/$20 vs
$2/$10 per MTok) with the same token use assumed. Generated by `reports/cost_model.py`.

Groups A, B, C, D, E together (F not runnable). Pilot is 1 rep, full is 3 reps.

| track | phase | units | model | USD low | USD mid | USD high | wall hours mid |
|---|---|---|---|---|---|---|---|
| Terminal-Bench 2.0 | pilot | 15 tasks | sonnet-5-5 | 89 | 223 | 536 | 3.8 |
| Terminal-Bench 2.0 | full | 89 tasks | sonnet-5-5 | 1,589 | 3,972 | 9,532 | 66.8 |
| SWE-Milestone | pilot | 41 milestones (go-zero, Element Web) | sonnet-5-5 | 366 | 732 | 1,464 | 25.6 |
| SWE-Milestone | full | 66 milestones (4 repos) | sonnet-5-5 | 1,767 | 3,534 | 7,069 | 123.8 |
| SlopCodeBench | pilot | 58 checkpoints (8 problems) | sonnet-5-5 | 173 | 518 | 1,035 | 7.2 |
| SlopCodeBench | full | 196 checkpoints (36 problems) | sonnet-5-5 | 1,749 | 5,248 | 10,496 | 73.5 |
| Terminal-Bench 2.0 | pilot | 15 tasks | opus-5-5 | 178 | 446 | 1,071 | 3.8 |
| Terminal-Bench 2.0 | full | 89 tasks | opus-5-5 | 3,177 | 7,943 | 19,064 | 66.8 |
| SWE-Milestone | pilot | 41 milestones | opus-5-5 | 732 | 1,464 | 2,927 | 25.6 |
| SWE-Milestone | full | 66 milestones | opus-5-5 | 3,534 | 7,069 | 14,137 | 123.8 |
| SlopCodeBench | pilot | 58 checkpoints | opus-5-5 | 345 | 1,035 | 2,071 | 7.2 |
| SlopCodeBench | full | 196 checkpoints | opus-5-5 | 3,499 | 10,496 | 20,992 | 73.5 |

Mid totals: pilot of all three tracks about $1,470 on Sonnet 5.5 and $2,950 on Opus 5.5. Full plan
about $12,750 on Sonnet 5.5 and $25,500 on Opus 5.5.

Why the numbers could be far off:

- TB2 pilot tasks were drawn from hard tasks only, and four of them have 3,600 to 7,200 s timeouts. The
  per-task guess is a mean over the whole benchmark, so the pilot is more likely to land near "high".
- SWE-Milestone runs one session per repo with an 18,000 s cap. go-zero's 23 milestones at the assumed
  15 min each exceed that cap, so real cost may be capped below the estimate and some milestones may go
  unattempted. Context grows over a five-hour session, so cost per minute rises late in the run.
- The per-session cap (`--max-budget-usd`) bounds the high side per task, but only if it is set.

Cheaper paths with the same rigor per claim:

- Run groups A, B and E first (A vs E is the main claim, B is the pstack comparison). Add C and D only
  where A vs E shows a difference worth explaining. That cuts each row by about 40%.
- Run the full TB2 set for groups A and E only. The official comparison needs the full set, not every group.

## 5. Facts that differ from the mission spec

- `github.com/SprocketLab/scb-problems` does not exist. Problems are at `gabeorlanski/scb-problems`.
- tbench.ai's current benchmark is Terminal-Bench 4.0 (3.0 and 2.1 also listed). 2.0 still has its own
  leaderboard, which this plan uses. Its data comes from the site's own feed, not the page HTML.
- No verified TB2 leaderboard row uses a Claude 5.x model. Best Claude Code rows: 2.1.34 + Opus 4.6 at
  58.0% +/-2.9; 2.0.72 + Opus 4.5 at 52.1% +/-2.5. NexAU-AHE (84.7%, rank 1) and Capy (83.1% and 75.3%) exist,
  both unverified on the board with version "unknown".
- SWE-Milestone "Fully Resolved" is named "Resolve" in the paper and site. Score is the harmonic mean of
  Precision and Recall with a +1 smoothing in Precision (code). The README gives no formula.
- SWE-CI has no Claude Code adapter and its README clones a repo path that returns 404.
- SkillsBench's current repo runs on BenchFlow, not Harbor. Task counts disagree across its sources.
- ABBD has no domain state graph (group F) and no incremental KG.

## 6. Decisions only the user can make

1. Budget cap for the pilot and for the full plan (see section 4).
2. Model id for same-model comparison: `claude-sonnet-5-5` or `claude-opus-5-5`, and the effort level.
3. Container runtime: Docker Engine inside WSL2 Ubuntu-24.04 (recommended), Docker Desktop, or a cloud
   sandbox account (Daytona or Modal) for Harbor.
4. API access: an `ANTHROPIC_API_KEY` with enough rate limit for 4 concurrent sessions, or a plan login.
   Harbor and SWE-Milestone default to an API key in the container.
5. Network policy: the benchmarks need outbound access for Claude Code install, model API and package
   registries. Confirm that is allowed from this machine.
6. Which groups to run in the pilot: all of A to E, or A, B, E first.
7. Whether SWE-CI and SkillsBench stay out of scope, given no Claude Code adapter (SWE-CI) and a different
   claim (SkillsBench).
8. The ABBD commit to freeze for all runs, since `internal/` and `plugins/` are being edited now.
