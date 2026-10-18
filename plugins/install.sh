#!/usr/bin/env sh
# Install the abbd skills for a host that has no plugin install from a GitHub subfolder.
# Usage: install.sh --host codex|antigravity|claude-code [--scope project|user] [PROJECT_DIR]
# Copies plugins/abbd/skills/* (the single source) to the host's skill path as abbd-<name>,
# replacing only those directories. Writes the MCP config only when the file does not exist yet;
# otherwise prints the snippet to merge, because merging someone's config is not ours to guess.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
src="$here/abbd/skills"
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
  # Without a plugin namespace the skill names carry the abbd prefix (PRD v1.5 CR-13).
  name="abbd-$(basename "$dir")"
  rm -rf "${skills:?}/$name"
  cp -R "$dir" "$skills/$name"
  sed "s/^name: .*/name: $name/" "$dir/SKILL.md" > "$skills/$name/SKILL.md"
  count=$((count + 1))
done
[ "$count" -gt 0 ] || { echo "no skills under $src" >&2; exit 1; }
echo "installed $count abbd skills into $skills"

case $fmt in
  toml) snippet='[mcp_servers.abbd]
command = "abbd"
args = ["mcp"]' ;;
  json) snippet='{
  "mcpServers": {
    "abbd": { "command": "abbd", "args": ["mcp"] }
  }
}' ;;
  none) snippet="" ;;
esac
if [ -n "$mcp" ]; then
  if [ -e "$mcp" ]; then
    if grep -q 'abbd' "$mcp"; then
      echo "MCP: $mcp already mentions abbd; left unchanged"
    else
      printf 'MCP: %s exists; merge this entry yourself:\n\n%s\n' "$mcp" "$snippet"
    fi
  else
    mkdir -p "$(dirname "$mcp")"
    printf '%s\n' "$snippet" > "$mcp"
    echo "MCP: wrote $mcp"
  fi
fi
command -v abbd >/dev/null 2>&1 || echo "next: go install github.com/whyjp/abbd/cmd/abbd@latest (the skills and the MCP server call it)"
