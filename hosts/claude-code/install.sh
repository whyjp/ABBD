#!/usr/bin/env sh
# Install For My Harness skills into a project for Claude Code.
# Usage: install.sh [PROJECT_DIR]   (default: current directory)
# Re-running replaces only the harness skill directories; other skills are left alone.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
src="$script_dir/../../skills"
project=${1:-.}

if [ ! -d "$project" ]; then
  echo "project directory not found: $project" >&2
  exit 1
fi

dest="$project/.claude/skills"
mkdir -p "$dest"

count=0
for skill_dir in "$src"/*/; do
  [ -f "$skill_dir/SKILL.md" ] || continue
  name=$(basename "$skill_dir")
  rm -rf "${dest:?}/$name"
  cp -R "$skill_dir" "$dest/$name"
  echo "installed $dest/$name"
  count=$((count + 1))
done

if [ "$count" -eq 0 ]; then
  echo "no skills found under $src" >&2
  exit 1
fi

cat <<'EOF'

Add this server to <project>/.mcp.json (merge with existing "mcpServers" if present):

{
  "mcpServers": {
    "cccv": {
      "command": "cccv",
      "args": ["mcp"]
    }
  }
}
EOF
