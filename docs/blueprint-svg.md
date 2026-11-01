# Blueprint drawing rules

A person should understand and judge an architecture from `.abbd/blueprint/index.html` quickly. What makes a blueprint hard to read is not the number of relations. It is lines that run the same way but are drawn apart, lines that run against the eye or out of their way, and views that try to show too much at once. The bar is a diagram a person polished by hand in a tool like draw.io, not a dump of extracted edges. So the blueprint follows three principles, and every rule below comes from one of them.

- **Decomposition.** The blueprint is a tree of views, all drawn the same way. The overview shows the top level. Every unit that holds others has a view of its children, with that unit's contracts on the frame. A box that holds units opens its view. Each view stays small because the state is decomposed, never because relations are left out.
- **Convergence.** Lines that come from one place travel together as one trunk and branch only near their ends. Lines that go to one place, or to boxes side by side that the same places reach, merge midway into one trunk and split only in the gap above them. Every trunk carries a count. A reader follows one trunk instead of a braid.
- **Flow.** A drawing reads top to bottom and left to right. What calls or uses sits above and to the left. What it calls or uses sits below and to the right. Lines point down or right, and run on the grid in the spaces between boxes, the short way. The few that must climb are drawn dashed.

`internal/screen/legible_test.go` checks every rule on the rendered SVG, which is what a reader sees. `internal/screen/fixtures_test.go` applies the checks to every view of four real states: this repository, ripgrep, Orca and pstack's skill tree. The rule numbers below match the checker's messages.

## Decomposition

1. **At most 16 boxes per view.** Top-level contracts don't count, because they are drawn as chips. A level that holds more is split into groups (see Supplying groups).
2. **No relation is dropped.** Every live relation between present units appears in some view. It can be a line, a bundle, a contract chip, the list of a reader's couplings under a module's inside, or the list of relations between two contracts. Each wire and chip records the stored relations it stands for, and the test compares those records with the state.
3. **Width budget.** The overview is at most 1320px wide and a unit's view at most 920px. That is what a 1440px screen shows of each. A layer that is too wide wraps into rows. A module on the overview whose chips need more than 480px wraps them into up to three rows inside its box instead of widening, so lines can pass beside it; the last row folds what still does not fit into one `+N` chip that names them all. A frame never grows past the budget: chips that do not fit on their side move to the other side, and those that fit on neither fold into one `+N` chip. On a 390px screen the drawing scrolls inside its box and the page itself never scrolls sideways.

A cycle that is only part of a view folds into one box, `서로 쓰는 N개`, and that box opens a view of the cycle. A cycle folds when it has at least 3 units and holds at most two thirds of the view's boxes, so the view keeps showing mostly real structure. The overview is never folded. Folding is a unit of the drawing only: the store never holds it, and the unit catalogue still lists each unit under its real parent. A cycle that fills most of its view cannot fold, and rule 13 governs it instead.

## Convergence

4. **One bundle per place and direction.** Lines of one kind that leave one edge of a box share a departure bus: one stub out of the box, then one trunk down through every row at least two of its lines still pass, heading row by row toward the places they reach. A line leaves the trunk in the gap above the first row it would pass alone. Lines of one kind that reach one edge of a box share an arrival bus: one trunk through every row they pass, one track in the gap above the box, and a drop into each chip. A line joins that trunk once it is off its departure trunk, so lines from different places merge midway and split only at the end; a line that rides its departure trunk into its last gap steps onto the arrival track where the trunk crosses that gap. Boxes side by side in one row that are reached mostly from the same places, such as a layer of widely used libraries, share one arrival bus that splits only in the gap above them. Each drop shows how many of the bus's lines reach that box as `×N`. The checker counts lines that leave or reach a box edge beside the bundle of their kind, and expects 0 of each.
5. **At most 4 lines per channel.** Vertical lines closer than 12px count as one channel. A fifth line takes a lane past a wider gap.
6. **Parallel lines stay apart.** Two lines never run closer than 5px to each other for more than 24px, except within 26px of a shared end.
7. **At most 2 crossings per drawn line, counted after bundling.** A drawn line is counted where it arrives: a bus is one line however many pieces draw it. Lines run on a grid, so every crossing is at a right angle.
14. **No braids.** Lines of different bundles may run side by side within one channel for at most 200px per drawn line, summed over every pair, away from their ends. Lines that run that far together belong on one trunk.

Two modules on the overview that depend on each other directly are joined by one line with a head at each end, drawn the way the eye reads, instead of two lines of which one would climb. Its tooltip names both directions and it stands for both relations.

## Legibility

