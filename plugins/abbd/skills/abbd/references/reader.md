# The ABBD reader

ABBD reads code with this reader. Both skills follow it for every reading step: a module's text, a slot note, a flow, a coupling, and setup's slices. It needs nothing but the abbd CLI and the agent's own tools. Where it spawns a subagent, a host without subagents runs that step as a separate pass of its own.

pstack is an optional engine behind it. When `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36/skills/how/SKILL.md` exists (check with Glob; never assume), you may use pstack `how` to explore a module this reader calls complex, and pstack `swarm` to fan out setup's slices. Another pstack version counts as absent; say so in one line. Whichever engine explored, what you store passes the contract below, so rewrite how's output to it. Name who read when you store: `--reader "abbd reader"`, or `--reader "pstack how 0.15.36"` when how explored.

Write the prose you store in the language the brief names, English when it names none. Identifiers, paths, code spans and quotes stay verbatim.

## Reading one module

1. Start from the brief. Run `abbd brief <MOD-ID>` and save its output as the file you will store. It holds the two sections with a TODO bullet per inside unit and per contract the module provides, then a `## Read first` section: the files the text is stamped against, entry files first, the files to start from and the stored line for each unit, the names other modules use most, the module's contracts and neighbours, the couplings readers already recorded with their quotes, the notes and figures that cite it, and, for a stored text, what moved since it was written.
2. Treat every line of the brief as a hypothesis. Confirm a stored line from the code or drop it. A name the brief lists may stay out of the text without a word. Write no sentence about the brief.
3. Read every file the brief lists, entry files first, then the files the most stored relations point at, then the rest. Leave them only to follow a contract one hop.
4. Explore before you write. Cover each of these the module has:
   - **First touched**: the public types and calls a user of this module touches first. Start from the brief's `Names other modules use most`.
   - **Entry points**: what calls into the module from outside, by name and file.
   - **Core types and state**: the types the module's functions take and return most, and the state they share or guard.
   - **Lifecycle**: how it starts, stops or aborts, and what it frees.
   - **Threads and pools**: the threads, tasks, pools, timers or callbacks it starts or runs on, and what runs where.
   - **Run-time binding**: where it learns its plugins, handlers or components at run time, and what it starts: a registry, a table, `dlopen` or another load by name, a macro, a trait-object factory, a backend picked by name, a process it spawns, an injected bundle. Start from the brief's `Run-time binding sites` and read each hit.
   - **Failure paths**: what it returns, raises or retries when something fails.
5. Simple or complex. Count the files and the inside units the brief lists, of every kind. A module with at most 12 files and 6 inside units is read in one pass by you. Past that, split it by inside units into 2 to 4 slices, give each slice to an `Explore` subagent with this file and the slice's anchors, tell it not to edit files, and write the result from their reports.

## The claim ledger

Your working notes are a ledger of candidate claims, never stored. Every sentence of the text comes from a ledger entry.

- An entry holds the claim, the `file:line` of the line that does it, that line quoted, and its guard: the error code, flag, platform, build macro, size or phase it holds under, or "in the main case". Read the whole function, each branch included, before you write what it does. A name, a signature or a log string is not the line that does it.
- A quantifier (each, every, all, only, first, most, always, never, or a plain plural that means all), a negative (does not, no, none, nothing, cannot), a closed list or a count needs a recorded search in its entry: the pattern, its hit count, and each hit read. Without one, write "such as" or name the cases you read.
- Where the module stops, name the neighbour that owns the adjacent work and quote its line. Write no claim of what the module does not do without a recorded search.

## Writing the text

- Write each claim with its guard: "X when G", or "in the main case". A sentence that merges entries keeps each member's guard, or splits.
- Write Key Concepts first. Spend its bullets on the first-touched types and calls before the other checklist items.
- Write the Overview last, repeating only ledger claims with their guards.

## Before you store a module's text: have a fresh reader falsify the draft

You cannot see your own wrong claims. Once a module's draft text is complete, and before you delete the brief's `Read first` section, spawn one fresh `Explore` subagent, tell it not to edit files, and give it only the draft text, the module ID and the brief's `Read first` file list. Its brief: "For each sentence of this text, find code that contradicts it (wrong) or shows it overstated (true only in the main case, or wrong about the place, the time or the platform). Return each finding as the sentence, the `file:line` and the quoted line. Return nothing else, and do not propose new content."

