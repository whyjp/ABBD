---
unit: DATA-0006
slot: entries.blueprint
origin: INFERRED
digest: bb5851efb7f434e2
format: 3
evidence:
    - path: cmd/abbd/screen.go
      lines: "36"
      quote: in.Synced.Blueprint = in.Inputs
    - path: internal/screen/screen.go
      lines: "87"
      quote: 'case in.Synced.Merged || in.Synced.Blueprint != in.Inputs:'
    - path: internal/store/store.go
      lines: "351"
      quote: 'case p == s.path("state.yaml"):'
code:
    cmd/abbd/screen.go#quote-22ad4b42: c7ae54f56076262b
    internal/screen/screen.go#quote-1fb26f46: 64e2d1d8867b3145
    internal/store/store.go#quote-86797714: aa52130fc1b9ae19
---

The `InputsHash` of `.abbd/` the blueprint was last rendered from, which `publish` writes after it renders `index.html` and `graph.json`. status calls the blueprint STALE when it differs from the hash now, and UNKNOWN when it is empty or `graph.json` is missing. `InputsHash` leaves `state.yaml` out, so writing this value does not change the hash it records.
