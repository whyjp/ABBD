#!/usr/bin/env sh
# Install the cccv skills for a host that has no plugin install from a GitHub subfolder.
# Usage: install.sh --host codex|antigravity|claude-code [--scope project|user] [PROJECT_DIR]
# Copies plugins/cccv/skills/* (the single source) to the host's skill path, replacing only
# the cccv skill directories. Writes the MCP config only when the file does not exist yet;
# otherwise prints the snippet to merge, because merging someone's config is not ours to guess.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
src="$here/cccv/skills"
host="" scope="project" project="."
while [ $# -gt 0 ]; do
  case "$1" in
    --host) host=$2; shift 2 ;;
    --scope) scope=$2; shift 2 ;;
    -h|--help) sed -n '2,6p' "$0"; exit 0 ;;
    *) project=$1; shift ;;
  esac
done

case "$host:$scope" in
  codex:project)       skills="$project/.agents/skills";  mcp="$project/.codex/config.toml";            fmt=toml ;;
  codex:user)          skills="$HOME/.agents/skills";     mcp="$HOME/.codex/config.toml";               fmt=toml ;;
  antigravity:project) skills="$project/.agents/skills";  mcp="$project/.agents/mcp_config.json";        fmt=json ;;
  antigravity:user)    skills="$HOME/.gemini/config/skills"; mcp="$HOME/.gemini/config/mcp_config.json"; fmt=json ;;
  claude-code:project) skills="$project/.claude/skills";  mcp="$project/.mcp.json";                    fmt=json ;;
  claude-code:user)    skills="$HOME/.claude/skills";     mcp="";                                      fmt=none ;;
  *) echo "usage: install.sh --host codex|antigravity|claude-code [--scope project|user] [PROJECT_DIR]" >&2; exit 1 ;;
esac
if [ "$scope" = project ] && [ ! -d "$project" ]; then
  echo "project directory not found: $project" >&2
  exit 1
fi

mkdir -p "$skills"
count=0
for dir in "$src"/*/; do
  [ -f "$dir/SKILL.md" ] || continue
  name=$(basename "$dir")
  rm -rf "${skills:?}/$name"
  cp -R "$dir" "$skills/$name"
  count=$((count + 1))
done
[ "$count" -gt 0 ] || { echo "no skills under $src" >&2; exit 1; }
echo "installed $count cccv skills into $skills"

case $fmt in
  toml) snippet='[mcp_servers.cccv]
command = "cccv"
args = ["mcp"]' ;;
  json) snippet='{
  "mcpServers": {
    "cccv": { "command": "cccv", "args": ["mcp"] }
  }
}' ;;
  none) snippet="" ;;
esac
if [ -n "$mcp" ]; then
  if [ -e "$mcp" ]; then
    if grep -q 'cccv' "$mcp"; then
      echo "MCP: $mcp already mentions cccv; left unchanged"
    else
      printf 'MCP: %s exists; merge this entry yourself:\n\n%s\n' "$mcp" "$snippet"
    fi
  else
    mkdir -p "$(dirname "$mcp")"
    printf '%s\n' "$snippet" > "$mcp"
    echo "MCP: wrote $mcp"
  fi
fi
command -v cccv >/dev/null 2>&1 || echo "next: go install github.com/whyjp/cccv/cmd/cccv@latest (the skills and the MCP server call it)"
