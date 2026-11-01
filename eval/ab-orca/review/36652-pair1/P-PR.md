# Add a configurable separator between the branch prefix and the workspace name

Closes #36652

## Why

Teams whose branch convention uses a dash (`jd-feat-login-form` instead of `jd/feat-login-form`) can't use the branch prefix setting today. `computeBranchName` always joins the prefix with `/`, so they edit the branch field on every new workspace or pass `--branch` from the CLI. Putting the dash in the prefix doesn't help either. `jd-` still gets a slash appended and becomes `jd-/feat-login-form`.

## What changes for users

Settings > Git > Branch Prefix now has a small separator field next to the prefix input. It defaults to `/`, so existing setups produce the same branch names as before. With prefix `jd` and separator `-`, the workspace name `feat: login form` produces `jd-feat-login-form`.

The live feedback line shows the real example (`jd-feature`). It turns red if the separator is empty or something Git would reject: spaces, `..`, `//`, `@{`, `/.`, or `~ ^ : ? * [ \`. A separator that sits inside a ref is checked with the same `getBranchPrefixIssue` rules as the prefix.

## Implementation

- `src/shared/branch-prefix.ts` owns the setting's rules. It adds `branchPrefixSeparator` to `BranchPrefixSettings`, along with `DEFAULT_BRANCH_PREFIX_SEPARATOR`, `getBranchPrefixSeparatorIssue`, `resolveBranchPrefixSeparator` and `assertBranchPrefixSeparatorValid`. The issue check validates `a<sep>a` through `getBranchPrefixIssue`, so the separator obeys the same Git rules as the prefix. It also rejects empty values and `//`, because prefix normalization would otherwise hide them.
- `src/main/ipc/worktree-branch-name.ts` joins with the separator. `computeValidatedBranchName`, used by workspace creation, throws a clear "update it in Settings → Git" error for an invalid separator, matching how an invalid custom prefix is handled. `computeBranchName`, used by background auto-rename, falls back to `/` instead of throwing. A background task shouldn't fail on a setting the user is still typing. `computeValidatedBranchName` now runs its `.lock` rewrite on the joined branch too, because a `.` separator plus a leaf named `lock` would otherwise form `jd.lock`.
- The settings feedback shows the separator error only when a prefix will actually apply, so it doesn't contradict what create does.
- Auto-rename from first work (`first-work-branch-rename.ts`) previously found the creature leaf with `lastIndexOf('/')`. With a `-` separator, `jd-otter` was never recognized as auto-generated, so it was never renamed. The new `getBranchLeaf` strips the configured prefix plus separator, and falls back to the last path segment for branches built some other way. The git username lookup moved a few lines earlier so the leaf check can use it.
- Stripping a prefix the model echoed back (`stripConfiguredBranchPrefix`) needed no change. It compares kebab slugs, and both the prefix and the generated slug are kebab-cased whatever the separator is.
- `GlobalSettings.branchPrefixSeparator` defaults to `/` in `buildDefaultSettings`. Persisted profiles get the default through the existing defaults merge on load. The runtime-store contract and the runtime create paths type the field as optional, so a settings object from an older server without it resolves to `/`. The renderer applies the same fallback.
- `GitPane` had an inline draft/echo-suppression pattern for the prefix input. It now lives in `setting-text-draft.ts` (`useSettingTextDraft`) and serves both the prefix and separator inputs.
- New strings and a `separator` search keyword are added to `en.json`. Other locales fall back to English until translated.

## Not in scope

- A per-project prefix and separator (#18103). The separator is global, like the prefix.
- Collapsing a prefix that already ends in the separator (`jd-` plus `-` gives `jd--feat`). That's valid in Git, and the feedback line shows it.
- Mobile has no branch prefix settings UI, so there is nothing to mirror.

## Known limitations

- A new client connected to an older server shows the separator control, but the old server ignores the field and keeps creating `prefix/leaf`. This is the usual cost of a new setting under independent client and server updates. Gating the control on a server capability would need a capability flag that doesn't exist yet.
- Auto-rename recognizes a creature branch by stripping the current prefix and separator. A workspace created as `jd-otter` won't auto-rename if the prefix or separator changes before its first agent turn, because the fallback only splits on `/`. Guessing separators to fix this would risk renaming user-named branches.

## Testing

- `src/shared/branch-prefix.test.ts` covers accepted separators (`/ - _ .`) and rejected ones (empty, space, `..`, `//`, `/.`, `:`, `~`, `@{`).
- `src/main/ipc/worktree-logic.test.ts` covers joining with `-`, the `.lock` rewrite with a `.` separator, falling back to `/` for an invalid separator in the nonthrowing path, throwing in the validated path, and `getBranchLeaf` both with and without the configured prefix.
- `src/main/agent-hooks/first-work-branch-rename.test.ts` covers end-to-end auto-rename of `you-Nautilus` to `you-fix-auth` with a `-` separator. I checked that this test fails with the old `lastIndexOf('/')` leaf logic. The two rename suites now partially mock `worktree-branch-name` so the real `getBranchLeaf` runs.
- The local environment has unbuilt workspace packages and broken `node_modules` links, so `pnpm test` and `pnpm tc` could not run. I ran the affected suites (branch-prefix, worktree-logic, both first-work rename suites, persistence settings, runtime worktree create) through a local vitest harness. 499 tests passed. The 18 failures all call `sanitizeWorktreeName`, which `require`s the missing `emojibase-data`. They fail before reaching the code this PR changes.
- `oxlint` and `oxfmt --check` pass on all changed files. CI should still run `pnpm tc`, because typechecking couldn't resolve `react`/`node` types locally.
- Not done: a rendered check of the Settings > Git row (needs a built Electron app).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
