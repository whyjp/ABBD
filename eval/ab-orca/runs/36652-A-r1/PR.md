# Configurable separator between branch prefix and worktree name

Closes #36652.

## Why

Teams whose branch convention uses a dash (`jd-feat-login-form`) could not use the branch prefix setting, because Orca always joined the prefix with a slash (`jd/feat-login-form`). They had to edit the branch field by hand on every new workspace or pass `--branch` from the CLI. Putting the dash inside the prefix (`jd-`) did not help either, because the join still added a slash and produced `jd-/feat-login-form`.

## What changes for users

Settings > Git > Branch Prefix now has a small separator field next to the prefix input. It defaults to `/`, so existing setups behave exactly as before. With prefix `jd` and separator `-`, a workspace named `feat: login form` gets the branch `jd-feat-login-form`.

- The live feedback line previews the result with the chosen separator (`Branches will be named jd-feature`).
- A separator git cannot accept, such as `~`, `:`, a space, `..`, or `//`, shows an inline error. Creating a worktree with it fails with an error that points to Settings > Git, which matches how an invalid custom prefix is handled.
- A blank separator falls back to `/`, so clearing the field never glues the prefix and the name together.
- The setting applies to both the Git username and the custom prefix strategies. It has no effect when the strategy is None.

## How

- `BranchPrefixSettings` and `GlobalSettings` gain `branchPrefixSeparator`. The default is `'/'`, and loaded profiles merge over the defaults, so older profiles pick it up without a migration.
- `src/shared/branch-prefix.ts` adds `normalizeBranchPrefixSeparator` (trim, blank becomes `/`) and `getBranchPrefixSeparatorIssue`. The separator check reuses `getBranchPrefixIssue` on `a<separator>a`. The issue suggested running the separator through `getBranchPrefixIssue` directly, but that would reject a bare `-`, since a prefix cannot start with a dash. Slash runs are rejected first because `normalizeBranchPrefix` would otherwise collapse them and hide the empty ref segment.
- `src/main/ipc/worktree-branch-name.ts` adds `getBranchNameHead`, which returns everything that comes before the leaf (prefix plus separator, or `''`). `computeBranchName` and `computeValidatedBranchName` both build the name from it, and the validated path asserts the separator.
- The first-work auto rename (`first-work-branch-rename.ts`) used to find the leaf with `lastIndexOf('/')`. With a dash separator that returned the whole branch, which put the prefix into the sidebar name and the folder name. It now slices off `getBranchNameHead(...)`, which gives the same result for slash separators.
- `stripConfiguredBranchPrefix` needs no change. It compares in kebab-slug space, where any separator in the model's echo has already become `-`.
- The runtime create paths carry the field through their settings slice types.

## Not in this PR

- Translations for the three new strings exist only in `en.json`. The other locales fall back to the English default passed to `translate` until the locale pipeline fills them in.
- Per-project prefixes (#18103) and the folder template (#5082) are unaffected. A future branch token in the folder template can read `getBranchNameHead`.

## Verification

- New unit tests cover the separator join, the blank fallback, the unprefixed case, `getBranchNameHead`, rejecting an invalid separator, and the accepted and rejected separator tables. `src/shared/branch-prefix.test.ts` passes in full. The separator and branch-name tests in `src/main/ipc/worktree-logic.test.ts` pass (18 tests).
- The local workspace has unbuilt packages and is missing `emojibase-data`, `zod`, and React types. So the tests ran under a minimal vitest config that resolves workspace packages from source. The `sanitizeWorktreeName` cases and `first-work-branch-rename.test.ts` cannot load there, and the full typecheck and `verify-localization-catalog` cannot run. CI should run all three.
- A typecheck of the changed main and shared files found only module-resolution errors from the environment. It found no type errors in this change.

## Topology

`abbd sync` then `abbd diff` against `715efe58`: no topology change. No units came or went, no contract changed shape, and no relation between modules appeared or disappeared. The change stays inside the existing branch-naming helpers, their callers, and the Git settings pane.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
