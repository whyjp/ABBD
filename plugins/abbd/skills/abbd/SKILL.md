---
name: abbd
description: Read, query and keep current the repository's ABBD blueprint, the architecture topology of the merged code in .abbd/. Use for /abbd, for a question about the architecture, topology, modules, contracts or couplings of a repository that has .abbd/, before changing code there, and after the change to put the topology difference in the PR.
argument-hint: "[unit or question]"
---

# ABBD

`<pstack>` is `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36`. Open files under it with the Read tool.

`.abbd/` holds the architecture topology of the merged code as its final state: top-level modules and the contracts between them, the units inside each module, and pstack how's account of each module. `.abbd/blueprint/index.html` draws it for a human. `abbd` is a lever that extracts, checks evidence, keeps IDs, stores the topology, answers queries and renders the blueprint. It never judges, blocks or asks. If the repository has no `.abbd/`, run the abbd-setup skill first.

This skill works under whatever mode is active. It adds the blueprint to the work and leaves analysis, design, decisions and implementation to that mode and the pstack skills it uses.

Every fact carries its origin: `EXTRACTED` (the project's script read it), `USER_STATED` (a human said it, with where), `INFERRED` (a pstack reader found it, with a verbatim quote that keeps it alive). Weigh an answer by its weakest origin. A reader's quote is live, drifted (its file no longer holds it as quoted; the claim still counts and is flagged drifted) or dead (its file is gone, or it stayed drifted past one revision).

## Before work

1. Open `.abbd/blueprint/index.html` for the modules the request touches: the overview, and each module's inside (`#m-<ID>`) with its contracts, inside units, a reader's couplings and how's text. Run `abbd status` if you need to know whether the state matches the working tree.
2. Ask the topology before grepping the code. Each query takes what you have in hand: a unit ID or name, a file or directory path (`src/main/ipc/worktree-branch-name.ts`, or a bare file name such as `worktree-branch-name`), a symbol, or a few words such as `"plugin theme"`. When the query is not a unit's own ID or name, the answer first lists the units it matched, best first, with why: for a file, the units whose anchors or evidence hold it, innermost first. It then answers for the first. Pass an ID from that list to ask about another.
   - `abbd explain <QUERY>`: one unit at a glance, with its contracts, neighbours, inside units, a reader's couplings with their quotes, and how's text, STALE when the code moved on.
   - `abbd path <A> <B>`: how two units connect, each hop with its kind, via and origin, ending with the path's weakest origin. A `contains` hop steps between a module and a unit inside it. Two units that only use the same contract are not connected through it. The path goes through the contract's provider instead, and when it cannot, the hop says "both consume X".
   - `abbd impact <QUERIES>`: what a change to those comma-separated units reaches. Read the direct tier closely and the far tier briefly. Starting from the files you will change finds the units to read first. `abbd impact <QUERIES> --features` groups the result by each feature it reaches, a part of a feature under the feature, and lists the reached contracts each feature uses with the entries it names, such as the endpoints a page calls.
3. Where the question needs the code itself, follow `<pstack>/skills/how/SKILL.md` unchanged, or `<pstack>/skills/blast-radius/SKILL.md` for a change that spans several modules. Start them from the units the queries named.
4. Record each coupling or unit a reader found that the state lacks, with a verbatim quote:
   - `abbd record relation <FROM> <KIND> <TO> --via <VIA> --evidence <PATH:LINES:QUOTE>`
   - `abbd record unit <KIND> <NAME> --parent <MOD-ID> --evidence <PATH:LINES:QUOTE>`

   When the reader finds a stored INFERRED claim wrong, retract it in one step with `abbd retract <TARGET> --reason "<why>"`. The target is a unit ID, an edge such as `MOD-0001.depends_on.MOD-0002`, or `<ID>.parent`. It removes your own claim and hides the supplement's until that claim's content changes.
5. When a module's text in the blueprint is STALE and how read it again, store the new Overview and Key Concepts with `abbd understand <MOD-ID> <FILE>`. Any module takes a text, a module inside another too. A text describes every file of its module and of the units under it at any depth, so a change inside a nested module stales its own text and the text of each module above it. The blueprint shows a text beside its module's inside, and a module that holds no units has none, so its text sits under its entry in its parent's inside. A text describes a submodule by the commit the superproject's index records, an ignored directory by its files on disk, and the lines around each place a quote is cited in a file the module only quotes, so the same words cited at two places count at both. Because abbd reads a submodule's recorded commit, an edit inside a submodule counts only once it is committed there and staged in the superproject. The text records one hash per anchor and per cited place. When a sync moved units out of the module, or between units inside it, and none of its code changed, `abbd understand <MOD-ID> --restamp` stamps the stored text again. abbd refuses it when the code of an anchor or a quote changed or is gone, or when the module now holds an anchor or a quote the text was not written against. A refusal names the anchor, so a change under a directory anchor names that directory. A text stored by ABBD 0.6 records no hashes, one stored by format 1 recorded a hash per file, and one stored by format 2 hashed one place of a quote cited at several. After the upgrade, sync keeps such a text current only when its old digest still matches and read every file and every cited place the new one reads. Otherwise sync says the text was checked against only part of its module, and it reads STALE until how reads the module again.

6. When how, why or your own reading finds what a module or a contract means in more depth than its Overview holds, store it in that unit's slot with `abbd understand <ID>.<SLOT> <FILE> --evidence <PATH:LINES:QUOTE>`. Cite the lines the note rests on, each with a quote its file holds only once: the note is stamped with the lines around each quote, wherever the quote sits now, and abbd refuses a quote it could not follow. So it goes STALE when that code changes and not when other code of the unit does. Re-storing the same words on the same quotes after their code moved is refused, so read the code again first. The slots are:
   - `semantics`: what the unit means, such as what a key identifies or when a value counts as current.
   - `failure`: how it fails, which failures it retries, and what a caller sees.
   - `states`: its states and what moves it between them, as a table with a header row (`| state | when | next |`).
   - `decisions`: one note per decision, as `<ID>.decisions.<KEY>`, saying what the code chose over what. When the human states why, record it with `abbd record fact <ID>.decisions.<KEY> --value "<why>" --source <REF>`. Storing the decision again on the same quotes keeps that why. Storing it on other quotes drops it, since the human did not speak about that code; ask again.
   - `wire_sample`: a sample of what crosses the contract, in a fenced code block.
   - `entries`: one note per entry of a contract, as `<ID>.entries.<KEY>`, saying what the endpoint, method, flag or field is for and its caveats. `abbd explain <ID>` lists the keys. abbd refuses a key the contract does not list. The blueprint shows the note on the entry's row of the contract's table.

   The blueprint shows a module's notes beside its inside, and a contract's in the inside of the module that provides it. `abbd explain <ID>` prints each one with whether it is current. Delete `.abbd/understanding/<ID>/<SLOT>.md` when the code no longer has what the note says.

7. When how or your own reading traces the order something happens in, such as what one command does, how a request crosses modules, or a pipeline's stages, store it as a flow with `abbd flow <KEY> <FILE>`. A flow answers one question. The blueprint draws it as numbered steps, each on the unit whose code runs it, and lists it on the overview and on each module it passes through. Write the FILE as frontmatter and a caption:

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

   Take two to 16 steps in the order the code runs them, and keep `does` under 80 characters. Each step cites the lines that run it, each quote held once in its file and one at least in the code of the step's unit or of a unit it calls, and goes STALE on its own when that code changes. `calls` names the units the step reaches beyond its own, and abbd refuses a call no stored relation carries, so record a missing coupling first. `abbd flow <KEY>` prints the stored flow as a draft to edit and store again. A human states a flow with `--source <REF>`. A reader then replaces only its STALE steps, and keeps each current human step exactly as `abbd flow <KEY>` prints it. A STALE step stored again unchanged is refused, so read its code first. Delete `.abbd/flows/<KEY>.md` when the code no longer runs that way.

   Write the text as the code is now, in the present tense. Leave out dates, issue and PR numbers, and tags such as "migrated from X", because git keeps the history. Keep each section under 2,000 characters of prose, and split a longer one under `###` headings or into shorter sections. `abbd understand <MOD-ID> <FILE>` and `abbd status` report each section over that length and each phrase that reads as history. Neither refuses the text or fails the check, because a version number or a name can read as history too.

## What the human states

When the human states a purpose, a responsibility or a coupling in the conversation, record it with where they said it. `<REF>` is the PR URL, or `conversation <date>` before a PR exists.

- `abbd record fact <ID.FIELD> --value "<text>" --source <REF>`, where the field is `purpose` or `responsibility`
- `abbd record relation <FROM> <KIND> <TO> --source <REF>`, for a coupling the code does not show
- `abbd retract <TARGET> --reason "<why>" --source <REF>`, when the human says a reader's or the supplement's claim is wrong

## After work

1. Run `abbd sync` so the state and the blueprint match the changed code, then `abbd diff`. When sync reports drifted evidence, run `abbd evidence`. For each drifted item under the paths you changed, refresh it with `abbd record evidence <TARGET> --evidence <PATH:LINES:QUOTE>` if the claim still holds, or retract it if it does not. A drifted item dies at the first sync of a later revision, so this PR is where it gets fixed.
2. Put the topology difference in the reply and in the PR description under its own heading, in plain words: which units came and went, which contracts changed shape or code, which relations between modules appeared or went, which couplings a reader added.
3. Commit `.abbd/` with the code, so the PR shows the blueprint's change.

A sync rewrites the lines that tie the state to one tree in `.abbd/state.yaml` alone: the revision, the fingerprint, the hash of the inputs the blueprint was rendered from and the hash of the files that render wrote. The blueprint carries none of them, and it reads STALE whenever its files are not the ones the state names: a page missing, added, edited by hand or checked out from an older render. Two branches that synced unrelated changes therefore conflict in that file at most. Resolve a merge or a rebase by running `abbd sync` on the merged tree and committing `.abbd/`. Sync reads a state that still holds conflict markers, keeps the higher ID counter of each kind and writes one state. With `.abbd/state.yaml merge=union` in `.gitattributes`, git merges that file without stopping, and `abbd status` reads the merged state STALE until that sync. A unit file both branches changed still conflicts the usual way. Resolve it by hand before the sync.

`abbd evidence` lists each quote and whether it is live, drifted or dead. `abbd status` counts drifted and dead items, and lists retractions and a human's parent the script has since moved. If a write says the state is in the old format, run `abbd migrate` once and commit the result. `abbd diff --check` exits 1 when a contract's signature changed, one of its entries was added, removed or changed shape, a contract was added or removed, or a relation between top-level modules appeared or disappeared, for a CI job that labels the PR. A contract's code change under the same signature is reported but does not fail it, and so is a unit coming to use one more entry of a contract it already used. The revision that first lists a contract's entries is one line, not one per entry.

## Gate a hook or CI

`abbd status --check` exits 1 when the change left something an agent can still bring current in the same PR: the state or the blueprint is STALE (`abbd sync` was not run or not committed), a module text is STALE (run how again and `abbd understand <MOD-ID> <FILE>`), a note is STALE (status names the quotes whose code changed, so read them again and `abbd understand <ID>.<SLOT> <FILE> --evidence <PATH:LINES:QUOTE>`), a flow's step is STALE (status names the step and why, so read that code again and `abbd flow <KEY> <FILE>`), or an evidence item drifted (refresh or retract it). Dead evidence is reported but does not fail it: a dead item is already out of every view, and only `abbd retract <TARGET> --reason "<why>"` would clear it. The readability report, long sections and phrases that read as history, never fails it. The state fingerprints untracked files too, so run the check on a clean checkout before any step writes files. It does not run the extraction script.

The check also exits 1 when the state or the blueprint is UNKNOWN, meaning abbd could not compare them, and the message names the cause. The state is UNKNOWN outside a git work tree or when git fails to fingerprint the tree, and no sync fixes that, so run the check in a git checkout. The blueprint is UNKNOWN when `blueprint/` holds no files or the state does not say what was rendered, and `abbd sync` writes both. An orphaned text, one whose module left the state or is no longer a module, is reported and skipped, because no command can restamp it. Delete its `.abbd/understanding/<MOD-ID>.md` to drop it. An orphaned note, one whose unit left the state, whose slot abbd no longer knows, or whose entry its contract no longer lists, is reported and skipped the same way, and status names its file.

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
