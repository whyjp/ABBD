# Blueprint drawing rules

A person should understand and judge an architecture from `.abbd/blueprint/index.html` quickly. What makes a blueprint hard to read is not the number of relations. It is lines that run in the same direction but are drawn apart, and views that try to show too much at once. So the blueprint follows two principles, and every rule below comes from one of them.

- **Decomposition.** The blueprint is a tree of views, all drawn the same way. The overview shows the top level. Every unit that holds others has a view of its children, with that unit's contracts on the frame. A box that holds units opens its view. Each view stays small because the state is decomposed, never because relations are left out.
- **Convergence.** Lines that go the same way share a trunk, branch off near their ends, and carry a count. A reader follows one trunk instead of a braid.

`internal/screen/legible_test.go` checks every rule on the rendered SVG, which is what a reader sees. `internal/screen/fixtures_test.go` applies the checks to every view of three real states: this repository, ripgrep and Orca. The rule numbers below match the checker's messages.

## Decomposition

1. **At most 16 boxes per view.** Top-level contracts don't count, because they are drawn as chips. A level that holds more is split into groups (see Supplying groups).
2. **No relation is dropped.** Every live relation between present units appears in some view. It can be a line, a bundle, a contract chip, the list of a reader's couplings that the overview leaves out, or the list of relations between two contracts. Each wire and chip records the stored relations it stands for, and the test compares those records with the state.
3. **Width budget.** The overview is at most 1320px wide and a unit's view at most 920px. That is what a 1440px screen shows of each. A layer that is too wide wraps into rows. Frame ports that don't fit on their side move to the other side. On a 390px screen the drawing scrolls inside its box and the page itself never scrolls sideways.

## Convergence

4. **One bundle per place and direction.** Lines of one kind that leave one edge of a box are drawn as one bundle. The checker counts split bundles and expects 0. Lines of one kind that go into one place merge into a trunk when that crosses fewer lines. A bundle's count shows as `×N`.
5. **At most 4 lines per channel.** Vertical lines closer than 12px count as one channel. A fifth line takes a lane past a wider gap.
6. **Parallel lines stay apart.** Two lines never run closer than 5px to each other for more than 24px, except within 26px of a shared end.
7. **At most 2 crossings per drawn line, counted after bundling.** Lines run on a grid, so every crossing is at a right angle.

## Legibility

8. **Labels are whole.** A name wraps to two lines and widens its box. It is cut only past the hard width (280px for a module box, 220px for a unit box or chip), and then the tooltip carries the full name. No label is wider than its box.
9. **Nothing overlaps.** No two boxes or chips overlap, and no line passes through a box it does not join.
10. **A port sits near the units it joins.** A port goes on the top or bottom edge of the frame, whichever is nearer the mean of the units its lines join, within one row of tolerance. A filled chip is a contract the unit provides and an outlined chip is one it uses. Position on the frame no longer carries that meaning.
11. **A line's style shows its kind, not its origin.** Lines are solid, and their colour shows the relation kind. A line a pstack reader found (판독) starts with a hollow purple dot. A line a human stated (확인) starts with a filled green dot. A line with no dot was extracted from the code (추출). The dot's tooltip says what the word means. 판독 means a reader read the relation, ABBD checks on every sync that the quoted code still exists, and no human has confirmed it.
12. **Unwired contracts are grouped.** A contract that no inside unit is recorded as meeting is not spread along the frame. It is listed under the drawing as a chip group, with a shared first word shown once (`abbd`: diff, evidence, ...).

Every view is deterministic, so the same state renders as identical bytes. The overview draws only EXTRACTED and USER_STATED relations. A group's view draws its members' relations as the group's own, but the overview never draws a module's inside, so changing a module's inside never moves the overview.

## Supplying groups

When the top level holds more than 16 modules, or a module holds more than 16 units, the state needs a grouping level. The grouping is stored as units (MOD) and `parent` links. The renderer never invents one at render time.

- **The reader supplies it (preferred).** The supplement step of `abbd-setup` asks the pstack readers which parts of the system's behaviour the units fall into. Each part is added as a MOD with `evidence`, and each member gets that part as its `parent`, with `parent_evidence`. `abbd record parent ID PARENT --evidence ...` places a single unit later, and `--source` records a human's statement instead. Such a parent is INFERRED or USER_STATED, and sync keeps it unless the extraction script reports a parent for that unit. A reader's parent lasts only as long as its evidence. A parent that would close a cycle is refused.
- **Sync's fallback (last resort).** If nobody grouped a crowded level, `abbd sync` groups the units the script placed by the shallowest directory that splits them. It stores these groups as MOD units whose anchor is that directory with the symbol `(group)`. It never moves a unit a reader or a human placed. It recomputes the groups on every sync and reuses IDs by directory.

## Known gaps

Nine views still break a rule. Their current measurements are recorded in `fixtures_test.go` as debt. They are allowed to improve but never to get worse, and the test fails once a view keeps every rule until it is taken off the list.

- **Orca, dense cyclic graphs.** Five Orca views are dense cyclic import graphs among sibling modules that no directory split can break up: mobile app, desktop main process, runtime, renderer, and `src/`. They break rules 5, 6 and 7. The fix is a reader's grouping by behaviour.
- **Orca, too wide or too many boxes.** `src/main/` and `shared types and schemas` are wider than the 920px budget because of their frame ports. `src/shared/` holds 22 units in one directory.
- **This repository's CLI inside.** It has one port on the far side.
