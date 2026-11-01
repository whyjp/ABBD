---
name: abbd-mode
description: Run a pstack poteto-mode playbook inside an ABBD turn, so every change is gated against the human-approved intended topology. Use for /abbd-mode, "abbd mode", or work in a repository that ABBD has aligned.
disable-model-invocation: true
argument-hint: "[request]"
---

# ABBD mode

`<pstack>` is `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36`. ABBD is pinned to that version. Open files under `<pstack>` with the Read tool, because the shell may not reach outside the project. If `<pstack>/skills/poteto-mode/SKILL.md` does not exist, stop and tell the user to install pstack 0.15.36.

This skill is a sibling of poteto-mode. Read `<pstack>/docs/harness/claude-code.md`, then `<pstack>/skills/poteto-mode/SKILL.md` in full, and follow it: its non-negotiables, principles, autonomy, subagent defaults and reply rules all apply. Run the playbook it matches from `<pstack>/skills/poteto-mode/playbooks/` unchanged, with one exception: its **Opening a PR** step waits until End below has run. Never copy a playbook. This skill only adds the turn around it, one check between its steps, and that hold on the PR.

The session ID is `${CLAUDE_SESSION_ID}`. Pass it as `--session` to every command below. The CLI owns every gate and state change. When a command exits 2, a human decides next. Never work around it.

## Commands that are not yours

While the mode is on, a hook denies the abbd commands approve, reject, align, confirm, couple, setup, serve and mode off, and any request to the running `abbd serve` screen. It also denies any write or delete under `.abbd/`, of the session state in `<git-dir>/abbd/sessions/`, and of the registered extraction script's files, by a tool or by a shell command such as `rm`, `cp`, a redirect, or `git checkout`, `restore`, `clean`, `stash` or `reset --hard` reaching `.abbd/`. These belong to the human or the CLI. When one is needed, stop and send the human to the screen: they run `abbd serve` in their own terminal and decide on the page, or run the exact command you give them. Read `.abbd/` files with the Read tool. The hook is a text check, not a boundary, so never look for a way around it.

The one setup you run is a greenfield project's first script registration, when `abbd turn diff --session ${CLAUDE_SESSION_ID}` says `project.yaml names no extraction script`. Write the script as the abbd-setup skill's code-only step 3 says, then register it with `abbd setup -- <interpreter> .abbd/extractor/extract.<ext>`.

## Route

Match the request to a poteto-mode playbook as poteto-mode says. Then:

| Playbooks | Turn |
| --- | --- |
| Bug fix, Perf issue, Hillclimb, Feature, Refactoring, Prototype, Visual parity, Authoring or modifying a skill, Eval, Autonomous run, figure-it-out | Full turn: start, declare, diff between steps, end |
| Investigation, Runtime forensics, Trace forensics | Start and end only. The end closes it as CLOSED when the tree did not change |
| Multi-phase or multi-PR plan | One full turn per phase |
| Session pickup, Pause safely | No new turn. `abbd mode on --session ${CLAUDE_SESSION_ID}` resumes the open turn. Leave an unfinished turn open when pausing |
| Opening a PR, Babysit, Shipping, Worktree and simulator cleanup | Pass through, but only for a turn that ended SYNCED or CLOSED. Any code edit they need, such as a CI fix, gets its own full turn first |
| Orchestrate, Autopilot-full, Autopilot-stack | Pass through. Their workers run in other worktrees, outside this session's mode, so their changes come back as external changes the next time the mode turns on |

## Start

1. Turn the mode on:

   ```
   abbd mode on --session ${CLAUDE_SESSION_ID}
   ```

   - `active`: go on.
   - `active: resuming CHG-... (OPEN)` or `(BLOCKED)`: an unfinished turn is yours now. Run `abbd turn show` and pick it up at the step it reached. Skip to Between steps, or End if the work is done.
   - `active: resuming CHG-... (DRAFT)`: finish it the same way when the request continues it. For new work, open a new turn. A DRAFT turn does not block one.
   - Exit 2 (`drifted`, `bootstrapped`, `uninitialized`): show the output to the human and stop. Realigning is theirs.
