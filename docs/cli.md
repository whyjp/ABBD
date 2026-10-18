# abbd CLI (Phase 1 draft)

All commands run from the target project root, or from anywhere with `--root DIR`. Output is YAML text; add
`--json` for the Project Intelligence API shape, which is identical to the MCP tool result. Flags and
positional arguments may appear in any order. A repeatable flag (`--option`, `--subject`, `--cmd`, `--anchor`,
`--allow`, `--forbid`, `--add`, `--remove`, `--preserve`) adds one value per use. When a project still has a
`.cccv/` or `.myharness/` directory, the first command moves it to `.abbd/` and says so once on stderr.

The block below is the verbatim output of `help` (also `--help`, `-h`, or no arguments). The tables after it
describe every command and flag in it.

```
abbd — ABBD Core CLI (Phase 1 draft)

global flags: --root DIR, --json

commands:
  abbd bootstrap --mode M [--scan] [--verify-available] [--from PATH] [--preview]
  abbd check
  abbd confirm [--list] [ID [--value TEXT|--purpose TEXT] [--reject] [--external-consumers true|false]] [--merge CAND-x|--split CAND-x] [--all]
  abbd contracts list [--module MOD-x] | contracts impact [--diff]
  abbd decide --question Q --option key=label[=effect]... --default KEY [--answer KEY] [--subject ID]... [--tier B|C] [--title T] [--proceeds-without SCOPE] | decide --adr ADR-x --answer KEY [--reason TEXT]
  abbd decisions [QUERY]
  abbd domain ... (reserved for Phase 3)
  abbd eval run (optional appendix A benchmark)
  abbd graph trace --from ID --to ID | graph neighbors ID [--depth N]
  abbd impact
  abbd init [DIR] [--mode M] [--from PATH] [--verify-available] [--preview]
  abbd inspect ID
  abbd intent feature|invariant|module|contract|rule|delta ... (module: --id MOD-x | --path DIR for a module this turn creates)
  abbd learn --kind architecture|type|lint|test|doc|skill --proposal TEXT | learn --preference key=value
  abbd mcp (stdio MCP server)
  abbd open-docs
  abbd plan "REQUEST"
  abbd policy classify --action ACTION
  abbd reconcile
  abbd render
  abbd review [CHG-ID] [--apply FILE] [--decide CHG-x#n --decision approve|reject|revert|option:KEY --reason TEXT] [--relayout]
  abbd run "REQUEST" [--type T] [--tier T0..T3]
  abbd serve [--addr 127.0.0.1:7787] (local review screen: topology, structural diff, approve/reject)
  abbd status [--knowledge]
  abbd sync [--commit]
  abbd turn show | turn note --model-calls N | turn abort [--reason TEXT]
  abbd verification-plan
  abbd verify [--affected|--all] [--cmd "CMD"]... [--unverifiable "CHECK" --reason TEXT]
```

## Session and bootstrap (turn step 0)

| Command | What it does |
|---|---|
| `abbd init [DIR] [--mode M] [--from PATH] [--verify-available] [--preview]` | Without `.abbd/`, runs Zero-Map Bootstrap and writes `.abbd/` and the BootstrapReport. Never asks questions. With `.abbd/` present and no `--mode` or `--from`, returns `status` instead. |
| `abbd bootstrap --mode M [--scan] [--verify-available] [--from PATH] [--preview]` | Re-runs bootstrap explicitly. Stable IDs and confirmed intent are kept through anchor matching. Refused during an active turn. |
| `abbd status [--knowledge]` | Revision, fingerprint, freshness of feature map, architecture map, graph and HTML, active turn, pending external changes (`pending_changes`), UNCONFIRMED count. |
| `abbd reconcile` | External code change: creates a `CHG` with `intent_source: external`, refreshes EXTRACTED facts only, asks nothing. A conflict with a CONFIRMED contract or feature becomes a tier B ADR with default `accept`, and the CHG state is NEEDS_USER. Refused while a turn is active. |

