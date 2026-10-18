#!/usr/bin/env python3
"""Build the group C and D plugin dirs from plugins/abbd without editing it.

  C: .claude-plugin/ + skills/                     (no hooks, no MCP)
  D: .claude-plugin/ + skills/ + hooks/ + scripts/ (no MCP) + a PreToolUse deny for graph queries
  E: plugins/abbd as shipped (not copied)

Usage: python build_arms.py <repo_root> <out_dir>
"""
import json
import os
import shutil
import sys

GRAPH_DENY = "abbd-deny-graph.sh"
GRAPH_DENY_SH = """#!/bin/sh
# Group D hides graph queries from the agent; the engine still builds graph.json on publish.
input=$(cat)
if printf '%s' "$input" | grep -Eq '"command"[[:space:]]*:[[:space:]]*"[^"]*abbd[[:space:]]+(graph|inspect|impact)\\b'; then
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"graph queries are disabled in this benchmark arm (group D)"}}'
  exit 0
fi
exit 0
"""


def build(repo, out):
    src = os.path.join(repo, "plugins", "abbd")
    for arm, parts in {"C": (".claude-plugin", "skills"), "D": (".claude-plugin", "skills", "hooks", "scripts")}.items():
        dst = os.path.join(out, arm)
        shutil.rmtree(dst, ignore_errors=True)
        for p in parts:
            shutil.copytree(os.path.join(src, p), os.path.join(dst, p))
    d = os.path.join(out, "D")
    open(os.path.join(d, "scripts", GRAPH_DENY), "w", newline="\n").write(GRAPH_DENY_SH)
    hooks_path = os.path.join(d, "hooks", "hooks.json")
    hooks = json.load(open(hooks_path, encoding="utf-8"))
    hooks.setdefault("hooks", {}).setdefault("PreToolUse", []).append(
        {"matcher": "Bash", "hooks": [{"type": "command", "command": f"sh ${{CLAUDE_PLUGIN_ROOT}}/scripts/{GRAPH_DENY}"}]})
    json.dump(hooks, open(hooks_path, "w", encoding="utf-8"), indent=2)
    print(f"built {out}/C and {out}/D from {src}")


if __name__ == "__main__":
    build(sys.argv[1], sys.argv[2])
