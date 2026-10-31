---
name: abbd-setup
description: Bootstrap ABBD on a repository. Extracts the actual topology with pstack, generates the deterministic project extraction script, and runs abbd setup. Use for /abbd-setup, "set up abbd", or a project with no .abbd directory.
---

# ABBD setup

`<pstack>` is `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36`. ABBD is pinned to that version. If `<pstack>/skills/swarm/SKILL.md` does not exist, stop and tell the user to install pstack 0.15.36.

Setup never asks the human anything. Make every call yourself and let the bootstrap report show it.

Do not run `abbd align` or `abbd confirm` yourself. Confirming design units is the human's step.

## Choose the mode

- The human described the design units of a project with no code yet: greenfield.
- The repository holds design documents (architecture notes, ADRs, an old SSOT, YAML or HTML maps): code-only plus adopt.
- Otherwise: code-only.

## Code-only

1. List design unit candidates from the layout, entry points, manifests and schemas. A design unit is something a human would say "this change must not touch that" about: a responsibility (`MOD`), an external contract (`IF`, `EVT`, `DATA`, `CFG`), a user-facing feature (`FEAT`). Imports, calls and file placement are material for finding units, never units.
2. Read and follow `<pstack>/skills/swarm/SKILL.md`, partitioned into slices of related candidates, with two changes. Give each worker a slice of the tree, such as a few packages or the plugin files, and at most five workers in all; a real repository has dozens of candidates, and one worker each costs more than it finds. Run every worker locally, with no worktree isolation, because a worktree checks out the last commit and the worker would not see uncommitted code. Each worker reads its slice the way `<pstack>/skills/how/references/explorer-prompt.md` describes and reports its unit in the extraction format below, with the relations it found by key, instead of `PASS`, `ISSUES` or `BLOCKED`.
3. Build the lever. Write `.abbd/extractor/extract.<ext>`, a script that prints the extraction JSON from the code alone: imports, routes, schemas, manifests. It runs from the repository root, sorts its output, and uses no clock, network or model. Run it and diff against the swarm result. Every unit the swarm found must come out of the script with the same kind and at least one shared anchor, or be left out on purpose with the reason in the bootstrap summary you give the human. The script's anchors are the ones ABBD keeps, so pick them mechanically and stably: every non-test file of a package plus its directory, or a file plus the symbol that defines a contract. A worker's hand-picked symbols drift with every refactor. Have the script fail when a unit has fewer than two anchors or an anchor names a symbol its file does not declare. Fix the script until the diff is explained.
4. Write `.abbd/extractor/supplement.json` in the same format with what only reading can tell: `purpose`, `responsibility`, and couplings the script cannot see, such as a shared table or an execution order. Each unit repeats `key`, `kind`, `name` and at least one anchor of the script unit it describes.
5. Run setup:

   ```
   abbd setup --supplement .abbd/extractor/supplement.json --verify "<VERIFY-COMMAND>" -- <interpreter> .abbd/extractor/extract.<ext>
   ```

   `--verify` registers the command every turn verifies with, so the human sees it on the screen before any turn trusts it. Use the project's verify skill (`.claude/skills/verify-*/SKILL.md`) when it has one, otherwise the project's test command, such as `go test -count=1 ./...`. It must write nothing git does not ignore. Leave `--verify` out when the project has no verification yet. It registers once; setup refuses a different command later.

   - Exit 1 `not deterministic`: the script printed different output across runs, one of them with a different `PYTHONHASHSEED` and `PERL_HASH_SEED`. Sort what it prints, remove the source of variation, and rerun.
   - Exit 1 `extraction script is fixed`: the project is aligned, and its script is the one `project.yaml` records. Rerun setup without `--`.
   - Exit 1 `fix the script`: the supplement names a unit the script did not report. Teach the script that unit, or drop it from the supplement.
6. Commit `.abbd/` with the code. The extraction script is the lever that `abbd mode on --session <ID>` reruns every time the mode turns on.
7. Tell the human how many candidates there are and that they finish alignment on the screen: `abbd serve` in their own terminal prints a link, and the page lists the candidates to keep and the confirm queue. Give the terminal form too, for a human who prefers it: `abbd align --all`, `abbd align --confirm <IDS>`, or `abbd align --exclude <IDS>`, with IDs from `.abbd/bootstrap-report.md`.

## Adopt

Reconstruct the existing documents into the extraction format at `.abbd/extractor/adopt.json`. Take names, purposes, responsibilities, signatures and relations from the documents. Take anchors from the code each documented unit describes. A documented unit with no code gets the anchor where the document says it lives. Then add `--adopt .abbd/extractor/adopt.json` to the setup command in step 5.

ABBD records adopted facts as ADOPTED and lists conflicts and gaps in the report. An unreadable adopt file only adds a warning. The code-only result stands either way.

## Greenfield

Write the units the human stated to `.abbd/extractor/stated.json` in the extraction format. Anchors are the paths where each unit will live. Run:

```
abbd setup --greenfield .abbd/extractor/stated.json
```

Setup refuses greenfield when the directory holds any file other than dotfiles, dot directories such as `.git/`, `.abbd/` and `.claude/`, and a root `CLAUDE.md` or `AGENTS.md`. Use code-only for a repository that has code.

The project is aligned at once, with every fact USER_STATED. In the first turn that adds code, write the extraction script as in code-only step 3 and register it:

```
abbd setup -- <interpreter> .abbd/extractor/extract.<ext>
```

Setup runs it three times and refuses it if the output changes. Registration happens once. After that, setup refuses any other argv, because `abbd mode on --session <ID>` trusts the recorded script.

## Extraction format

```json
{
  "units": [
    {"key": "billing", "kind": "MOD", "name": "billing",
     "anchors": [{"path": "billing/charge.go", "symbol": "Charge"}],
     "purpose": "takes payments", "responsibility": "owns the charges table"},
    {"key": "api", "kind": "IF", "name": "http api", "signature": "GET /charges",
     "anchors": [{"path": "api/http.go", "symbol": "Serve"}]}
  ],
  "relations": [
    {"from": "billing", "to": "api", "kind": "provides"}
  ]
}
```

- `anchors` is required on every unit. A path is relative to the repository root, with forward slashes. Two shared anchors, or an identical anchor set, keep a unit's ID across re-extraction. Prefer two or more anchors per unit.
- `kind` is one of `MOD`, `IF`, `EVT`, `DATA`, `CFG`, `FEAT`. `DOM`, `STATE`, `TRN` and `INV` are reserved.
- `signature` is the observable shape of a contract, and only contracts carry one. Changing it later is a grade C change, so make it exact and stable.
- `relations` name units by `key`. `kind` is `depends_on`, `consumes` or `provides`.
- The script's output never carries `purpose` or `responsibility`. Those belong in the supplement, where they stay INFERRED until a human confirms them.
- Unknown fields are rejected.

## After setup

`abbd status` shows the phase and the size of the confirm queue. `abbd confirm --list` lists inferred items, design units first, named like `MOD-0001.purpose` or `MOD-0002.consumes.MOD-0001`. Neither blocks alignment.

Rerunning setup replaces every guess the last supplement made that no human has confirmed. Put every guess you still stand by in the new supplement.
