# myharness CLI (Phase 1 draft)

All commands run from the target project root (or `--root DIR`). Output is text; add `--json` for the
Project Intelligence API shape (identical to the MCP tool result).

| Command | Turn step | What it does |
|---|---|---|
| `myharness init [DIR] [--mode greenfield\|code-only\|adopt] [--from PATH] [--verify-available]` | 0 | Detect existing knowledge; without it run Zero-Map Bootstrap. Never asks questions. Writes `.myharness/` and `BootstrapReport`. |
| `myharness bootstrap --mode M [--scan] [--verify-available] [--from PATH]` | 0 | Re-run bootstrap explicitly. Stable IDs are preserved through anchor matching. |
| `myharness status [--knowledge]` | 0 | Revision, fingerprint, freshness of maps/graph/HTML, active turn, pending external changes. |
| `myharness reconcile` | 0 | External code change: create `CHG` with `intent_source: external`, refresh EXTRACTED facts only, no questions. |
| `myharness run "REQUEST"` | 0-3 | Start a turn: session bootstrap, route, select context, resolve intent, write a draft `CHG`, print the WorkflowPlan. |
| `myharness plan "REQUEST"` | 1-4 | Dry run of route + intent + impact. Writes nothing. |
| `myharness inspect ID` | 2 | Feature / module / contract / test context with refs and evidence. |
| `myharness intent feature --name N --purpose P [--origin USER_STATED\|INFERRED] [--id FEAT-x] [--anchor kind:ref]...` | 3 | Record the resulting intent of the active turn. |
| `myharness intent invariant --feature FEAT-x --claim C [--origin O] [--test TEST-x]` | 3 | Add an invariant claim. |
| `myharness decide --question Q --option key=label[=effect] ... --default KEY [--answer KEY] [--subject ID] [--tier B]` | 3-4 | Tier B decision: records ADR; unanswered = default + UNCONFIRMED; max 3 asked per turn. |
| `myharness impact` | 4 | Change impact of the working tree vs the turn base: contracts first, then modules, features, tests, unknowns. |
| `myharness contracts list [--module MOD-x]` / `myharness contracts impact` | 4 | External contract view and the tier B/C triggers of the current diff. |
| `myharness graph trace --from ID --to ID` / `myharness graph neighbors ID [--depth N]` | 2,4 | Bounded subgraph queries with edge provenance. |
| `myharness policy classify --action ACTION` | any | Approval tier A/B/C of an action (push, deploy, delete-external, secret, local-commit, ...). |
| `myharness verify [--affected\|--all] [--cmd "CMD"]` | 6 | Run verification commands, store Evidence, update the turn. |
| `myharness sync` | 7 | Gate: only after PASSED verification. Update maps, graph, HTML at one revision; write ChangeReceipt. Failure leaves DRAFT. |
| `myharness turn show` / `myharness turn note --model-calls N` / `myharness turn abort` | 8 | Inspect the receipt, record host telemetry, abandon a turn. |
| `myharness confirm [--list] [ID [--purpose TEXT\|--reject --value TEXT]] [--merge CAND-x\|--split CAND-x]` | any | Lazy confirmation of UNCONFIRMED items. |
| `myharness render` | 7 | Regenerate `generated/explorer.html` and `generated/confirm-queue.html` from the Markdown sources. |
| `myharness check` | 7 | Schema, provenance rules, reference integrity, md/html/graph revision agreement. |
| `myharness open-docs` | any | Print the path of the generated HTML Explorer. |
| `myharness mcp` | any | MCP stdio server exposing the same API. |
| `myharness domain ...` | - | Reserved for Phase 3. Exits with a "reserved" message. |
| `myharness eval run` | - | Optional appendix A benchmark. Not part of Phase 1. |
