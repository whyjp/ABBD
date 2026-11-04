---
name: abbd-lint
description: Hold ABBD's blueprint drawings to the drawing criteria with abbd lint, and improve the renderer or the layout one measured change at a time. Use for /abbd-lint, before and after any change to internal/screen or to how a drawing is laid out or routed, and when a drawing breaks a criterion or a person asks why a line turns.
argument-hint: "[view or unit ID]"
---

# ABBD lint

`<pstack>` is `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36`. Open files under it with the Read tool.

The criteria are in `docs/blueprint-svg.md`, under "Drawing criteria". Each has an ID. A hard one must hold. A target one is minimized, and its total over the fixtures has a ceiling. Read that section before you change a drawing.

## Read the breaches

- `abbd lint` checks every drawing of the repository's blueprint. It prints each breach with its view, its criterion, the boxes, lines or chips it involves, what it measures against what is allowed, and for a line that turns too often the obstacle or the reason. It ends with the totals per view and in all, and exits 1 only on a hard breach.
- `abbd lint <VIEW>` checks the views whose name starts with VIEW or holds it as a word. A unit ID such as `abbd lint MOD-0098` selects that unit's inside and its focus views.
- `abbd lint --json` prints the same as JSON, with the criteria, every breach and the totals, for a script or an agent.
- `abbd lint --root internal/screen/testdata/orca MOD-0098` lints one view of a fixture.

## The measure

The target is the steps, the measure to lower. A step is each turn past a line's second and each jog, a stretch under 24px between two turns. The user judges a drawing by the turns a reader sees, so steps come first (`docs/blueprint-svg.md`, "Staircases first"). B1 and B2 list the steps line by line.

A change may not raise any of these, the regression-forbidden measures:

- the hard breaches of any view, J1, J2, J3, O3 and R, past the debt `internal/screen/lint_test.go` records
- the totals of P1, P2, H1, H2, O1, O2 and F1
- the phantom pairs, the paths that turn more than twice, the traced bends and the climbing lines
- the weighed sum, the crossings plus 4 for each step, so a step saved may cost at most 4 crossings

`TestLintFixtures` in `internal/screen/lint_test.go` fails when any of them rises past its ceiling, and when a view breaks a hard criterion past its debt.

## Change one thing at a time

Follow `<pstack>/skills/poteto-mode/playbooks/hillclimb.md` for the loop, with this measure.

1. Measure the base. Write the fixtures' table with `ABBD_LINT_FIXTURES=<before.tsv> go test ./internal/screen -run TestLintFixtures -count=1`, and lint the views you mean to change.
2. Read the breaches of those views. Pick one cause, such as a chip that stands past its box or a step where a bus changes track, and write down the one change you expect to remove it and what you expect it to cost.
3. Make that one change.
4. Measure again into `<after.tsv>`, and compare with `python eval/lint/compare.py before <before.tsv> after <after.tsv>`. It totals both tables and lists every view that got worse.
5. Keep the change only when the steps fall, no hard breach is new, and no regression-forbidden total rises. Otherwise revert it, whatever else it improved.
6. Record the decision, kept or dropped, as one row of `eval/lint/decisions.tsv`: the hypothesis, the change, the totals before and after, and the verdict. The repository keeps the log, as `eval/render-r6/decisions.tsv` does, so the next agent reads what was tried. A dropped change's numbers go in "Known gaps" when they explain a gap.
7. Commit each kept change on its own. Lower the ceilings in `lint_test.go` to the new totals, write the new table to `eval/lint/baseline.tsv`, and say in `docs/blueprint-svg.md` what moved.

Never raise a ceiling or a debt, or loosen a criterion, to keep a change. Run `go test ./internal/screen -count=1` before each commit, since the rule checks of `fixtures_test.go` still hold. Look at the changed views in the rendered pages too. `ABBD_BLUEPRINTS=<dir> go test ./internal/screen -run TestEveryRealDrawingIsLegible -count=1` writes the fixtures' pages there. A number that falls on a drawing that reads worse means the measure counts the wrong thing, so say so instead of keeping the change.
