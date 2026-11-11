---
unit: DATA-0006
slot: entries.rendered
origin: INFERRED
digest: 0be45b7826f4d0fc
format: 3
evidence:
    - path: cmd/abbd/screen.go
      lines: "34"
      quote: in.Synced.Blueprint, in.Synced.Rendered = in.Inputs, store.HashRender(files)
    - path: internal/screen/screen.go
      lines: "100"
      quote: 'case in.Synced.Rendered != in.Rendered:'
    - path: internal/screen/screen.go
      lines: "74"
      quote: in.Rendered, err = st.BlueprintHash()
code:
    cmd/abbd/screen.go#quote-061a5173: 91f35d6e30a925d0
    internal/screen/screen.go#quote-0fd8e74c: 9d9ceb50e21ba078
    internal/screen/screen.go#quote-68a3e03c: 9d30b4fd53175aa0
---

The hash of the files the last render wrote under `blueprint/`, read with CRLF as LF. status compares it with the hash of the files there now, so a page missing, added, edited by hand or checked out from an older render reads STALE even when `blueprint` still matches the inputs. With no files there at all, as in a checkout that lacks the committed pages, the blueprint is NOT DRAWN rather than STALE.
