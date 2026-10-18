# cccv CLI (Phase 1 draft)

All commands run from the target project root, or from anywhere with `--root DIR`. Output is YAML text; add
`--json` for the Project Intelligence API shape, which is identical to the MCP tool result. Flags and
positional arguments may appear in any order. A repeatable flag (`--option`, `--cmd`, `--anchor`, `--allow`,
`--forbid`, `--add`, `--remove`, `--preserve`, `--subject`) adds one value per use.

The block below is the verbatim output of `help` (also `--help`, `-h`, or no arguments). The tables after it describe every command and
flag in it.

```
cccv — For My Harness Core CLI (Phase 1 draft)

global flags: --root DIR, --json

commands:
  cccv bootstrap --mode M [--scan] [--verify-available] [--from PATH] [--preview]
  cccv check
  cccv confirm [--list] [ID [--value TEXT|--purpose TEXT] [--reject] [--external-consumers true|false]] [--merge CAND-x|--split CAND-x] [--all]
  cccv contracts list [--module MOD-x] | contracts impact [--diff]
  cccv decide --question Q --option key=label[=effect]... --default KEY [--answer KEY] [--subject ID] [--tier B|C]
  cccv decisions [QUERY]
  cccv domain ... (reserved for Phase 3)
  cccv eval run (optional appendix A benchmark)
  cccv graph trace --from ID --to ID | graph neighbors ID [--depth N]
  cccv impact
  cccv init [DIR] [--mode M] [--from PATH] [--verify-available] [--preview]
  cccv inspect ID
  cccv intent feature|invariant|module|contract|delta ...
  cccv learn --kind architecture|type|lint|test|doc|skill --proposal TEXT | learn --preference key=value
  cccv mcp (stdio MCP server)
  cccv open-docs
  cccv plan "REQUEST"
  cccv policy classify --action ACTION
  cccv reconcile
  cccv render
  cccv run "REQUEST" [--type T] [--tier T0..T3]
  cccv status [--knowledge]
  cccv sync [--commit]
  cccv turn show | turn note --model-calls N | turn abort [--reason TEXT]
  cccv verification-plan
  cccv verify [--affected|--all] [--cmd "CMD"]... [--unverifiable "CHECK" --reason TEXT]
```

## Session and bootstrap (turn step 0)

| Command | What it does |
|---|---|
| `cccv init [DIR] [--mode M] [--from PATH] [--verify-available] [--preview]` | Without `.cccv/`, runs Zero-Map Bootstrap and writes `.cccv/` and `BootstrapReport`. Never asks questions. `DIR` is the same as `--root DIR`. `--mode` is `greenfield`, `code-only` or `adopt`; omitted, it is detected. `--from PATH` is the adopt source. `--verify-available` runs the detected build and test commands and records OBSERVED evidence. `--preview` prints the report and writes nothing. With `.cccv/` present and no `--mode` or `--from`, returns `status` instead. |
| `cccv bootstrap --mode M [--scan] [--verify-available] [--from PATH] [--preview]` | Re-runs bootstrap explicitly. Stable IDs and confirmed intent are kept through anchor matching. `--scan` is always on; the flag is accepted for readability. Other flags as in `init`. |
| `cccv status [--knowledge]` | Revision, fingerprint, freshness of feature map, architecture map, graph and HTML, active turn, pending external changes (`pending_changes`), UNCONFIRMED count. |
| `cccv reconcile` | External code change: creates a `CHG` with `intent_source: external`, refreshes EXTRACTED facts only, asks nothing. A conflict with a CONFIRMED contract or feature becomes a tier B ADR with default `accept`, and the CHG state is NEEDS_USER. Refused while a turn is active. |

## Turn (steps 1 to 4)

