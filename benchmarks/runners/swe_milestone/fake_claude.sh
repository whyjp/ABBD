#!/bin/sh
# Dry-run stand-in for the in-container `claude`. Makes no model call. Per invocation it reports what
# the adapter set up, then submits the first queued milestone unchanged so the harness's capture,
# DAG unlock and official evaluator run for real. `git add -A` is the worst case for the .abbd/ leak
# check: the blueprint gets committed, and the snapshot must still leave it out.
case "$1" in
  --version|-v) exec /usr/local/bin/claude-real --version ;;
esac

session=""
plugin=""
prev=""
for a in "$@"; do
  case "$prev" in
    --session-id|--resume) session="$a" ;;
    --plugin-dir) plugin="$a" ;;
  esac
  prev="$a"
done

say() { echo "SHIM $*" >&2; }
say "argv: $*"
say "user=$(id -un) cwd=$(pwd)"
if [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]; then say "auth: CLAUDE_CODE_OAUTH_TOKEN set"; else say "auth: CLAUDE_CODE_OAUTH_TOKEN unset"; fi
if [ -n "${ANTHROPIC_API_KEY:-}" ]; then say "auth: ANTHROPIC_API_KEY set"; else say "auth: ANTHROPIC_API_KEY unset"; fi
if [ -n "$plugin" ]; then
  say "plugin: $plugin entries=$(ls -A "$plugin" | tr '\n' ' ')"
  say "plugin validate: $(/usr/local/bin/claude-real plugin validate "$plugin" 2>&1 | tr '\n' ' ' | cut -c1-300)"
else
  say "plugin: none"
fi
if command -v abbd >/dev/null 2>&1; then say "abbd: $(command -v abbd)"; else say "abbd: not on PATH"; fi
if [ -d .abbd ]; then say ".abbd: present files=$(find .abbd -type f | wc -l)"; else say ".abbd: absent"; fi
say "prompt: $(wc -c | tr -d ' ') bytes on stdin"
say "queue: $(grep -c '^- ' /e2e_workspace/TASK_QUEUE.md 2>/dev/null) task(s)"

mid=$(sed -n 's/^- \([^:]*\): See SRS.*/\1/p' /e2e_workspace/TASK_QUEUE.md 2>/dev/null | head -1)
if [ -n "$mid" ] && ! git rev-parse -q --verify "refs/tags/agent-impl-$mid" >/dev/null; then
  git add -A >/dev/null 2>&1
  git commit -q --allow-empty -m "dry-run: submit $mid unchanged" >/dev/null 2>&1
  git tag "agent-impl-$mid"
  say "submitted $mid at $(git rev-parse --short HEAD); tracked .abbd files=$(git ls-files .abbd | wc -l)"
else
  say "nothing to submit"
fi

printf '{"type":"result","subtype":"success","is_error":false,"duration_ms":0,"num_turns":0,"result":"dry-run shim","session_id":"%s","total_cost_usd":0,"usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}\n' "$session"
