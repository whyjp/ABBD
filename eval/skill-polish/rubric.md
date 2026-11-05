# Skill polish rubric

The judge compares two drawings of the same view of one repository, shown as A and B, and says for each criterion below which reads better: `A`, `B`, or `=` when it cannot tell them apart on that criterion. It gives one short reason per comparison, naming what decided it. It sees drawings by sanitized file names only, and never learns which version drew which.

A `=` is an answer, not a dodge. Two drawings that differ only in ways a criterion does not cover are `=` on it. Two identical drawings are `=` on every criterion.

The criteria come from `docs/blueprint-svg.md` (Decomposition, Focus views, Drawing criteria, Grouping levels) and from the user's direction: ABBD draws every relation, so a view that would be crowded is split by abstraction level into the overview, part views and single views, and each relation that matters is drawn one at a time, with clear rules for placement, bends, hubs and one-way boxes. The bar is a diagram an engineer polished by hand in draw.io.

## Per view

1. **Abstraction level.** The view shows one level. The overview shows the system's top-level parts, the ones an engineer would name in a sentence, and nothing that belongs inside one of them. A part view shows one part's insides. No box is an implementation detail beside a subsystem, and no level is a flat dump of every file or package.
2. **Density and split.** The view can be read in one look: about 4 to 12 boxes, and few enough lines that each one can be followed from end to end. A dense level the reader can open one box at a time, through part or single views, beats the same level drawn as one hairball.
3. **Placement by role.** Users stand above what they use. A box many places use (a hub) stands below the middle of its users. A box that only uses stands at the top, and a box that is only used at the bottom. External contracts sit on the frame next to the boxes that use them.
4. **Lines.** Lines run straight or turn once or twice. No staircases, no jogs, few crossings, no bundle that seems to join two boxes that do not relate. No line climbs unless it closes a cycle.
5. **Labels.** Boxes carry names a reader of the project knows. Lines between top-level parts carry a short phrase that says what crosses (an endpoint, a type, a call), not the kind of coupling and not one generic phrase on many lines.
6. **Overall.** Which would an engineer rather put in a design doc as it is?
7. **Interaction.** Judged only when the pack holds hover shots of the view. Hovering a box lights that box, every line that meets it and every box at the other end, in the line's own colour, and dims the rest. Hovering a line lights the whole route from one end box to the other. A lit route that stops partway, a lit box no lit line reaches, or one route drawn in two colours is a defect.

## Per repository

- **Set of views.** Which set's overview plus the views one click away tells the system's architecture better? Are the important relations visible somewhere, and is each view at its own level?

## What the judge is shown

- Every view is the whole drawing, captured as one image at its drawn size, never a viewport crop. A drawing the judge must scroll in the product is still shown whole, and its size counts under density.
- Hover shots, when given, are listed beside each side of a comparison in `pairs.tsv`, one image per hovered box or line: the box most lines meet, then the hovers the tool's checks flagged. Two sides may show different hovers; judge what each side's hovers show of its own drawing.

## Measured beside the verdicts

- `abbd lint` totals: hard breaches, target breaches, excess turns, crossings, weighed sum, placement debt, phantom pairs, climbing lines.
- Per view: boxes, chips, drawn relations, relations per box (dense when above 2), rows, drawn width and height.
- `shoot.py` hover findings per view: cut routes (a lit line end that touches no lit box and no lit line), stray lights (a lit box no lit line reaches), mixed routes (lines of two colours joined into one lit route), and cut line hovers (hovering one line lights a segment whose ends do not both reach a box).
- Whether `abbd lint` and `abbd status --check` exit 0 without a debt file the agent recorded to silence them.
