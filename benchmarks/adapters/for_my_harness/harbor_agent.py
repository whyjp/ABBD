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
