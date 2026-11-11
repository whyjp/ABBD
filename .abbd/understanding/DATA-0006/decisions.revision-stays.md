---
unit: DATA-0006
slot: decisions.revision-stays
origin: INFERRED
digest: 67de959511433ee8
format: 3
evidence:
    - path: cmd/abbd/main.go
      lines: "532"
      quote: if state.Fingerprint != tree || rev == "" {
    - path: cmd/abbd/main.go
      lines: "533"
      quote: rev = gitx.Head(e.root)
code:
    cmd/abbd/main.go#quote-6baa56c0: 2fbf51b0c5fa2aae
    cmd/abbd/main.go#quote-b765a468: 1a833eab583d912b
reread:
    by: concept-orch (Claude Opus 5.5)
    "on": "2026-11-11"
    moved:
        - cmd/abbd/main.go (the lines around it changed)
---

`revision` stays the commit where a tree was first synced. A sync whose tree matches the stored fingerprint keeps the stored revision, and only a new tree, or a state without a revision, takes `HEAD`. Committing `.abbd/` after a sync moves `HEAD` without changing the tree, so the next sync writes the same bytes and `.abbd/` stays clean. A drifted quote dies at the first sync of a later revision, and this keeps one revision from counting as two.