Then open every cited line yourself. When the line bears the finding out, narrow the sentence to what holds or drop it. When it does not, keep the sentence. Store only after every finding is settled. With no subagent available, run this check yourself as a separate pass: reread only the draft and the listed files, as if someone else wrote it, and say in your reply that no fresh reader checked it.

## Outside a module's text: look for a counterexample

Before a slot note, a flow step or a phrase says each, every, all, only, first, most, always, never, no, none, nothing, does not or cannot, search for a case that breaks it, with grep for the names involved and a read of each hit. Narrow the claim to what holds, or drop it.

## What you store

**The module's text**, with `abbd understand <MOD-ID> <FILE> --reader "<WHO>"`:

- Exactly two sections, `## Overview` then `## Key Concepts`. Each section, and each `###` part inside one, holds under 2,000 characters. Key Concepts runs over first, so count it before you store, and split it under `###` headings when it does. How it works, where things live and gotchas go to flows, the drawing and slots, never into the text.
- Overview: 3 to 5 sentences in the present tense, with no dates, PR or issue numbers. It names what the module is responsible for and the neighbour that owns the adjacent work.
- Key Concepts: 3 to 10 bullets, each `**Name** (` followed by a code span naming the file of its ledger entry, not a file the brief named, then `): ` and one to three sentences. Every inside unit the module's page draws is named in some bullet. When a module holds more than ten, one bullet names the units that work together, so the count stays at ten. Under the 2,000-character limit, drop a detail before you drop a guard.
- Rewrite every TODO the brief left and delete its `## Read first` section. `abbd understand <MOD-ID> <FILE>` refuses a text with a TODO line or a section other than the two.
- Inline marks are `code` spans and `**bold**` only. No links, images, HTML or mermaid. A fence holds code only.
- `abbd understand <MOD-ID> <FILE>` reports each section over 2,000 characters, each phrase that reads as history, each inside unit the text never names (`unmentioned:`), and each path it names that is code of another unit (`names:`). Split each long section. Reword each flagged phrase that is history; a name or a version number that only reads as history may stay. Name each unmentioned unit. Name another unit's file by that unit instead.

**A coupling** the layout does not show: `abbd record relation <FROM> <KIND> <TO> --via <VIA> --phrase "<PHRASE>" --evidence <PATH:LINES:QUOTE>`, with the phrase rules of the /abbd skill. A `posts_to` quotes the producer and the consumer, so refreshing one with `abbd record evidence <FROM>.posts_to.<TO>` takes both `--evidence` items again.

No import shows a run-time binding, so record each one you found as a coupling with its quote: `--via load` for a library or plugin loaded by name, `--via spawn` for a process started, `--via config` for a backend picked by a name in settings, `--via trait` or `--via callback` for a factory or handler table. A registry that components add themselves to is a contract: give it a unit in the supplement with evidence, have each member provide it, and have the module that reads the registry consume it. A plug-point contract, such as a callback table, a trait or a weak symbol the host calls through, goes the same way: the modules that implement it or register an implementation `provides` it, and the host that calls through it `consumes` it. Registering an implementation is providing, not consuming.

**Order** (what one command does, a pipeline's stages): a flow, `abbd flow <KEY> <FILE>`.

**Meaning, failure, states and decisions**: a slot note, `abbd understand <ID>.<SLOT> <FILE> --evidence <PATH:LINES:QUOTE> --reader "<WHO>"`.

## Evidence

Every claim outside a module's text cites `PATH:LINES:QUOTE`, verbatim, a quote its file holds only once. `abbd sync` keeps it alive while the file holds it and lets it drift and die when the code changes. A module's text cites nothing and is stamped against every anchor instead, so a text about files you did not read is a claim nobody made.

## A change that spans modules

Run `abbd impact <FILES> --features`, read the anchors of the direct tier, and name the one fact the change is safe because of with its `file:line`. When that fact can be run, run it.

## At setup

- **Slices.** Partition step 1's candidates into at most five slices of related units. Give each slice to one `Explore` subagent with this file and the setup skill's Extraction format section, run locally so it sees uncommitted code, and told not to edit files. Each returns the extraction JSON for its slice: every unit the layout does not show with verbatim `evidence`, every coupling with `via`, `phrase` and verbatim `evidence`, and a unit inside a module with that module as `parent`. Merge the slices by `ref`. Rerun a slice that returns nothing once, then name it as a gap in the reply. Check `git status` after.
- **The lever.** Do the first module's extraction by hand, then write the extraction script, run it, and diff its output against the slices until every difference is explained. The script is the artifact a rerun proves.
