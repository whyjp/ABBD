# The ABBD reader

ABBD reads code with this reader. Both skills follow it for every reading step: a module's text, a slot note, a flow, a coupling, and setup's slices. It needs nothing but the abbd CLI and the agent's own tools. Where it spawns a subagent, a host without subagents runs that step as a separate pass of its own.

Write everything you store in the language the first line of the `## Read first` of `abbd brief <MOD-ID>` names, keeping identifiers, paths and quotes verbatim.

pstack is an optional engine behind it. When `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36/skills/how/SKILL.md` exists (check with Glob; never assume), you may use pstack `how` to explore a module this reader calls complex, and pstack `swarm` to fan out setup's slices. Another pstack version counts as absent; say so in one line. Whichever engine explored, what you store passes the contract below, so rewrite how's output to it. Name who read when you store: `--reader "abbd reader"`, or `--reader "pstack how 0.15.36"` when how explored.

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
   The text covers every one of these lists: name the lifecycle, the threads, the failure paths and the run-time binding in Key Concepts, or say in one line that the module has none.
4. Simple or complex. Count the files and the inside units the brief lists, of every kind. A module with at most 12 files and 6 inside units is read in one pass by you. Past that, split it by inside units into 2 to 4 slices, give each slice to an `Explore` subagent with this file and the slice's anchors, tell it not to edit files, and write the result from their reports.

## Run-time pass

Run this pass when `abbd status` prints a `runtime:` line. `abbd runtime` lists each spawn site, channel pair, registration and impl that the CLI's catalog matched by name and the state does not cite yet. Each line is a lead, never a fact. Read its code before you store anything, and leave a line that does not hold.

A spawn site is the line that creates a new execution context: the create or spawn call itself, or a call to a project wrapper whose caller passes in the work to run. A wrapper is a function whose body hands one of its own parameters, as the work, to a catalog spawn call or to another wrapper, two levels deep at most. Its line names the wrapper as the API. A call to a function that starts a thread, task or pool on a fixed entry function of its own, such as a pool's constructor or an `open` that starts its worker, is not a spawn site: that context's site is the create call inside it.

`abbd runtime --records` prints the same sites as JSON lines. In each, `module` is the owning unit, or empty where no unit anchors the file. `to` is `{name, def}`: the name the call runs, and `def`, the `path:line` of that name's one definition. An unresolved target is `{}`. A call through a wrapper also carries `wrapper`, the wrapper's definition as `path:line`, then that of the wrapper it calls, down to the catalog call. Read those bodies to confirm the work reaches the spawn.

1. **A spawn site** (`open EXEC`). Open the function the line names after `runs`, or the closure it passes, and read its loop. Set `run` from that loop: `tick` for a periodic loop, `wait` for one that blocks on a queue or a condition, `ready` for select, epoll or an event loop, `complete` for a completion queue, `steal` for a work-stealing pool such as tokio's multi-thread runtime, `inline` when the work runs on its caller. Set the count to `1`, to `N` when one site starts a fixed number, or to `pool`. Store an `EXEC` in the supplement with the owner the line names as `parent` and the spawn line as `evidence`. One site that starts N threads is one `EXEC`. A wrapper that only forwards to another spawn call is not an `EXEC` of its own. Store the `EXEC` at the call that names what runs.
2. **A channel pair** (`open posts_to`). The line quotes a push and a pop on one channel. Read both. The push is the producer, and the context whose loop pops is the target `EXEC`. Store `posts_to` from the producer's module or `EXEC` to that `EXEC`, with `via` `queue`, `channel`, `signal` or `proxy`, a phrase that names what crosses, and both quotes. The catalog pairs ends only by name inside one module, so find the pairs that cross modules yourself: follow where the channel is made and where each end is passed.
3. **What runs where.** For each `EXEC`, find the calls its entry function makes into other modules' code. Store `runs_on` from each such module to the `EXEC`, quoting the call. A registration line (`open runs_on`) quotes where a handler is handed to a context. Quote the line that calls the handler too.
4. **An impl** (`open provides`). The line quotes code that implements a contract the host calls through. Store it as `provides`, per the plug-point rule under What you store.
5. **A counterexample.** Before you store a `runs_on`, grep for the other callers of the same function. When it is also called on another thread, store a `runs_on` to each context, or none when you cannot tell which.

Rerun setup with the supplement. `abbd runtime` drops each line the state now cites. A line you left stays listed, and the `runtime:` count never fails a check.

## Before you store: look for a counterexample

A universal or exclusive claim is the easiest one to get wrong. Before you keep a sentence that says "never", "always", "only", "each", "every", "all", "no", "none" or "cannot", search the module for a case that breaks it, with grep for the names involved and a read of each hit. Keep the sentence only when no counterexample turns up. When one does, narrow the claim to what holds, such as "on the main thread" or "except when", or drop it. A claim you could not check is worded as what you read, not as a law.

A role is a claim too. Before you write that a function loads, starts, frees or dispatches something, open its body: an empty or stub function, or one that only forwards, does not do it, so name the one that does. Before you write that a part goes where (a combinator before sinks, a dock a plugin adds), when something happens (while building, at startup, once ready), or that a function writes rather than only reads, find the line that shows it.

## Before you store a module's text: have a fresh reader falsify the draft

You cannot see your own wrong claims. Once a module's draft text is complete, and before you delete the brief's `Read first` section, spawn one fresh `Explore` subagent, tell it not to edit files, and give it only the draft text, the module ID and the brief's `Read first` file list. Its brief: "For each sentence of this text, find code that contradicts it (wrong) or shows it overstated (true only in the main case, or wrong about the place, the time or the platform). Return each finding as the sentence, the `file:line` and the quoted line. Return nothing else, and do not propose new content."

Then open every cited line yourself. When the line bears the finding out, narrow the sentence to what holds or drop it. When it does not, keep the sentence. Store only after every finding is settled. With no subagent available, run this check yourself as a separate pass: reread only the draft and the listed files, as if someone else wrote it, and say in your reply that no fresh reader checked it.

## What you store

**The module's text**, with `abbd understand <MOD-ID> <FILE> --reader "<WHO>"`:

- Exactly two sections, `## Overview` then `## Key Concepts`. Each section, and each `###` part inside one, holds under 2,000 characters. Key Concepts runs over first, so count it before you store, and split it under `###` headings when it does. How it works, where things live and gotchas go to flows, the drawing and slots, never into the text.
- Overview: one or two paragraphs, under 2,000 characters, in the present tense, with no dates, PR or issue numbers. It names what the module is responsible for, the contracts it provides and uses by their stored names, and one thing it does not do.
- Key Concepts: 3 to 10 bullets, each `**Name** (` followed by a code span naming the file or symbol, then `): ` and one to three sentences. Every inside unit the module's page draws is named in some bullet. When a module holds more than ten, one bullet names the units that work together, so the count stays at ten. The 2,000-character limit wins over coverage: group units, key units and the lifecycle, thread and failure lists into shared bullets before you add one.
- Rewrite every TODO the brief left and delete its `## Read first` section. `abbd understand <MOD-ID> <FILE>` refuses a text with a TODO line or a section other than the two.
- Inline marks are `code` spans and `**bold**` only. No links, images, HTML or mermaid. A fence holds code only.
- `abbd understand <MOD-ID> <FILE>` reports each section over 2,000 characters, each phrase that reads as history, each inside unit the text never names (`unmentioned:`), and each path it names that is code of another unit (`names:`). Split each long section. Reword each flagged phrase that is history; a name or a version number that only reads as history may stay. Name each unmentioned unit, or say in one line why a key unit stays out. Name another unit's file by that unit instead.

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
