---
name: abbd-mode
description: Work in poteto's pstack style with the repository's ABBD blueprint in hand, so the human sees and judges the architecture each change leaves. Use for /abbd-mode, "abbd mode", or work in a repository that has .abbd/.
disable-model-invocation: true
argument-hint: "[request]"
---

# ABBD mode

`<pstack>` is `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36`. Open files under it with the Read tool. If `<pstack>/skills/poteto-mode/SKILL.md` does not exist, stop and tell the user to install pstack 0.15.36.

This skill is a sibling of poteto-mode. Read `<pstack>/docs/harness/claude-code.md`, then `<pstack>/skills/poteto-mode/SKILL.md` in full, and follow it: its non-negotiables, principles, autonomy, subagents, playbooks and reply rules all apply unchanged. Analysis, design, decisions and implementation stay with pstack and the human. This skill adds three things: read the stored topology before the work, record what readers and the human establish, and show the topology difference at the end.

`abbd` is a lever. It extracts, checks evidence, keeps IDs, stores the topology and renders `.abbd/blueprint/index.html`. It never decides, blocks or asks. If the repository has no `.abbd/`, run the abbd-setup skill first.

## Start

1. Run `abbd sync` so the state matches the working tree.
2. Open `.abbd/blueprint/index.html` for the modules the request touches: the overview, and each module's inside (`#m-<ID>`) with its contracts, inside units, reader's couplings and how's text.
3. Run `abbd impact <IDS>` with the units the request touches. Read the direct tier closely and the far tier briefly.
4. For each direct module, follow `<pstack>/skills/how/SKILL.md` unchanged, or `<pstack>/skills/blast-radius/SKILL.md` when the change spans several modules. When the module's text in the blueprint is STALE, store how's new Overview and Key Concepts with `abbd understand <MOD-ID> <FILE>`.
5. Record each coupling or unit a reader found that the state lacks, with a verbatim quote: `abbd record relation <FROM> <KIND> <TO> --via <VIA> --evidence <PATH:LINES:QUOTE>` and `abbd record unit <KIND> <NAME> --parent <MOD-ID> --evidence <PATH:LINES:QUOTE>`.
6. Route to the playbook as poteto-mode says.

## During

The playbook runs as pstack says. A change to a contract's signature or meaning, or a new coupling between top-level modules, is a design decision. Raise it with the human the way poteto-mode's Autonomy section says, and let the human decide in the conversation.

Record what the human states, with where they said it:

- a purpose or responsibility: `abbd record fact <ID.FIELD> --value "<text>" --source <REF>`
- a coupling the code does not show: `abbd record relation <FROM> <KIND> <TO> --source <REF>`

`<REF>` is the PR URL, or `conversation <date>` before a PR exists.

## End

1. Run `abbd sync`, then `abbd diff`.
2. Put the topology difference in the reply and in the PR description, under its own heading, in the human's words: which units came and went, which contracts changed, which couplings between modules appeared.
3. Commit `.abbd/` with the code, so the PR shows the blueprint's change. The human judges it in the PR review.
4. Open the PR through `<pstack>/skills/poteto-mode/playbooks/opening-a-pr.md`.

`abbd status` says whether the state and the blueprint are current. `abbd evidence` lists each quote and whether the code still holds it.