| Flag | Commands | Meaning |
|---|---|---|
| `DIR` | `init` | Same as `--root DIR`. |
| `--mode M` | `init`, `bootstrap` | `greenfield`, `code-only` or `adopt`. Omitted on `init`, it is detected. |
| `--from PATH` | `init`, `bootstrap` | Adopt source. A failed adopt input does not stop the default bootstrap. |
| `--verify-available` | `init`, `bootstrap` | Runs the detected build and test commands and records OBSERVED evidence. |
| `--preview` | `init`, `bootstrap` | Prints the report and writes nothing. |
| `--scan` | `bootstrap` | Always on; accepted for readability. |
| `--knowledge` | `status` | Adds knowledge counts to the status. |

## Turn (steps 1 to 4)

| Command | What it does |
|---|---|
| `abbd run "REQUEST" [--type T] [--tier T0..T3]` | Starts a turn. Bootstraps first when `.abbd/` is missing, then reconciles pending external changes, routes, selects context, resolves intent and writes a draft `CHG` (state INTENT_RESOLVED). Prints the WorkflowPlan: `change`, `task_type`, `verification_tier`, `steps`, `context_strategy`, `context_reason`, `matches`, `context`, `ambiguous`, `new_feature`, `requirement`, `confirm_if_purpose_used`, `budget`, plus `rework`, `bootstrap` and `reconciled` when present. `rework` lists structural change cards rejected or sent back on the review screen since the last turn; each line is also appended to this turn's IntentDelta additions, once. Refused while another turn is active. |
| `abbd plan "REQUEST"` | Dry run of route, intent and impact. Writes nothing. |
| `abbd inspect ID` | Feature, module, contract, test or change context with refs and evidence. |
| `abbd intent feature [--id FEAT-x] [--name N] [--purpose P] [--origin O] [--anchor kind:ref]...` | Records the resulting intent of the active turn. Without `--id`, creates a new draft feature. |
| `abbd intent invariant --feature FEAT-x --claim C [--origin O] [--test TEST-x]` | Adds an invariant claim and appends it to the turn's preserved invariants. `--test` links that test's evidence. |
| `abbd intent module (--id MOD-x \| --path DIR) [--responsibility T] [--allow MOD-y]... [--forbid MOD-z]... [--origin O]` | Records a module's responsibility and dependency policy. `--path DIR` names a module this turn creates; sync assigns its MOD ID and moves the recorded responsibility onto it. Any `--allow` or `--forbid` replaces the whole policy. |
| `abbd intent contract --id IF-x [--compatibility T] [--external-consumers true\|false] [--origin O]` | Records a contract's compatibility policy and whether it has external consumers. |
| `abbd intent rule --claim TEXT \| --req REQ-x [--retire]` | Records a standing rule the user stated for the whole project (USER_STATED, project scope), promotes a requirement to one, or retires it. Standing rules are listed in `.abbd/BLUEPRINT.md` and in every `run` plan. A request that reads as a project rule ("항상", "모든", "어떤 경로로도", "always", "never" ...) is stored as one automatically. |
| `abbd intent delta [--add T]... [--remove T]... [--preserve "FEAT-x: claim"]...` | Appends to the turn's IntentDelta and moves the turn from INTENT_RESOLVED to IMPLEMENTING. |
| `abbd decide --question Q --option key=label[=effect]... --default KEY [--answer KEY] [--subject ID]... [--tier B\|C] [--title T] [--proceeds-without SCOPE]` | Records a decision as an ADR on the active turn. See the decide table below. |
| `abbd decide --adr ADR-x [--answer KEY]` | Answers an existing ADR (left by sync, reconcile or an earlier turn) inside or outside a turn. Without `--answer`, returns its current state. An answer that is not one of its options fails; a tier C ADR takes `approve`. |
| `abbd decisions [QUERY]` | Lists ADRs whose question, title or subjects contain QUERY, or all ADRs without one. |
| `abbd impact` | Change impact of the working tree against the last sync: `contracts`, `modules`, `dependents`, `features`, `tests`, `unknowns`, the test commands and the highest tier. |
| `abbd contracts list [--module MOD-x]` | External contract view, optionally for one module. |
| `abbd contracts impact [--diff]` | Contracts the working tree adds, removes or modifies, each with `change`, `breaking`, `tier` and `reason`, plus `highest_tier`. `--diff` is the default and may be omitted. |
| `abbd graph trace --from ID --to ID` | Path in the graph with edge provenance. `found: false` means no static path, not no relation. |
| `abbd graph neighbors ID [--depth N]` | Bounded subgraph around ID. `--depth` defaults to 2. Every graph answer carries `freshness`. |
| `abbd policy classify --action ACTION` | Approval tier A, B or C of an action, with the reason. An unknown action fails and lists the known ones, for example `push`, `force-push`, `deploy`, `delete-external`, `secret`, `production-data`, `breaking-external`, `compatibility-change`, `add-dependency` and `local-commit`. |
| `abbd verification-plan` | Commands, tests, expected results and unknowns that verification of the current change should cover. T2 and T3 turns add the full test commands. |

