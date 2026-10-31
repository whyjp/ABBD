---
name: abbd-mode
description: Run a pstack poteto-mode playbook inside an ABBD turn, so every change is gated against the human-approved intended topology. Use for /abbd-mode, "abbd mode", or work in a repository that ABBD has aligned.
disable-model-invocation: true
argument-hint: "[request]"
---

# ABBD mode

`<pstack>` is `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36`. ABBD is pinned to that version. Open files under `<pstack>` with the Read tool, because the shell may not reach outside the project. If `<pstack>/skills/poteto-mode/SKILL.md` does not exist, stop and tell the user to install pstack 0.15.36.

This skill is a sibling of poteto-mode. Read `<pstack>/docs/harness/claude-code.md`, then `<pstack>/skills/poteto-mode/SKILL.md` in full, and follow it: its non-negotiables, principles, autonomy, subagent defaults and reply rules all apply. Run the playbook it matches from `<pstack>/skills/poteto-mode/playbooks/` unchanged. Never copy a playbook. This skill only adds the turn around it and one check between its steps.

The session ID is `${CLAUDE_SESSION_ID}`. Pass it as `--session` to every command below. The CLI owns every gate and state change. When a command exits 2, a human decides next. Never work around it.

## Commands that are not yours

While the mode is on, a hook denies the abbd commands approve, align, confirm, setup and mode off, and writes to `.abbd/intended.yaml`, `.abbd/project.yaml`, `.abbd/decisions/` and `.abbd/changes/`. These belong to the human. When one is needed, stop and give the human the exact command. Read `.abbd/` files with the Read tool. A shell command that names those files is denied too.

The one setup you run is a greenfield project's first script registration, when `abbd turn diff --session ${CLAUDE_SESSION_ID}` says `project.yaml names no extraction script`. Write the script as the abbd-setup skill's code-only step 3 says, then register it with `abbd setup -- <interpreter> .abbd/extractor/extract.<ext>`.

## Route

Match the request to a poteto-mode playbook as poteto-mode says. Then:

| Playbooks | Turn |
| --- | --- |
| Bug fix, Perf issue, Hillclimb, Feature, Refactoring, Prototype, Visual parity, Authoring or modifying a skill, Eval, Autonomous run, figure-it-out | Full turn: start, declare, diff between steps, end |
| Investigation, Runtime forensics, Trace forensics | Start and end only. The end closes it as CLOSED when the tree did not change |
| Multi-phase or multi-PR plan | One full turn per phase |
| Session pickup, Pause safely | No new turn. `abbd mode on --session ${CLAUDE_SESSION_ID}` resumes the open turn. Leave an unfinished turn open when pausing |
| Opening a PR, Babysit, Shipping, Worktree and simulator cleanup | Pass through. Any code edit they need, such as a CI fix, gets its own full turn first |
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
3. Open the turn with the user's request verbatim on stdin:

   ```
   abbd turn start --session ${CLAUDE_SESSION_ID} --seeds <IDS> --request -
   ```

   The output lists the computed impact: the seeds, the contracts they provide, and every consumer of those contracts.
4. Read and follow `<pstack>/skills/blast-radius/SKILL.md` over the request. Add each design unit it finds that the impact lacks. You can only add.

   ```
   abbd turn impact --session ${CLAUDE_SESSION_ID} --add <IDS>
   ```

5. Declare the IntentDelta: the units you will change and every topology change, as item keys (`new-relation:MOD-0001.depends_on.MOD-0002`, `removed-relation:...`, `contract-changed:IF-0001`, `unit-removed:MOD-0003`).

   ```
   abbd turn declare --session ${CLAUDE_SESSION_ID} --units <IDS> --changes <KEYS>
   ```

   Show the declaration and its output to the human in your reply before you work. Do not wait for them unless a line asks.
   - `A KEY` and `B PROCEED KEY`: proceed. B is recorded as an UNCONFIRMED ADR when the turn syncs.
   - `C APPROVED KEY`: a human allowed it for this turn. Proceed.
   - `C QUESTION KEY`, exit 2: stop before any edit. Give the human each `abbd approve <KEY>` line and end your reply. Declare again after they answer.
6. Design. When the declaration spans more than one design unit or has any change key, read and follow `<pstack>/skills/architect/SKILL.md`, with the impacted units' boundaries and contract signatures as fixed constraints. Otherwise skip it.
7. Hand the playbook its context: the impact, the signatures of the impacted contracts, and the declaration. Start the playbook.

## Between steps

After every playbook step that changed files:

```
abbd turn diff --session ${CLAUDE_SESSION_ID}
```

- Exit 0: continue. An `UNEXPECTED KEY` line is a relation change you did not declare. Undo it, or redeclare with it and read the new grade.
- Exit 2 (`stop:`): a change crosses the intended boundary. Stop the playbook. Run `abbd turn end --session ${CLAUDE_SESSION_ID}` to record the turn as BLOCKED, then give the human the `C BLOCKED` lines and two choices: `abbd approve <KEY>` or undoing the change.

When the declaration or a diff printed a grade B or C line, read and follow `<pstack>/skills/interrogate/SKILL.md` before verifying, scoped to the contracts and relations those lines name. Skip it otherwise. It is costly.

## End

1. Verify with the project's verify skill (`.claude/skills/verify-*/SKILL.md`). If the project has none, `<pstack>/skills/create-verification-skill/SKILL.md` makes one. Record the run against the current tree. The first run registers the command. A run that writes any file git does not ignore fails, so gitignore build and cache output first, such as `__pycache__/` for Python:

   ```
   abbd turn verify --session ${CLAUDE_SESSION_ID} -- <VERIFY-ARGV>
   ```

   Later runs reuse it with `abbd turn verify --session ${CLAUDE_SESSION_ID}`.
2. End the turn. This is the only way it completes, whatever the playbook says:

   ```
   abbd turn end --session ${CLAUDE_SESSION_ID}
   ```

   - `SYNCED` or `CLOSED`: done.
   - `DRAFT`, exit 2: verification is missing, failed, or older than the tree. Fix it, verify, and end again. If it cannot pass, report the DRAFT turn.
   - `BLOCKED`, exit 2: report the `C BLOCKED` lines to the human, each with its `abbd approve <KEY>` line.
3. Reply as the playbook and poteto-mode say. Add the turn ID and state, the gate lines, and the declared-but-not-done and done-but-not-declared lines from `abbd turn show`.

Explain a design unit or a change with `<pstack>/skills/teach/SKILL.md` or `<pstack>/skills/how/SKILL.md` only when the human asks.

## Subagents

Spawn every subagent inside a playbook step as `subagent_type: "abbd:abbd-agent"`, where poteto-mode says `poteto-agent`. Put this file's path, the poteto-mode path and `${CLAUDE_SESSION_ID}` in its brief. The routed skills (`how`, `why`, `interrogate`, `swarm`, `arena`) keep the types they prescribe. You own the turn. A subagent never starts, declares, verifies or ends it.
