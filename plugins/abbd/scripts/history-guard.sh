#!/usr/bin/env sh
# PreToolUse hook: keep the agent's context on the current blueprint. Per-request history under
# .abbd/ (receipts, request texts, decision log, run evidence, derived views) is the audit trail;
# read as context it mixes superseded intent into the current one. Reads of it are refused and
# pointed at .abbd/BLUEPRINT.md and the abbd queries. The abbd CLI is not affected.
input=$(cat)
dir='\.(abbd|cccv|myharness)'
printf '%s' "$input" | grep -Eq "$dir" || exit 0

history="$dir[/\\\\]+(changes|requirements|decisions|verification[/\\\\]+runs|generated)|$dir[/\\\\]+(candidates\.yaml|state\.yaml|learning\.yaml|bootstrap-report)"
# A wildcard right under .abbd/ (".abbd/*", ".abbd/ch*", ".abbd/**") can reach history.
wild="$dir[/\\\\]+[^/\\\\\" ]*[*?{]"
current="$dir[/\\\\]+(BLUEPRINT\.md|features|architecture|verification[/\\\\]+TEST-|project\.yaml|\.ignore)"

tool=$(printf '%s' "$input" | sed -n 's/.*"tool_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
deny=0
case "$tool" in
  Bash|PowerShell)
    command=$(printf '%s' "$input" | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\(.*\)".*/\1/p')
    single=1
    case "$command" in *'&&'*|*';'*|*'|'*|*'`'*|*'$('*) single=0 ;; esac
    if [ $single = 1 ] && printf '%s' "$command" | grep -Eq '^[[:space:]]*(rtk[[:space:]]+)?(abbd[[:space:]]|git[[:space:]]+(add|commit|status|rm|mv|restore|checkout|stash|reset)([[:space:]]|$))'; then
      exit 0
    fi
    if printf '%s' "$command" | grep -Eq "$history|$wild"; then
      deny=1
    elif printf '%s' "$command" | grep -Eq "(^|[[:space:];&|])cd[[:space:]]+[^;&|]*$dir"; then
      deny=1
    elif ! printf '%s' "$command" | grep -Eq "$current"; then
      # Mentions .abbd without naming a current blueprint path: a whole-directory read.
      deny=1
    fi
    ;;
  *)
    if printf '%s' "$input" | grep -Eq "$history|$wild"; then
      deny=1
    fi
    ;;
esac
[ $deny = 1 ] || exit 0
cat <<'EOF'
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"ABBD: .abbd/changes, requirements, decisions, verification/runs and generated are history, not context. Read .abbd/BLUEPRINT.md (current rules, modules, contracts, features, open decisions) or the current md under .abbd/architecture and .abbd/features; for one item use `abbd inspect ID`, for decisions `abbd decisions QUERY`, for a past turn `abbd turn show` / `abbd review CHG-x`, for a stored run `abbd inspect run-...`."}}
EOF
exit 0
