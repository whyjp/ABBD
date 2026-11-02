---
name: abbd-setup
description: Set ABBD up on a repository. Writes the deterministic extraction script with pstack, supplements it with what pstack's readers find, stores and renders the topology, and adds the always-on block to the project's CLAUDE.md. Use for /abbd-setup, "set up abbd", or a repository with no .abbd directory.
---

# ABBD setup

`<pstack>` is `~/.claude/plugins/cache/cursor-plugins/pstack/0.15.36`. Open files under it with the Read tool. If `<pstack>/skills/swarm/SKILL.md` does not exist, stop and tell the user to install pstack 0.15.36.

Setup never asks the human anything. The first state you commit is the baseline; the human reads it in the blueprint and the PR, and there is no alignment step.

## Steps

1. List unit candidates from the layout, entry points, manifests and schemas. A unit is something a human would say "this change must not touch that" about: a responsibility (`MOD`), an external contract (`IF`, `EVT`, `DATA`, `CFG`), a user-facing feature (`FEAT`). Imports, calls and file placement are material for finding units, never units.
2. Follow `<pstack>/skills/swarm/SKILL.md`, partitioned into slices of related candidates, at most five workers, each run locally without worktree isolation so it sees uncommitted code. Each worker reads its slice the way `<pstack>/skills/how/references/explorer-prompt.md` describes and reports in the extraction format below instead of `PASS`, `ISSUES` or `BLOCKED`. ABBD adds one thing to how's explorer: every coupling and every unit the layout does not show comes with `via` and verbatim `evidence`, and a unit inside a module names that module as `parent`.
3. Build the lever, per `<pstack>/skills/principle-build-the-lever/SKILL.md`. Write `.abbd/extractor/extract.<ext>`, a script that prints the extraction JSON from the code alone: imports, routes, schemas, manifests, and each contract's `signature` and `digest`. It runs from the repository root, sorts its output, and uses no clock, network or model. Pick anchors mechanically and stably: every non-test file of a package plus its directory, or a file plus the symbol that defines a contract. Have it fail when a unit has fewer than two anchors or an anchor names a symbol its file does not declare. Diff its output against the swarm result until every difference is explained.
4. Write `.abbd/extractor/supplement.json` with what only reading tells:
   - `purpose` and `responsibility` for script units, repeating `key`, `kind`, `name` and one anchor of the unit they describe, with no `evidence`. When that anchor is also held by the unit's module, such as the directory of an inside module, the innermost unit is matched
   - units the script cannot see, such as a shared table or a wire format, with `evidence` instead of anchors and their module as `parent`, by the supplement's key or the script's; the module need not be restated. A quote may sit on an anchor of that parent or of a module around it, as long as the parent holds some anchor the unit does not. A quote on an anchor of any other script unit of the same kind makes setup refuse the unit and name both. A contract the script reports no `signature` for keeps the one the supplement gives, as INFERRED
   - couplings the script cannot see, such as a shared table, an execution order, a trait used through generics, a callback, a flag passed through config, a build feature or a convention two units keep in step, each with `via` and `evidence`
   - a grouping the project itself shows, and only that: a subsystem its docs name, a package or workspace, a skill family, or an abstraction the code itself shows (a layer, a family of implementers of one interface). Add it as a `MOD` whose `evidence` quotes the project naming or showing it (the README section, manifest, entry point or code), and give each member that part as `parent` with `parent_evidence` quoting the project placing it there. Never add a group to make a crowded view smaller or a dense view easier to draw. Such a merge goes past what an engineer would draw and makes the system harder to understand. A crowded level stays as the project has it, and the blueprint reads it one box at a time through focus views. Sync's fallback places a crowded level's units in the directories their code lives in, and never moves a unit a reader placed
5. Run setup:

   ```
   abbd setup --supplement .abbd/extractor/supplement.json -- <interpreter> .abbd/extractor/extract.<ext>
   ```

   - `not deterministic`: the script printed different output across three runs, one with a different `PYTHONHASHSEED` and `PERL_HASH_SEED`. Sort what it prints and remove the variation.
   - `fix the script, or cite the code`: a supplement unit matches no script unit and cites no evidence. Teach the script the unit, add `evidence`, or drop it.
   - `would merge into it`: a supplement unit with evidence shares an anchor with a script unit that is not its parent. Name that unit as `parent` if the unit sits inside it, quote code of its own, or drop the evidence if it describes that script unit.
   - `cites no evidence`: a supplement coupling has no quote. Add one or drop it.
   - `is not a relation ABBD knows`: a module or feature `provides` a contract, anything `consumes` a contract, and anything `depends_on` any unit.
