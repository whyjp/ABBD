# Blueprint drawing rules

A person should understand and judge an architecture from `.abbd/blueprint/index.html` quickly. What makes a blueprint hard to read is not the number of relations. It is lines that run the same way but are drawn apart, lines that run against the eye, and views that try to show too much at once. So the blueprint follows three principles, and every rule below comes from one of them.

- **Decomposition.** The blueprint is a tree of views, all drawn the same way. The overview shows the top level. Every unit that holds others has a view of its children, with that unit's contracts on the frame. A box that holds units opens its view. Each view stays small because the state is decomposed, never because relations are left out.
- **Convergence.** Lines that go to the same place merge midway into one trunk, split only near their ends, and carry a count. A reader follows one trunk instead of a braid.
- **Flow.** A drawing reads top to bottom and left to right. What calls or uses sits above and to the left. What it calls or uses sits below and to the right. Lines point down or right. The few that must climb are drawn dashed.

`internal/screen/legible_test.go` checks every rule on the rendered SVG, which is what a reader sees. `internal/screen/fixtures_test.go` applies the checks to every view of three real states: this repository, ripgrep and Orca. The rule numbers below match the checker's messages.

## Decomposition

1. **At most 16 boxes per view.** Top-level contracts don't count, because they are drawn as chips. A level that holds more is split into groups (see Supplying groups).
2. **No relation is dropped.** Every live relation between present units appears in some view. It can be a line, a bundle, a contract chip, the list of a reader's couplings under a module's inside, or the list of relations between two contracts. Each wire and chip records the stored relations it stands for, and the test compares those records with the state.
3. **Width budget.** The overview is at most 1320px wide and a unit's view at most 920px. That is what a 1440px screen shows of each. A layer that is too wide wraps into rows. A frame never grows past the budget: chips that do not fit on their side move to the other side, and those that fit on neither fold into one `+N` chip that names them all. On a 390px screen the drawing scrolls inside its box and the page itself never scrolls sideways.

A cycle that is only part of a view folds into one box, `서로 쓰는 N개`, and that box opens a view of the cycle. A cycle folds when it has at least 3 units and holds at most two thirds of the view's boxes, so the view keeps showing mostly real structure. The overview is never folded. Folding is a unit of the drawing only: the store never holds it, and the unit catalogue still lists each unit under its real parent. A cycle that fills most of its view cannot fold, and rule 13 governs it instead.

## Convergence

4. **One bundle per place and direction.** Lines of one kind that leave one edge of a box share a departure bus: one stub out of the box and one track in the gap below it. Lines of one kind that reach one edge of a box share an arrival bus. A line joins the trunk in the first gap it crosses and rides it through every row. The trunk ends on a track in the gap above the box, and a drop goes into each chip. A box's lines bundle by the box, whichever of its chips they use. A bundle's count shows as `×N`. The checker counts lines that leave or reach a box edge beside the bundle of their kind, and expects 0 of each.
5. **At most 4 lines per channel.** Vertical lines closer than 12px count as one channel. A fifth line takes a lane past a wider gap.
6. **Parallel lines stay apart.** Two lines never run closer than 5px to each other for more than 24px, except within 26px of a shared end.
7. **At most 2 crossings per drawn line, counted after bundling.** A drawn line is counted where it arrives: a bus is one line however many pieces draw it. Lines run on a grid, so every crossing is at a right angle.

## Legibility

8. **Labels are whole.** A name wraps to two lines and widens its box. It is cut only past the hard width (280px for a module box, 220px for a unit box or chip), and then the tooltip carries the full name. No label is wider than its box.
9. **Nothing overlaps.** No two boxes or chips overlap, and no line passes through a box it does not join.
10. **A port sits on the side its lines flow from.** On a unit's frame, a contract the unit provides and a caller from outside sit on the top edge. What the unit uses sits on the bottom edge, even a contract it also provides. While the view is layered, the frame's top and bottom take part as two extra nodes, so a unit that meets a port on top rises toward it and one that uses a port below sinks toward it. A filled chip is a contract the unit provides and an outlined chip is one it uses.
11. **A line's colour shows its kind, not its origin.** A line a pstack reader found (판독) starts with a hollow purple dot. A line a human stated (확인) starts with a filled green dot. A line with no dot was extracted from the code (추출). The dot's tooltip says what the word means. 판독 means a reader read the relation, ABBD checks on every sync that the quoted code still exists, and no human has confirmed it.
12. **Unwired contracts are grouped.** A contract that no inside unit is recorded as meeting is not spread along the frame. It is listed under the drawing as a chip group, with a shared first word shown once (`abbd`: diff, evidence, ...).