| Command | What it does |
|---|---|
| `cccv run "REQUEST" [--type T] [--tier T0..T3]` | Starts a turn. Bootstraps first when `.cccv/` is missing, then reconciles pending external changes, routes, selects context, resolves intent and writes a draft `CHG` (state INTENT_RESOLVED). Prints the WorkflowPlan: change, task type, verification tier, steps, context strategy and reason, matches, `new_feature`, `requirement` and budget, plus `bootstrap` and `reconciled` when they ran. `--type` overrides the task type (`docs`, `bug-fix`, `perf`, `interface-change`, `refactor`, `feature`). `--tier` overrides the verification tier. Refused while another turn is active. |
| `cccv plan "REQUEST"` | Dry run of route, intent and impact. Writes nothing. |
| `cccv inspect ID` | Feature, module, contract or test context with refs and evidence. |
| `cccv intent feature [--id FEAT-x] [--name N] [--purpose P] [--origin O] [--anchor kind:ref]...` | Records the resulting intent of the active turn. Without `--id`, creates a new draft feature. `--anchor` kinds are `symbol`, `path`, `test` and `entry`. |
| `cccv intent invariant --feature FEAT-x --claim C [--origin O] [--test TEST-x]` | Adds an invariant claim and appends it to the turn's preserved invariants. `--test` links that test's evidence. |
| `cccv intent module --id MOD-x [--responsibility T] [--allow MOD-y]... [--forbid MOD-z]... [--origin O]` | Records an existing module's responsibility and dependency policy. Any `--allow` or `--forbid` replaces the whole policy. |
| `cccv intent contract --id IF-x [--compatibility T] [--external-consumers true\|false] [--origin O]` | Records a contract's compatibility policy and whether it has external consumers. |
| `cccv intent delta [--add T]... [--remove T]... [--preserve "FEAT-x: claim"]...` | Appends to the turn's IntentDelta and moves the turn from INTENT_RESOLVED to IMPLEMENTING. |
| `cccv decide --question Q --option key=label[=effect]... --default KEY [--answer KEY] [--subject ID] [--tier B\|C]` | Records a decision as an ADR on the active turn. Tier B, the default, needs 2 or 3 options, also when answering; without `--default` the first option is the default. Without `--answer`, the result has `ask: true` while fewer than `policy.max_questions_per_turn` (3) questions were asked this turn; past that the default is recorded with `ask: false`. An unanswered ADR stays UNCONFIRMED and the turn ends NEEDS_USER. A question already answered in any ADR is not asked again: the result is `reused: true` with the earlier answer. The same question with `--answer KEY` answers the existing ADR, which becomes USER_STATED and CONFIRMED. Tier C needs no options: the result is `blocked: true`, and the stop joins the turn's `blocked` list, until the same question is recorded with `--answer approve`. |
| `cccv decisions [QUERY]` | Lists ADRs whose question, title or subjects contain QUERY, or all ADRs without one. |
| `cccv impact` | Change impact of the working tree against the last sync: contracts first, then modules, dependents, features, tests, unknowns and the highest tier. |
| `cccv contracts list [--module MOD-x]` | External contract view, optionally for one module. |
| `cccv contracts impact [--diff]` | Contracts the working tree adds, removes or modifies, each with `breaking`, `tier` and `reason`, plus `highest_tier`. `--diff` is the default and may be omitted. |
| `cccv graph trace --from ID --to ID` | Path in the graph with edge provenance. `found: false` means no static path, not no relation. |
| `cccv graph neighbors ID [--depth N]` | Bounded subgraph around ID. `--depth` defaults to 2. Every graph answer carries `freshness`. |
| `cccv policy classify --action ACTION` | Approval tier A, B or C of an action. An unknown action fails and lists the known ones, for example `push`, `force-push`, `deploy`, `delete-external`, `secret`, `production-data` and `local-commit`. |
| `cccv verification-plan` | Commands, tests, expected results and unknowns that verification of the current change should cover. T2 and T3 turns add the full test commands. |

## Prove, synchronize, review (steps 5 to 9)

