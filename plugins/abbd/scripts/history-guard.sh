#!/usr/bin/env sh
# PreToolUse hook: keep the agent's context on the current blueprint. Per-request history under
# .abbd/ (receipts, request texts, decision log, run evidence, derived views) is the audit trail;
# read as context it mixes superseded intent into the current one. Direct reads of it are refused
# and pointed at .abbd/BLUEPRINT.md and the abbd queries. The abbd CLI and git are not affected.
input=$(cat)
history='\.(abbd|cccv|myharness)[/\\]+(changes|requirements|decisions|verification[/\\]+runs|generated)([/\\"]|$)|\.(abbd|cccv|myharness)[/\\]+(candidates\.yaml|state\.yaml|learning\.yaml|bootstrap-report)'
printf '%s' "$input" | grep -Eq "$history" || exit 0
tool=$(printf '%s' "$input" | sed -n 's/.*"tool_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [ "$tool" = "Bash" ] || [ "$tool" = "PowerShell" ]; then
  command=$(printf '%s' "$input" | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\(.*\)".*/\1/p')
  # Only a single abbd or git command passes; chaining (&&, ;, |) could append a read.
  case "$command" in
    *'&&'*|*';'*|*'|'*) ;;
    abbd\ *|git\ *|rtk\ git\ *) exit 0 ;;
  esac
fi
cat <<'EOF'
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"ABBD: .abbd/changes, requirements, decisions, verification/runs and generated are history, not context. Read .abbd/BLUEPRINT.md (current rules, modules, contracts, features, open decisions) or the current md under .abbd/architecture and .abbd/features; for one item use `abbd inspect ID`, for decisions `abbd decisions QUERY`, for a past turn `abbd turn show` / `abbd review CHG-x`, for a stored run `abbd inspect run-...`."}}
EOF
exit 0