2. Find the seeds. Read `.abbd/intended.yaml` for the design units, contract signatures and relations, and `.abbd/architecture/` and `.abbd/features/` for each unit's name and anchors. The seeds are the design units the request touches.
3. Open the turn with the user's request verbatim on stdin, from a quoted heredoc (`<<'EOF'`):

   ```
   abbd turn start --session ${CLAUDE_SESSION_ID} --seeds <IDS> --request -
   ```

   The output lists the computed impact, one `impact` line per unit, direct items first. Each line ends with its tier, `direct` or `far`. Direct items are the seeds, the units that share a relation with a seed in either direction, and the consumers of a contract that shares one. Far items are the rest of the transitive closure. A line that says `(semantic, via V)` was reached through a coupling a reader found and no human has confirmed yet; read it like any other.
   - Read each direct item closely: its code, its contract signature, and how the change reaches it. The change most likely breaks there.
   - Check each far item briefly: open its anchor, confirm whether the change still reaches it through the path its line names, and move on. Read one closely only when that check finds the change does reach it.
4. Read and follow `<pstack>/skills/blast-radius/SKILL.md` over the request. Add each design unit it finds that the impact lacks. You can only add, and an added unit counts as direct.

   ```
   abbd turn impact --session ${CLAUDE_SESSION_ID} --add <IDS>
   ```

5. Scan each direct design unit for the couplings the extraction script cannot see: traits used through generics, callbacks, macros, config and flag plumbing, build features, and conventions two units must keep in step. Read and follow `<pstack>/skills/how/SKILL.md` over the unit, unchanged, or `<pstack>/skills/blast-radius/SKILL.md` when the change spans several units. ABBD adds only where the result goes. Register each coupling it finds that the impact does not show, with a short verbatim quote of the code it rests on:

   ```
   abbd propose relation <FROM> <KIND> <TO> --via <VIA> --evidence "<PATH>:<LINES>:<QUOTE>"
   ```

   `KIND` is `depends_on`, `consumes` or `provides`, and only a module or feature provides a contract. `VIA` is one of `import`, `trait`, `callback`, `macro`, `config`, `build-feature`, `data`, `convention` or `call`. When an end of the coupling is a unit the topology lacks, such as a shared type or a cargo feature, register it first with `abbd propose unit <KIND> "<NAME>" --evidence "<PATH>:<LINES>:<QUOTE>" --parent <MODULE-ID>`. A proposal stays INFERRED and UNCONFIRMED, never gates, and joins the impact at once; run `abbd turn impact --session ${CLAUDE_SESSION_ID}` to read it. Then record the scan, even when it found nothing:

   ```
   abbd turn scan --session ${CLAUDE_SESSION_ID} --unit <ID> --by how
   ```

   Use `--by blast-radius` for a unit you scanned that way. A direct design unit without a scan leaves a grade B card, `semantic-unscanned:<ID>`, when the turn ends.
6. Declare the IntentDelta: the units you will change and every topology change, as item keys (`new-relation:MOD-0001.depends_on.MOD-0002`, `removed-relation:...`, `contract-changed:IF-0001`, `unit-removed:MOD-0003`).

   ```
   abbd turn declare --session ${CLAUDE_SESSION_ID} --units <IDS> --changes <KEYS>
   ```

   Show the declaration and its output to the human in your reply before you work. Do not wait for them unless a line asks.
   - `A KEY` and `B PROCEED KEY`: proceed. B is recorded as an UNCONFIRMED ADR when the turn syncs.
   - `C APPROVED KEY`: a human allowed it for this turn. Proceed.
   - `C QUESTION KEY`, exit 2: stop before any edit. Tell the human the question waits on the `abbd serve` screen, give each `abbd approve <KEY>` line as the terminal form, and end your reply, leaving the turn open. Declare again after they answer.
   - `C REJECTED KEY by ADR-...: REASON`, exit 2: the human said no. Declare again without that key and plan around the reason.
