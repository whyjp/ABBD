"""SWE-Milestone agent framework: the harness's own Claude Code agent plus one ABBD study arm.

Written against DeepCommit-ai/SWE-Milestone @ 17a8f15 (harness/e2e/agents/base.py, claude_code.py).

The class replaces the registry entry for "claude-code" instead of adding a new agent name. The harness
keys rate-limit detection, credential refresh, cost recalculation and log parsing on the literal agent
name "claude-code" (agent_runner._OAUTH_AGENTS, collect_results, trial_metrics, log_parser), so a new
name would silently lose usage-limit handling. Every method calls the stock implementation first and
only appends, so the prompt, the task queue, the tests, the timeouts and the evaluator are untouched.

Loaded by swe_milestone_site/sitecustomize.py when ABBD_GROUP is set (PYTHONPATH, set by
runners/swe_milestone/run_group.py). The harness checkout is not edited.

Host environment read here:
  ABBD_GROUP              A, B, C, D or E (required)
  ABBD_PLUGIN             plugin dir mounted for B to E
  ABBD_BIN                static linux abbd binary mounted for C to E
  ABBD_INIT_ARGS          arguments of the one `abbd init` per repo for D and E
  CLAUDE_CODE_OAUTH_TOKEN subscription token; passed by name, its value never enters argv or logs
  ABBD_CLAUDE_SHIM        dry run only: a script installed over the in-container `claude`
"""
import os
import shlex
from dataclasses import dataclass
from typing import List

from harness.e2e.agents.base import register_framework
from harness.e2e.agents.claude_code import ClaudeCodeFramework

REMOTE_PLUGIN = "/opt/bench-plugin"
REMOTE_BIN = "/opt/abbd/abbd"
REMOTE_SHIM = "/opt/bench-shim/claude"
HOST_CREDENTIALS_TARGET = "/tmp/host-claude-credentials/.credentials.json"
DEFAULT_INIT_ARGS = "--mode code-only --verify-available"
INIT_LOG = "/tmp/abbd-init.log"


@dataclass(frozen=True)
class Arm:
    plugin: bool
    abbd_on_path: bool
    abbd_init: bool


ARMS = {
    "A": Arm(plugin=False, abbd_on_path=False, abbd_init=False),
    "B": Arm(plugin=True, abbd_on_path=False, abbd_init=False),
    "C": Arm(plugin=True, abbd_on_path=True, abbd_init=False),
    "D": Arm(plugin=True, abbd_on_path=True, abbd_init=True),
    "E": Arm(plugin=True, abbd_on_path=True, abbd_init=True),
}


def _required_path(name: str) -> str:
    value = os.environ.get(name, "")
    if not value or not os.path.exists(value):
        raise RuntimeError(f"{name} must name an existing host path (got {value!r})")
    return os.path.abspath(value)


class AbbdClaudeCodeFramework(ClaudeCodeFramework):
    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        group = os.environ.get("ABBD_GROUP", "")
        if group not in ARMS:
            raise RuntimeError(f"ABBD_GROUP must be one of {sorted(ARMS)} (got {group!r})")
        self.group = group
        self.arm = ARMS[group]
        self.plugin = _required_path("ABBD_PLUGIN") if self.arm.plugin else None
        self.abbd_bin = _required_path("ABBD_BIN") if self.arm.abbd_on_path else None
        self.init_args = shlex.split(os.environ.get("ABBD_INIT_ARGS", DEFAULT_INIT_ARGS))
        self.shim = _required_path("ABBD_CLAUDE_SHIM") if os.environ.get("ABBD_CLAUDE_SHIM") else None
        self.oauth = bool(os.environ.get("CLAUDE_CODE_OAUTH_TOKEN"))

    def get_container_env_vars(self) -> List[str]:
        env = super().get_container_env_vars()
        if self.oauth:
            # Name only: docker copies the value from the worker's environment.
            env += ["-e", "CLAUDE_CODE_OAUTH_TOKEN"]
        return env

    def get_container_mounts(self) -> List[str]:
        mounts = super().get_container_mounts()
        if self.oauth:
            mounts = _drop_mount(mounts, HOST_CREDENTIALS_TARGET)
        if self.plugin:
            mounts += ["-v", f"{self.plugin}:{REMOTE_PLUGIN}:ro"]
        if self.abbd_bin:
            mounts += ["-v", f"{self.abbd_bin}:{REMOTE_BIN}:ro"]
        if self.shim:
            mounts += ["-v", f"{self.shim}:{REMOTE_SHIM}:ro"]
        return mounts

    def get_container_init_script(self, agent_name: str) -> str:
        script = super().get_container_init_script(agent_name)
        script += f"\n# === ABBD study arm {self.group} ===\nimport os, shutil\n"
        if self.abbd_bin:
            script += f"""
if not os.path.lexists('/usr/local/bin/abbd'):
    os.symlink({REMOTE_BIN!r}, '/usr/local/bin/abbd')
print('abbd on PATH for arm {self.group}')
"""
        if self.shim:
            script += f"""
if not os.path.exists('/usr/local/bin/claude-real'):
    os.rename('/usr/local/bin/claude', '/usr/local/bin/claude-real')
shutil.copy({REMOTE_SHIM!r}, '/usr/local/bin/claude')
os.chmod('/usr/local/bin/claude', 0o755)
print('DRY RUN: /usr/local/bin/claude is a shim; the installed CLI is /usr/local/bin/claude-real')
"""
        return script

    def _with_arm(self, cmd: str) -> str:
        if not cmd.startswith("claude "):
            raise RuntimeError(f"unexpected stock Claude Code command shape: {cmd[:40]!r}")
        if self.plugin:
            cmd = cmd.replace("claude ", f"claude --plugin-dir {REMOTE_PLUGIN} ", 1)
        if self.arm.abbd_init:
            # Once per repo: the container is never reset between milestones, so .abbd/ persists
            # across milestones, resumes and session rotations. Runs after the harness has
            # truncated git history and locked the network. No model calls.
            init = " ".join(["abbd", "init", *map(shlex.quote, self.init_args)])
            cmd = (
                f"{{ [ -d .abbd ] || {{ {init} > {INIT_LOG} 2>&1; "
                f"echo \"abbd init exit=$? (log {INIT_LOG})\" >&2; }}; }}; {cmd}"
            )
        return cmd

    def build_run_command(self, model: str, session_id: str, prompt_path: str) -> str:
        return self._with_arm(super().build_run_command(model, session_id, prompt_path))

    def build_resume_command(self, model: str, session_id: str, message_path: str) -> str:
        return self._with_arm(super().build_resume_command(model, session_id, message_path))


def _drop_mount(mounts: List[str], target_suffix: str) -> List[str]:
    out: List[str] = []
    pairs = iter(mounts)
    for flag in pairs:
        value = next(pairs)
        if flag == "-v" and target_suffix in value:
            continue
        out += [flag, value]
    return out


register_framework("claude-code")(AbbdClaudeCodeFramework)
