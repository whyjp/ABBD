---
unit: DATA-0006
slot: entries.blueprint
origin: INFERRED
digest: 5d7c5d4de38c0255
format: 3
evidence:
    - path: cmd/abbd/screen.go
      lines: "34"
      quote: in.Synced.Blueprint, in.Synced.Rendered = in.Inputs, store.HashRender(files)
    - path: internal/screen/screen.go
      lines: "96"
      quote: 'case in.Synced.Merged || in.Synced.Blueprint != in.Inputs:'
    - path: internal/screen/screen.go
      lines: "99"
      quote: return Undrawn
    - path: internal/store/store.go
      lines: "464"
      quote: 'case p == s.path("state.yaml"), p == s.path("lint-debt.yaml"):'
code:
    cmd/abbd/screen.go#quote-061a5173: 91f35d6e30a925d0
    internal/screen/screen.go#quote-1fb26f46: 7ef8936112d6079d
    internal/screen/screen.go#quote-e45d6ddd: 4be59cdfd830ac48
    internal/store/store.go#quote-29b1c1a4: e7406a0d740d07ef
---

The `InputsHash` of `.abbd/` the blueprint was last rendered from, which `publish` writes after `SaveBlueprint` has written every page and `graph.json`. status calls the blueprint STALE when it differs from the hash now, and UNKNOWN when it is empty. While it matches and `blueprint/` holds no files, as in a fresh clone since git keeps no pages, status says the blueprint is NOT DRAWN and the check passes. `InputsHash` leaves `state.yaml` out, so writing this value does not change the hash it records. It leaves `lint-debt.yaml` out too, since no page draws it, so `abbd lint --record` leaves the blueprint current.
