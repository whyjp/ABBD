#!/usr/bin/env sh
# Stop hook: the ABBD completion gate at the session level. If the blueprint does not describe
# the working tree (an open turn or unsynced edits), the agent is sent back once to finish the
# turn. A second stop in a row is let through so a stuck session can still end and report.
input=$(cat)
case "$input" in
  *'"stop_hook_active":true'*|*'"stop_hook_active": true'*) exit 0 ;;
esac
command -v abbd >/dev/null 2>&1 || exit 0
[ -d .abbd ] || exit 0
status=$(abbd --json status 2>/dev/null) || exit 0
case "$status" in
  *'"feature_map": "CURRENT"'*) exit 0 ;;
esac
cat <<'EOF'
{"decision":"block","reason":"ABBD: the blueprint does not match the working tree (an open turn or unsynced changes). Finish the turn: abbd verify until PASSED, then abbd sync until it exits 0. If the edits were not meant to stay, abbd turn abort --reason \"...\". Report a gate you cannot pass instead of claiming done."}
EOF
exit 0