8. **Labels are whole.** A name wraps to two lines and widens its box. It is cut only past the hard width (280px for a module box, 220px for a unit box or chip), and then the tooltip carries the full name. No label is wider than its box.
9. **Nothing overlaps.** No two boxes or chips overlap, and no line passes through a box it does not join.
10. **A port sits on the side its lines flow from.** On a unit's frame, a contract the unit provides and a caller from outside sit on the top edge. What the unit uses sits on the bottom edge, even a contract it also provides. While the view is layered, the frame's top and bottom take part as two extra nodes, so a unit that meets a port on top rises toward it and one that uses a port below sinks toward it. A filled chip is a contract the unit provides and an outlined chip is one it uses.
11. **A line's colour shows its kind, not its origin.** A line a pstack reader found (판독) starts with a hollow purple dot. A line a human stated (확인) starts with a filled green dot. A line with no dot was extracted from the code (추출). The dot's tooltip says what the word means. 판독 means a reader read the relation, ABBD checks on every sync that the quoted code still exists, and no human has confirmed it.
12. **Unwired contracts are grouped.** A contract that no inside unit is recorded as meeting is not spread along the frame. It is listed under the drawing as a chip group, with a shared first word shown once (`abbd`: diff, evidence, ...).

## Flow

13. **Lines read down and right.** The checker takes every line where it arrives at a box or a chip. The line runs forward when its end lies below its start, or level with it and to the right. A line that provides a contract reads from the contract to the unit that meets it, so it counts the other way round. A line that runs back, up or left, must be dashed, and the checker expects 0 back lines drawn solid. At most one line, or 20% of the arriving lines if that is more, may run back.
15. **Lines take the short way.** No line runs more than 240px longer than the shortest grid path between its ends. A line that passes a row between two boxes takes a lane in that space. A line that passes beside a row goes straight down where it is headed, a lane outward from any line already there, rather than out to the row's end box and back.

The layout keeps rule 13 by layering what uses what: a unit sits above everything it uses, and a cycle's edges are reversed for the layering only (Eades, Lin and Smyth), so the fewest lines climb. Folding a cycle removes its climbing lines from the view that holds it. Within a layer, boxes are ordered by the mean position of what they are joined to, so lines cross as little as they can.

## What the overview draws

Every view is deterministic, so the same state renders as identical bytes. Every view, the overview included, draws every live relation whatever its origin. A reader's coupling (판독) carries its hollow dot there too, because sync re-checks its quote on every run. A dead coupling, whose quote is gone, is drawn nowhere. The overview used to leave reader's couplings out, and on Orca (evaluation round 5) that erased every real platform boundary: the phone-to-relay WebSocket, the E2EE handshake, the push gateway and the native helper sockets were all found by readers, mostly on units inside a module.

So a relation from anywhere inside a top-level unit to another top-level unit is drawn on the overview as that unit's own: it crosses the boundary the overview shows. A unit inside a module is never drawn on the overview, and a change that stays inside a module never moves it.

## Supplying groups

When the top level holds more than 16 modules, or a module holds more than 16 units, the state needs a grouping level. The grouping is stored as units (MOD) and `parent` links. Apart from folding a cycle, the renderer never invents a grouping at render time.

- **The reader supplies it (preferred).** The supplement step of `abbd-setup` asks the pstack readers which parts of the system's behaviour the units fall into. Each part is added as a MOD with `evidence`, and each member gets that part as its `parent`, with `parent_evidence`. `abbd record parent ID PARENT --evidence ...` places a single unit later, and `--source` records a human's statement instead. Such a parent is INFERRED or USER_STATED, and sync keeps it unless the extraction script reports a parent for that unit. A reader's parent lasts only as long as its evidence. A parent that would close a cycle is refused.
- **Sync's fallback (last resort).** If nobody grouped a crowded level, `abbd sync` groups the units the script placed by the shallowest directory that splits them. It stores these groups as MOD units whose anchor is that directory with the symbol `(group)`. It never moves a unit a reader or a human placed. It recomputes the groups on every sync and reuses IDs by directory.

## Known gaps

Sixteen views of 73 still break a rule. Their current measurements are recorded in `fixtures_test.go` as debt. They are allowed to improve but never to get worse, and the test fails once a view keeps every rule until it is taken off the list.

- **Cycles that fill their view.** Orca's mobile app (12 of 12 units in one cycle), desktop main process (13 of 15), the runtime (10 of 13) and the renderer (8 of 9) cannot fold. Their climbing lines are dashed, but there are more than rule 13 allows: 11, 10, 7 and 6. Desktop main also has 73 crossings and the runtime a channel of 6. The fold of `src/`'s five desktop modules has 2 split arrivals, and a second `src/` view and the fold of `src/shared/` have 2 and 3 back lines; that fold also has one line 252px out of its way. pstack's principles inside has 3 back lines. The fix is a reader's grouping by behaviour.
- **This repository's insides.** The CLI, `/abbd`, `internal/model` and `internal/topology` views have 5, 5, 3 and 2 climbing lines, more than rule 13 allows. In the CLI, its units use more than fits on the bottom edge, so used ports move to the top and their lines climb.
- **pstack's insides.** `setup and help` and the front door have 4 split arrivals each, `writing and prose` 746px of braid for 3 lines, and `verification principles` one cut label.
- **Dense overviews.** pstack's twelve skill families use each other in 42 of 66 pairs. The overview keeps every rule, but it is still a busy picture; the remaining fix is decomposition, a reader's grouping of the families, not a drawing trick.
