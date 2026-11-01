# Evidence identity and one precedence table

Status: design, not implemented. Scope: two changes to ABBD's data model. Nothing else is redesigned here.

ABBD keeps a repository's architecture topology as the verified final state of merged code. The script extracts what is mechanical, pstack readers record what needs reading (INFERRED, with verbatim evidence), humans' statements are USER_STATED, and `abbd sync` keeps all of it true without an agent. A defect review found that most ABBD bugs come from two parts of the data model. One is how a reader's evidence stays alive. The other is how several sources writing the same field are reconciled. This document designs both together, because the second needs the first's statuses to say why something went away.

## 1. Evidence today

### What the code does

`internal/evidence.Checker.Check` decides whether one `model.Evidence` (`path`, `lines`, `symbol`, `quote`) is live. It runs the following steps in order.

1. Normalize the quote by deleting all whitespace. Find whole-word matches in the file named by `path`.
2. Take a match within `lines` plus or minus `Slack` (3). Otherwise take the only match in the file. Two or more matches outside the window mean dead.
3. If the file holds no match and the quote is at least `MinMoveQuote` (24) characters, search every committable (`gitx.WorkingFiles`), non-test (`isTest`) file with the same extension under the owner's anchors (`covered`). Exactly one match moves the evidence there. Otherwise it is dead.

`Checker.Apply` runs this at every sync. A semantic unit goes absent when all its items are dead. A reader's parent is cleared when its parent evidence dies. A semantic relation gets `dead: true`. The `symbol` field exists in the format and readers fill it (304 of about 490 stored quotes have one), but no code reads it.

### Defects mapped to causes

| Defect | Cause in the model |
|---|---|
| A quote dies when its line is edited | The quote is both the locator and the liveness test. An edit to the quoted line removes both at once |
| Relocation needed three fixes (513c41f: tracked files vs the walk, test files, other owners) | With no identity beyond the text, a move can only be found by searching text across files, and every search rule is a heuristic |
| Dead quotes pile up (24 at one point, 12 today) | A unit stays present while any one item lives, so its other dead items never surface. Nothing records why an item died, and no command clears one |
| A unit's own evidence cannot be refreshed | `abbd record unit` only creates units. No operation replaces the evidence of an existing unit, relation or parent |
| Prose evidence (SKILL.md lines) has no symbols | The 109 prose quotes rely on the same text search as code. That works, but a symbol-only identity would leave them with none |

### The 12 dead items in this repository today

Measured with `abbd evidence` on main `d413920` and classified by reading the current code.

| Class | Count | Example |
|---|---|---|
| Quoted line edited, enclosing function still exists | 8 | `r := c.Check(ev)` became `c.Check(ev, within)` in `checkEvidence` |
| Function moved to another file | 1 | `func Path(...)` moved from `topology/query.go` to `topology/path.go`, outside MOD-0053's anchors, so the anchor rule killed it |
| Function deleted | 1 | `(*layout).route` in `screen/layout.go` |
| Coupling removed inside a surviving function | 2 | `r.Counts()` left `top` in `overview.go`, and `top` stayed. Two units cite the same line |

Commit b6776b7 had the same mix. Nine dead couplings were re-recorded because they still held. One (`r.Counts`) stayed dead because the coupling really was gone. Today's 12 fit the same pattern. Nine items still describe true facts (8 edits and 1 move), one is gone for good (`route`), and two record a coupling that really ended inside a function that survives. Those two are exactly the case a symbol-only identity would keep alive.

## 2. Evidence options

### Option 1A. Improve quotes

Keep quote identity and relocation. Add a per-item status with a reason, plus `refresh` and `retract` commands. This fixes the pile-up and the refresh gap. It does not stop false deaths from line edits. It keeps every relocation heuristic, and the `Path` move still dies.

### Option 1B. Symbol identity (the proposal as stated)

Evidence is `path#Qualified.Symbol`, live while an index finds the symbol. The quote is kept for display and for a digest that flags edits. This survives edits and follows moves by name. It has three costs.

