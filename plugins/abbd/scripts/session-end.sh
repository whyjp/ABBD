#!/usr/bin/env sh
# SessionEnd hook: the agent session is closed, so later unsynced changes may be reconciled as
# outside changes again. Silent; never fails the host.
command -v abbd >/dev/null 2>&1 || exit 0
abbd note-session end >/dev/null 2>&1
exit 0
