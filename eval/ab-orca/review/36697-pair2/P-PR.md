# Expose agent and workspace status colors as plugin theme tokens

Closes #36697.

## Why

Plugin app themes should be able to recolor the agent and workspace status hues. Today a monochrome theme still shows Orca's yellow working dots and orange waiting dots on every workspace card. They are the loudest color in the sidebar, and no public token reaches them.

## What changes for theme authors

These eight tokens are now public, so a theme may set them in `tokens`:

- `--agent-working`, `--agent-question`, `--agent-question-text`
- `--workspace-status-done`, `--workspace-status-review`, `--workspace-status-progress`
- `--status-success`, `--status-warning`

An omitted token keeps Orca's built-in color, exactly as before. The `-background`, `-border` and `-text` tints of `--status-success` and `--status-warning` already derive from them with `color-mix`, so they follow the theme's value with no extra tokens.

Each new token goes through the existing renderer check, which drops only the failing token and keeps the shared surface:

| Token | Checked against | Minimum |
| --- | --- | --- |
| `--agent-working`, `--agent-question`, `--workspace-status-*` | `--worktree-sidebar` | 1.5:1 |
| `--status-success`, `--status-warning` | `--background` | 1.5:1 |
| `--agent-question-text` | `--background` | 3:1 |

Values must also be opaque, like every other checked token.

## Why 1.5:1 for the hues

The 3:1 floor used for `--git-decoration-*` would refuse Orca's own defaults. Orca's light working yellow is about 1.8:1 on the light workspace sidebar, and its light done and progress hues are about 2.1:1. A theme that copied Orca's light palette would lose them. These hues mark state next to a label, so the check only has to keep them from vanishing. That matches the reasoning behind the ring check. `--agent-question-text` is legible text, so it keeps the 3:1 status-text floor, which Orca's own values clear in both modes (about 5.2:1 light and 11.6:1 dark).

The ratios above come from the sRGB equivalents of Tailwind v4's oklch colors. Treat them as approximate.

## Known limit

Native chat re-declares `--status-success` and `--status-warning` per chat scheme, as it already does for `--git-decoration-*` and the diff tokens. Inside chat, those two keep Orca's colors. The themes page now says so. Making chat follow the theme is a separate change.

## Compatibility

The public token list is additive. Older Orca clients skip unknown tokens in tolerant parsing, so a theme that sets these still loads there and simply keeps the built-in hues. `pnpm validate:plugin` on an older CLI would refuse them, as with any newly added token.

## Changes

- `plugin-theme-artifact.ts` adds the tokens to `PLUGIN_APP_THEME_TOKENS`.
- `plugin-theme-token-reference-doc.ts` says what each token paints. The `Record` keyed by token makes that required.
- `plugin-theme-tokens.ts` adds the contrast rules from one token-to-surface table.
- `docs/reference/plugin-theme-tokens.md` is regenerated from source, and `docs/reference/plugin-themes.md` documents the checks and fallback.
- `document-theme.test.ts` covers a theme with visible and vanishing status hues.

## Verification

- `document-theme.test.ts` and `plugin-theme-artifact.test.ts` pass (41 tests). The new test fails when the contrast rules are reverted.
- `oxlint` and `oxfmt --check` pass on the changed files.
- `tsc -p config/tsconfig.tc.web.json` reports nothing in the changed source files. Its remaining errors are unresolved modules (`react`, `vitest`, `sonner`) from this environment's install.
- `plugin-theme-tokens.md` was rendered with `renderPluginThemeTokenReference()`. Its diff is exactly the eight new rows.
- This environment's workspace packages are unbuilt and some `node_modules` links are broken, so `pnpm test` and `pnpm generate:plugin-schemas` could not run as-is. The tests ran under a minimal vitest config with the same transform settings. CI should rerun the full gates.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
