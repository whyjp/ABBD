## Why

Teams whose branch convention joins the prefix with a dash (`jd-feat-login-form`) could not use the branch prefix setting. The join was always a slash, so they had to edit the branch field on every new workspace or pass `--branch` from the CLI. Closes #36652.

## What changes for users

Settings > Git > Branch Prefix now has a small separator field next to the prefix input. It defaults to `/`, so existing setups produce the same branch names as before. With prefix `jd` and separator `-`, typing `feat: login form` creates the branch `jd-feat-login-form`. The preview under the field ("Branches will be named jd-feature") uses the separator. A separator Git cannot store, such as `..`, `//` or `~`, shows an error there. Creating a worktree with such a separator fails with a message pointing at Settings > Git.

An empty or whitespace-only separator falls back to `/`. Settings saved before this change have no separator and keep the slash join.

## How

- `src/shared/branch-prefix.ts` gains `branchPrefixSeparator` on `BranchPrefixSettings` and `normalizeBranchPrefixSeparator`, which maps a missing or blank value to `/`. It also gains `getBranchPrefixSeparatorIssue` and its assert. The Git ref rules that `getBranchPrefixIssue` applied inline move into `violatesRefFormat`, which both checks share. The separator is checked where it lands, as `a<sep>a`, so `-`, `_` and `.` pass while `..`, `//` and `/.` fail. `//` was added to the shared rules. It cannot change prefix results, because prefix normalization already collapses slash runs.
- `src/main/ipc/worktree-branch-name.ts` adds `getConfiguredBranchHead`, the prefix plus separator that every prefixed branch starts with. `computeBranchName` joins with it. `computeValidatedBranchName` validates the separator after the prefix. A git-username prefix that fails validation still skips prefixing entirely, so the separator only matters when a prefix is applied.
- `src/main/agent-hooks/first-work-branch-rename.ts` found branch leaves by slicing after the last `/` in two places. With a dash separator both kept the prefix. First, the eligibility check saw `jd-Nautilus` rather than the creature name `Nautilus`, so auto-rename from first work never fired. Second, the renamed branch's display name became "Jd fix auth" and its folder `jd-fix-auth`. The check now accepts either the slash leaf or the name after the configured head. The slash leaf keeps branches created before a separator change eligible. The renamed leaf is the branch minus the head. Settings and the git username are now read before the eligibility check, where they used to be read after generation. `stripConfiguredBranchPrefix` needed no change. It strips the prefix from the model's kebab-case slug, where every separator already reads as `-`.
- `GlobalSettings` gains `branchPrefixSeparator` with default `/`. The runtime settings contracts that carry `branchPrefixCustom` carry it too.
- `GitPane.tsx` moves the existing draft-and-echo logic of the prefix input into a `useSettingDraft` hook, and the new separator input reuses it.

Known gaps, left for follow-ups:

- The draft title for a new pull request comes from the branch leaf after the last `/` (`create-review-draft-title.ts`). With a dash separator it keeps the prefix ("Jd feat login form"). The field is editable. Fixing it needs the configured head in the renderer, including the git username.
- The separator is checked on its own, between two plain characters. Some contrived prefix and separator pairs pass both checks and Git still rejects the joined name, for example prefix `foo.lo` with separator `k/`. Git stays authoritative at creation, and the error names the branch.
- There is no way to choose an empty separator. A blank field means `/`.

Out of scope: per-project prefixes (#18103) and the folder template (#5082). A branch token in that template would pick the separator up through `computeBranchName`.

## Verification

- `src/shared/branch-prefix.test.ts` and `src/shared/branch-name-from-work.test.ts` pass (67 tests). They include new cases for accepted separators (`/ - _ . --`, empty, missing) and rejected ones (`.. // /. .lock/ @{ ~ : space \`).
- The new `computeBranchName` and `computeValidatedBranchName` cases in `src/main/ipc/worktree-logic.test.ts` pass: dash join, blank-separator fallback, no-prefix, `jd/` + `-` giving `jd-feature`, and rejection of `..`. The other failures in that file are pre-existing and environmental. Every one goes through `sanitizeWorktreeName`, which cannot load `emojibase-data` in this checkout.
- `src/main/agent-hooks/first-work-branch-rename.test.ts` passes (41 tests). Running it here needed a throwaway vitest config outside the repo that resolves packages from the pnpm store. It has a new case: current branch `jd-Nautilus`, head `jd-`. The branch is renamed to `jd-fix-auth`, the display name is "Fix auth" and the folder leaf is `fix-auth`. With the head-stripped leaf removed from the eligibility check, that case fails, so the test guards the fix. The module mock gains `getConfiguredBranchHead`, which defaults to the same `you/` behaviour as the existing mocks.
- The checkout has unbuilt workspace packages and missing React types, so `tsc` cannot load the project configs. A standalone strict typecheck of the changed files reports no errors in the changed `.ts` sources. `oxlint` and `oxfmt --check` are clean on every changed file.
- The settings UI was not exercised in a running app.

## Topology

`abbd diff`: no topology change. No units, contracts or relations between modules came or went. The change stays inside existing modules and their current couplings. `.abbd/` was re-synced and is committed with the code.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
