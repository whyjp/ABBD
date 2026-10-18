#!/usr/bin/env sh
# SessionStart context for the cccv plugin. Prints at most a few lines and never fails the session.
if ! command -v cccv >/dev/null 2>&1; then
  echo "cccv plugin: the cccv CLI is not on PATH. Install it with: go install github.com/whyjp/cccv/cmd/cccv@latest"
  exit 0
fi
if [ -d .cccv ] || [ -d .myharness ]; then
  echo "cccv: this repository has project knowledge. Start each development request with the cccv:session-bootstrap skill (cccv run \"<request>\")."
  cccv status 2>/dev/null | grep -E '^( +(feature_map|graph|html):|external_change: true|unconfirmed:|active_turn:)' | head -6
else
  echo "cccv: no .cccv/ here yet. The cccv:session-bootstrap skill creates it without asking questions (cccv init)."
fi
exit 0