## Flow

13. **Lines read down and right.** The checker takes every line where it arrives at a box or a chip. The line runs forward when its end lies below its start, or level with it and to the right. A line that provides a contract reads from the contract to the unit that meets it, so it counts the other way round. A line that runs back, up or left, must be dashed, and the checker expects 0 back lines drawn solid. At most one line, or 20% of the arriving lines if that is more, may run back.

The layout keeps the rule by layering what uses what: a unit sits above everything it uses, and a cycle's edges are reversed for the layering only (Eades, Lin and Smyth), so the fewest lines climb. Folding a cycle removes its climbing lines from the view that holds it.

## What the overview draws

Every view is deterministic, so the same state renders as identical bytes. Every view, the overview included, draws every live relation whatever its origin. A reader's coupling (판독) carries its hollow dot there too, because sync re-checks its quote on every run. A dead coupling, whose quote is gone, is drawn nowhere. The overview used to leave reader's couplings out, and on Orca (evaluation round 5) that erased every real platform boundary: the phone-to-relay WebSocket, the E2EE handshake, the push gateway and the native helper sockets were all found by readers, mostly on units inside a module.

So a relation from anywhere inside a top-level unit to another top-level unit is drawn on the overview as that unit's own: it crosses the boundary the overview shows. A unit inside a module is never drawn on the overview, and a change that stays inside a module never moves it.

## Supplying groups

When the top level holds more than 16 modules, or a module holds more than 16 units, the state needs a grouping level. The grouping is stored as units (MOD) and `parent` links. Apart from folding a cycle, the renderer never invents a grouping at render time.

- **The reader supplies it (preferred).** The supplement step of `abbd-setup` asks the pstack readers which parts of the system's behaviour the units fall into. Each part is added as a MOD with `evidence`, and each member gets that part as its `parent`, with `parent_evidence`. `abbd record parent ID PARENT --evidence ...` places a single unit later, and `--source` records a human's statement instead. Such a parent is INFERRED or USER_STATED, and sync keeps it unless the extraction script reports a parent for that unit. A reader's parent lasts only as long as its evidence. A parent that would close a cycle is refused.
- **Sync's fallback (last resort).** If nobody grouped a crowded level, `abbd sync` groups the units the script placed by the shallowest directory that splits them. It stores these groups as MOD units whose anchor is that directory with the symbol `(group)`. It never moves a unit a reader or a human placed. It recomputes the groups on every sync and reuses IDs by directory.

## Known gaps

Twelve views of 51 still break a rule. Their current measurements are recorded in `fixtures_test.go` as debt. They are allowed to improve but never to get worse, and the test fails once a view keeps every rule until it is taken off the list.

- **Orca, cycles that fill their view.** Mobile app (12 of 12 units in one cycle), desktop main process (13 of 15), the runtime (10 of 13) and the renderer (8 of 9) cannot fold. Their climbing lines are dashed, but there are more than rule 13 allows: 11, 10, 7 and 6. Desktop main and the runtime also break rules 5, 6 and 7 (117 and 77 crossings). The fold of `src/`'s five desktop modules breaks rules 4 and 6, and a second `src/` view and the fold of `src/shared/` have 2 and 3 back lines. The fix is a reader's grouping by behaviour.
- **This repository's CLI inside.** Its units use more than fits on the bottom edge, so 5 used ports move to the top and their lines climb.
- **Near misses.** The self and ripgrep overviews and Orca's `native/` have one or two pairs of lines too close. This repository's `internal/model` view has 7 crossings for 3 lines: seven callers on top feed two trunks into three units.
