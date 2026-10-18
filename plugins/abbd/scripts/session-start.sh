#!/usr/bin/env sh
# SessionStart context for the abbd plugin. Prints at most a few lines and never fails the session.
if ! command -v abbd >/dev/null 2>&1; then
  echo "abbd plugin: the abbd CLI is not on PATH. Install it with: go install github.com/whyjp/abbd/cmd/abbd@latest"
  exit 0
fi
if [ -d .abbd ] || [ -d .cccv ] || [ -d .myharness ]; then
  echo "abbd: this repository has project knowledge. Start each development request with the abbd:session-bootstrap skill (abbd run \"<request>\")."
  abbd status 2>/dev/null | grep -E '^( +(feature_map|graph|html):|external_change: true|unconfirmed:|active_turn:)' | head -6
else
  echo "abbd: no .abbd/ here yet. The abbd:session-bootstrap skill creates it without asking questions (abbd init)."
fi
exit 0
