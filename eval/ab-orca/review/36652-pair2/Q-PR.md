# Configurable separator between branch prefix and workspace name

Closes #36652

## Why

Teams whose branch convention uses a dash (`jd-feat-login-form`) could not use the branch prefix setting, because Orca always joined prefix and name with `/`. They had to edit the branch field on every new workspace or pass `--branch` from the CLI. This adds a separator setting so the prefix setting works for them, and leaves everyone else on `/`.

## What changes for users

Settings > Git > Branch Prefix gets a small separator input next to the prefix field, defaulting to `/`. With prefix `jd` and separator `-`, a workspace named `feat: login form` gets the branch `jd-feat-login-form`. The feedback line under the inputs previews the real join (`jd-feature`) and flags a separator git would reject.

A blank separator falls back to `/`. Existing settings files have no value and load the default, so nothing changes for current setups.

## How

- `GlobalSettings.branchPrefixSeparator` (default `/`) is a new field. It is optional on `BranchPrefixSettings` and on the runtime store contract, so a runtime that receives settings from an older client keeps the slash.
- `computeBranchName` joins with `normalizeBranchPrefixSeparator(settings.branchPrefixSeparator)`. `computeValidatedBranchName` now delegates the join to it, so the join lives in one place.
- `getBranchPrefixSeparatorIssue` runs the separator through `getBranchPrefixIssue` between two plain segments (`a<sep>b`). Separators with spaces, `~ ^ : ? * [ \`, `..`, `@{`, or a dot-leading path segment are rejected. Slash runs (`//`) are also rejected, because prefix normalization would otherwise hide them. Worktree creation throws a settings-pointing error for an invalid separator. Unlike an unusable git username, the separator is always user-configured, so it is never silently skipped.
- The first-work auto-rename hook used to find the branch leaf by slicing after the last `/`. With a dash separator, `jd-crab` would never be recognized as an auto-generated creature name, and auto-rename would silently stop. `getBranchLeaf` strips the configured `prefix + separator` when it matches and otherwise falls back to the last `/` segment, so branches created under older settings still work. The hook resolves the git username before the creature check to make this possible, and only under the git-username strategy, matching the creation path.
- `stripConfiguredBranchPrefix` needed no change. It compares dash-slugified text, so a model echoing `jd/` or `jd-` is stripped either way.
- The prefix input's draft-and-echo handling moved into a small `useSettingTextDraft` hook, which the separator input reuses.

## Known limitation

With a non-slash separator, auto-rename can only find the creature leaf when the current prefix resolves to what the branch was created with. A branch like `jdoe-crab` is skipped for good in two cases. One is a git username lookup that comes back empty at rename time. The other is a change to the prefix or separator after the branch exists. The slash separator is unaffected because the last-`/` fallback still applies. Treating an unresolved prefix as retry rather than settled would close the first case. That changes the hook's retry policy, so it is left for a follow-up.

## Not in scope

The per-project prefix (#18103) and the folder template branch token (#5082) are not implemented here. Both would read the same separator through `computeBranchName`.

## Verification

- New `src/main/ipc/worktree-branch-name.test.ts` covers the dash join from the issue, the slash fallback for missing and blank separators, rejection of `~`, `- -`, `//`, `..`, and `/.`, separators ignored when no prefix applies, and leaf recovery for dash and legacy slash branches.
- `branch-prefix`, `branch-name-from-work`, `worktree-branch-name`, `worktree-logic`, and both `first-work-branch-rename` suites pass, except `worktree-logic`'s `sanitizeWorktreeName` cases. Those fail identically on the base commit because `emojibase-data` does not resolve in the local install.
- The runtime worktree-create, `worktree-remote`, and GitPane suites show the same 63 failed / 25 passed on the base commit and on this branch. All failures are the same `emojibase-data` resolution error.
- `oxlint` and `oxfmt --check` are clean on every changed file.
- Typecheck of the node and web projects reports no errors in changed files beyond missing-module noise from the unbuilt local install, and no error anywhere names `branchPrefixSeparator`.
- Locale strings were added for all 11 catalogs by hand. `sync:localization-catalog` could not run locally because the `typescript-api` package does not resolve, so CI's localization check is the authority.
- No rendered UI check. The Electron app and component fixtures need built workspace packages that this checkout lacks.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