`--origin` takes `USER_STATED`, `ADOPTED` or `INFERRED` and defaults to `INFERRED`, so an agent-authored meaning
is INFERRED. `--anchor` kinds are `symbol`, `path`, `test` and `entry`. `intent feature` and `intent module`
refuse to replace a CONFIRMED purpose or responsibility with a value that is not USER_STATED; that change goes
through `abbd decide --subject ID`. `intent`, `decide --question` and `turn note` need an active turn.

| decide flag | Meaning |
|---|---|
| `--question Q` | The question. A question already answered in any ADR is not asked again: the result is `reused: true` with the earlier answer. The same unanswered question with `--answer KEY` answers the existing ADR. |
| `--option key=label[=effect]` | One option. Tier B needs 2 or more, also when answering. `effect` says what that choice changes. |
| `--default KEY` | The option that proceeds without an answer. Without it, the first option. |
| `--answer KEY` | The user's answer. The ADR becomes USER_STATED and CONFIRMED. |
| `--subject ID` | Item the decision is about; repeatable. A tier C approval releases the sync gate for the subjects it names. |
| `--tier B\|C` | Default B. Tier C needs no options: without `--answer approve` the result is `blocked: true`, the stop joins the turn's `blocked` list, and the command exits 2. |
| `--title T` | Short name shown in ADR lists. |
| `--proceeds-without SCOPE` | What goes ahead if nobody answers. |
| `--adr ADR-x` | Answer an existing ADR instead of asking a new question. |

Without `--answer`, a tier B result has `ask: true` while fewer than `policy.max_questions_per_turn` (3)
questions were asked this turn; past that the default is recorded with `ask: false`. An unanswered ADR stays
UNCONFIRMED and the turn ends NEEDS_USER.

## Prove and synchronize (steps 5 to 7)

| Command | What it does |
|---|---|
| `abbd verify [--affected\|--all] [--cmd "CMD"]... [--unverifiable "CHECK" --reason TEXT]` | Runs verification and stores Evidence under `.abbd/verification/runs/`. The report has `overall`, `runs`, `ran`, `could_not_run`, `failed_tests`, `failures` (each failing test with its own `output` and `evidence`) and `warnings`. A command whose policy tier is C (for example `git push`) is refused. Exits 2 when `overall` is FAILED. |
| `abbd sync [--commit]` | Completion gate. See below. |
| `abbd check` | Schema, provenance rules, reference integrity, evidence links, md/html/graph revision agreement. Prints the report (`ok`, `errors`, `warnings`, `stale`, `freshness`) and exits 2 when `errors` is not empty. |
| `abbd render` | Regenerates `generated/explorer.html`, `generated/confirm-queue.html` and `generated/review.html` from the Markdown sources. |
| `abbd open-docs` | Prints the path of the generated HTML Explorer. |

