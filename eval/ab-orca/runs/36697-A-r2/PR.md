Expose agent and workspace status colors as plugin theme tokens

## Why

A plugin app theme should be able to recolor the agent and workspace status indicators. Today a monochrome theme still shows yellow working dots and orange question icons on every busy workspace card, because those colors are not public theme tokens. Closes #36697.

## What changes

Eight variables join the public app theme tokens:

- `--agent-working`, `--agent-question`, `--agent-question-text`
- `--workspace-status-done`, `--workspace-status-review`, `--workspace-status-progress`
- `--status-success`, `--status-warning`

A theme that omits one keeps today's `main.css` value in both modes, so no existing theme changes. `main.css` already derives `--status-*-background`, `--status-*-border` and `--status-warning-text` from their hue, so those tints follow a theme's hue with no extra token.

Each new token goes through the same contrast check as the others, and a failing token is dropped alone (it keeps Orca's color; the shared surface stays):

| Token | Checked against | Floor |
| --- | --- | --- |
| `--agent-working`, `--agent-question`, `--workspace-status-*` | `--worktree-sidebar` | 1.5:1, opaque |
| `--status-warning` (as an icon hue) | `--background` | 1.5:1, opaque |
| `--agent-question-text` | `--background` | 4.5:1 |
| `--status-success` | its own 10% tint over `--background` | 4.5:1 |
| `--status-warning-text` (derived), dropping `--status-warning` | its 10% tint over `--background` | 4.5:1 |

The status hues paint dots, rings and icons, not text, so they get a floor that only stops a status from vanishing. Orca's own light `--agent-working` (Tailwind yellow-500) is 1.76:1 on the light workspace sidebar, so a 3:1 floor would refuse Orca's own color. Every built-in value clears its floor in both modes. The lowest margins are 1.76:1 for the light working dot, 4.96:1 for success text on its tint, and 4.76:1 for light warning text on its tint. These were computed from the `main.css` hex values and Tailwind v4's oklch values.

To check text on the status tints, `plugin-theme-css-fallbacks.ts` now mirrors the `main.css` rules for `--status-success-background`, `--status-warning-background` and `--status-warning-text`.

`docs/reference/plugin-theme-tokens.md` is regenerated from the token list, and `docs/reference/plugin-themes.md` lists the new checks.

## Known limit

Inside a native chat forced to the opposite light or dark scheme, `main.css` re-declares `--status-success` and `--status-warning` on that subtree. A theme's value doesn't reach there. Theme diff and git decoration colors already behave the same way there.

## Verification

- New test in `document-theme.test.ts`. A black and white theme with a lime accent paints all eight tokens. A near-black progress dot, a translucent done dot, unreadable question text and a near-black warning are each dropped alone. When I broke the compiled indicator floor or the question-text rule, the test failed both times.
- `document-theme.test.ts` (13 tests) and `plugin-theme-artifact.test.ts` (28 tests) pass. Vitest can't load the project tsconfig in this checkout because `@electron-toolkit/tsconfig` is not linked. So the files were compiled with `tsc` and run under a minimal vitest-compatible shim.
- `tsc -p config/tsconfig.web.json` reports no errors in the touched files beyond the pre-existing unresolved `zod` and `vitest` modules. The `TOKEN_ROLES` record is complete, so every new token documents what it paints.
- `oxlint` and `oxfmt --check` are clean on the touched files.

## Topology difference (abbd diff)

```
topology since 715efe584e47:
no topology change
```

No units, contracts or module relations came or went. The change stays inside the plugin theme artifact and renderer theme units. `.abbd/` is re-synced only to the new revision.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