- False life. The `r.Counts()` coupling would live as long as the function that once called it. A digest change shows that something changed, not that the coupling went away, and every edit to a large function changes the digest.
- Prose has no symbols, so prose needs a second identity anyway.
- Projects without a symbol index (Orca's extractor emits none today) get nothing, which leaves them with 1A.

### Option 1C. Scoped quote (chosen)

Evidence keeps the quote as the claim and adds a scope that locates it. The scope is a symbol when the project's index knows one, and the whole file otherwise. Prose is simply file-scoped evidence. An item has three statuses.

| Scope found? | Quote found in scope? | Status | Effect |
|---|---|---|---|
| yes | yes | live | Lines and digest move to the match |
| yes | no | drifted | The fact stays and is flagged. A reader must refresh or retract it |
| no | (none) | dead | The fact is gone, with the reason stored on the item |

### Option 1D. Fuzzy matching (rejected)

Edit-distance matching would keep `c.Check(ev, within)` alive. A similarity threshold is a heuristic nobody can explain from the state, and it can confirm a coupling that the edit removed.

### Comparison

| Defect | 1A | 1B | 1C |
|---|---|---|---|
| Line edit kills a true fact | still dies | lives | drifted (kept, flagged) |
| Coupling removed, function stays | dies (correct) | lives (wrong) | drifted (flagged, never silently confirmed) |
| Move to another file | heuristic | by name | by name where indexed; otherwise drifted |
| Relocation heuristics | all kept | deleted | deleted |
| Pile-up and refresh | fixed | fixed | fixed |
| Prose | text | none | text, file scope |
| Works without an index | yes | no | yes, at 1A's level with no relocation |

**Are symbols worth it?** Mostly the relief comes from the three statuses and the operations, and those work without symbols. On its own, 1A plus a drifted status covers the pile-up, the refresh gap and the false-death noise for unindexed files. Symbols add two things: following a move by name, and telling "the function is gone" (dead) from "the line changed" (drifted). They cost little here, because this repository's extractor already parses Go declarations (`decls` in `.abbd/extractor/extract.go`) and readers already write `symbol`. So 1C is chosen. Without an index it reduces to 1A, minus the cross-file text search, which it deletes. If the extractor change turned out expensive for a project, that project keeps file scope and loses nothing it has today except that search.

## 3. Evidence design

### Data shape

```go
// Status is "" while live. It is stored so that diff, screen and explain read
// it without the tree.
type Status string

const (
	Drifted Status = "drifted" // the scope exists and the quote is gone from it
	Dead    Status = "dead"    // the scope itself is gone
)

type Evidence struct {
	Path   string `yaml:"path"`
	Symbol string `yaml:"symbol,omitempty"` // the scope; "" is the whole file
	Lines  string `yaml:"lines,omitempty"`  // a hint, rewritten each sync
	Quote  string `yaml:"quote"`
	Digest string `yaml:"digest,omitempty"` // the lines holding the quote, as Relation.Digest today
	Status Status `yaml:"status,omitempty"`
}

// Span is one entry of the symbol index. The project extractor supplies it.
// ABBD core knows no language.
type Span struct {
	Path, Symbol string
	First, Last  int
}
```

`Relation.Digest` and `Relation.Dead` go away. The digest moves onto each item. A claim's status is derived: live if any item is live, else drifted if any is drifted, else dead.

### The index

The extractor's JSON gains an optional `symbols` list of `{path, symbol, lines}`. ABBD parses it at the boundary and rejects a symbol named twice in one path. Core sees only paths, names and line ranges, so it stays language-agnostic. The extractor decides which files it indexes, so test, vendored and generated files are its call, made in the language it knows. That deletes `isTest`, `covered`, `MinMoveQuote` and `candidates` from core. This repository's extractor emits Go declarations as `Name` and methods as `Type.Method`. It may also emit markdown headings as symbols, but that is not required (see Left out).

### Resolving one item at sync

1. Apply the git renames since the last sync to `path`, as `applyRenames` already does for anchors.
2. If `symbol` is set, look it up in `path`. If `path` no longer has it, look for the same symbol in exactly one other indexed path, and move there. If no path or several paths hold it, the item is dead.
3. If `symbol` is empty, the scope is the file. A missing file means dead.
4. Inside the scope, run today's whitespace-insensitive whole-word match. Prefer a match near `lines`. Then take the only match. Several matches with none near count as drifted, not dead.
5. When live and `symbol` is empty, set `symbol` to the innermost indexed span holding the match, if there is one. Recording never needs the index, because the next sync fills the scope.

The same tree and index give the same statuses, so sync stays idempotent. `abbd evidence` runs the registered extractor to get the index and prints status per item. It never writes.

### What a reader does with each status

- live: nothing.
- drifted: the claim still counts. Its unit stays present, and `abbd impact` still follows its relation, marked drifted. `abbd evidence` prints the scope's current lines beside the old quote. The reader re-quotes with `refresh`, or removes the claim with `retract`.
- dead: the claim no longer counts. The item stays stored with `status: dead` until it is refreshed or retracted. `abbd status` prints the drifted and dead counts, so they cannot pile up unseen.

The `/abbd` after-work step gains one line. For each drifted or dead item under a path the change touched, refresh or retract it.

## 4. Reconciliation today

### Writers

Four writers set unit fields today. Three origins name them, and the gap between four and three is a source of bugs.

| Writer | Command | Origin it writes |
|---|---|---|
| script | `abbd sync` (and setup) | EXTRACTED |
| supplement | `abbd setup --supplement` | INFERRED |
| reader | `abbd record` | INFERRED |
| human | `abbd record ... --source` | USER_STATED |

`group.Fallback` also rewrites the parent of script-placed units into directory groups. It is a fifth writer.

### Per-field rules, as the code has them

| Field | Rule today | Where |
|---|---|---|
| name | Script replaces an EXTRACTED name. A unit moves between script and reader whole: whichever side reports it sets the name | `identity.merge` |
| kind | Fixed by the ID prefix | `model.Unit.Validate` |
| anchors | Script always replaces. A semantic unit's anchors come from its evidence paths | `merge`, `recordUnit` |
| parent | Script's parent wins over a reader's and a human's. A kept parent is dropped if its unit is absent or it closes a cycle. `ParentOrigin` empty means EXTRACTED | `setParents`, `group.Fallback`, `Apply` |
| purpose, responsibility | Kept unless nil. Under setup, an INFERRED value is replaced by the supplement's | `merge` (`replace`) |
| signature | Script wins. A reader's survives while the script reads none. A disagreement in setup is a printed conflict | `merge`, `Overlay` |
| digest | Script only | `merge` |
| unit evidence | Replaced by the extraction's on every match | `merge` |
| relations | Script owns EXTRACTED edges of units it reports. Others survive, except INFERRED ones under setup. On the same edge, EXTRACTED wins, then the stored one | `setRelations` |
| absent | Not matched by the script, unless semantic under sync or held by a human under setup. Semantic with all evidence dead | `Reconcile`, `heldByHuman`, `Apply` |

### Defects mapped to causes

| Defect | Cause |
|---|---|
| A reader's parent shown as the script's (2404d01, 62 units) | Origin is a separate optional field whose zero value means EXTRACTED |
| A unit vanished when its parent went absent (f0d043b) | The parent was overwritten in place, so the fallback had to be repaired in two writers |
| Cycle repair dropped parents off the cycle (c05d076) | Several writers wrote one parent slot, and cycle repair had to guess which write to undo |
| A reader cannot retract a wrong unit or correct an INFERRED fact | Each writer can only add. The only way out is deleting files by hand (`eval/skill-structure/README.md`) |
| Unit evidence cannot be refreshed | See section 1 |
| Re-running setup drops `abbd record` findings (inferred from `setRelations`, not run) | Supplement and reader share the INFERRED origin, so "replace the supplement's guesses" also replaces the reader's |

The common premise is that every field holds one value, and each writer overwrites it under its own rule. The losing value is gone, so nothing can fall back to it or say why it went.

## 5. Reconciliation options

### Option 2A. Document the rules and add the invariant test

Keep the code and write the table above as documentation. The invariant test would fail on today's code (the setup and record case), and each fix would add one more branch. This is the shape that produced the defects.

### Option 2B. One table in code, one value per field

Make every field a typed fact with a required writer, and route all merging through one resolver keyed by a `precedence` table. The mislabelled origin can no longer happen. But a value that loses is still overwritten. When the winner goes away, nothing is left to fall back to, and retracting one claim needs the writer of the one below to run again.

### Option 2C. Claims per writer, resolved by one table (chosen)

Each unit file stores one layer of claims per writer. A writer only ever replaces or edits its own layer. The effective unit that every reader of the state sees (screen, topology, diff, explain) is computed on load by a single `Resolve` from the layers and one precedence table. A loser stays stored, so falling back is automatic. Retracting is the removal of one's own claim, or a stored retraction claim. The reason for a disappearance is data that already sits in the file.

This follows the principle of separating before serializing shared state. Four writers sharing one slot is the shared state, and giving each writer its own layer removes the sharing.

| Defect | 2A | 2B | 2C |
|---|---|---|---|
| Mislabelled origin | possible | impossible | impossible |
| Parent absent, unit lost | patched | patched | falls through to the next claim |
| Cycle repair | heuristic | heuristic | skip claims in table order, never delete |
| Retract or correct | no | one-off command | own layer, one rule |
| Setup drops reader findings | yes | yes | no, separate layers |
| Code deleted | none | some | `merge`, `setParents` keep logic, `heldByHuman` search, `Guesses`, `ParentOrigin`/`ParentSource`/`ParentEvidence` |

## 6. Reconciliation design

### Data shape

```go
type Writer string // "script" | "supplement" | "reader" | "human"

// Origin is what the user sees. The script writes EXTRACTED, supplement and
// reader write INFERRED, and the human writes USER_STATED.
func (w Writer) Origin() Origin

type Claims struct {
	Name           string           `yaml:"name,omitempty"`
	Anchors        []Anchor         `yaml:"anchors,omitempty"`
	Parent         *ParentClaim     `yaml:"parent,omitempty"`
	Purpose        *Stated          `yaml:"purpose,omitempty"`
	Responsibility *Stated          `yaml:"responsibility,omitempty"`
	Signature      string           `yaml:"signature,omitempty"`
	Digest         string           `yaml:"digest,omitempty"`   // script only
	Evidence       []Evidence       `yaml:"evidence,omitempty"` // why this layer says the unit exists
	Relations      []RelationClaim  `yaml:"relations,omitempty"`
	Retracted      []Retraction     `yaml:"retracted,omitempty"` // reader and human only
}

type Stated struct {
	Value  string `yaml:"value"`
	Source string `yaml:"source,omitempty"` // required in the human layer, forbidden elsewhere
}

type Record struct { // what a unit file stores
	ID     ID
	Kind   Kind
	Layers map[Writer]*Claims
	Absent *Absence // nil while present
	Notes  string
}

type Absence struct {
	Reason AbsentReason // not-reported | evidence-dead | withdrawn | retracted
	Since  string       // the revision of the sync that first found it absent
}

// Resolve is the only place a field is chosen. It is pure and total.
func Resolve(rs []Record) []Unit
```

`model.Unit` stays the read model and keeps its field names, so consumers barely change. `Fact.Origin` and `Relation.Origin` are filled by `Resolve` from the winning writer. `ParentOrigin`, `ParentSource`, `ParentEvidence`, `Absent bool` and `Dead` leave the stored format.

### The precedence table

The table reads left to right: the first writer with a usable claim wins. A claim is usable when its evidence status is live or drifted, no higher writer's retraction masks it, and the field's own check passes. A dash means the writer may not claim the field.

| Field | 1st | 2nd | 3rd | 4th | When the winner goes |
|---|---|---|---|---|---|
| name | script | reader | supplement | (none) | The next writer's name. A unit always has one, since existence needs a layer that names it |
| kind | the ID | (none) | (none) | (none) | Never changes |
| anchors | script | reader | supplement | (none) | The next writer's anchors. Reader and supplement anchors are their evidence paths and symbols |
| parent | human | script | reader | supplement | Skip a claim whose parent is absent or would close a cycle. With no claim left, the unit is top level. Grouping applies only when the script's claim wins |
| purpose, responsibility | human | reader | supplement | (none) | The next writer's text |
| signature | script | reader | supplement | (none) | The next writer's text. A reader or supplement value that differs from the script's is listed as a conflict, as setup does today |
| digest | script | (none) | (none) | (none) | Empty |
| relation (per `to` and `kind`) | script | human | reader | supplement | The next writer's claim, whose origin is then shown. `via` and evidence come from the winner |
| existence | script | reader | supplement | human holds | Absent, with a reason (below) |

There are three rules for retractions. A reader's retraction masks supplement claims on the same target. A human's retraction masks reader and supplement claims. Nothing masks the script, because a wrong code fact is fixed in the extractor.

Cycles are handled in one order. Parents are resolved in writer order (script first, then human, reader, supplement), and within a writer in unit ID order. A claim that would close a cycle is skipped. Script parents form a tree, which `extract.parse` already checks. So only a non-script claim on the cycle is ever skipped, and it stays stored, and `abbd explain` names it.

Existence works like this. A unit is present when the script reports it, or a reader or supplement layer names it with usable evidence and no retraction masks it. A human claim on a unit that the supplement withdrew keeps it present, as `heldByHuman` does today. Otherwise the reason is chosen in order: `retracted` when a retraction masks the last claim, `evidence-dead` when the last claim's items are all dead, `withdrawn` when setup removed the supplement claim, and `not-reported` for a script unit that the script no longer reports.

Two choices change behavior. First, the human now outranks the script for `parent`. Today the script wins silently, and the human's placement disappears with no record. Grouping is a decision about how to read the code, and the script's claim stays stored underneath. Second, the reader outranks the supplement. A targeted `abbd record` is newer than the bulk setup it corrects.

### State format, before and after

Before (MOD-0002 today, trimmed to one relation and two evidence items):

```yaml
id: MOD-0002
kind: MOD
name: {value: review commands, origin: INFERRED}
anchors:
    - {path: cmd/abbd/diff.go, symbol: diff}
    - {path: cmd/abbd/evidence.go, symbol: checkEvidence}
    - {path: cmd/abbd/evidence.go, symbol: status}
parent: MOD-0001
parent_origin: INFERRED
parent_evidence:
    - {path: cmd/abbd/diff.go, lines: "44", quote: "if e.check && d.Boundary() {"}
evidence:
    - {path: cmd/abbd/diff.go, lines: "44", symbol: diff, quote: "if e.check && d.Boundary() {"}
    - {path: cmd/abbd/evidence.go, lines: "24", symbol: checkEvidence, quote: "r := c.Check(ev)"}
purpose: {value: "so a PR and its reviewer see what changed ...", origin: INFERRED}
relations:
    - to: MOD-0013
      kind: depends_on
      origin: INFERRED
      via: call
      evidence: [{path: cmd/abbd/evidence.go, lines: "21", symbol: checkEvidence, quote: "c := evidence.New(e.root)"}]
      digest: 381edbea023c2268
```

The second evidence item is one of today's 12 dead items. The unit stays present because the first item lives, so nothing shows the dead one.

After (MOD-0002 came from the supplement, and the `human` layer shows a hypothetical correction of its purpose):

```yaml
id: MOD-0002
kind: MOD
supplement:
    name: review commands
    anchors:
        - {path: cmd/abbd/diff.go, symbol: diff}
        - {path: cmd/abbd/evidence.go, symbol: checkEvidence}
        - {path: cmd/abbd/evidence.go, symbol: status}
    parent:
        to: MOD-0001
        evidence: [{path: cmd/abbd/diff.go, symbol: diff, lines: "44", quote: "if e.check && d.Boundary() {", digest: <sha>}]
    evidence:
        - {path: cmd/abbd/diff.go, symbol: diff, lines: "44", quote: "if e.check && d.Boundary() {", digest: <sha>}
        - {path: cmd/abbd/evidence.go, symbol: checkEvidence, lines: "24", quote: "r := c.Check(ev)", status: drifted}
    purpose: {value: "so a PR and its reviewer see what changed ..."}
    relations:
        - to: MOD-0013
          kind: depends_on
          via: call
          evidence: [{path: cmd/abbd/evidence.go, symbol: checkEvidence, lines: "21", quote: "c := evidence.New(e.root)", digest: 381edbea023c2268}]
human:
    purpose: {value: "Shows a reviewer what a PR changed in the topology.", source: "conversation 2026-11-02"}
```

`<sha>` stands for the digest that sync computes. The human's purpose wins, and the supplement's stays stored beneath it.

A unit the script reports carries a `script:` layer. Sync rewrites that layer wholesale, so it is never edited by hand. An absent unit adds `absent: {reason: evidence-dead, since: 4ce1ba4}`. The markdown body still shows the effective name and anchors for a human reading the file. `abbd explain` prints each field with its winning writer and any claims it outranked or skipped.

### Operations

Each operation edits one layer and is idempotent. Running it twice leaves the same bytes.

| Operation | Command | Layer | Effect |
|---|---|---|---|
| sync | `abbd sync` | script | Replace the script layer of every matched unit from the extraction. Recompute evidence statuses in all layers. Set or clear `absent` |
| setup | `abbd setup --supplement F` | supplement | Replace every supplement layer from F (via `Overlay`'s matching), then sync |
| record | `abbd record unit/relation/parent ... --evidence` | reader | Upsert the reader's claim on that target. Evidence must be live now |
| state | `abbd record ... --source S` | human | Upsert the human's claim with its source |
| correct | (none of its own) | reader or human | Record over a lower claim. The table makes the new claim win, and the old one stays stored |
| retract | `abbd retract TARGET [--source S] --reason TEXT` | reader or human | Remove one's own claim on TARGET, or store a retraction that masks lower claims on it. TARGET is an ID, an edge or `ID.field` |
| refresh | `abbd record evidence TARGET --evidence ...` | reader | Replace the evidence of the winning INFERRED claim on TARGET. If that claim is the supplement's, copy it into the reader layer with the new evidence, which then outranks it |

`retract` and `refresh` are the two new commands. `record fact` becomes a case of `state`.

### Invariants

The invariants are tested over the pure functions (`identity.Reconcile` together with the evidence pass, `Overlay` together with the layer replacement, and each operation), not through the CLI.

1. **Nothing silently disappears.** Take any state S, inputs I and operation op, with S' = op(S, I). Every unit present in S is present in S' or has `absent.reason` in S'. Every reader or human claim in S is still stored in S', unless op is `retract` on that claim. When an effective field value differs between S and S', at least one of these holds: its old winning claim is gone from the script or supplement layer, a status in S' marks it drifted or dead, a retraction masks it, or a higher claim appeared.
2. **Idempotence.** op(op(S, I), I) = op(S, I), byte for byte.
3. **Writer isolation.** sync changes only script layers, statuses and `absent`. setup changes only supplement layers, then whatever sync changes. record, state, retract and refresh change only their own layer of the target unit.
4. **One tree.** Resolve never yields a parent cycle, and every present unit's effective parent is present or empty.
5. **Origin is the writer's.** Every effective origin equals `Writer.Origin()` of the claim that won.

## 7. Migration

The migration is one command, `abbd migrate`. It runs once per repository and is deleted in the release after. It is the lever, and it can be re-run on a copy.

1. Run the extractor (including the new `symbols`) and the overlay of `.abbd/extractor/supplement.json`, exactly as setup would.
2. For each stored unit, send each field to a layer by its origin. An EXTRACTED or empty origin goes to `script`, and that layer is then replaced by step 1's output. USER_STATED goes to `human` with its source. An INFERRED value equal to what the step 1 overlay gives this unit goes to `supplement`. Any other INFERRED value goes to `reader`, because it came from `abbd record`.
3. Move `Relation.Digest` onto the relation's evidence items. Drop `dead`, `parent_origin`, `parent_source` and `parent_evidence`, whose content is now `parent` in its layer. Turn `absent: true` into `absent: {reason: not-reported}` for a script unit and `{reason: evidence-dead}` for a semantic unit, with `since` set to the state's revision.
4. Check each stored `symbol` against the index. A symbol the index lacks is cleared, and the next sync assigns the innermost span.
5. Run sync. Compare `blueprint/graph.json` before and after.

Expected differences in this repository: 10 of the 12 dead items become drifted, `topology.Path` becomes live in `path.go`, and `route` stays dead. No unit or relation leaves the graph, and step 5 asserts that. In Orca, the extractor emits no symbols, so every item is file-scoped. An item that lives today only because the cross-file search moved it already has its new `path` stored, so the migration loses nothing. Later moves across files show as drifted until Orca's extractor emits symbols.

## 8. Test plan

Evidence tests are table-driven over the status table, with one fixture per case taken from this repository's 12 dead items.

- Edit the quoted line inside a kept function: drifted, and the new lines are shown.
- Move the function to another file (`Path`): live at the new path.
- Delete the function (`route`): dead.
- Remove the call and keep the function (`r.Counts()`): drifted, never live.
- Same symbol in two other files: dead, because the target is ambiguous.
- No index: file scope. An edit gives drifted, and a deleted file gives dead.
- A git rename of the file is followed.
- Prose: insert a line above, and it stays live with the lines moved. Reword the sentence, and it is drifted. Delete the file, and it is dead.

Precedence tests read the `precedence` value the code uses and pin each row of section 6. Because the table is one Go value, a new field cannot ship without a row, and a test checks that every `Claims` field has one.

Regression tests come from history.

- 2404d01: a reader's parent stays the reader's after the script takes the unit over.
- f0d043b: a parent goes absent, the unit sits at the top level, and the claim stays stored.
- c05d076: only the claims that close the cycle are skipped.
- Retracting the duplicate CLAUDE.md block unit from the skill-structure run.
- Refreshing a supplement unit's evidence.
- Re-running setup keeps `abbd record` relations.

An invariant property test generates random records, random extractions and supplements, random code edits (edit, move, delete for a quoted line and for a symbol), and random operation sequences, then checks invariants 1 to 5 after every step. The test is written first and fails on today's code through the setup and record case, which shows that it can see the defect.

## 9. Left out on purpose

- Tree-sitter or any parser in ABBD core. The extractor supplies symbols, and core stays a single Go binary that knows no language.
- Markdown headings as built-in scopes. An extractor may emit them as symbols, and file scope is enough for the prose seen so far.
- Grouping as a layer. `group.Fallback` keeps working on the resolved parents, and it still applies only where the script's parent wins.
- A human-stated name or signature. No one has asked for either, and they can be added as rows later.
- Negative claims against script facts. A wrong extraction is fixed in the extractor.
- ID pairing (`identity.Reconcile`'s anchor matching) and `candidates.yaml`. They are unchanged, and they pair on effective anchors.
- Showing drifted INFERRED relations differently in the overview. They are drawn the way INFERRED relations are drawn today, with a drift mark in the inside view and in `explain`.
