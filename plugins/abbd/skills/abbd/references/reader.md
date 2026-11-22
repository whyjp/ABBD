# The ABBD reader

ABBD reads code with this reader. Both skills follow it for every reading step: a module's text, a slot note, a flow, a coupling, and setup's slices. It needs nothing but the abbd CLI and the agent's own tools. Where it spawns a subagent, a host without subagents runs that step as a separate pass of its own.

Write everything you store in the language the first line of the `## Read first` of `abbd brief <MOD-ID>` names, keeping identifiers, paths and quotes verbatim.

pstack is an optional engine behind it. When `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36/skills/how/SKILL.md` exists (check with Glob; never assume), you may use pstack `how` to explore a module this reader calls complex, and pstack `swarm` to fan out setup's slices. Another pstack version counts as absent; say so in one line. Whichever engine explored, what you store passes the contract below, so rewrite how's output to it. Name who read when you store: `--reader "abbd reader"`, or `--reader "pstack how 0.15.36"` when how explored.

## Language

A person reads every text you store to judge an agent's change, so write it in the project's language, the `language` of `.abbd/project.yaml`. That covers module texts, purposes, responsibilities, slot notes, decisions, phrases, the words and names of the overview record, and the texts of flows, stacks and tables. Keep code identifiers, paths, IDs and quotes verbatim. `abbd lint` names each stored text not yet in that language. Write such a text in it whenever you read it again, because it went STALE or you touched it. Never translate a text you did not read again.

## Words on lines

A phrase labels a coupling, and a word labels a line of the overview record. Each is a short verb or noun phrase. A zone and a stage of the overview take a name. The blueprint shows each at rest, so each follows these rules:

- Say what crosses: an endpoint, a message, a query, a file, a command or the types one part hands another. A line's word may say instead what the caller does with the other box. Name the thing itself.
- Say which symbol carries it. The phrase says what happens; where a function, a type or a channel in the code carries it, name that with `--symbol NAME`, up to 3, each a name the line you quote holds, such as `uv_async_send` for `완료 알림 전송`, so a reader of the line can find the code that does the carrying. A name you did not read in the quoted line is no symbol: quote the line that holds it. the command refuses a symbol the quotes do not hold, and `abbd lint` lists the live phrases that name none under `lint: symbols:`, which never fails.
- Never restate the via, in its own words or in others. An `import` edge is not `model types` or `imports`. A `call` edge is not `function call`. If nothing more specific than the via crosses, give no phrase.
- Never give many edges one generic phrase such as `shell command` or `render inputs`.
- Keep it to 4 words and 24 characters.
- Quote the code where the thing crosses, such as the call that sends the request or the line that runs the query, so the phrase goes when that code changes. A line's word comes from the line of code its `evidence` quotes. Translate what that code means; do not copy it.
- Write no quantifier (each, every, all, only, first, a plural that means all) and no negative (no, never, does not) unless the quoted line or a search you made shows it. A quote shows one case, so name that case.
- Write it in the project's language. An identifier may stand in it. `abbd lint` judges the words that read as prose: words that read as code, such as paths, calls, camelCase or capitals, do not count. A text passes when at least 30% of its prose words are in the language's script, so a bare Latin word such as `Matcher` fails. A phrase with no prose word at all passes. A word, a zone name or a stage name with no prose word fails, so a bare code word such as `LoadUnits` may stand as a phrase but is no word.
- In a Korean project, `HTTP /api/v1`, `IsReady 조회 (health)`, `샘플 조회 SQL`, `LoadUnits 호출` and `Unit, Evidence 타입` pass. In an English project, `schema fetch`, `NDJSON /ingest push` and `Unit, Evidence, Claim` pass. A Korean project fails those three, because all their prose words are Latin.

## Reading one module

