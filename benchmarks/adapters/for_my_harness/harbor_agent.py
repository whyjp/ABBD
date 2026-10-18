"""UNTESTED sketch. Harbor 0.24.0 agent that wraps the built-in Claude Code agent with ABBD.

Written against harbor-framework/harbor @ 7331d8a (src/harbor/agents/installed/claude_code.py,
src/harbor/agents/installed/base.py, src/harbor/environments/base.py), read 2026-10-18.

  harbor run -d terminal-bench@2.0 -m anthropic/<model> \
    -a benchmarks.adapters.for_my_harness.harbor_agent:AbbdClaudeCode \
    --ae ABBD_GROUP=E --ae ABBD_BIN=<abs path to abbd-linux> --ae ABBD_PLUGIN=<abs path to plugin dir>

Group A is the stock agent: `-a claude-code`. Group B uses PstackClaudeCode with ABBD_PLUGIN pointing
at the pstack dir. The env vars are read on the host by this class; they are not forwarded into the
sandbox except where noted.
"""
import os
import shlex

from harbor.agents.installed.claude_code import ClaudeCode
from harbor.environments.base import BaseEnvironment

REMOTE_PLUGIN = "/opt/bench-plugin"
REMOTE_BIN = "/usr/local/bin/abbd"


class PluginClaudeCode(ClaudeCode):
    """Stock Claude Code plus one extra --plugin-dir. Nothing else changes."""

    @staticmethod
    def name() -> str:
        return "claude-code-plugin"

    def _plugin_src(self) -> str:
        return os.environ["ABBD_PLUGIN"]

    async def install(self, environment: BaseEnvironment) -> None:
        await super().install(environment)
        await environment.upload_dir(self._plugin_src(), REMOTE_PLUGIN)
        await self.exec_as_root(environment, command=f"chmod -R a+rX {REMOTE_PLUGIN}")

    def build_cli_flags(self) -> str:
        base = super().build_cli_flags()
        return f"{base} --plugin-dir {shlex.quote(REMOTE_PLUGIN)}".strip()


class PstackClaudeCode(PluginClaudeCode):
    @staticmethod
    def name() -> str:
        return "claude-code-pstack"


class SuperpowersClaudeCode(PluginClaudeCode):
    """ABBD_PLUGIN = obra/superpowers checkout (prepare_cross_harness.sh). Its SessionStart hook is bash only."""

    @staticmethod
    def name() -> str:
        return "claude-code-superpowers"


GSTACK_ROOT = "/opt/gstack"
GSTACK_SETTINGS = "/opt/gstack-claude-settings.json"
# The section gstack asks for in CLAUDE.md: README Install step 1, exact wording from bin/gstack-doctor at the pin.
GSTACK_CLAUDE_MD = """## gstack

Use /browse from gstack for all web browsing. Never use mcp__claude-in-chrome__* tools.
"""


class GstackClaudeCode(ClaudeCode):
    """gstack installed the way its README says: checkout + `./setup` (Bun build, skill links, hooks).

    Host env: GSTACK_SRC (pinned clean checkout), GSTACK_BUN (Bun binary whose archive hash matches
    gstack's bin/gstack-bun-version.sh). Both come from runners/terminal_bench/prepare_cross_harness.sh.

    Harbor runs Claude with CLAUDE_CONFIG_DIR=<logs>/agent/sessions and copies only ~/.claude/skills
    into it, so two things `./setup` writes under ~/.claude would be lost: the hooks it registers in
    ~/.claude/settings.json (passed back with --settings) and the CLAUDE.md section (written into
    the per-run config dir). The checkout lives at /opt/gstack and ./setup links
    ~/.claude/skills/gstack to it (its documented "checkout outside skills/" path), so Harbor's
    per-run skill copy is ~0.5 MB of links and stubs, not the 1.7 GB built tree (measured 2026-10-19).
    """

    # ./setup runs `bun install` and downloads Chromium; the stock 360 s is too tight. Setup time is not
    # agent time: the task's agent timeout is unchanged.
    DEFAULT_SETUP_TIMEOUT_SEC = 1200.0

    @staticmethod
    def name() -> str:
        return "claude-code-gstack"

    async def install(self, environment: BaseEnvironment) -> None:
        await super().install(environment)
        await self.ensure_system_dependencies(environment, ("git", "tar"))
        await environment.upload_file(os.environ["GSTACK_BUN"], "/usr/local/bin/bun")
        await environment.upload_dir(os.environ["GSTACK_SRC"], GSTACK_ROOT)
        uid = (await self.exec_as_agent(environment, command="id -u")).stdout.strip()
        await self.exec_as_root(
            environment,
            command=f"chmod 755 /usr/local/bin/bun && ln -sf bun /usr/local/bin/bunx && chown -R {uid} {GSTACK_ROOT}",
        )
        # No TTY here, so setup's timed prompts (prefix, plan-tune hooks) take their non-interactive defaults.
        # update_check=false keeps the pin: no "upgrade available" notices mid-benchmark. Telemetry is off by default.
        # Setup output and gstack-doctor go to the trial's agent logs: whether the browser came up depends on
        # the image (see cross_harness.md), so each trial records what it actually got.
        logs = self.environment_logs_dir.as_posix()
        sessions = (self.environment_logs_dir / "sessions").as_posix()
        await self.exec_as_agent(
            environment,
            command=(
                'export PATH="$HOME/.local/bin:$PATH"; '  # as in Harbor's run step, so doctor finds claude
                f"mkdir -p {logs} && (cd {GSTACK_ROOT} && ./setup) 2>&1 | tee {logs}/gstack-setup.txt && "
                f"{GSTACK_ROOT}/bin/gstack-config set update_check false && "
                f"(cat ~/.claude/settings.json 2>/dev/null || echo '{{}}') > {GSTACK_SETTINGS} && "
                f"mkdir -p {sessions} && printf %s {shlex.quote(GSTACK_CLAUDE_MD)} > {sessions}/CLAUDE.md && "
                # From the task workdir with the run's config dir, so its CLAUDE.md check sees what Claude will see.
                f"{{ CLAUDE_CONFIG_DIR={sessions} {GSTACK_ROOT}/bin/gstack-doctor > {logs}/gstack-doctor.txt 2>&1 || true; }}"
            ),
            timeout_sec=int(self.DEFAULT_SETUP_TIMEOUT_SEC) - 120,
        )

    def build_cli_flags(self) -> str:
        base = super().build_cli_flags()
        return f"{base} --settings {shlex.quote(GSTACK_SETTINGS)}".strip()


class AbbdClaudeCode(PluginClaudeCode):
    """Groups C, D, E. C skips `abbd init`; D and E run it in the task's working directory."""

    @staticmethod
    def name() -> str:
        return "claude-code-abbd"

    async def install(self, environment: BaseEnvironment) -> None:
        await super().install(environment)
        await environment.upload_file(os.environ["ABBD_BIN"], REMOTE_BIN)
        await self.exec_as_root(environment, command=f"chmod 755 {REMOTE_BIN} && command -v git sh grep sed")
        if os.environ.get("ABBD_GROUP", "E") in ("D", "E"):
            # Runs in the image's WORKDIR during Harbor's agent setup phase. No model calls. No `git init`:
            # that would change the task's starting state; abbd falls back to revision "nogit".
            # Open risk: .abbd/ appears in the task directory, which a verifier that lists files could see.
            await self.exec_as_agent(environment, command="abbd init --mode code-only --verify-available")
