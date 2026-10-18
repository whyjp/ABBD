"""UNTESTED sketch. SWE-Milestone agent framework that wraps the built-in Claude Code framework with ABBD.

Written against DeepCommit-ai/SWE-Milestone @ 17a8f15 (harness/e2e/agents/base.py AgentFramework,
harness/e2e/agents/claude_code.py ClaudeCodeFramework, @register_framework), read 2026-10-18.

The framework registry is populated by imports in harness/e2e/agents/__init__.py, so this module must be
imported before `get_agent_framework("claude-code-abbd")` runs. The smallest way is one import line added
to a local, uncommitted copy of that __init__.py; record that patch in the run metadata.

trial_config.yaml: agent: claude-code-abbd (or claude-code for group A), model: <exact id>, timeout: 18000
Host env read by this class: ABBD_GROUP (B, C, D, E), ABBD_BIN, ABBD_PLUGIN (absolute host paths).
"""
import os
from typing import List

from harness.e2e.agents.base import register_framework
from harness.e2e.agents.claude_code import ClaudeCodeFramework

REMOTE_PLUGIN = "/opt/bench-plugin"
REMOTE_BIN = "/opt/abbd/abbd"


@register_framework("claude-code-abbd")
class AbbdClaudeCodeFramework(ClaudeCodeFramework):
    def _group(self) -> str:
        return os.environ.get("ABBD_GROUP", "E")

    def get_container_mounts(self) -> List[str]:
        mounts = super().get_container_mounts()
        mounts += ["-v", f"{os.environ['ABBD_PLUGIN']}:{REMOTE_PLUGIN}:ro"]
        if self._group() != "B":
            mounts += ["-v", f"{os.environ['ABBD_BIN']}:{REMOTE_BIN}:ro"]
        return mounts

    def get_container_init_script(self, agent_name: str) -> str:
        script = super().get_container_init_script(agent_name)
        if self._group() == "B":
            return script
        # The init script is Python run as root after launch (base.py docstring).
        extra = f"""
import os, subprocess
if not os.path.exists("/usr/local/bin/abbd"):
    os.symlink({REMOTE_BIN!r}, "/usr/local/bin/abbd")
"""
        if self._group() in ("D", "E"):
            # No model calls. Runs once per repo, before the first milestone; .abbd/ is outside src_dirs.
            extra += """
subprocess.run(["abbd", "init", "--mode", "code-only", "--verify-available"], cwd="/testbed", check=False)
subprocess.run(["chmod", "-R", "a+rwX", "/testbed/.abbd"], check=False)
"""
        return script + extra

    def build_run_command(self, model: str, session_id: str, prompt_path: str) -> str:
        cmd = super().build_run_command(model, session_id, prompt_path)
        return cmd.replace("claude ", f"claude --plugin-dir {REMOTE_PLUGIN} ", 1)

    def build_resume_command(self, model: str, session_id: str, message_path: str) -> str:
        cmd = super().build_resume_command(model, session_id, message_path)
        return cmd.replace("claude ", f"claude --plugin-dir {REMOTE_PLUGIN} ", 1)
