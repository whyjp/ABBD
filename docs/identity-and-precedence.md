# Evidence statuses and one precedence table

Status: implemented on branch `abbd-identity-design` (PR #9). This is the reduced design an adversarial review asked for. The first draft (commit 1f77540) proposed a symbol index and one layer of claims per writer. Section 7 lists what the review found and what was deferred.

ABBD keeps a repository's topology true across code edits and several writers. The script extracts what is mechanical. pstack readers record what needs reading, with verbatim evidence, through the setup supplement or `abbd record`. Humans state facts with a source. `abbd sync` keeps all of it true without an agent. Most past defects came from two parts of the data model: how reader evidence stays alive, and how writers reconcile one field. This design changes those two parts and nothing else.

## 1. The defects

| Defect | Cause |
|---|---|
| A quote dies when its line is edited, and nothing says why | An item is live or dead, with no reason and no state in between |
| Dead quotes pile up (12 today) | A unit stays present while any item lives, so its dead items never surface, and no command clears one |
| Evidence cannot be refreshed, a wrong claim cannot be retracted | `abbd record` only adds |
| Relocation needed three fixes (513c41f) | A cross-file text search (`candidates`, `covered`, `isTest`, `MinMoveQuote`) is the only way to follow a move |
| A reader's parent was shown as the script's (2404d01) | `parent_origin` is optional and its zero value means EXTRACTED |
| A unit vanished when its parent went absent (f0d043b), and cycle repair dropped the wrong parents (c05d076) | Several writers overwrite one parent slot, so nothing is left to fall back to |
| Re-running setup drops `abbd record` findings | Supplement and reader share INFERRED, so "replace the supplement's guesses" also replaces the reader's |

## 2. Writers

Every fact, relation and parent claim stores the writer that made it. Origin is derived from the writer and is never stored. No writer is implied by an empty value: a value without a writer fails validation.

| Writer | Made by | Origin |
|---|---|---|
| `script` | `abbd sync` from the extraction | EXTRACTED |
| `group` | `group.Fallback` during sync, placing a crowded level's units in the directories their code lives in | EXTRACTED |
| `supplement` | `abbd setup --supplement F` | INFERRED |
| `reader` | `abbd record` with `--evidence` | INFERRED |
| `human` | `abbd record` with `--source` | USER_STATED |

## 3. Evidence statuses

An item is `{path, lines, symbol, quote}` plus a status. Its scope is the whole file named by `path`. Git renames since the last sync move `path`. The cross-file search is deleted, so a quote never follows into another file.

| Status | When | Effect |
|---|---|---|
| live (no `status` key) | The file holds the quote: near `lines`, else its only occurrence | `lines` moves to the match |
| `drifted`, with `since` | The file exists but no longer holds the quote, or holds it twice with none near `lines` | The claim still counts. `abbd impact` follows it and flags it drifted |
| `dead`, with `reason` | The file is gone, or a sync at a later revision than `since` still finds it drifted | The claim stops counting. The item stays stored |

The bound on drift is one revision. `since` is the revision of the sync that first found the item drifted. A later sync at the same revision leaves it drifted, so committing the work does not kill it. A sync at a later revision that still finds it drifted makes it dead with the reason `drifted since REV and not refreshed`. So the PR that drifted an item is where it must be refreshed or retracted. Dead is terminal: sync never re-checks a dead item. Only `abbd record evidence` or `abbd retract` clear it.

Script evidence (the extractor may cite quotes for a relation) is live or dead and never drifts, because the script restates it on every sync.

A claim is live if any item is live, else drifted if any item is drifted, else dead. A semantic unit, one only a reader or the supplement vouches for, goes absent with reason `evidence-dead` when its claim is dead. `abbd status` prints the drifted and dead item counts, so they cannot pile up unseen. The `/abbd` after-work step tells the agent to refresh or retract the drifted items under the paths its change touched.

When setup or sync replaces a claim whose items are unchanged (same path and quote), the items keep their status and `since`, whatever symbol the new item names. An item the claim did not hold before takes the status of an item the unit stores elsewhere with the same path and quote, such as a supplement parent that restates the unit's own evidence. Without that, re-running setup would revive dead items.

## 4. The precedence table

`model.Precedence` is one Go value. It lists the writers that may claim each field, highest first.

| Field | Order | Shape |
|---|---|---|
| name | script, reader, supplement | one value |
| purpose, responsibility | human, reader, supplement | one value |
| signature | script, reader, supplement | one value. A differing supplement value is printed as a conflict, as before |
| relation, per `to` and `kind` | script, human, reader, supplement | every writer's claim is stored |
| parent | human, group, script, reader, supplement | every writer's claim is stored |

A one-value field is replaced only by its own writer or by a writer above it. Setup replaces supplement values and leaves reader and human values alone.

For relations and parents, the shown claim is the first eligible one by status, then in table order: a live claim beats a drifted one and a drifted one beats a dead one, whatever the writer. A claim without evidence is live, so the script's and group's claims always are. A dead claim, or one a mask hides, is not eligible. The other claims stay stored. On a relation, they sit under `hidden:` in the unit file. When the shown claim goes, the next one shows, with its own origin.

A relation claim may carry a `phrase`, a few words that say what crosses the edge, such as a route, a message or a query. It is at most 24 characters and 4 words, on one line, and it does not only repeat the `via`. The script and the group never write one. A reader writes it with `abbd record relation ... --phrase`, the supplement with `"phrase"`, and a human with `--source`. A reader may record a claim on an edge the script reads only with a phrase. That claim stays under `hidden:`, the script's claim still shows, and the reader's claim only names what crosses the line. The phrase an edge shows is that of its shown claim, or else of the first other claim on it, by status and then the table, that is neither dead nor masked (`Unit.PhraseClaim`). It is drawn only while that claim is live. A phrase whose quote drifted is left off the drawing, and `abbd status --check` fails until a reader refreshes or retracts it, because `abbd evidence` and the check read the evidence of that claim as if it showed (`Unit.Shown`).

A parent claim is also skipped when its parent is absent or is not a module or feature. Cycles are resolved in tier order: script and group claims first, then human, reader and supplement, each in unit ID order. A claim that would close a cycle is skipped, never deleted, and the unit falls through to its next claim. Acceptance then starts again from the first tier, so that next claim is weighed before any lower tier's. With no claim left, the unit is top level.

A human's parent outranks the script's. The human claim stores `over`, the script's parent when the human stated it. When the script's claim later differs from `over` and from the human's own parent, sync and `abbd status` print it as a conflict, so a refactor cannot leave the human's placement stale silently.

### Grouping

`group.Fallback` is the fifth writer. It runs at sync and never on read. It first drops every group claim and resolves parents without them. A unit whose resolved parent is the script's, or which has none, may then be grouped by directory. Each grouping is stored as a `group` parent claim, and the group unit's own place is a `group` claim on its container. A group unit that is no longer needed goes absent with reason `unneeded`. Grouping never moves a unit placed by a human, reader or supplement.

## 5. Existence

A unit is present or absent, and an absent unit always stores `absent_reason`.

| Reason | When |
|---|---|
| `not-reported` | The script no longer reports a unit whose name it wrote |
| `withdrawn` | Setup's supplement no longer reports a unit whose name it wrote. A unit that holds any human claim, or that a human relation points at, stays present |
| `evidence-dead` | A semantic unit's claim is dead |
| `retracted` | A mask hides the unit, or a reader retracted its own unit |
| `unneeded` | A group unit that no view needs |

Setup pairs a supplement unit with the stored unit of the same `supplement_ref` before it pairs anything on anchors. The ref is the supplement's own `ref` for the unit. A store written before the ref spells it `supplement_key`, and setup pairs on that the same way. Identity therefore never depends on evidence locators, which sync rewrites.

## 6. Operations

| Operation | Command | Changes |
|---|---|---|
| sync | `abbd sync` | Script claims of the units it reports, all group claims, evidence statuses, `absent` and its reason |
| setup | `abbd setup --supplement F` | Supplement claims of the units the extraction and F report, then what sync changes |
| record | `abbd record unit/relation/parent ...` | The reader's claim on the target (a human's with `--source`). `record relation --phrase` adds what crosses the edge, and on an edge the script reads it is the only claim a reader may add. `record fact` writes a human's purpose or responsibility |
| refresh | `abbd record evidence TARGET --evidence ...` | Replaces the evidence of the reader's claim on TARGET. If the shown INFERRED claim is the supplement's, a reader claim with the new evidence is added above it. On a supplement unit, the unit becomes the reader's |
| retract | `abbd retract TARGET --reason TEXT [--source REF]` | In one step, removes the writer's own claim on TARGET and stores a mask over each lower INFERRED claim on it |
| migrate | `abbd migrate [--supplement F]` | Converts an old-format state once (section 8) |

