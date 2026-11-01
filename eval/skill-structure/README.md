# Can ABBD draw the structure of skill markdown?

The question was whether ABBD can draw a traceable, abstracted structure of skills written in natural language, not code. The rule for this run: an agent reads the markdown. Code may record only what is literally written. That means one unit per skill file, a file path one skill writes to another skill or doc, and an `abbd <cmd>` token, each cited at its file and line. Every other structure came from pstack readers. It is stored INFERRED, with a verbatim quote that keeps it alive.

## This repository's two skills

A how-explorer reader read `/abbd`, `/abbd-setup` and the CLAUDE.md block. It named 7 parts of `/abbd` and 8 parts of `/abbd-setup`. Each part is recorded as a feature inside its skill, along with the abbd commands, pstack skills and sibling parts it uses. The block now points at `/abbd` and at the two parts it restates. The extractor's only change is that each skill-to-command relation now carries the line where the skill writes the command.

- `abbd explain FEAT-0006` shows the part "record what a reader found", the skill it sits inside, and each use, cited for example as `plugins/abbd/skills/abbd/SKILL.md:26`.
- `abbd impact cmd/abbd/record.go` reaches the parts that document `record`. Those are "record what a reader found", "what the human states" and "extraction format".
- `abbd-skill-parts.png` and `abbd-setup-parts.png` show each skill's inside.

pstack lives outside this repository, so the four pstack skills the parts hand work to sit under one module named "pstack (external)". ABBD has no kind for an external dependency.

## pstack's skill tree

pstack 0.15.36 was copied into a scratch repository and set up with a 166-line extractor plus a supplement from five readers.

| Source | Relations | What it holds |
|---|---|---|
| Extractor (written paths) | 81 | 30 point at docs, 23 of them at the Claude Code adapter. 51 go from one skill to another. |
| Readers (name or meaning) | 139 | Each pair the extractor already had was dropped. |
| Readers (grouping) | 17 families, 69 placements | 12 top-level families, plus 5 principle sub-families taken from poteto-mode's Principles index. |

Only reading found these:

- poteto-mode depends on all 24 principle skills. It names each one by title ("**Laziness Protocol.**"). It writes none of them as a path, so the extractor sees 0 of these 24 edges.
- Eight skills (architect, arena, swarm, interrogate, how, why, reflect, poteto-mode) take their models from the role lines in `pstack-models.mdc`. That rule is written by setup-pstack, and none of them names setup-pstack. Without the readers, setup-pstack looks like an island.
- teach runs how and why. recall and blast-radius call into why. These are named in prose such as "Teach sits on top of `how` and `why`."

## Can a human see pstack's structure from the overview?

Partly. `pstack-overview.png` shows twelve family boxes a pstack user would recognise. They are layered from "poteto-mode, the front door" at the top down to principles, writing and build at the bottom, and that layering is the true direction of use. Single relations cannot be traced on the overview. Every family touches every other, and the 23 adapter links all pull edges into "setup and help". The family insides read well. `pstack-understand-code.png` shows how, why, teach and recall with the edges between them, and lists every reader relation with its quote and line. `pstack-principles.png` shows the five sub-families. The overview gives the shape, and a family's inside is where a relation becomes traceable.

## Defects met

- ABBD has no kind for a dependency outside the repository, so pstack skills became features under an invented module, anchored on the quoting line.
- A reader's unit cannot be retracted. A duplicate CLAUDE.md block unit had to be removed by deleting its file and its relation entries by hand.
- `explain` cannot find a supplement unit by its key (`fam-understand`), only by its name.
- Overview edges have no weight or filter. A hub doc such as the adapter turns the overview into a mesh.
- The pstack families have no how text. Running how per family did not fit this run, and the view says so.
- Aside screenshots time out on a tab that is not visible. Opening the page with `aside <url>` first, then attaching to the active tab, works.
