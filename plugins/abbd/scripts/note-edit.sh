#!/usr/bin/env sh
# PostToolUse hook (Edit/Write/MultiEdit/NotebookEdit): record the file the agent changed. Such
# files are verified in a turn; reconcile refuses to absorb them as outside changes. Silent.
command -v abbd >/dev/null 2>&1 || exit 0
abbd note-edit >/dev/null 2>&1
exit 0
