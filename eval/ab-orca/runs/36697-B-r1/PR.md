# Expose agent and workspace status colors as plugin theme tokens

## Why

Plugin app themes should be able to recolor the agent and workspace status dots. A monochrome theme today still shows yellow working dots and orange question dots on every busy workspace card, because those colors are hard-coded in `main.css` and the public token list refuses them. Closes #36697.

## What changes

A theme's `tokens` may now set eight more keys:

- `--agent-working`, `--agent-question`, `--agent-question-text`
- `--workspace-status-done`, `--workspace-status-review`, `--workspace-status-progress`
- `--status-success`, `--status-warning`

An omitted key keeps Orca's built-in value. `main.css` already derives `--status-*-background`, `--status-*-border` and `--status-warning-text` from the base hue with `color-mix`, so those tints follow a theme's value without becoming public tokens themselves.

The new keys go through the same contrast check as every other plugin token. A translucent value, or one that fails its pair, is dropped and the built-in color paints instead:

| Token | Checked against | Minimum |
| --- | --- | --- |
| `--agent-working`, `--agent-question`, `--workspace-status-*` | `--worktree-sidebar` | 3:1 |
| `--status-success`, `--status-warning` | `--background` | 3:1 |
| `--agent-question-text` | `--background` | 4.5:1 |

The 3:1 floor matches the existing git-decoration status hues. A failure drops only the status color, never the theme's surface. The git-decoration rules now use the same small `statusHue` rule builder.

`docs/reference/plugin-theme-tokens.md` is regenerated from `renderPluginThemeTokenReference`.

## Known limit

Inside a native chat view, `.native-chat-appearance[data-native-chat-scheme]` redeclares `--status-success` and `--status-warning` for the chat's own scheme. A theme's values for those two do not reach inside chat. The git-decoration and diff tokens already behave this way.

## Verification

- `oxlint` and `oxfmt --check` pass on every changed file.
- `safePluginThemeTokens` was run on light and dark inputs. A lime `--agent-working` stays on a dark sidebar and is dropped on a near-white one. A `#303030` question dot on a `#202020` sidebar is dropped. A translucent review color is dropped. A black-and-white light theme with a lime accent keeps all of its status colors.
- `parsePluginAppThemeArtifact(..., 'strict')` accepts the new keys and still refuses a derived key such as `--status-warning-text`.
- `document-theme.test.ts` gains one assertion for the workspace-sidebar case. Vitest could not run in the authoring environment (the node_modules links do not resolve), so CI is the first run of that test.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