7. Design. When the declaration spans more than one design unit or has any change key, read and follow `<pstack>/skills/architect/SKILL.md`, with the impacted units' boundaries and contract signatures as fixed constraints. Otherwise skip it.
8. Hand the playbook its context: the impact with its tiers, the signatures of the impacted contracts, and the declaration. Start the playbook.

## Between steps

After every playbook step that changed files:

```
abbd turn diff --session ${CLAUDE_SESSION_ID}
```

- Exit 0: continue. An `UNEXPECTED KEY` line is a relation change you did not declare. Undo it, or redeclare with it and read the new grade.
- Exit 2 (`stop:`): a change crosses the intended boundary. Stop the playbook. Run `abbd turn end --session ${CLAUDE_SESSION_ID}` to record the turn as BLOCKED, then give the human the `C BLOCKED` lines and send them to the `abbd serve` screen, where each one has Approve and Reject. The terminal forms are `abbd approve <KEY>` and `abbd reject <KEY> --reason "<why>"`.
- A `REJECTED KEY by ADR-...: REASON` line, from turn diff, turn show or turn end: the human refused that change. Undo it as the reason says, then diff again. Never declare it again in this turn.

When the declaration or a diff printed a grade B or C line, read and follow `<pstack>/skills/interrogate/SKILL.md` before verifying, scoped to the contracts and relations those lines name. Skip it otherwise. It is costly.

## End

1. Verify with the project's verify skill (`.claude/skills/verify-*/SKILL.md`). If the project has none, `<pstack>/skills/create-verification-skill/SKILL.md` makes one. Record the run against the current tree. When `.abbd/project.yaml` has no `verify` yet, the first run registers the command. A run that writes any file git does not ignore fails, so gitignore build and cache output first, such as `__pycache__/` for Python:

   ```
   abbd turn verify --session ${CLAUDE_SESSION_ID} -- <VERIFY-ARGV>
   ```

   Once `project.yaml` records a `verify`, from setup or an earlier turn, run `abbd turn verify --session ${CLAUDE_SESSION_ID}` without `--`.
2. End the turn. This is the only way it completes, whatever the playbook says:

   ```
   abbd turn end --session ${CLAUDE_SESSION_ID}
   ```

   - `SYNCED` or `CLOSED`: done. A `B PROCEED contract-touched:<ID>` line means the turn changed a consumed contract's code while its signature stayed the same. A `B PROCEED evidence-touched:<EDGE>` line means it edited the quoted lines of a coupling a human confirmed. Name each in the reply with what its meaning now is, so the human can judge it on the screen. A `B PROCEED semantic-unscanned:<ID>` line means a direct design unit went unscanned; say why in the reply.
   - `DRAFT`, exit 2: verification is missing, failed, or older than the tree. Fix it, verify, and end again. If it cannot pass, report the DRAFT turn.
   - `BLOCKED`, exit 2: report the `C BLOCKED` lines and send the human to the `abbd serve` screen to approve or reject each. When a `REJECTED` line comes back, undo that change, verify, and end again.
3. Ship. Only now run the playbook's **Opening a PR** step, and only when the turn ended `SYNCED` or `CLOSED`. Never open a PR on a `DRAFT` or `BLOCKED` turn. Commit `.abbd/` with the code, so the PR carries the turn record `.abbd/changes/<CHG>.md` and what the sync wrote to `.abbd/intended.yaml` and `.abbd/decisions/`.
4. Reply as the playbook and poteto-mode say. Add the turn ID and state, the gate lines, and the declared-but-not-done and done-but-not-declared lines from `abbd turn show`.

Explain a design unit or a change with `<pstack>/skills/teach/SKILL.md` or `<pstack>/skills/how/SKILL.md` only when the human asks.

## Subagents

Spawn every subagent inside a playbook step as `subagent_type: "abbd:abbd-agent"`, where poteto-mode says `poteto-agent`. Put this file's path, the poteto-mode path and `${CLAUDE_SESSION_ID}` in its brief. The routed skills (`how`, `why`, `interrogate`, `swarm`, `arena`) keep the types they prescribe. You own the turn. A subagent never starts, declares, verifies or ends it.