| verify flag | Meaning |
|---|---|
| `--affected` | Default. The build commands plus the tests the impact selects. A T2 or T3 turn widens to the full test commands. |
| `--all` | The build commands and the full test commands. |
| `--cmd "CMD"` | Runs exactly the given commands; repeatable. |
| `--unverifiable "CHECK" --reason TEXT` | Records a check that cannot run here as UNVERIFIABLE evidence. `--reason` is required. It does not override an earlier PASSED or FAILED result and does not count as a test run. |

A `go test` package with no test files is recorded as PASSED with reason `no test files`.

`sync` completes only when the last verification PASSED on exactly the current working tree; a turn that changed
only documents passes as T0, with `abbd check` recorded as its evidence. It applies the turn to a copy first and
computes the turn's structural changes (below). An unapproved tier C card refuses the sync with state BLOCKED and
the card IDs in `reason`, and the Blueprint is not changed. Each tier B card gets an UNCONFIRMED ADR with default
`approve`, and the turn ends NEEDS_USER. On success, sync updates maps, graph, HTML and the review screen at one
revision, writes the ChangeReceipt and runs `check`. The result has `completed`, `reason`, `receipt`,
`knowledge`, `impact`, `consistency`, `freshness`, and `review` only when the turn has structural changes. On
refusal, artifacts stay DRAFT and the turn stays open. `--commit` makes one local commit after success when policy
puts `local-commit` in tier A; otherwise the sync still completes, `commit` is empty and `reason` starts with
"commit 안 함". With no active turn, sync reconciles external changes or regenerates artifacts.

## Review screen (PRD 08B)

| Command | What it does |
|---|---|
| `abbd serve [--addr 127.0.0.1:7787]` | Serves the review screen on a loopback address and prints its URL. Decisions made on the page are written straight to `.abbd/`. Non-loopback addresses are refused. Runs until stopped. |
| `abbd review [CHG-ID]` | Summary of one turn's structural change cards: `path` (the `generated/review.html` file), `change`, `open` (tier B and C cards waiting for a decision), `by_tier`, `deltas`, `message`. Without `CHG-ID`, the active turn when it has cards, else the latest turn that has cards. |
| `abbd review --decide CHG-x#n --decision D [--reason TEXT]` | Records a decision on one card, then prints the summary. |
| `abbd review --apply FILE` | Applies a decision file downloaded from a review page opened without `abbd serve`. |
| `abbd review --relayout` | Drops the stored layout (`generated/layout.json`) and places every node again. Only on an explicit user request; otherwise node positions stay fixed across revisions. |

| `--decision` | Allowed on | Effect |
|---|---|---|
| `approve` | A, B, C | Card approved. The meaning field of the card's subject becomes CONFIRMED and, for B and C, the card's ADR is answered. A tier C card no longer blocks sync. |
| `reject` | A, B, C | Card rejected. Needs `--reason`. The next `abbd run` puts it in `rework` and the IntentDelta. |
| `revert` | A | Revert requested. Needs `--reason`. Handled like a rejection by the next turn. |
| `option:KEY` | B | Picks another option of the card's ADR, for example `option:compat` on a contract change. |

A decision on a card that changed again in a later turn fails; that card is `superseded` and the newer card is
the one to decide.

| Card kind | Tier |
|---|---|
| `dependency_added`, `dependency_removed`, `feature_path_changed` | A |
| `module_added`, `module_removed`, `module_split`, `module_merged`, `forbidden_dependency`, `ownership_moved`, `deploy_mapping_changed` | B |
| `contract_added`, `contract_removed` | B, or C when the contract has `external_consumers: true` |
| `contract_compat_changed` | B, or C when the change is breaking and the contract has `external_consumers: true` |

Card status starts as `applied` (A), `default_applied` (B) or `pending` (C), and moves to `approved`, `rejected`,
`revert_requested` or `superseded`. A turn that only changes code inside modules has no cards and no `review`
field.

## Review and learn (steps 8 and 9)

