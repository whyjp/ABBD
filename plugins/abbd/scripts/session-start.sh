#!/usr/bin/env sh
# SessionStart context for the ABBD plugin. Prints a short mandate plus the blueprint state.
# Never fails the session.
if ! command -v abbd >/dev/null 2>&1; then
  echo "ABBD plugin: the abbd CLI is not on PATH. Install it with: go install github.com/whyjp/abbd/cmd/abbd@latest"
  exit 0
fi
cat <<'EOF'
<ABBD>
This repository is developed from its blueprint (ABBD). The blueprint is .abbd/: Architecture Map,
Feature Map, external contract view, review topology. It must describe the code at every turn end.

Every request that changes code, tests or configuration runs the ABBD turn, however small:
  1. abbd run "<the request>"        opens the turn (bootstraps .abbd/ and reconciles outside edits itself)
  2. make the change; abbd impact / abbd inspect ID when you need the blueprint's view
  3. abbd verify                     must report PASSED on the final working tree
  4. abbd sync                       the turn is done only when sync exits 0
A small fix runs the turn lightly; it does not skip it. Exit code 2 from verify or sync means
not done: fix and repeat. Never describe structure in chat; if sync returns `review`, give the
user the review link (abbd serve) and leave decisions to them. Skills: abbd:session-bootstrap first.
</ABBD>
EOF
if [ -d .abbd ] || [ -d .cccv ] || [ -d .myharness ]; then
  abbd status 2>&1 | grep -E '^(notice:| +(feature_map|graph|html):|external_change: true|unconfirmed:|active_turn:)' | head -7
else
  echo "No .abbd/ yet: abbd run (or abbd init) creates it without asking questions."
fi
exit 0