| Command | What it does |
|---|---|
| `cccv verify [--affected\|--all] [--cmd "CMD"]... [--unverifiable "CHECK" --reason TEXT]` | Runs verification and stores Evidence under `.cccv/verification/runs/`. `--affected`, the default, runs the build commands plus the tests the impact selects. `--all` runs the build and full test commands. `--cmd` runs exactly the given commands. `--unverifiable` records a check that cannot run here as UNVERIFIABLE evidence and requires `--reason`. An UNVERIFIABLE record does not override an earlier PASSED or FAILED result. The report lists `ran` and `could_not_run` separately. |
| `cccv sync [--commit]` | Completion gate. Completes only when the last verification PASSED on exactly the current working tree; a turn that changed only documents passes as T0. Refuses with exit 2 and state BLOCKED when the diff has a breaking change to a contract with `external_consumers: true` and no tier C decision with `--answer approve` names that contract as `--subject`. A tier B contract change without a decision gets an UNCONFIRMED ADR with default `proceed`, and the turn ends NEEDS_USER. On success, updates maps, graph and HTML at one revision, writes the ChangeReceipt and runs `check`. On refusal, artifacts stay DRAFT and the turn stays open. `--commit` makes one local commit after success, only when policy puts `local-commit` in tier A. With no active turn, sync reconciles external changes or regenerates artifacts. |
| `cccv turn show` | The active turn, or the latest receipt. |
| `cccv turn note --model-calls N` | Adds N host-reported model calls to the active turn. |
| `cccv turn abort [--reason TEXT]` | Abandons the active turn. Its draft features are dropped and the CHG stays as a BLOCKED record. Working-tree changes are kept, and the next `run` reconciles them. |
| `cccv learn --kind architecture\|type\|lint\|test\|doc\|skill --proposal TEXT` | Records a correction proposal in `.cccv/learning.yaml` with `status: proposed`, and in the active turn's next steps. Prefer the earliest kind that can encode the lesson. |
| `cccv learn --preference key=value` | Writes a preference to `preferences` in `.cccv/project.yaml`. |
| `cccv confirm [--list]` | The confirm queue of UNCONFIRMED items. Also the result with no arguments. |
| `cccv confirm ID [--value TEXT\|--purpose TEXT] [--reject] [--external-consumers true\|false]` | Lazy confirmation by the user. `ID` alone accepts the inferred text: CONFIRMED, origin unchanged. `--value`, or its alias `--purpose`, replaces it with USER_STATED CONFIRMED text. `--reject` alone marks it REJECTED; with `--value`, the old text is kept as `rejected_value`. `--external-consumers` sets a contract's consumers flag as USER_STATED. |
| `cccv confirm --merge CAND-x\|--split CAND-x` | Resolves a bootstrap merge candidate. |
| `cccv confirm --all` | Accepts every pending meaning field except candidates and consumer flags. Use only on an explicit user request. |
| `cccv render` | Regenerates `generated/explorer.html` and `generated/confirm-queue.html` from the Markdown sources. |
| `cccv check` | Schema, provenance rules, reference integrity, md/html/graph revision agreement. |
| `cccv open-docs` | Prints the path of the generated HTML Explorer. |
| `cccv mcp` | MCP stdio server exposing the same API. |
| `cccv domain ...` | Reserved for Phase 3. Exits 1 with a "reserved" message. |
| `cccv eval run` | Optional appendix A benchmark, not part of Phase 1. Exits 1. |

`--origin` takes `USER_STATED`, `ADOPTED` or `INFERRED` and defaults to `INFERRED`, so an agent-authored
meaning is INFERRED. `intent feature` and `intent module` refuse to replace a CONFIRMED purpose or
responsibility with a value that is not USER_STATED. That change goes through `cccv decide --subject ID`.
`intent`, `decide` and `turn note` need an active turn.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success. A FAILED verification and an unapproved tier C decision also exit 0, so read `overall` and `blocked`. |
| 1 | Error: unknown command, usage error (the usage line goes to stderr), or a refused operation such as `run` during an active turn. |
| 2 | A completion gate refused, with `BLOCKED: completion gate refused` on stderr. Only `sync` returns it: verification not PASSED, working tree changed since verification, a tier C breaking change without approval, or artifacts inconsistent after sync. The turn stays open and artifacts stay DRAFT. |

## Telemetry

Each turn's ChangeReceipt has a `telemetry` block. The harness counts four figures itself. `graph_queries`
counts each `inspect`, `contracts list`, `contracts impact`, `impact`, `graph trace` and `graph neighbors`
during a turn. `test_runs` counts each command `verify` runs; UNVERIFIABLE records do not count.
`questions_asked` counts each tier B `decide` that returned `ask: true`. `duration_sec` is set by `sync` or
`turn abort`. Model calls are the only host-reported figure, added with `cccv turn note --model-calls N`.
Per-tier limits are in `budgets` of `.cccv/project.yaml`, and `verify` warns when `test_runs` exceeds the
turn's limit.