1. Start from the brief. Run `abbd brief <MOD-ID>` and save its output as the file you will store. It holds the two sections with a TODO bullet per inside unit and per contract the module provides, then a `## Read first` section: the files the text is stamped against, entry files first, the module's contracts and neighbours, the couplings readers already recorded with their quotes, the notes and figures that cite it, and, for a stored text, what moved since it was written.
2. Read every file the brief lists, entry files first, then the files the most stored relations point at, then the rest. Leave them only to follow a contract one hop.
3. Explore before you write. Keep working notes, never stored, and fill each of these before you draft a line:
   - **Entry points**: what calls into the module from outside, by name and file.
   - **Lifecycle**: how it starts, how it stops, how it aborts or shuts down, and what it frees.
   - **Core types and state**: the types the module's functions take and return most, and the state they share or guard.
   - **Threads and pools**: every thread, task, pool, timer or callback it starts or runs on, and what runs where.
   - **Run-time binding**: where the module learns its plugins, handlers or components at run time, and what it starts: a registry, a table, `dlopen` or another load by name, a macro, a trait-object factory, a backend picked by name, a process it spawns, an injected bundle. Start from the brief's `Run-time binding sites` and read each hit.
   - **Failure paths**: what it returns or raises when something fails, and what it retries.
   - **Key units**: the `Key units by fan-in` the brief lists. Name each in the text, or say in one line why it stays out.
   Note where the module stops, what is not obvious, open questions and the files you read.
   The standard library, the runtime, system headers and the system environment the module uses are outside the repository: no unit, no relation, no contract. Name them in at most one line of the text. A third-party library the module uses is one `EXT` unit that `abbd libs` reads from the manifest, with one `depends_on` from the module: never record the library, its modules or its calls yourself. Where no `EXT` stands for it, because abbd does not read its manifest, name it in that same line. A sample, example, build artifact or test tool of the repository is its own unit but not the product: say so in its text, and the overview record marks it with `scope`.
   The text covers every one of these lists: name the lifecycle, the threads, the failure paths and the run-time binding in Key Concepts, or say in one line that the module has none.
4. Simple or complex. Count the files and the inside units the brief lists, of every kind. A module with at most 12 files and 6 inside units is read in one pass by you. Past that, split it by inside units into 2 to 4 slices, give each slice to an `Explore` subagent with this file and the slice's anchors, tell it not to edit files, and write the result from their reports.

## Before you store: look for a counterexample

A universal or exclusive claim is the easiest one to get wrong. Before you keep a sentence that says "never", "always", "only", "each", "every", "all", "no", "none" or "cannot", search the module for a case that breaks it, with grep for the names involved and a read of each hit. Keep the sentence only when no counterexample turns up. When one does, narrow the claim to what holds, such as "on the main thread" or "except when", or drop it. A claim you could not check is worded as what you read, not as a law.

A role is a claim too. Before you write that a function loads, starts, frees or dispatches something, open its body: an empty or stub function, or one that only forwards, does not do it, so name the one that does. Before you write that a part goes where (a combinator before sinks, a dock a plugin adds), when something happens (while building, at startup, once ready), or that a function writes rather than only reads, find the line that shows it.

## Before you store a module's text: have a fresh reader falsify the draft

You cannot see your own wrong claims. Once a module's draft text is complete, and before you delete the brief's `Read first` section, spawn one fresh `Explore` subagent, tell it not to edit files, and give it only the draft text, the module ID and the brief's `Read first` file list. Its brief: "For each sentence of this text, find code that contradicts it (wrong) or shows it overstated (true only in the main case, or wrong about the place, the time or the platform). Return at most 5 findings, the ones that falsify a sentence before the ones that only narrow it, each as the sentence, the `file:line` and the quoted line. Return nothing else, and do not propose new content."

Then open every cited line yourself. When the line bears the finding out, narrow the sentence to what holds or drop it. When it does not, keep the sentence. Store only after every finding is settled. With no subagent available, run this check yourself as a separate pass: reread only the draft and the listed files, as if someone else wrote it, and say in your reply that no fresh reader checked it.

## What you store

**The module's text**, with `abbd understand <MOD-ID> <FILE> --reader "<WHO>"`:

