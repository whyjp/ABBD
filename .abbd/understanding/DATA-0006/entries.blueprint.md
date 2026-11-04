---
unit: DATA-0006
slot: entries.blueprint
origin: INFERRED
digest: 356042be036174f3
format: 3
evidence:
    - path: cmd/abbd/screen.go
      lines: "34"
      quote: in.Synced.Blueprint, in.Synced.Rendered = in.Inputs, store.HashRender(files)
    - path: internal/screen/screen.go
      lines: "85"
      quote: 'case in.Rendered == "" || in.Synced.Blueprint == "" || in.Synced.Rendered == "":'
    - path: internal/store/store.go
      lines: "428"
      quote: 'case p == s.path("state.yaml"), p == s.path("lint-debt.yaml"):'
code:
    cmd/abbd/screen.go#quote-061a5173: 91f35d6e30a925d0
    internal/screen/screen.go#quote-d414126d: e3c72928ae868344
    internal/store/store.go#quote-29b1c1a4: a9b079924db104fe
---

The `InputsHash` of `.abbd/` the blueprint was last rendered from, which `publish` writes after `SaveBlueprint` has written every page and `graph.json`. status calls the blueprint STALE when it differs from the hash now, and UNKNOWN when it is empty or `blueprint/` holds no files. `InputsHash` leaves `state.yaml` out, so writing this value does not change the hash it records. It leaves `lint-debt.yaml` out too, since no page draws it, so `abbd lint --record` leaves the blueprint current.