TARGET is a unit ID, an edge (`FROM.KIND.TO`), or `ID.parent`, `ID.purpose`, `ID.responsibility`. The evidence given to `record` must be live now.

A mask is `{target, writer, digest, reason, source}`. The digest covers the masked claim's content: the edge, `via` and any `phrase` for a relation, the parent ID for a parent, the value for a fact, the name for a unit. A claim that changes no longer matches, so it shows again. A reader's mask hides supplement claims. A human's mask hides reader and supplement claims. Nothing masks the script or group, because a wrong code fact is fixed in the extractor. Running `retract` again finds no own claim and an identical mask, so it changes nothing and says so. `abbd status` lists the active masks. Removing a mask is a hand edit of the unit file in this round.

## 7. What the review found, and what was deferred

The adversarial review of the first draft asked to reduce scope before implementing. Each finding and what this design does with it:

1. A symbol index would kill a third of the supplement's live evidence, because readers stored bare method names where the index emits `Type.Method`. Deferred. When an index comes, an unknown or bare symbol falls back to file scope, and a duplicate symbol in one file (several `init`) never fails sync.
2. Anchors derived from evidence would change under a symbol migration, so setup would re-mint IDs. Setup now pairs on the stored `supplement_ref` first.
3. `group.Fallback` minted IDs and had no reason for absence. It is now a writer with stored claims and the `unneeded` reason, and it also groups unplaced top-level units.
4. Without an index, a drifted claim would count forever. Drift is bounded to one revision (section 3). Migration keeps today's dead items dead.
5. A drifted reader claim beat a live supplement claim. A live claim now wins, between any writers.
6. Retract was a permanent mask in two steps. It is one step, idempotent, and bound to the masked content.
7. A human parent over the script could go stale silently. The `over` conflict covers it.
8. Migration's supplement-or-reader split by value equality was unreliable. It now asks whether the supplement makes any claim on the same target (section 8), and it stops when the supplement file is missing.
9. `abbd diff --base` reads states at old revisions. The old-format loader stays permanently.
10. The stated invariants were false or too weak. Section 9 restates them as tests of what each operation really changes.
11. The `route` fixture: `(*layout).route` was moved to `internal/screen/route.go`, not deleted. It is no fixture of a deleted function.
12. Moving a symbol to the one other path holding it could jump to an unrelated function. There is no cross-file move in this round.
13. See 1.
14. Per-writer layers for every field hurt the file's legibility, and refresh changed what was withdrawable. Only relations and parents keep several claims. Name, anchors, purpose, responsibility and signature keep one value with a writer.
15. No listed defect needed layers for name, anchors, purpose or signature. They were dropped.

