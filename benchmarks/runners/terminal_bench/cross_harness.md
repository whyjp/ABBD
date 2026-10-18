# Cross-harness arms on Terminal-Bench 2.0

Prepared 2026-10-19. Not run yet: no model calls were made while preparing these arms.

Every arm runs the same way: Harbor 0.24.0, `terminal-bench@2.0` (registry pin in the manifest's
`terminal_bench_2.run_pin`), model `anthropic/claude-sonnet-5-5`, Claude Code `2.1.302` (`--ak version=2.1.302`;
the pilot's A/B/E trials all reported 2.1.302 without the pin), the task's own image, CPU/RAM limits and
agent timeout, and subscription auth (`CLAUDE_FORCE_OAUTH=1`). The only difference is what the adapter
installs before `claude --print` starts. Pins are also in `manifests/frozen_task_manifest.json` under
`cross_harness_arms`.

## Arms

| arm | adapter class | pin | installed in the container | missing / caveats |
|---|---|---|---|---|
| A baseline | `-a claude-code` (stock) | Claude Code 2.1.302 | nothing extra | n/a |
| B pstack | `PstackClaudeCode` | pstack 0.9.93 (local plugin cache) | plugin dir at `/opt/bench-plugin`, `--plugin-dir` | same arm as the pilot |
| E ABBD full | `AbbdClaudeCode`, `ABBD_GROUP=E` | abbd-bench-pub `arms/E` + `abbd-linux` (commit 4ec437c tag) | plugin dir, `/usr/local/bin/abbd`, `abbd init --mode code-only` in the task dir | same arm as the pilot |
| superpowers | `SuperpowersClaudeCode` | obra/superpowers v7.0.0 @ `bb92a77741419a4ab5f06e711a283343f1ada0c3` | full plugin (15 skills, SessionStart hook) at `/opt/bench-plugin`, `--plugin-dir` | none found. The hook needs only bash. Same files as the local cache 7.0.0 (`diff -r`: only the cache's `.in_use` marker differs) |
| gstack | `GstackClaudeCode` | garrytan/gstack v1.91.139.0 @ `3a64ab8a1f6bfa613546a99522f144ac724771b5`; Bun 1.4.2 | see below | browser: none (see below) |
| omc | not built | n/a | n/a | skipped, reasons below |

### gstack: what the adapter does

The README installs gstack with `git clone ... ~/.claude/skills/gstack && cd ~/.claude/skills/gstack && ./setup`,
then adds a "gstack" section to CLAUDE.md telling the agent to use `/browse` for web browsing. Its
requirements on Linux are Git and Bun (tested 1.4.2, floor 1.3.3). In `GstackClaudeCode.install()`:

1. Stock Claude Code install (the same as every arm), then `git` and `tar` if the image lacks them.
2. Upload the pinned checkout to `/opt/gstack` and the Bun 1.4.2 binary to `/usr/local/bin/bun`
   (`bunx` links to it). The Bun archive hash is checked on the host against gstack's own
   `bin/gstack-bun-version.sh`. This avoids running the bun.sh installer inside every container.
3. `cd /opt/gstack && ./setup` with no flags. `./setup` supports a checkout outside `skills/` and links
   `~/.claude/skills/gstack -> /opt/gstack` itself. It runs `bun install --frozen-lockfile` (network:
   npm registry), builds the compiled tools, renders the skills into `~/.claude/skills/<name>/`, tries the
   Chromium bootstrap, and registers its default-on Stop hook (`timeline-stop-hook`) in
   `~/.claude/settings.json`. No TTY, so the timed prompts take their non-interactive defaults: no
   skill prefix, and the opt-in plan-tune hooks are not installed (setup prints this).
4. `gstack-config set update_check false`, so a newer upstream release cannot show upgrade notices
   during the benchmark. Telemetry is off by default and stays off.
5. Two Harbor-specific steps, because Harbor runs Claude with `CLAUDE_CONFIG_DIR=/logs/agent/sessions`
   and copies only `~/.claude/skills` into it:
   - the hooks setup wrote to `~/.claude/settings.json` are copied to `/opt/gstack-claude-settings.json`
     and passed with `--settings` (the arm's CLI flags are `--permission-mode=bypassPermissions --settings /opt/gstack-claude-settings.json`);
   - the README's CLAUDE.md section is written to `/logs/agent/sessions/CLAUDE.md` (user memory for that config dir),
     worded exactly as `bin/gstack-doctor` recommends at the pin: `## gstack` / `Use /browse from gstack for all web browsing. Never use mcp__claude-in-chrome__* tools.`
6. `./setup` output and `gstack-doctor` (run with the run's `CLAUDE_CONFIG_DIR`) are saved per trial as
   `agent/gstack-setup.txt` and `agent/gstack-doctor.txt`. Dry run: `12 ok, 0 warn, 2 not configured, 0 fail`
   (not configured: Codex CLI, artifacts sync).

`DEFAULT_SETUP_TIMEOUT_SEC` is 1200 for this class only (Harbor's default is 360). This is the
agent-setup phase, not the agent's task time.

Present in the dry-run container (cancel-async-tasks image, Debian 12, root):
59 entries in `~/.claude/skills` (the skills plus the `gstack` link), the `bin/` helpers (gstack-config,
gstack-doctor, gstack-settings-hook, ...), compiled `browse`, `find-browse`, `design`, `make-pdf`
(`gstack-doctor`: "binaries 4 of 4 launch", "hooks 9 hook(s) parse"), the CSO native helper (this
image has gcc; an image without a C compiler gets `/cso` "not assessed"), skill-scoped hooks
(`/careful`, `/freeze`, `/guard` declare theirs in SKILL.md frontmatter), and the Stop hook through `--settings`.
Harbor's run-time copy of `~/.claude/skills` came to 508 KB in a simulated copy, and gstack's canonical
root still resolved to `/root/.claude/skills/gstack`.

Not present:
- **Browser (Chromium).** `/browse`, `/qa`, `/qa-only`, `/design-review`, `/scrape`, `/benchmark`,
  `/canary`, `make-pdf`, `/diagram` and `/pair-agent` have no browser. Setup reports
  `Browser unavailable: Chromium bootstrap did not complete (chromium-install)` and goes on, which is
  its documented best-effort path. Two separate causes, both checked in the dry-run container:
  1. `bun x playwright install chromium` ran Playwright's CLI under the image's Node 18.20.4, which
     Harbor installs with apt for every arm, and Playwright needs Node >= 20.
  2. Forcing Bun (`bun x --bun playwright install chromium`) downloaded headless Chromium (656 MB in
     `~/.cache/ms-playwright`), but it did not launch: `libnspr4.so`, `libnss3.so`, `libatk-1.0`,
     `libxkbcommon`, `libasound` and other system libraries are missing. gstack's setup does not install
     OS packages.
  A browser-enabled variant would need apt packages and a Node >= 20 that the README does not ask for.
  It would no longer be the README install, so it is not an arm here. Whether the browser comes up is
  image-dependent (Node version, system libs); each trial's `gstack-doctor.txt` records what it got.
- Aside browser (macOS only), Codex CLI for outside reviews (`/review`, `/ship`, `/autoplan` report
  no Codex coverage), gbrain, artifacts sync. These are optional in gstack and absent on any fresh Linux box.
- Network at setup time: the TB2 task configs allow internet. `bun install` reached the npm registry in the
  dry run. If a task image blocks it, setup fails and Harbor records an agent-setup error (env_error,
  not a task failure).

### superpowers

Plugin dir, the same mechanism as pstack. The source is a `git archive` of the pinned commit (no `.git`).
`claude plugin validate .../.claude-plugin/plugin.json` passes on the host and inside the container.
The SessionStart hook (`hooks/run-hook.cmd session-start`) is a bash polyglot; run by hand in the
container it printed the `additionalContext` JSON with the using-superpowers skill.

### oh-my-claudecode: skipped

- No plugin-cache copy on this machine. The local copy is the npm package `oh-my-claude-sisyphus` 4.9.3
  installed on Windows (`%APPDATA%\npm\node_modules`, 194 MB with `node_modules`). Its
  `.claude-plugin/plugin.json` says 4.9.2, and it ships a native module (`better-sqlite3`) built for win32.
- It requires Node >= 20 (`engines`). Its hooks (UserPromptSubmit, SessionStart, PreToolUse,
  PermissionRequest, ...) and its MCP server (`bridge/mcp-server.cjs`) all run under `node`, and TB2
  containers get Debian/Ubuntu apt Node 18 from Harbor. Giving only this arm a newer Node would change the
  runtime under test.
- Its documented install finishes with an in-session `/oh-my-claudecode:omc-setup` (or `/setup`) that
  writes the orchestration CLAUDE.md. That step is interactive and uses the model. Reproducing it
  without a model means hand-writing its output, which is not the vendor's install.
- An honest OMC arm needs: a Linux `npm install -g oh-my-claude-sisyphus@<pin>` (native build), Node >= 20
  in the container for this arm only, and a recorded copy of the CLAUDE.md that omc-setup writes.
  That is a separate decision. It is not done here.

## Prepare (once, on the WSL Docker host)

```sh
bash /mnt/c/Users/cxx/orca/projects/cccv/benchmarks/runners/terminal_bench/prepare_cross_harness.sh
# -> ~/bench/xh-src/{superpowers-bb92a77, gstack-3a64ab8, bun-1.4.2/bun}; validates superpowers; refuses a dirty gstack checkout
```

## Launch: 15 pilot tasks x 3 reps x 5 arms

```sh
bash /mnt/c/Users/cxx/orca/projects/cccv/benchmarks/runners/terminal_bench/run_cross_harness.sh
# env knobs: ARMS="A B E superpowers gstack"  REPS="1 2 3"  N=1 (concurrent trials)  TAG=4ec437c
# log: ~/bench/xh-tb2-4ec437c.log ; jobs: ~/bench/jobs/xh-tb2-<arm>-r<rep>-4ec437c
```

Reps are the outer loop and arms the inner loop. The usage-limit check and stop are the same as the pilot
script. A finished job is skipped on rerun. A job that stopped on the usage limit already has a
`result.json`, so move it aside (or rerun its trials under an `-infra-rerun` name, as in the pilot)
before resuming. `N` defaults to 1 because the Docker host is shared with other pilots.

What one job runs, spelled out (`$COMMON` and `$TASKS` exactly as in the script):

```sh
set -a; . ~/bench/.env; set +a; export CLAUDE_FORCE_OAUTH=1 PYTHONPATH=/mnt/c/Users/cxx/orca/projects/cccv
PUB=/mnt/c/Users/cxx/orca/projects/abbd-bench-pub; X=~/bench/xh-src; M=benchmarks.adapters.for_my_harness.harbor_agent
TASKS=$(python3 -c "import json;m=json.load(open('/mnt/c/Users/cxx/orca/projects/cccv/benchmarks/manifests/frozen_task_manifest.json'));print(' '.join('-i '+t['id'] for t in m['terminal_bench_2']['pilot']))")
COMMON="-d terminal-bench@2.0 -m anthropic/claude-sonnet-5-5 --ak version=2.1.302 -n 1 -o $HOME/bench/jobs -q"
R=1   # rep 1..3
harbor run $COMMON $TASKS -a claude-code --job-name xh-tb2-A-r$R-4ec437c
ABBD_PLUGIN=/mnt/c/Users/cxx/.claude/plugins/cache/pstack-claude/pstack/0.9.93 \
  harbor run $COMMON $TASKS -a $M:PstackClaudeCode --job-name xh-tb2-B-r$R-4ec437c
ABBD_GROUP=E ABBD_PLUGIN=$PUB/arms/E ABBD_BIN=$PUB/abbd-linux \
  harbor run $COMMON $TASKS -a $M:AbbdClaudeCode --job-name xh-tb2-E-r$R-4ec437c
ABBD_PLUGIN=$X/superpowers-bb92a77 \
  harbor run $COMMON $TASKS -a $M:SuperpowersClaudeCode --job-name xh-tb2-superpowers-r$R-4ec437c
GSTACK_SRC=$X/gstack-3a64ab8 GSTACK_BUN=$X/bun-1.4.2/bun \
  harbor run $COMMON $TASKS -a $M:GstackClaudeCode --job-name xh-tb2-gstack-r$R-4ec437c
```

## How the arms were checked (no model calls)

- `scratchpad/dry_install.py` (session scratchpad, not in the repo) builds the real adapter class and calls
  its `install()` against a throwaway `docker run alexgshaw/cancel-async-tasks:20251031` container,
  through a `docker exec`/`docker cp` shim in place of Harbor's environment. The calls and their order
  are the ones Harbor makes. The container was deleted afterwards.
  - gstack: install exit 0 in 2-4.5 min over four runs (the last one with the final adapter code), including apt Node/npm and Claude Code 2.1.302. Results above.
  - superpowers: install exit 0. Plugin files at `/opt/bench-plugin`, validate passes, the hook emits its context.
- Not checked: that Claude Code inside a real trial loads the superpowers plugin and gstack's `--settings`
  hook and CLAUDE.md. Proving that needs a session (a model call). Check it in the first real trial:
  the stream-json `init` line lists plugins and skills, and `agent/sessions/` holds the CLAUDE.md.
- Not checked: the other 14 pilot images. gstack's setup depends on each image having git/tar (the adapter
  installs them if missing) and reaching npm.
