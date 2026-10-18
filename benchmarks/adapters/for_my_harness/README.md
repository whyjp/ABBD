# ABBD adapter (For My Harness)

Status: design plus code sketches. Nothing here has been run. Every file is marked UNTESTED.

The adapter does one thing on every benchmark. It runs the benchmark's own built-in Claude Code
agent and adds ABBD around it. It does not change the prompt, the task, the tests, the timeouts or the
scoring. Group A uses the benchmark's stock Claude Code agent with no changes at all.

## What gets added per group

| group | in the sandbox | added to the `claude` command |
|---|---|---|
| A | nothing | nothing |
| B | pstack plugin dir | `--plugin-dir <pstack>` |
| C | `abbd` binary on PATH, plugin dir with `skills/` only | `--plugin-dir <arm C>` |
| D | `abbd` on PATH, plugin dir with `skills/`, `hooks/`, `scripts/`; `abbd init` before the agent starts; a PreToolUse deny for `abbd graph/inspect/impact` | `--plugin-dir <arm D>` |
| E | `abbd` on PATH, the shipped `plugins/abbd`; `abbd init` before the agent starts | `--plugin-dir <plugins/abbd>` |

`build_arms.py` builds the C and D plugin dirs from `plugins/abbd` without editing it. E uses
`plugins/abbd` as is. The `abbd` binary is cross-built once per ABBD commit:
`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o abbd-linux ./cmd/abbd` (verified on this host to give
a static ELF x86-64 binary). The sandbox also needs `git`, `sh`, `grep` and `sed` for the hooks.

## Per benchmark

| benchmark | built-in Claude Code agent | where ABBD plugs in | file |
|---|---|---|---|
| Terminal-Bench 2.0 (Harbor 0.24.0) | `harbor.agents.installed.claude_code.ClaudeCode` | subclass: `install()` uploads `abbd` and the plugin dir; `build_cli_flags()` appends `--plugin-dir`. Run with `-a benchmarks.adapters.for_my_harness.harbor_agent:AbbdClaudeCode` | `harbor_agent.py` |
| SWE-Milestone v1.0.2 | `harness.e2e.agents.claude_code.ClaudeCodeFramework` (`@register_framework("claude-code")`) | subclass registered as `claude-code-abbd`: extra `-v` mounts, init script that puts `abbd` on PATH and runs `abbd init` in `/testbed`, run command with `--plugin-dir` | `swe_milestone_agent.py` |
| SlopCodeBench | `claude_code` agent config | agent YAML with `extra_args: [--plugin-dir, ...]` and an environment image that has `abbd` and the plugin dirs baked in (same image for every group; PATH set only for C/D/E via `env`) | `scb/` |
| SWE-CI | none (iflow, opencode only) | not adapted in phase 1 | |

## Known mismatches between ABBD's turn model and the benchmarks

These are reported, not worked around by changing the benchmark.

- **One session, many requirements (SWE-Milestone).** The official agent runs one continuous Claude Code
  session per repo and reads milestones from `/e2e_workspace/TASK_QUEUE.md`. ABBD's UserPromptSubmit hook
  records only the first prompt as a requirement. Later milestones reach the agent as file reads, not
  prompts. Each milestone becomes an ABBD turn only if the skills drive `abbd run` per milestone.
- **History guard paths.** `history-guard.sh` refuses reads of `.abbd/` history. It does not touch
  `/e2e_workspace`, task files or tests.
- **Blueprint files in the scored workspace.** `.abbd/` is created inside the repo or workspace. SWE-Milestone
  scores only `src_dirs` and root build manifests, so `.abbd/` is outside its scope. SlopCodeBench's
  code-quality metrics must be checked to confirm they ignore `.abbd/` (open item).
- **Language coverage.** The code graph is full only for Go. Java, Rust and shell repos get no graph; TS and
  Python get a regex-only one. See `configs/harnesses.yaml`.
- **Bootstrap mode.** Terminal-Bench and SWE-Milestone start from existing files (`code-only`).
  SlopCodeBench checkpoint 1 starts from an empty workspace (`greenfield`).

## Fairness rules the adapter enforces

- Same base image, same Claude Code version, same model, same effort, same timeouts for every group.
- ABBD bootstrap (`abbd init`) makes no model calls. On Harbor it runs in the agent setup phase, which
  Harbor times separately; the normalizer adds setup time to the run's duration for every group.
- `.abbd/` lands in the task directory. A Terminal-Bench verifier that inspects directory contents could
  see it. Any task whose verifier fails only for ABBD groups is flagged and inspected by hand.
- Nothing from tests, solutions or future milestones is mounted into the sandbox for any group.