Also left out: fuzzy matching, a human-stated name or signature, negative claims against script facts, a command to remove a mask, and a drift mark in the drawing.

## 8. Migration

An old-format unit file is recognized by `origin` on its name. The old-format loader converts it on load and stays in the code permanently, because `abbd diff --base` reads committed states. EXTRACTED becomes `script`, USER_STATED `human`, and INFERRED `supplement`. A dead relation's items become dead, and an absent unit gets `not-reported`, `evidence-dead` or `unneeded`. Commands that write refuse an old-format state and point at `abbd migrate`.

`abbd migrate` runs once per repository and can be re-run on a copy.

1. Load the old state, run the registered extraction, and fold in the supplement (`--supplement`, default `.abbd/extractor/supplement.json`). With no supplement file it stops and says so, because it cannot tell supplement claims from reader claims.
2. Pair the supplement's units with the stored units as setup would. An INFERRED claim is the reader's when the supplement makes no claim on the same target (the unit, the edge, its parent, or the field). Otherwise it stays the supplement's, which is also the default for anything unclear. A parent claim is the supplement's only when the supplement places the unit in the same parent. A paired supplement unit stores its `supplement_ref`, and a human's parent stores the script's parent the extraction reports as its `over`.
3. Mark every non-script item that is not live in the tree now as dead, with reason `dead before migration`. Today's dead items stay dead, and nothing is revived.
4. Compare the graph before and after: present units, their parents, and each shown relation with its origin and deadness. Any difference stops the migration before it writes.
5. Save every unit in the new format and print the classification.

## 9. Invariants, as tests

- **Nothing silently disappears.** Over random sequences of sync, setup, record, retract and code edits, every unit present before a step is present after it or stores `absent_reason`, and every reader and human claim is still stored unless the step retracted it.
- **Idempotence.** `sync; setup F; sync; setup F` mints no ID, and the second `setup F` and the second sync change no byte. Retract and refresh run twice leave the bytes of one run.
- **Writer isolation.** Sync changes only script and group claims, statuses and absence. Setup changes only supplement claims plus what sync changes. Record, refresh and retract change only their writer's claims and masks on one unit.
- **One tree.** Resolved parents never form a cycle, and every present unit's parent is present or empty.

Regression tests come from history: 2404d01 (a reader's parent stays the reader's), f0d043b (a unit falls to its next parent claim when its parent goes), c05d076 (only the claim closing a cycle is skipped), setup keeping `abbd record` relations, live beating drifted, drift dying one revision later, and a retract that shows the claim again once its content changes.