6. For each top-level module, follow `<pstack>/skills/how/SKILL.md` unchanged and store its Overview and Key Concepts with `abbd understand <MOD-ID> <FILE>`. Write the text as the code is now, in the present tense. Leave out dates, issue and PR numbers, and tags such as "migrated from X", because git keeps the history. Keep each section under 2,000 characters of prose, and split a longer one under `###` headings or into shorter sections. `abbd understand <MOD-ID> <FILE>` and `abbd status` report each section over that length and each phrase that reads as history. Neither refuses the text or fails the check, because a version number or a name can read as history too.
7. Open `.abbd/blueprint/index.html` and check that the overview and each inside read the way the swarm described the code. Fix the script or the supplement and rerun setup until they do.
8. Run `abbd install`. It writes a short block between `<!-- abbd:begin -->` and `<!-- abbd:end -->` into `CLAUDE.md` at the project root, so any assistant asks `abbd explain <QUERY>`, `abbd path <A> <B>` and `abbd impact <QUERIES>` before it greps, and runs `abbd sync` and puts `abbd diff` in the PR after it changes code. A rerun replaces the block and keeps the rest of the file.
9. Commit `.abbd/` and `CLAUDE.md` with the code, and tell the human where the blueprint is. From here on the abbd skill reads and queries the blueprint.

## Extraction format

```json
{
  "units": [
    {"key": "billing", "kind": "MOD", "name": "billing",
     "anchors": [{"path": "billing"}, {"path": "billing/charge.go", "symbol": "Charge"}]},
    {"key": "api", "kind": "IF", "name": "http api", "signature": "GET /charges", "digest": "3f9a1c0d",
     "anchors": [{"path": "api/http.go", "symbol": "Serve"}, {"path": "api/http.go", "symbol": "Routes"}]},
    {"key": "charges", "kind": "DATA", "name": "charges table", "parent": "billing",
     "evidence": [{"path": "billing/charge.go", "lines": "41", "quote": "INSERT INTO charges"}]}
  ],
  "relations": [
    {"from": "billing", "to": "api", "kind": "provides"},
    {"from": "ledger", "to": "charges", "kind": "consumes", "via": "data",
     "evidence": [{"path": "ledger/book.go", "lines": "12-13", "quote": "SELECT amount FROM charges"}]}
  ]
}
```

- `kind` is one of `MOD`, `IF`, `EVT`, `DATA`, `CFG`, `FEAT`. Only contracts carry a `signature`.
- `digest` fingerprints the code a contract's meaning lives in, so `abbd diff` sees a change of meaning under the same signature.
- `relations` name units by `key`; `kind` is `depends_on`, `consumes` or `provides`.
- `via` is `import`, `trait`, `callback`, `macro`, `config`, `build-feature`, `data`, `convention` or `call`.
- `evidence` is a list of `{"path", "lines", "symbol", "quote"}`; `lines` and `symbol` are optional. The quote is verbatim; whitespace does not matter, and it must sit within three lines of `lines`. A unit or coupling only a reader found lives while any of its quotes is in its file. A quote its file no longer holds drifts, and dies at a sync of a later revision unless `abbd record evidence <TARGET> --evidence <PATH:LINES:QUOTE>` refreshes it.
- `parent` names, by key, the module that contains a unit. A unit with no parent is drawn on the overview; a unit with one is drawn inside its module. In the supplement, `parent_evidence` is the code that places a unit there; without it the unit's own `evidence` stands in. A parent from the supplement is INFERRED and is stored beside the script's. The script's parent outranks it, and a human's outranks both. `abbd record parent ID PARENT --evidence ...` places one unit later.
- The script's output carries no `purpose` or `responsibility`. Unknown fields are rejected.

Rerunning setup replaces every claim the last supplement made and finds its units again by `key`, so keep each unit's key stable. What a reader stored through abbd record and what the human stated stay. A state from before writers existed is converted once with `abbd migrate`, which needs this supplement to tell its claims from a reader's.
