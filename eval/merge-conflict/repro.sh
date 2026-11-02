#!/usr/bin/env bash
# Reproduces issue #18: two branches off one synced commit each edit one
# handler in a different module, each run abbd sync, then merge. Prints the
# files git left conflicted, then whether `abbd sync` on the merged tree
# brings the state back to CURRENT.
#
# usage: [REV=COMMIT] [NO_UNION=1] repro.sh ABBD_BINARY [comment|symbol] [REPO]
#   NO_UNION=1 drops the repository's `.abbd/state.yaml merge=union` line first
#   comment  each branch appends a comment, so only the tree fingerprint moves
#   symbol   each branch adds an exported func, so the rendered units change too
set -euo pipefail

abbd=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
scenario=${2:-comment}
repo=${3:-$(git -C "$(dirname "$0")" rev-parse --show-toplevel)}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

git clone -q "$repo" "$work/r"
cd "$work/r"
git -c advice.detachedHead=false checkout -q "${REV:-$(git -C "$repo" rev-parse HEAD)}"
git switch -q -c base
git config user.email repro@example.com
git config user.name repro
if [ "${NO_UNION:-}" = 1 ]; then
	sed -i '/merge=union/d' .gitattributes
fi
"$abbd" sync >/dev/null
git add -A && git commit -q -m "sync base" --allow-empty

edit() {
	local file=$1 name=$2
	case $scenario in
	comment) printf '\n// touched on branch %s\n' "$name" >>"$file" ;;
	symbol) printf '\n// Touched%s reports the branch that added it.\nfunc Touched%s() string { return "%s" }\n' "$name" "$name" "$name" >>"$file" ;;
	esac
}

for side in A:cmd/abbd/flow.go B:internal/topology/find.go; do
	name=${side%%:*} file=${side#*:}
	git switch -q -c "$name" base
	edit "$file" "$name"
	"$abbd" sync >/dev/null
	git add -A && git commit -q -m "edit $file and sync"
done

git switch -q A
git merge -q --no-edit B >/dev/null 2>&1 || true
conflicts=$(git diff --name-only --diff-filter=U)
echo "scenario: $scenario${NO_UNION:+, no merge=union}"
echo "conflicted files: $(printf '%s' "$conflicts" | grep -c . || true)"
hunks=0
for f in $conflicts; do hunks=$((hunks + $(grep -c '^<<<<<<<' "$f"))); done
echo "conflict hunks: $hunks"
printf '%s\n' "$conflicts" | sed '/^$/d; s/^/  /'
echo "status after merge:"
"$abbd" status 2>&1 | grep -E '^(state|blueprint):' | sed 's/^/  /' || true
if "$abbd" sync >/dev/null 2>"$work/sync.err"; then
	echo "abbd sync on the merged tree: ok"
	git add -A
	"$abbd" status 2>&1 | grep -E '^(state|blueprint):' | sed 's/^/  /' || true
else
	echo "abbd sync on the merged tree: failed: $(head -1 "$work/sync.err")"
fi