- Write the text as the code is now, in the present tense. Leave out dates, issue and PR numbers, and tags such as "migrated from X", because git keeps the history.
- Exactly two sections, `## Overview` then `## Key Concepts`. Each section, and each `###` part inside one, holds under 2,000 characters; split a longer one under `###` headings. The page shows the first section and folds the rest. Of the length rules, `abbd understand <MOD-ID> <FILE>` refuses only a text whose first section runs over 2,000 characters. Key Concepts runs over first, so count it before you store, and split it under `###` headings when it does. How it works, where things live and gotchas go to flows, the drawing and slots, never into the text.
- Overview: one or two paragraphs that open with a short summary of the module. It names what the module is responsible for, the contracts it provides and uses by their stored names, and one thing it does not do.
- Key Concepts: 3 to 10 bullets, each `**Name** (` followed by a code span naming the file or symbol, then `): ` and one to three sentences. Name the file the concept lives in or the unit's ID, such as `route.go` or `MOD-0042`, so the page sets the concept beside that unit's box. Every inside unit the module's page draws is named in some bullet. When a module holds more than ten, one bullet names the units that work together, so the count stays at ten. The 2,000-character limit wins over coverage: group units, key units and the lifecycle, thread and failure lists into shared bullets before you add one.
- Rewrite every TODO the brief left and delete its `## Read first` section. `abbd understand <MOD-ID> <FILE>` refuses a text with a TODO line or a section other than the two.
- Inline marks are `code` spans and `**bold**` only. No links, images, HTML or mermaid. A fence holds code only.
- `abbd understand <MOD-ID> <FILE>` reports each section over 2,000 characters, each phrase that reads as history, each inside unit the text never names (`unmentioned:`), and each path it names that is code of another unit (`names:`). Neither fails the check, because a version number or a name can read as history too. Split each long section. Reword each flagged phrase that is history; a name or a version number that only reads as history may stay. Name each unmentioned unit, or say in one line why a key unit stays out. Name another unit's file by that unit instead.

**A coupling** the layout does not show: `abbd record relation <FROM> <KIND> <TO> --via <VIA> --phrase "<PHRASE>" --evidence <PATH:LINES:QUOTE>`, with a phrase by "Words on lines" above. A `posts_to` quotes the producer and the consumer, so refreshing one with `abbd record evidence <FROM>.posts_to.<TO>` takes both `--evidence` items again.

No import shows a run-time binding, so record each one you found as a coupling with its quote: `--via load` for a library or plugin loaded by name, `--via spawn` for a process started, `--via config` for a backend picked by a name in settings, `--via trait` or `--via callback` for a factory or handler table. A registry that components add themselves to is a contract: give it a unit in the supplement with evidence, have each member provide it, and have the module that reads the registry consume it. A plug-point contract, such as a callback table, a trait or a weak symbol the host calls through, goes the same way: the modules that implement it or register an implementation `provides` it, and the host that calls through it `consumes` it. Registering an implementation is providing, not consuming.

**Order** (what one command does, a pipeline's stages): a flow, `abbd flow <KEY> <FILE>`.

**Meaning, failure, states and decisions**: a slot note, `abbd understand <ID>.<SLOT> <FILE> --evidence <PATH:LINES:QUOTE> --reader "<WHO>"`.

## Evidence

Every claim outside a module's text cites `PATH:LINES:QUOTE`, verbatim, a quote its file holds only once. `abbd sync` keeps it alive while the file holds it and lets it drift and die when the code changes. A module's text cites nothing and is stamped against every anchor instead, so a text about files you did not read is a claim nobody made.

## A change that spans modules

Run `abbd impact <FILES> --features`, read the anchors of the direct tier, and name the one fact the change is safe because of with its `file:line`. When that fact can be run, run it.

## At setup

- **Slices.** Skip them only for a repository of fewer than 10,000 lines of non-test code, however many candidate modules it has: read it yourself in one pass, with the same evidence rules, and say in the reply that you did. Otherwise partition step 1's candidates into at most five slices of related units. Give each slice to one `Explore` subagent with this file and the setup skill's Extraction format section, run locally so it sees uncommitted code, and told not to edit files. Each returns the extraction JSON for its slice: every unit the layout does not show with verbatim `evidence`, every coupling with `via`, `phrase` and verbatim `evidence`, and a unit inside a module with that module as `parent`. Merge the slices by `ref`. Rerun a slice that returns nothing once, then name it as a gap in the reply. Check `git status` after.
- **The lever.** Do the first module's extraction by hand, then write the extraction script, run it, and diff its output against the slices until every difference is explained. The script is the artifact a rerun proves.