| Command | What it does |
|---|---|
| `abbd turn show` | The active turn, or the latest receipt. |
| `abbd turn note --model-calls N` | Adds N host-reported model calls to the active turn. |
| `abbd turn abort [--reason TEXT]` | Abandons the active turn. Its draft features are dropped and the CHG stays as a BLOCKED record with `aborted: TEXT` in `blocked`. Working-tree changes are kept, and the next `run` reconciles them. |
| `abbd learn --kind architecture\|type\|lint\|test\|doc\|skill --proposal TEXT` | Records a correction proposal in `.abbd/learning.yaml` with `status: proposed`, and in the active turn's next steps. Kinds are listed in order of preference. |
| `abbd learn --preference key=value` | Writes a preference to `preferences` in `.abbd/project.yaml`. |
| `abbd confirm [--list]` | The confirm queue: UNCONFIRMED meaning fields, decisions and merge candidates. Also the result with no arguments. |
| `abbd confirm ID [--value TEXT\|--purpose TEXT] [--reject] [--external-consumers true\|false]` | Lazy confirmation by the user. `ID` alone accepts the inferred text: CONFIRMED, origin unchanged. `--value`, or its alias `--purpose`, replaces it with USER_STATED CONFIRMED text. `--reject` alone marks it REJECTED; with `--value`, the old text is kept as `rejected_value`. `--external-consumers` sets a contract's consumers flag as USER_STATED. |
| `abbd confirm --merge CAND-x\|--split CAND-x` | Resolves a bootstrap merge candidate. |
| `abbd confirm --all` | Accepts every pending meaning field except candidates and consumer flags. Use only on an explicit user request. |

## Other

| Command | What it does |
|---|---|
| `abbd mcp` | MCP stdio server exposing the same API (tools below). |
| `abbd domain ...` | Reserved for Phase 3. Exits 1 with a "reserved" message. |
| `abbd eval run` | Optional appendix A benchmark, not part of Phase 1. Exits 1. |

MCP tools: `project_status`, `resolve_intent`, `start_turn`, `get_feature_context`, `get_architecture_context`,
`trace_implementation`, `analyze_change_impact`, `get_verification_plan`, `run_verification`,
`sync_project_knowledge`, `check_consistency`, `ask_user_decision`, `get_external_contracts`,
`analyze_contract_impact`, `list_review_cards`, `decide_structural_change`, `list_unconfirmed`, `confirm_item`.
`list_review_cards` returns the same summary as `abbd review`; `decide_structural_change` records the user's
decision on one card like `abbd review --decide`.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success. |
| 1 | Error: unknown command, usage error (the usage line goes to stderr), or a refused operation such as `run` during an active turn, `decide --question` outside a turn, or a card decision without a required reason. Nothing is printed on stdout. |
| 2 | A gate refused, with `BLOCKED: ...` on stderr. The result is still printed. `sync`: verification not PASSED, working tree changed since verification, an unapproved tier C card, or artifacts inconsistent after sync; the turn stays open and artifacts stay DRAFT. `verify`: `overall` is FAILED. `decide --tier C` without `--answer approve`: `blocked: true`. `check`: `errors` is not empty. |

## Telemetry

Each turn's ChangeReceipt has a `telemetry` block. The harness counts four figures itself. `graph_queries`
counts each `inspect`, `contracts list`, `contracts impact`, `impact`, `graph trace` and `graph neighbors`
during a turn. `test_runs` counts each command `verify` runs; UNVERIFIABLE records do not count.
`questions_asked` counts each tier B `decide` that returned `ask: true`. `duration_sec` is set by `sync` or
`turn abort`. Model calls are the only host-reported figure, added with `abbd turn note --model-calls N`; `model_calls_note`
reads `host-reported` once a figure arrives. Per-tier limits are in `budgets` of
`.abbd/project.yaml`, and `verify` adds a warning when `test_runs` exceeds the turn's limit. The other two limits
are recorded, not enforced.
