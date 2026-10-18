#!/usr/bin/env sh
# UserPromptSubmit hook: keep the user's message verbatim so the next `abbd run` records the
# user's own words (USER_STATED) and every rule sentence in them. Silent; never blocks the prompt.
command -v abbd >/dev/null 2>&1 || exit 0
abbd note-prompt >/dev/null 2>&1
exit 0
