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
  abbd check [--accept-edits [--user]]
  abbd confirm [--list] | confirm --user [ID [--value TEXT|--purpose TEXT] [--reject] [--external-consumers true|false]] [--merge CAND-x|--split CAND-x] [--all]
  abbd contracts list [--module MOD-x] | contracts impact [--diff]
  abbd decide --question Q --option key=label[=effect]... --default KEY [--answer KEY [--user]] [--subject ID]... [--tier B|C] [--title T] [--proceeds-without SCOPE] | decide --adr ADR-x --answer KEY --user [--reason TEXT]
  abbd decisions [QUERY]
  abbd domain ... (reserved for Phase 3)
  abbd driver set FEAT-x|IF-x --run CMD --expect TEXT | driver list (user-level checks per entry point)
  abbd eval run (optional appendix A benchmark)
  abbd graph trace --from ID --to ID | graph neighbors ID [--depth N]
  abbd impact
  abbd init [DIR] [--mode M] [--from PATH] [--verify-available] [--preview]
  abbd inspect ID
  abbd intent feature|invariant|module|contract|rule|delta ... (module: --id MOD-x | --path DIR for a module this turn creates)
  abbd learn --kind architecture|type|lint|test|doc|skill --proposal TEXT | learn --preference key=value | learn --from-receipts
  abbd mcp (stdio MCP server)
  abbd note-edit < hook-json (host PostToolUse hook: records a file the agent edited, so it is verified in a turn)
  abbd note-prompt < hook-json (host UserPromptSubmit hook: keeps the user's message verbatim for the next run)
  abbd note-session start|end < hook-json (host SessionStart/SessionEnd hooks: while a session is open, unsynced changes are the agent's)
  abbd open-docs
  abbd plan "REQUEST"
  abbd policy classify --action ACTION
  abbd reconcile [--user]
  abbd render
  abbd review [CHG-ID] [--apply FILE --user] [--decide CHG-x#n --decision approve|reject|revert|option:KEY --reason TEXT --user] [--relayout]
  abbd run "REQUEST" [--type T] [--tier T0..T3 [--reason TEXT]]
  abbd serve [--addr 127.0.0.1:7787] (local review screen: topology, structural diff, approve/reject)
  abbd status [--knowledge]
  abbd sync [--commit] [--user]
  abbd turn show | turn note --model-calls N | turn abort [--reason TEXT]
  abbd verification-plan
  abbd verify [--affected|--all] [--cmd "CMD"]... [--drive] [--unverifiable "CHECK" --reason TEXT] [--no-repro --reason TEXT]
```

## Session and bootstrap (turn step 0)

| Command | What it does |
|---|---|
| `abbd init [DIR] [--mode M] [--from PATH] [--verify-available] [--preview]` | Without `.abbd/`, runs Zero-Map Bootstrap and writes `.abbd/` and the BootstrapReport. Never asks questions. With `.abbd/` present and no `--mode` or `--from`, returns `status` instead. |
| `abbd bootstrap --mode M [--scan] [--verify-available] [--from PATH] [--preview]` | Re-runs bootstrap explicitly. Stable IDs and confirmed intent are kept through anchor matching. Refused during an active turn. |
| `abbd status [--knowledge]` | Revision, fingerprint, freshness of feature map, architecture map, graph and HTML, active turn, pending external changes (`pending_changes`), UNCONFIRMED count. |
| `abbd reconcile [--user]` | External code change: creates a `CHG` with `intent_source: external`, refreshes EXTRACTED facts only, asks nothing. A conflict with a CONFIRMED contract or feature becomes a tier B ADR with default `accept`, and the CHG state is NEEDS_USER. An unapproved tier C structural change is not taken into the blueprint: the CHG stays BLOCKED and the command exits 2 until the card is decided. Refused while a turn is active, and refused when a changed file was edited by the agent (recorded by the PostToolUse hook, `abbd note-edit`): those files are verified in a turn. Also refused while a host agent session is open (`abbd note-session`), since the agent may have made the change with any tool; `--user` says the user made it. |

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
| `abbd run "REQUEST" [--type T] [--tier T0..T3]` | Starts a turn. Bootstraps first when `.abbd/` is missing, then reconciles pending external changes, routes, selects context, resolves intent and writes a draft `CHG` (state INTENT_RESOLVED). Prints the WorkflowPlan: `change`, `task_type`, `verification_tier`, `steps`, `context_strategy`, `context_reason`, `matches`, `context`, `ambiguous`, `new_feature`, `requirement`, `confirm_if_purpose_used`, `budget`, `read_first`, `standing_rules`, plus `rework`, `bootstrap` and `reconciled` when present. Files the agent edited since the last sync are not reconciled; they become part of this turn. Refused while an outside change waits for a tier C approval. `rework` lists structural change cards rejected or sent back on the review screen since the last turn; each line is also appended to this turn's IntentDelta additions, once. Refused while another turn is active, unless that turn only waits on rejected tier C cards: then `run` closes it and its files become this turn's rework. |
| `abbd plan "REQUEST"` | Dry run of route, intent and impact, with `read_first` and `standing_rules`. Writes nothing. |
| `abbd inspect ID` | Feature, module, contract, test or change context with refs and evidence. |
| `abbd intent feature [--id FEAT-x] [--name N] [--purpose P] [--origin O] [--anchor kind:ref]...` | Records the resulting intent of the active turn. Without `--id`, creates a new draft feature. |
| `abbd intent invariant --feature FEAT-x --claim C [--origin O] [--test TEST-x]` | Adds an invariant claim and appends it to the turn's preserved invariants. `--test` links that test's evidence. |
| `abbd intent module (--id MOD-x \| --path DIR) [--responsibility T] [--allow MOD-y]... [--forbid MOD-z]... [--origin O]` | Records a module's responsibility and dependency policy. `--path DIR` names a module this turn creates; sync assigns its MOD ID and moves the recorded responsibility onto it. Any `--allow` or `--forbid` replaces the whole policy. |
| `abbd intent contract --id IF-x [--compatibility T] [--external-consumers true\|false] [--origin O]` | Records a contract's compatibility policy and whether it has external consumers. |
| `abbd driver set FEAT-x|IF-x --run CMD --expect TEXT` | Registers a user-level check for an entry point in `project.yaml`: a command that exercises it the way a user reaches it, and text its output must contain. `--run` takes any command; the engine does not check that it reaches the entry point. A command with an outside effect (push, deploy, publish, `rm -rf /`) is tier C and refused here and again when `verify --drive` would run it. A driver proves only later work: one set or changed in a turn, in a turn that never completed (aborted), or for the same user message as the turn (the UserPromptSubmit mark) does not satisfy that turn's T3 gate or count as its failing-first run. |
| `abbd driver list` | Lists registered drivers. |
| `abbd verify --drive` | Runs the drivers of what the turn touches; each run is user-level evidence, PASSED only when the command succeeds and the output contains the expected text. A T3 turn needs a PASSED run of a driver registered in an earlier completed turn (or outside any turn, for an earlier user message), or `verify --unverifiable user-level --reason`, which lets sync complete with the turn NEEDS_USER and the reason in `risks`. |
| `abbd verify --no-repro --reason TEXT` | Waives the failing-first rule for a bug-fix turn. Needs an active turn and a reason of 10 or more characters. Without it, sync refuses a bug fix unless a test that FAILED on a working tree other than the one that passed PASSES, by name, on the final one, or a user-level driver from an earlier request FAILED before and PASSES on the final tree. A failing test deleted after the reproduction does not count. A command that fails with no failing test (for example `verify --cmd false`) does not count. |
| `abbd learn --from-receipts` | Lists problems that repeated in two or more turns (failing tests, rejected card kinds, blocked reasons) with the first structural fix to try. |
| `abbd intent rule --claim TEXT \| --req REQ-x [--retire]` | Records a standing rule the user stated for the whole project (USER_STATED, project scope), promotes a requirement to one, or retires it. Works without a turn. A rule with mostly the same words as one in force is not stored again; the existing rule is returned. Standing rules are listed in `.abbd/BLUEPRINT.md` and in every `run` and `plan` result. `run` also stores rules it detects in the user's words (see "The user's words"). |
| `abbd intent delta [--add T]... [--remove T]... [--preserve "FEAT-x: claim"]...` | Appends to the turn's IntentDelta and moves the turn from INTENT_RESOLVED to IMPLEMENTING. |
| `abbd decide --question Q --option key=label[=effect]... --default KEY [--answer KEY] [--subject ID]... [--tier B\|C] [--title T] [--proceeds-without SCOPE]` | Records a decision as an ADR on the active turn. See the decide table below. |
| `abbd decide --adr ADR-x [--answer KEY]` | Answers an existing ADR (left by sync, reconcile or an earlier turn) inside or outside a turn. Without `--answer`, returns its current state. An answer needs `--user` and fails without it. An answer that is not one of its options fails; a tier C ADR takes `approve` or `reject`. When the ADR belongs to a structural change card, the answer is recorded as that card's decision, so the card status and rework follow. |
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
through `abbd decide --subject ID`. `intent` (except `intent rule`), `decide --question` and `turn note` need an
active turn.

| decide flag | Meaning |
|---|---|
| `--question Q` | The question. A question already answered in any ADR is not asked again: the result is `reused: true` with the earlier answer. The same unanswered question with `--answer KEY` answers the existing ADR. |
| `--option key=label[=effect]` | One option. Tier B needs 2 or more, also when answering. `effect` says what that choice changes. |
| `--default KEY` | The option that proceeds without an answer. Without it, the first option. |
| `--answer KEY` | An answer. With `--user` it is the user's: the ADR becomes USER_STATED and CONFIRMED. Without `--user` it is the agent's choice: INFERRED, UNCONFIRMED, still open for the user, and it never passes a tier C stop. |
| `--user` | The answer is the user's own words. See "Who decides". |
| `--subject ID` | Item the decision is about; repeatable. A tier C approval releases the sync gate for the subjects it names. |
| `--tier B\|C` | Default B. Tier C needs no options: without `--answer approve --user` the result is `blocked: true`, the stop joins the turn's `blocked` list, and the command exits 2. |
| `--title T` | Short name shown in ADR lists. |
| `--proceeds-without SCOPE` | What goes ahead if nobody answers. |
| `--adr ADR-x` | Answer an existing ADR instead of asking a new question. |

Without `--answer`, a tier B result has `ask: true` while fewer than `policy.max_questions_per_turn` (3)
questions were asked this turn; past that the default is recorded with `ask: false`. An unanswered ADR stays
UNCONFIRMED and the turn ends NEEDS_USER.

## Prove and synchronize (steps 5 to 7)

| Command | What it does |
|---|---|
| `abbd verify [--affected\|--all] [--cmd "CMD"]... [--unverifiable "CHECK" --reason TEXT]` | Runs verification and stores Evidence under `.abbd/verification/runs/`. The report has `overall`, `runs`, `ran`, `could_not_run`, `failed_tests`, `failures` (each failing test with its own `output` and `evidence`) and `warnings`. A command matching a tier C pattern is refused: `git ... push`, `docker push`, `npm publish`, `kubectl apply\|delete\|rollout\|scale\|patch`, `terraform apply\|destroy`, `helm install\|upgrade\|uninstall\|rollback`, `rm -rf /`, `drop table\|database`, or a `deploy` script as the command itself (a path such as `./internal/deploy/...` does not match). Other external effects are not detected. Exits 2 when `overall` is FAILED. |
| `abbd sync [--commit]` | Completion gate. See below. |
| `abbd check [--accept-edits [--user]]` | Schema, provenance rules, reference integrity, evidence links, md/html/graph revision agreement. Prints the report (`ok`, `errors`, `warnings`, `stale`, `freshness`) and exits 2 when `errors` is not empty. A hand edit of an engine area in `.abbd/` md is recorded in state and stays an error, even after other commands rewrite the md, until `--accept-edits` acknowledges it. Until then the engine keeps what it last wrote for any meaning field whose hand edit raised user-only provenance (USER_STATED, CONFIRMED, an answered decision, an approved tier C card) or changed a user-stated value; the blueprint and the gates see that, not the typed text. `--accept-edits --user` keeps the typed text as it is; `--accept-edits` alone takes it with that provenance lowered to INFERRED/UNCONFIRMED, and a user-stated field keeps its recorded value. |
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
"commit 안 함". With no active turn, sync reconciles external changes or regenerates artifacts. That reconcile
refuses files the agent edited and changes made while an agent session is open (unless `--user`), and holds an unapproved tier C change out of the blueprint (BLOCKED, exit 2),
exactly like `abbd reconcile`.

## Review screen (PRD 08B)

| Command | What it does |
|---|---|
| `abbd serve [--addr 127.0.0.1:7787]` | Serves the review screen on a loopback address and prints its URL. Decisions made on the page are written straight to `.abbd/`. Non-loopback addresses are refused. Runs until stopped. |
| `abbd review [CHG-ID]` | Summary of one turn's structural change cards: `path` (the `generated/review.html` file), `change`, `open` (tier B and C cards waiting for a decision), `by_tier`, `deltas`, `message`. Without `CHG-ID`, the active turn when it has cards, else the latest turn that has cards. |
| `abbd review --decide CHG-x#n --decision D [--reason TEXT] [--user]` | Records a decision on one card, then prints the summary. A tier B or C card needs `--user`. |
| `abbd review --apply FILE --user` | Applies a decision file downloaded from a review page opened without `abbd serve`. B/C cards in it need `--user`: the agent can write such a file as easily as the user. |
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
| `abbd turn abort [--reason TEXT]` | Abandons the active turn. Its draft features are dropped and the CHG stays as a BLOCKED record with `aborted: TEXT` in `blocked`. Working-tree changes are kept. Files the agent edited are taken into the next `run`'s turn; other changed files are reconciled. Revert edits that should not stay. |
| `abbd learn --kind architecture\|type\|lint\|test\|doc\|skill --proposal TEXT` | Records a correction proposal in `.abbd/learning.yaml` with `status: proposed`, and in the active turn's next steps. Kinds are listed in order of preference. |
| `abbd learn --preference key=value` | Writes a preference to `preferences` in `.abbd/project.yaml`. |
| `abbd confirm [--list]` | The confirm queue: UNCONFIRMED meaning fields, decisions and merge candidates. Also the result with no arguments. |
| `abbd confirm ID [--value TEXT\|--purpose TEXT] [--reject] [--external-consumers true\|false]` | Lazy confirmation by the user; every form needs `--user` and is refused without it (MCP `confirm_item` is refused too). `ID` alone accepts the inferred text: CONFIRMED, origin unchanged. `--value`, or its alias `--purpose`, replaces it with USER_STATED CONFIRMED text. `--reject` alone marks it REJECTED; with `--value`, the old text is kept as `rejected_value`. `--external-consumers` sets a contract's consumers flag as USER_STATED. |
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
`sync_project_knowledge`, `check_consistency`, `ask_user_decision`, `answer_decision`, `record_tier_c_stop`,
`get_external_contracts`, `analyze_contract_impact`, `list_review_cards`, `decide_structural_change`,
`list_unconfirmed`, `confirm_item`. `list_review_cards` returns the same summary as `abbd review`. The MCP
server is called by the agent, so it is an agent channel with no `--user`: `decide_structural_change` records
only tier A cards, `answer_decision` cannot answer an existing ADR, and an answer passed to
`ask_user_decision` is the agent's INFERRED choice.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success. |
| 1 | Error: unknown command, usage error (the usage line goes to stderr), or a refused operation such as `run` during an active turn, `decide --question` outside a turn, or a card decision without a required reason. Nothing is printed on stdout. |
| 2 | A gate refused, with `BLOCKED: ...` on stderr. The result is still printed. `sync`: verification not PASSED, working tree changed since verification, an unapproved tier C card, or artifacts inconsistent after sync; the turn stays open and artifacts stay DRAFT. `verify`: `overall` is FAILED. `reconcile`, or `sync` with no active turn: an outside change waits for a tier C approval. `decide --tier C` without `--answer approve --user`: `blocked: true`. `check`: `errors` is not empty. |

## Telemetry

Each turn's ChangeReceipt has a `telemetry` block. The harness counts four figures itself. `graph_queries`
counts each `inspect`, `contracts list`, `contracts impact`, `impact`, `graph trace` and `graph neighbors`
during a turn. `test_runs` counts each command `verify` runs; UNVERIFIABLE records do not count.
`questions_asked` counts each tier B `decide` that returned `ask: true`. `duration_sec` is set by `sync` or
`turn abort`. Model calls are the only host-reported figure, added with `abbd turn note --model-calls N`; `model_calls_note`
reads `host-reported` once a figure arrives. Per-tier limits are in `budgets` of
`.abbd/project.yaml`, and `verify` adds a warning when `test_runs` exceeds the turn's limit. The other two limits
are recorded and, past the tier budget, warned in the receipt's `risks`.

## Who decides

`decide --answer`, `decide --adr` and `review --decide` without `--user` record the agent's choice
(INFERRED, still open for the user) or are refused. `--user` is the attestation that the answer is the
user's own; only then is it CONFIRMED, and only a user approval passes a tier C stop. The user's channels
are the `abbd serve` review page, the decision file (`review --apply FILE --user`), and the CLI with `--user` carrying the
user's own words. The MCP decision tools are an agent channel and cannot record the user's decision.

`run --tier` below the routed tier needs `--reason`, which is kept on the receipt.

## The user's words

The plugin's UserPromptSubmit hook runs `abbd note-prompt`, which keeps the user's last message verbatim.
The next `abbd run` records that text, not the agent's request summary, as the USER_STATED requirement,
and stores each sentence that states a project-wide rule as its own standing rule (a rule already in
force with mostly the same words is not stored twice). A sentence states a rule when it has both a scope
word ("항상", "모든", "앞으로", "always", "every", "any route" ...) and a duty word ("해야", "거쳐야 한다" and any
"-야 한다", "하지 않", "금지", "must", "never", "only", "require", "need" ...), or a strong scope on its own ("어디서든", "everywhere", "from now
on"). The words are the user's, but reading them as a rule is the engine's guess: such a rule is in force
and UNCONFIRMED, shown on `.abbd/BLUEPRINT.md` as "user's words, detected as a rule, unconfirmed". Hosts
without the hook fall back to the request text passed to `run`, so pass the user's words there.

## The agent's edits

The plugin's PostToolUse hook (Edit, Write, MultiEdit, NotebookEdit) runs `abbd note-edit`, which records
the edited path in state. Such files are the agent's work and are verified in a turn: `abbd reconcile` and
`sync` with no active turn refuse them, and the next `abbd run` takes them into its turn. A completed sync
clears the record. Edits made through shell commands, and every edit on a host without the hook, are not
recorded by that hook. The SessionStart and SessionEnd hooks run `abbd note-session start|end` (each
prompt also refreshes it): while a session is open, every unsynced change counts as the agent's, whatever
tool made it (`sed -i`, `perl -pi`, a script), so `reconcile` and a turnless `sync` refuse it and `run`
takes it into its turn. A change the user made meanwhile is reconciled with `--user`. A session whose end
hook never ran stops counting after 12 hours. Changes made with no session open (and every change on a host
without these hooks) are reconciled as outside changes. A recorded edit that was later reverted is
dropped from the record on the next command, so it never holds a later human edit of that file.
