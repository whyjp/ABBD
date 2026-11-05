# Skill polish rubric

The judge scores each screenshot of a blueprint view from 1 (bad) to 5 (reads like a hand-polished draw.io diagram), on each criterion below, and gives one line of reason per score. The judge sees views by a sanitized label only.

The criteria come from `docs/blueprint-svg.md` (Decomposition, Focus views, Drawing criteria, Grouping levels) and from the user's direction: ABBD draws every relation, so a view that would be crowded is split by abstraction level into the overview, part views and single views, and each relation that matters is drawn one at a time, with clear rules for placement, bends, hubs and one-way boxes.

## Per view

1. **Abstraction level.** The view shows one level. The overview shows the system's top-level parts, the ones an engineer would name in a sentence, and nothing that belongs inside one of them. A part view shows one part's insides. No box is an implementation detail beside a subsystem, and no level is a flat dump of every file or package.
2. **Density and split.** The view can be read in one look: about 4 to 12 boxes, and few enough lines that each one can be followed from end to end. A view too dense for that is a 1. A dense level the reader can open one box at a time, through part or single views, scores higher than the same level drawn as one hairball.
3. **Placement by role.** Users stand above what they use. A box many places use (a hub) stands below the middle of its users. A box that only uses stands at the top, and a box that is only used at the bottom. External contracts sit on the frame next to the boxes that use them.
4. **Lines.** Lines run straight or turn once or twice. No staircases, no jogs, few crossings, no bundle that seems to join two boxes that do not relate. No line climbs unless it closes a cycle.
5. **Labels.** Boxes carry names a reader of the project knows. Lines between top-level parts carry a short phrase that says what crosses (an endpoint, a type, a call), not the kind of coupling and not one generic phrase on many lines.
6. **Overall.** Would an engineer put this view in a design doc as it is?

## Per repository

- **Set of views.** Does the overview plus the views one click away tell the system's architecture? Are the important relations visible somewhere, and is each view at its own level?

## Measured beside the scores

- `abbd lint` totals: hard breaches, target breaches, excess turns, crossings, weighed sum, placement debt, phantom pairs, climbing lines.
- Per view: boxes, chips, drawn relations, relations per box (dense when above 2), rows.
- Whether `abbd lint` and `abbd status --check` exit 0 without a debt file the agent recorded to silence them.
