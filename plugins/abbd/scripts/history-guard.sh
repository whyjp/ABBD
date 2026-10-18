#!/usr/bin/env sh
# PreToolUse hook: keep the agent's context on the current blueprint. Per-request history under
# .abbd/ (receipts, request texts, decision log, run evidence, derived views) is the audit trail;
# read as context it mixes superseded intent into the current one. Reads of it are refused and
# pointed at .abbd/BLUEPRINT.md and the abbd queries. The abbd CLI is not affected.
# Quotes that split a word (.ab""bd, gi''t) are dropped, JSON "\\" becomes "/", and "/./" and
# repeated separators collapse (.abbd/./changes is .abbd/changes) before matching; matching
# ignores case (Windows and macOS paths do).
input=$(cat | sed -e 's/\\\\/\//g' -e 's/\\"//g' -e "s/'//g" -e ':a' -e 's#/\./#/#g' -e 'ta' -e 's#//*#/#g')
dir='\.(abbd|cccv|myharness)'
# The directory itself, not a longer name such as docs/.abbd-notes.md.
dirb="$dir([/\" ]|\$)"
# A wildcard that can match the directory's own name (.ab*, .a?bd, .*/changes) reaches history
# whatever else the command names.
dirglob='(^|[/" =:])\.(a|ab|abb|abbd|c|cc|ccc|cccv|m|my|myh|myha|myhar|myharn|myharne|myharnes|myharness)[*?[{]|(^|[/" =:])\.[*?[{][^/" ]*/'
printf '%s' "$input" | grep -Eiq "$dirb|$dirglob" || exit 0

history="$dir/+(changes|requirements|decisions|verification/+runs|generated)|$dir/+(candidates\.yaml|state\.yaml|learning\.yaml|bootstrap-report)"
# A wildcard right under .abbd/ (".abbd/*", ".abbd/ch*", ".abbd/**") can reach history, and so
# can a ".." anywhere below it (features/../changes).
wild="$dir/+[^/\" ]*[*?{]|$dir/[^\" ]*\.\.(/|\"|\$| )"
current="$dir/+(BLUEPRINT\.md|features|architecture|verification/+TEST-|project\.yaml|\.ignore)"
# Grep/Glob relative to .abbd name a current place through their glob or pattern.
currentRel='"(glob|pattern)"[[:space:]]*:[[:space:]]*"(BLUEPRINT\.md|features|architecture|verification/TEST-)'
# Commands that write or ask abbd rather than read history: abbd itself and git bookkeeping that
# prints no file content (not show, log, diff or stash, whose `stash show` prints a diff).
allowed='^[[:space:]]*(rtk[[:space:]]+)?(abbd([[:space:]]|$)|git[[:space:]]+(add|commit|status|rm|mv|restore|checkout|reset)([[:space:]]|$))'

tool=$(printf '%s' "$input" | sed -n 's/.*"tool_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
deny=0
case "$tool" in
  Bash|PowerShell)
    command=$(printf '%s' "$input" | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\(.*\)".*/\1/p')
    # A chain made only of allowed commands (git add .abbd && git commit -m x) passes; a pipe or a
    # substitution can feed history anywhere, so it never does.
    case "$command" in
      ''|*'`'*|*'$('*) ;;
      *)
        if ! printf '%s\n' "$command" | awk '{ gsub(/&&|\|\||;/, "\n"); print }' | grep -v '^[[:space:]]*$' | grep -Eqv "$allowed" \
          && ! printf '%s' "$command" | sed -e 's/||//g' | grep -q '|'; then
          exit 0
        fi
        ;;
    esac
    if printf '%s' "$command" | grep -Eiq "$history|$wild|$dirglob"; then
      deny=1
    elif printf '%s' "$command" | grep -Eiq "(^|[[:space:];&|])cd[[:space:]]+[^;&|]*$dir"; then
      deny=1
    elif ! printf '%s' "$command" | grep -Eiq "$current"; then
      # Mentions .abbd without naming a current blueprint path: a whole-directory read.
      deny=1
    fi
    ;;
  *)
    if printf '%s' "$input" | grep -Eiq "$history|$wild|$dirglob"; then
      deny=1
    elif ! printf '%s' "$input" | grep -Eiq "$current|$currentRel"; then
      # Grep/Glob/Read on .abbd itself (or an unlisted file in it) without naming a current path.
      deny=1
    fi
    ;;
esac
[ $deny = 1 ] || exit 0
cat <<'EOF'
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"ABBD: .abbd/changes, requirements, decisions, verification/runs and generated are history, not context. Read .abbd/BLUEPRINT.md (current rules, modules, contracts, features, open decisions) or the current md under .abbd/architecture and .abbd/features; for one item use `abbd inspect ID`, for decisions `abbd decisions QUERY`, for a past turn `abbd turn show` / `abbd review CHG-x`, for a stored run `abbd inspect run-...`."}}
EOF
exit 0
