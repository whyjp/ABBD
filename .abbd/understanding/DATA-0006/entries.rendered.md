---
unit: DATA-0006
slot: entries.rendered
origin: INFERRED
digest: 2b7b5cc53c6b16d3
format: 3
evidence:
    - path: cmd/abbd/screen.go
      lines: "34"
      quote: in.Synced.Blueprint, in.Synced.Rendered = in.Inputs, store.HashRender(files)
    - path: internal/screen/screen.go
      lines: "93"
      quote: in.Synced.Rendered != in.Rendered
    - path: internal/screen/screen.go
      lines: "74"
      quote: in.Rendered, err = st.BlueprintHash()
code:
    cmd/abbd/screen.go#quote-061a5173: 91f35d6e30a925d0
    internal/screen/screen.go#quote-0fd8e74c: 9d9ceb50e21ba078
    internal/screen/screen.go#quote-305379ee: 795c56b8085b0c5b
reread:
    by: concept-orch (Claude Opus 5.5)
    "on": "2026-11-11"
    moved:
        - internal/screen/screen.go (the lines around it changed)
---

The hash of the files the last render wrote under `blueprint/`, read with CRLF as LF. status compares it with the hash of the files there now, so a page missing, added, edited by hand or checked out from an older render reads STALE even when `blueprint` still matches the inputs.
