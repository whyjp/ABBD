# The ABBD reader

ABBD reads code with this reader. Both skills follow it for every reading step: a module's text, a slot note, a flow, a coupling, and setup's slices. It needs nothing but the abbd CLI and the agent's own tools.

pstack is an optional engine behind it. When `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36/skills/how/SKILL.md` exists (check with Glob; never assume), you may use pstack `how` to explore a module this reader calls complex, and pstack `swarm` to fan out setup's slices. Another pstack version counts as absent; say so in one line. Whichever engine explored, what you store passes the contract below, so rewrite how's output to it. Name who read when you store: `--reader "abbd reader"`, or `--reader "pstack how 0.15.36"` when how explored.

## Reading one module

1. Start from the brief. Run `abbd brief <MOD-ID>` and save its output as the file you will store. It holds the two sections with a TODO bullet per inside unit and per contract the module provides, then a `## Read first` section: the files the text is stamped against, entry files first, the module's contracts and neighbours, the couplings readers already recorded with their quotes, the notes and figures that cite it, and, for a stored text, what moved since it was written.
2. Read every file the brief lists, entry files first, then the files the most stored relations point at, then the rest. Leave them only to follow a contract one hop.
3. Explore before you write. Keep working notes, never stored, and fill each of these before you draft a line:
   - **Entry points**: what calls into the module from outside, by name and file.
   - **Lifecycle**: how it starts, how it stops, how it aborts or shuts down, and what it frees.
   - **Core types and state**: the types the module's functions take and return most, and the state they share or guard.
   - **Threads and pools**: every thread, task, pool, timer or callback it starts or runs on, and what runs where.
   - **Registration**: where the module learns its plugins, handlers or components at run time: a registry, a table, `dlopen`, a macro, a trait object, an injected bundle.
   - **Failure paths**: what it returns or raises when something fails, and what it retries.
   - **Key units**: the `Key units by fan-in` the brief lists. Name each in the text, or say in one line why it stays out.
   Note where the module stops, what is not obvious, open questions and the files you read.
   The text covers every one of these lists: name the lifecycle, the threads, the failure paths and the registration in Key Concepts, or say in one line that the module has none.
4. Simple or complex. Count the files and the inside units the brief lists, of every kind. A module with at most 12 files and 6 inside units is read in one pass by you. Past that, split it by inside units into 2 to 4 slices, give each slice to an `Explore` subagent with this file and the slice's anchors, tell it not to edit files, and write the result from their reports.

## Before you store: look for a counterexample

A universal or exclusive claim is the easiest one to get wrong. Before you keep a sentence that says "never", "always", "only", "each", "every", "all", "no", "none" or "cannot", search the module for a case that breaks it, with grep for the names involved and a read of each hit. Keep the sentence only when no counterexample turns up. When one does, narrow the claim to what holds, such as "on the main thread" or "except when", or drop it. A claim you could not check is worded as what you read, not as a law.

A role is a claim too. Before you write that a function loads, starts, frees or dispatches something, open its body: an empty or stub function, or one that only forwards, does not do it, so name the one that does. Before you write that a part goes where (a combinator before sinks, a dock a plugin adds), when something happens (while building, at startup, once ready), or that a function writes rather than only reads, find the line that shows it.

## What you store

**The module's text**, with `abbd understand <MOD-ID> <FILE> --reader "<WHO>"`:

- Exactly two sections, `## Overview` then `## Key Concepts`. Each section, and each `###` part inside one, holds under 2,000 characters. Key Concepts runs over first, so count it before you store, and split it under `###` headings when it does. How it works, where things live and gotchas go to flows, the drawing and slots, never into the text.
- Overview: one or two paragraphs, under 2,000 characters, in the present tense, with no dates, PR or issue numbers. It names what the module is responsible for, the contracts it provides and uses by their stored names, and one thing it does not do.
- Key Concepts: 3 to 10 bullets, each `**Name** (` followed by a code span naming the file or symbol, then `): ` and one to three sentences. Every inside unit the module's page draws is named in some bullet. When a module holds more than ten, one bullet names the units that work together, so the count stays at ten.
- Rewrite every TODO the brief left and delete its `## Read first` section. `abbd understand <MOD-ID> <FILE>` refuses a text with a TODO line or a section other than the two.
- Inline marks are `code` spans and `**bold**` only. No links, images, HTML or mermaid. A fence holds code only.
- `abbd understand <MOD-ID> <FILE>` reports each section over 2,000 characters, each phrase that reads as history, each inside unit the text never names (`unmentioned:`), and each path it names that is code of another unit (`names:`). Split each long section. Reword each flagged phrase that is history; a name or a version number that only reads as history may stay. Name each unmentioned unit. Name another unit's file by that unit instead.

**A coupling** the layout does not show: `abbd record relation <FROM> <KIND> <TO> --via <VIA> --phrase "<PHRASE>" --evidence <PATH:LINES:QUOTE>`, with the phrase rules of the /abbd skill.

**Order** (what one command does, a pipeline's stages): a flow, `abbd flow <KEY> <FILE>`.

**Meaning, failure, states and decisions**: a slot note, `abbd understand <ID>.<SLOT> <FILE> --evidence <PATH:LINES:QUOTE> --reader "<WHO>"`.

## Evidence

Every claim outside a module's text cites `PATH:LINES:QUOTE`, verbatim, a quote its file holds only once. `abbd sync` keeps it alive while the file holds it and lets it drift and die when the code changes. A module's text cites nothing and is stamped against every anchor instead, so a text about files you did not read is a claim nobody made.

## A change that spans modules

Run `abbd impact <FILES> --features`, read the anchors of the direct tier, and name the one fact the change is safe because of with its `file:line`. When that fact can be run, run it.

## At setup

- **Slices.** Partition step 1's candidates into at most five slices of related units. Give each slice to one `Explore` subagent with this file and the setup skill's Extraction format section, run locally so it sees uncommitted code, and told not to edit files. Each returns the extraction JSON for its slice: every unit the layout does not show with verbatim `evidence`, every coupling with `via`, `phrase` and verbatim `evidence`, and a unit inside a module with that module as `parent`. Merge the slices by `ref`. Rerun a slice that returns nothing once, then name it as a gap in the reply. Check `git status` after.
- **The lever.** Do the first module's extraction by hand, then write the extraction script, run it, and diff its output against the slices until every difference is explained. The script is the artifact a rerun proves.
