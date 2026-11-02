# Expose agent and workspace status colors as plugin theme tokens

Closes #36697.

## Why

Plugin app themes should be able to recolor the agent and workspace status indicators. Today a monochrome theme still shows yellow and orange dots on every working or waiting workspace card. Those are the most visible colors in the sidebar, and a theme cannot change them. `main.css` hard-codes them, and the theme schema refuses any token outside the public list.

## What changes

App themes can now set eight more tokens:

- `--agent-working`, `--agent-question`, `--agent-question-text`
- `--workspace-status-done`, `--workspace-status-review`, `--workspace-status-progress`
- `--status-success`, `--status-warning`

A theme that omits one keeps Orca's built-in color. Nothing new is derived. The `-background`, `-border` and `-text` variants of the status tokens already come from `color-mix` in `main.css`, so they follow the theme's value without being public themselves.

The renderer checks these tokens like the others. A translucent value is refused, and a failing check drops only the theme's status token, never the surface beneath it:

- `--agent-question-text` needs 4.5:1 on `--background`, since it is text.
- The agent dots and workspace statuses need 1.5:1 on `--worktree-sidebar`.
- `--status-success` and `--status-warning` need 1.5:1 on `--background`.

The 1.5:1 floor is lower than the 3:1 floor for git decorations, on purpose. Orca's own colors fall below 3:1 in light mode. The light `--agent-working` yellow measures 1.75:1 on the workspace sidebar, and the progress and done statuses about 2.1:1. A 3:1 floor would refuse a theme that copies Orca's look. The goal, as with rings, is a mark that never vanishes.

## Files

- `plugin-theme-artifact.ts` adds the tokens to `PLUGIN_APP_THEME_TOKENS`.
- `plugin-theme-token-reference-doc.ts` says what each token paints. The record is exhaustive, so a public token can't ship undocumented.
- `plugin-theme-tokens.ts` adds the contrast rules.
- `docs/reference/plugin-theme-tokens.md` is regenerated from the source, and `plugin-themes.md` lists the new checks.

## Verification

- A new test in `plugin-theme-artifact.test.ts` strictly parses a theme that sets the new tokens. Omitted ones stay unset, so the built-ins apply.
- A new test in `document-theme.test.ts` paints a black, white and lime status palette in full. It also confirms that invisible, translucent or unreadable status colors are dropped while the sidebar stays.
- Both tests fail without the change and pass with it. The theme, plugin artifact and contrast suites pass, 523 tests in total.
- An isolated `tsc` run on the three changed source files is clean.
- Caveat: in this checkout the pnpm links in `node_modules` dangle, so vitest ran with a minimal config that aliases `zod`. Ten suites under `src/shared/plugins` could not import `yaml`, `ajv`, `@noble/hashes` or `typescript-api` and did not run. `pnpm generate:plugin-schemas` could not load `esbuild`, so the reference doc was rendered with the same function it calls. CI should rerun the full suite and the generator check.

## Topology

`abbd sync` then `abbd diff` reports no topology change. No units, contracts or relations between modules came or went. The change extends a list and a rule table inside existing modules.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
