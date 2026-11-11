---
name: abbd
description: Read, query and keep current the repository's ABBD blueprint, the architecture topology of the merged code in .abbd/. Use for /abbd, for a question about the architecture, topology, modules, contracts or couplings of a repository that has .abbd/, before changing code there, after the change to put the topology difference in the PR, and with abbd lint before and after a change to how the blueprint is drawn.
argument-hint: "[unit or question]"
---

# ABBD

**Reader.** ABBD reads code with its own reader, `references/reader.md` in this skill. Follow it for every reading step below. It needs only the abbd CLI, and it says when pstack may serve as an optional engine.

`.abbd/` holds the architecture topology of the merged code as its final state: top-level modules and the contracts between them, the units inside each module, and a reader's account of each module. `.abbd/blueprint/index.html` draws it for a human. `abbd` is a lever that extracts, checks evidence, keeps IDs, stores the topology, answers queries and renders the blueprint. It never judges, blocks or asks. If the repository has no `.abbd/`, run the abbd-setup skill first.

This skill works under whatever mode is active. It adds the blueprint to the work and leaves analysis, design, decisions and implementation to that mode and the skills it uses.

Every fact carries its origin: `EXTRACTED` (the project's script read it), `USER_STATED` (a human said it, with where), `INFERRED` (a reader found it, with a verbatim quote that keeps it alive). Weigh an answer by whether its evidence still holds: the script reads an extracted fact again at every sync, a reader's claim stands only on its quotes, and a text or note goes STALE when the code under it changes. `abbd explain <ID>` names the commit, and the pull request, that recorded each reader's coupling. A reader's quote is live, drifted (its file no longer holds it as quoted; the claim still counts and is flagged drifted) or dead (its file is gone, or it stayed drifted past one revision).

## Before work

1. Open `.abbd/blueprint/index.html` for the modules the request touches. It holds only the overview of the modules and the contracts between them. Each module has its own page (`<ID>.html`, which a click on its box opens) with its surroundings on the overview, its inside, its contracts, inside units, a reader's couplings and the module's text. Each flow, stack and table has its own page too, which the overview lists. `contracts.html` lists every contract with the module that owns it and the units that provide and use it, and `units.html` lists every unit with its files, for the browser's find. Run `abbd status` if you need to know whether the state matches the working tree.
2. Ask the topology before grepping the code. Each query takes what you have in hand: a unit ID or name, a file or directory path (`src/main/ipc/worktree-branch-name.ts`, or a bare file name such as `worktree-branch-name`), a symbol, or a few words such as `"plugin theme"`. When the query is not a unit's own ID or name, the answer first lists the units it matched, best first, with why: for a file, the units whose anchors or evidence hold it, innermost first. It then answers for the first. Pass an ID from that list to ask about another.
   - `abbd explain <QUERY>`: one unit at a glance, with its contracts, neighbours, inside units, a reader's couplings with their quotes, and the module's text with who read it, STALE when the code moved on.
   - `abbd path <A> <B>`: how two units connect, each hop with its kind, via and origin, ending with the path's weakest origin. A `contains` hop steps between a module and a unit inside it. Two units that only use the same contract are not connected through it. The path goes through the contract's provider instead, and when it cannot, the hop says "both consume X".
   - `abbd impact <QUERIES>`: what a change to those comma-separated units reaches. Read the direct tier closely and the far tier briefly. Starting from the files you will change finds the units to read first. `abbd impact <QUERIES> --features` groups the result by each feature it reaches, a part of a feature under the feature, and lists the reached contracts each feature uses with the entries it names, such as the endpoints a page calls.
3. Where the question needs the code itself, read it per `references/reader.md`, or per its section on a change that spans modules when the change spans several. Start from the units the queries named.
4. Record each coupling or unit a reader found that the state lacks, with a verbatim quote:
   - `abbd record relation <FROM> <KIND> <TO> --via <VIA> --phrase "<PHRASE>" --evidence <PATH:LINES:QUOTE>`
   - `abbd record unit <KIND> <NAME> --parent <MOD-ID> --evidence <PATH:LINES:QUOTE>`

   Give every coupling you record a `--phrase`. The blueprint draws it beside the line, the way a hand-drawn diagram labels an arrow. It says what crosses the coupling: an endpoint, a message, a query, a file or a command. Examples are `HTTP /api/v1`, `schema fetch`, `Cypher`, `NDJSON /ingest push`, `IsReady 조회 (health)` and `샘플 조회 SQL`. Never restate the via, in its own words or in others. An `import` edge is not `model types` or `imports`, and a `call` edge is not `function call`. Name the thing itself: `Unit, Evidence, Claim` for the types a package takes from another, `LoadUnits` for the loader a page calls, `status --check` for the command a skill runs. Never give many edges one generic phrase such as `shell command` or `render inputs`. If nothing more specific than the via crosses, give no phrase. Keep it to 4 words and 24 characters. Quote the code where the thing crosses, such as the call that sends the request or the line that runs the query, so the phrase goes when that code changes. An edge that already shows takes a phrase too, through the same command, whoever found it. The command then stores only the note and leaves the edge's claims as they were. On an edge that does not show yet it records your coupling and the note. When the edge stops showing, for example because the script stops reading it, the note names no line, and `abbd status --check` lists it as dangling until you retract it with `abbd retract <FROM>.<KIND>.<TO>.phrase --reason "<why>"`. A phrase never brings back a coupling a human retracted. Name the module-level couplings the overview draws first. A phrase you wrote is a reader's, INFERRED, and the blueprint marks it 판독 whatever the line's own origin. Give `--source` only when a human said the phrase.

   When the reader finds a stored INFERRED claim wrong, retract it in one step with `abbd retract <TARGET> --reason "<why>"`. The target is a unit ID, an edge such as `MOD-0001.depends_on.MOD-0002`, or `<ID>.parent`. It removes your own claim and hides the supplement's until that claim's content changes.
5. When a module's text in the blueprint is STALE and you read it again per `references/reader.md`, store the new Overview and Key Concepts with `abbd understand <MOD-ID> <FILE> --reader "<WHO>"`. Any module or feature takes a text, one inside another too. A text describes every file of its module and of the units under it at any depth, so a change inside a nested module stales its own text and the text of each module above it. The blueprint shows a text beside its module's inside, and a module that holds no units has none, so its text sits under its entry in its parent's inside. A text describes a submodule by the commit the superproject's index records, an ignored directory by its files on disk, and the lines around each place a quote is cited in a file the module only quotes, so the same words cited at two places count at both. Because abbd reads a submodule's recorded commit, an edit inside a submodule counts only once it is committed there and staged in the superproject. The text records one hash per anchor and per cited place. When a sync moved units out of the module, or between units inside it, and none of its code changed, `abbd understand <MOD-ID> --restamp` stamps the stored text again. abbd refuses it when the code of an anchor or a quote changed or is gone, or when the module now holds an anchor or a quote the text was not written against. A refusal names the anchor, so a change under a directory anchor names that directory. A text stored by ABBD 0.6 records no hashes, one stored by format 1 recorded a hash per file, and one stored by format 2 hashed one place of a quote cited at several. After the upgrade, sync keeps such a text current only when its old digest still matches and read every file and every cited place the new one reads. Otherwise sync says the text was checked against only part of its module, and it reads STALE until a reader reads the module again. A module's purpose and responsibility are stamped with its code too, and read STALE when it changes. Pass the one line it is responsible for with `--sentence "<TEXT>"` when you store its text, so the line is read again with it.

6. When your reading finds what a module or a contract means in more depth than its Overview holds, store it in that unit's slot with `abbd understand <ID>.<SLOT> <FILE> --evidence <PATH:LINES:QUOTE>`. Cite the lines the note rests on, each with a quote its file holds only once: the note is stamped with the lines around each quote, wherever the quote sits now, and abbd refuses a quote it could not follow. So it goes STALE when that code changes and not when other code of the unit does. Re-storing the same words on the same quotes after their code moved is refused, so read the code again first. When you read it again and the note still holds as written, as after a whitespace, gofmt or comment edit near a quote, stamp it again with `abbd understand <ID>.<SLOT> --reread-by <WHO>`, naming who read it. abbd keeps the words, the quotes and a human's why, and records who, the date and what had moved. It refuses a note that is current and one whose quote is no longer in its file exactly once, word for word. Never pass `--reread-by` for a note you did not read again against the code that moved. When a note no longer describes the code, or it is orphaned or misfiled, remove it with `abbd forget <ID>.<SLOT>`, by the name `abbd status` gives it. The slots are:
   - `semantics`: what the unit means, such as what a key identifies or when a value counts as current.
   - `failure`: how it fails, which failures it retries, and what a caller sees.
   - `states`: its states and what moves it between them, as a table with a header row (`| state | when | next |`).
   - `decisions`: one note per decision, as `<ID>.decisions.<KEY>`, saying what the code chose over what. When the human states why, record it with `abbd record fact <ID>.decisions.<KEY> --value "<why>" --source <REF>`. Storing the decision again on the same quotes keeps that why. Storing it on other quotes drops it, since the human did not speak about that code; ask again. Forgetting the decision removes its why with it.
   - `wire_sample`: a sample of what crosses the contract, in a fenced code block.
   - `entries`: one note per entry of a contract, as `<ID>.entries.<KEY>`, saying what the endpoint, method, flag or field is for and its caveats. `abbd explain <ID>` lists the keys. abbd refuses a key the contract does not list. The blueprint shows the note on the entry's row of the contract's table.

   The blueprint shows a module's notes beside its inside, and a contract's in the inside of the module that provides it. `abbd explain <ID>` prints each one with whether it is current. When the code no longer has what the note says, remove it with `abbd forget <ID>.<SLOT>`.

7. When your reading traces the order something happens in, such as what one command does, how a request crosses modules, or a pipeline's stages, store it as a flow with `abbd flow <KEY> <FILE>`. A flow answers one question. The blueprint draws it as numbered steps, each on the unit whose code runs it, and lists it on the overview and on each module it passes through. Write the FILE as frontmatter and a caption:

   ```markdown
   ---
   question: What does abbd sync do, in order?
   steps:
     - unit: MOD-0017
       does: Parse(stdout) → units, anchors and relations
       note: one line on a cache, a TTL or a skip rule, if any
       fails: one line on what stops or degrades the step, if any
       calls: [DATA-0002]
       evidence:
         - {path: cmd/abbd/main.go, lines: "300", quote: "ex, err := extract.Parse(bytes.NewReader(out))"}
   ---

   One or two sentences on what the flow covers.
   ```

   Take two to 16 steps in the order the code runs them, and keep `does` under 80 characters. Each step cites the lines that run it, each quote held once in its file and one at least in code the step's own unit holds, and goes STALE on its own when that code changes. `calls` names the units the step reaches beyond its own, and abbd refuses a call no stored relation carries, so record a missing coupling first. `abbd flow <KEY>` prints the stored flow as a draft to edit and store again. A human states a flow with `--source <REF>`. A reader then replaces only its STALE steps, and keeps each current human step exactly as `abbd flow <KEY>` prints it. When the step's unit, or a contract it provides, anchors a symbol, give that quote `symbol: <NAME>`. abbd uses the name to choose the unit whose code holds the quote and does not check it, so name only the symbol whose code holds the quote. A reader's step that repeats a STALE step's words, in any case or spacing, is refused while it cites a file whose code changed under them, even after the step left the flow, so read that code and say what it does now. Delete `.abbd/flows/<KEY>.md` when the code no longer runs that way.

   Write the text as the code is now, in the present tense. Leave out dates, issue and PR numbers, and tags such as "migrated from X", because git keeps the history. Keep each section under 2,000 characters of prose, and split a longer one under `###` headings or into shorter sections. `abbd understand <MOD-ID> <FILE>` and `abbd status` report each section over that length and each phrase that reads as history. Neither fails the check, because a version number or a name can read as history too. `abbd understand <MOD-ID> <FILE>` refuses only a text whose first section, the summary a page shows before it folds the rest, runs over 2,000 characters.

8. When your reading finds the layers of a part of the code, such as packages that each use only the ones below them, store them as a stack with `abbd stack <KEY> <FILE>`. A stack answers one question. The blueprint draws it as bands from the top down, each a named layer with its role and the units it holds, and lists it on the overview and on each module whose units it holds. Write the FILE as frontmatter and a caption:

   ```markdown
   ---
   question: What are the layers of the abbd CLI?
   layers:
     - name: commands
       role: parses one command line, runs its verb, and renders the blueprint after a write
       units: [MOD-0001]
       evidence:
         - {path: cmd/abbd/main.go, quote: "var commands = map[string]command{"}
     - name: store
       role: reads and writes the .abbd directory
       units: [MOD-0041]
       evidence:
         - {path: internal/store/store.go, quote: "// Package store reads and writes the .abbd directory."}
   ---

   One or two sentences on what keeps the layers apart.
   ```

   Take two to 8 layers from the top down, with 16 units at most between them. Give each layer a short name and a one-line `role` under 80 characters. A layer holds each unit it names and every unit inside it, so name a unit in one layer only. abbd refuses a unit named in two layers, and a unit named in one layer while a unit that holds it is named in another. Each layer cites the lines that show its role, such as a package's doc comment, each quote held once in its file and one at least in code its own units hold. It goes STALE on its own when that code changes. `abbd stack <KEY>` prints the stored stack as a draft to edit and store again. A human states a stack with `--source <REF>`, and a reader then replaces only its STALE layers, as for a flow. When a unit moves and two layers come to hold it, status names both, and a reader may replace those layers too. A breach is a stored `consumes` or `depends_on` from a unit a lower layer holds up to a unit a higher layer holds. A lower unit that `provides` a contract homed higher up is no breach. abbd reports each breach when it stores the stack and in `abbd status`, and the blueprint draws it. A breach is debt in the code or a layer drawn wrong. Say which in the reply, and do not reorder the layers only to hide it. Delete `.abbd/stacks/<KEY>.md` when the code no longer layers that way.

9. When your reading answers which part does what to which thing, such as which command reads and writes which file, or which module handles which part of a request, store it as a table with `abbd table <KEY> <FILE>`. A table answers one question. The blueprint draws it as a grid, one row per unit, and lists it on the overview and on each module whose units head its rows. Write the FILE as frontmatter and a caption:

   ```markdown
   ---
   question: Which part of the abbd CLI reads and writes which .abbd file?
   columns:
     - {name: unit files, unit: DATA-0004}
     - {name: state.yaml, unit: DATA-0006}
   rows:
     - unit: MOD-0004
       cells:
         unit files: rewrites every unit file from the reconciled extraction
         state.yaml: reads the ID high-water mark, then writes revision and fingerprint
       evidence:
         - {path: cmd/abbd/main.go, symbol: syncActual, quote: "if err := st.SaveUnits(res.Units); err != nil {"}
         - {path: cmd/abbd/main.go, symbol: syncActual, quote: "return res, st.SaveState(state)"}
     - unit: MOD-0002
       cells:
         unit files: compares the stored units with the base revision's
       evidence:
         - {path: cmd/abbd/diff.go, symbol: diff, quote: "was, err := e.st.UnitsAt(base)"}
   ---

   One or two sentences on what the grid shows.
   ```

   Take two to 8 columns and two to 16 rows. Give each column a short label of 32 characters at most, with no spaces around it, and give it `unit:` when it stands for a stored unit, such as a contract. Each row is a stored unit, named in one row only. Fill a cell with one line under 80 characters on what the row's unit does, and leave a column out of `cells` when the cell is empty. Each row cites the lines that show its cells, each quote held once in its file and one at least in code its own unit holds. It goes STALE on its own when that code changes. `abbd table <KEY>` prints the stored table as a draft to edit and store again. A human states a table with `--source <REF>`, and a reader then replaces only its STALE rows, as for a flow, and keeps the human's columns. A reader may drop the unit of a column whose unit left the state, or point the column at another unit, and abbd says it changed the human's head. abbd reads each cell under a unit's column against the stored relations. A filled cell with no relation from the row's unit to the column's unit is unbacked, and an empty cell over such a relation is unsaid. abbd reports both when it stores the table and in `abbd status`, and the blueprint marks them. Neither fails the check. For an unbacked cell, record the coupling with `abbd record relation <FROM> <KIND> <TO> --evidence <PATH:LINES:QUOTE>` when the code shows it, or read the cell again. For an unsaid one, fill the cell when the row's code makes that use. Say in the reply which you did. Delete `.abbd/tables/<KEY>.md` when the question no longer fits the code.

## What the human states

When the human states a purpose, a responsibility or a coupling in the conversation, record it with where they said it. `<REF>` is the PR URL, or `conversation <date>` before a PR exists.

- `abbd record fact <ID.FIELD> --value "<text>" --source <REF>`, where the field is `purpose` or `responsibility`
- `abbd record relation <FROM> <KIND> <TO> --source <REF>`, for a coupling the code does not show
- `abbd retract <TARGET> --reason "<why>" --source <REF>`, when the human says a reader's or the supplement's claim is wrong

## After work

1. Run `abbd sync` so the state and the blueprint match the changed code, then `abbd diff`. When sync reports drifted evidence, run `abbd evidence`. For each drifted item under the paths you changed, refresh it with `abbd record evidence <TARGET> --evidence <PATH:LINES:QUOTE>` if the claim still holds, or retract it if it does not. A drifted item dies at the first sync of a later revision, so this PR is where it gets fixed.
2. Put the topology difference in the reply and in the PR description under its own heading, in plain words: which units came and went, which contracts changed shape or code, which relations between modules appeared or went, which couplings a reader added.
3. Commit `.abbd/` with the code, so the PR shows the blueprint's change. The blueprint's diff stays where the code diff is. A change inside one module rewrites that module's page and `graph.json`, and moving a quote's lines alone rewrites no page. A change to the lines around a quote marks its claim STALE, and the STALE mark lands on the pages that show the claim, the overview's count of a figure's STALE parts included. A blueprint page that changed far from the code you touched is worth a look. The blueprint draws only the current state. To set one module's page beside the one at the base, run `abbd diff --unit <ID> --page` and open `.abbd/compare/index.html`, which git ignores. `--base HEAD` compares against the last commit, the blueprint as it was before this session's uncommitted edits. Every write keeps `.abbd` as it was before its first change since the last commit in `.abbd/compare/prev/`, so `--base prev` compares against the blueprint from before the edits since the last commit. The first write after a commit takes a new copy, so a session that commits along the way compares against its last commit. `abbd review` puts in one place what a reviewer reads: the change, the units whose code changed, who uses each contract or entry it touched and the feature that holds them, what the change reaches directly, and what is worth a look, such as a coupling only a reader found or a text gone STALE. It exits 0 and judges nothing. Before you change code, declare the units the task means to touch with `abbd scope <IDS> --why "<TEXT>"`, so review names what the change touched outside them and what it left alone. The scope stays out of git. `abbd review --markdown` writes the same as one pull request comment, with a drawing of the touched modules, and `.github/workflows/abbd-review.yml` in the ABBD repository shows a step that posts it and updates it on each push.

A sync rewrites the lines that tie the state to one tree in `.abbd/state.yaml` alone: the revision, the fingerprint, the hash of the inputs the blueprint was rendered from and the hash of the files that render wrote. The blueprint carries none of them, and it reads STALE whenever its files are not the ones the state names: a page missing, added, edited by hand or checked out from an older render. Two branches that synced unrelated changes therefore conflict in that file at most. Resolve a merge or a rebase by running `abbd sync` on the merged tree and committing `.abbd/`. Sync reads a state that still holds conflict markers, keeps the higher ID counter of each kind and writes one state. With `.abbd/state.yaml merge=union` in `.gitattributes`, git merges that file without stopping, and `abbd status` reads the merged state STALE until that sync. A unit file both branches changed still conflicts the usual way. Resolve it by hand before the sync.

`abbd evidence` lists each quote and whether it is live, drifted or dead. `abbd status` counts drifted and dead items, and lists retractions and a human's parent the script has since moved. If a write says the state is in the old format, run `abbd migrate` once and commit the result. `abbd diff --check` exits 1 when a contract's signature changed, one of its entries was added, removed or changed shape, a contract was added or removed, or a relation between top-level modules appeared or disappeared, for a CI job that labels the PR. A contract's code change under the same signature is reported but does not fail it, and so is a unit coming to use one more entry of a contract it already used. The revision that first lists a contract's entries is one line, not one per entry.

## Gate a hook or CI

`abbd status --check` exits 1 when the change left something an agent can still bring current in the same PR: the state or the blueprint is STALE (`abbd sync` was not run or not committed), a module text is STALE (read it again per `references/reader.md` and store it with `abbd understand <MOD-ID> <FILE> --reader "<WHO>"`), a note is STALE (status names the quotes whose code changed, so read them again and `abbd understand <ID>.<SLOT> <FILE> --evidence <PATH:LINES:QUOTE>`, or `abbd understand <ID>.<SLOT> --reread-by <WHO>` when the note still holds as written), a flow's step is STALE (status names the step and why, so read that code again and `abbd flow <KEY> <FILE>`), a stack's layer is STALE (status names the layer and why, so read that code again and `abbd stack <KEY> <FILE>`), a table's row is STALE (status names the row and why, so read that code again and `abbd table <KEY> <FILE>`), or an evidence item drifted (refresh or retract it). Dead evidence is reported but does not fail it: a dead item is already out of every view, and only `abbd retract <TARGET> --reason "<why>"` would clear it. A breach, a stored relation that runs up a stack, is reported but does not fail it either. Only a design call clears it, by changing the code or drawing the layers again. An unbacked or unsaid table cell is reported and does not fail it. The readability report, long sections and phrases that read as history, never fails it. The state fingerprints untracked files too, so run the check on a clean checkout before any step writes files. It does not run the extraction script.

The check also exits 1 when the state or the blueprint is UNKNOWN, meaning abbd could not compare them, and the message names the cause. The state is UNKNOWN outside a git work tree or when git fails to fingerprint the tree, and no sync fixes that, so run the check in a git checkout. The blueprint is UNKNOWN when the state does not say what was rendered, and `abbd sync` writes it. The pages are committed with the state, so a fresh clone has them; only a checkout that lacks them reports NOT DRAWN, and it passes the check. An orphaned text, one whose module left the state or is no longer a module, is reported and skipped, because no command can restamp it. Delete its `.abbd/understanding/<MOD-ID>.md` to drop it. An orphaned note, one whose unit left the state, whose slot abbd no longer knows, or whose entry its contract no longer lists, is reported and skipped the same way. Status names its file and the `abbd forget <ID>.<SLOT>` that removes it.

```yaml
# lefthook.yml
pre-push:
  commands:
    abbd:
      run: abbd status --check
```

```yaml
# a GitHub Actions job, after actions/checkout
- uses: actions/setup-go@v5
- run: go install github.com/whyjp/abbd/cmd/abbd@main
- run: abbd status --check
```

## Hold the drawings to the criteria

`docs/blueprint-svg.md` sets out the drawing criteria under "Drawing criteria". Each has an ID. A hard one must hold. A target one is minimized. Read that section before you change how a drawing is laid out or drawn, and lint before and after.

- `abbd lint` checks every drawing of the blueprint. It prints each breach with its view, its criterion, the boxes, lines or chips it involves, what it measures against what is allowed, and for a line that turns too often the obstacle or the reason. It ends with the totals per view and in all.
- A `level` line names a full view drawn at the wrong level of abstraction, an overview of fewer than 4 or more than 12 modules, a nested module's page of fewer than 2 boxes, a top-level module's page of 1, a page of more than 16, or a view that draws more than two lines a box between its boxes past the lines into a box most of the view uses, which `--json` lists as `hubs`. A feature's page has no level. It never fails lint. Fix it in the units, as abbd-setup's step 1 chooses them, never in the renderer.
- It exits 1 on a hard breach past the debt `.abbd/lint-debt.yaml` records for its view, or, over every view, on a total past its ceiling there. A breach the view owes prints as `hard debt`.
- `abbd lint <VIEW>` checks the views whose name starts with VIEW or holds it as a word, so `abbd lint MOD-0098` checks that unit's inside and its focus views. `abbd lint --list` names every view.
- `abbd lint --json` prints the report with a `schema` version. Each breach carries `debt` and its boxes and chips by `id` and `name`. The report ends with the gate's `failures`.
- `abbd lint --record` writes what the drawings measure now to `.abbd/lint-debt.yaml`, as their debt and ceilings. It never raises a ceiling. With no debt file yet, it refuses while a view with hard breaches has a `level` line, unless `abbd lint --record --keep-level <VIEW>=<REASON>` keeps that view at its level and writes the reason to the debt file, which lint then prints beside the `level` line. A total above its ceiling but within the gate's slack keeps the ceiling it had.
- `abbd lint --root internal/screen/testdata/orca MOD-0098` lints a view of a fixture.

The target is the excess turns. They are each line's turns past what its two ends allow under B1, plus every jog under B2. A line in a clear column is allowed 0 turns, so a line that turns twice there has 2 excess turns though it has no step. The weighed sum is the crossings plus the excess turns plus 4 for each staircase step, what the chooser ranks a view's drawings by. A tee, a straight drop across a flat stretch of another line from the same place with no dot, counts once among the crossings. The placement debt is the breaches of P1, P2, H1, H2, O1, O2 and F1 added up. A change to the renderer is kept only through this gate:

1. No view has more hard breaches of a criterion than its debt records.
2. The weighed sum, the tees and the excess turns stay at or under their ceilings.
3. The placement debt rises by at most 2% of its ceiling.
4. The traced bends and the paths with more than two bends break ties. They have no ceiling.
5. Any view whose weighed sum rises by more than 8 gets a look by eye.

The phantom pairs and the climbing lines are held by J3 and rule 13, not by a ceiling. `TestLintFixtures` holds each fixture under `internal/screen/testdata` to its own `.abbd/lint-debt.yaml` with the same gate. It also fails when a debt or a ceiling stands above what the fixture measures. Change the renderer one thing at a time:

1. Write the fixtures' table with `ABBD_LINT_FIXTURES=<before.tsv> go test ./internal/screen -run TestLintFixtures -count=1`, and lint the views you mean to change.
2. Read their breaches. Pick one cause, such as a chip that stands past its box or a bus that turns twice in a clear column. Write down the one change you expect to remove it, and what you expect it to cost.
3. Make that one change, write `<after.tsv>` the same way, and run `python eval/lint/compare.py before <before.tsv> after <after.tsv>`. It totals both tables, says whether the after table keeps gates 2 and 3 against the before table, and lists every view whose hard breaches rose and every view that needs a look.
4. Keep the change only when the weighed sum falls, or holds while the tie-breakers fall, and gates 1 to 3 hold. Otherwise revert it, whatever else it improved. Look at every view `compare.py` lists as needing a look before you keep it.
5. Record the attempt, kept or reverted, as one row of `eval/lint/decisions.tsv`, with the hypothesis, the change, the totals before and after and the verdict.
6. Commit each kept change on its own. Lower the debts and ceilings with `abbd lint --record --root internal/screen/testdata/<fixture>` for each fixture, write the new table to `eval/lint/baseline.tsv`, and say in `docs/blueprint-svg.md` what moved.

Never raise a fixture's debt or ceiling, or loosen a criterion, to keep a change. A repository's own `.abbd/lint-debt.yaml` follows its code instead. When a change to the code adds units and lines, `abbd lint --record` there and commit the file with the change, so the review sees what the drawings owe. Run `go test ./internal/screen -count=1` before each commit, since the rule checks of `fixtures_test.go` still hold. Look at the changed views too. `ABBD_BLUEPRINTS=<dir> go test ./internal/screen -run TestEveryRealDrawingIsLegible -count=1` writes the fixtures' pages there. A number that falls on a drawing that reads worse means the measure counts the wrong thing, so say so instead of keeping the change.
