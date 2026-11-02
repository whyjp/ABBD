---
unit: DATA-0006
slot: decisions.revision-stays
origin: INFERRED
digest: bba516d619cc1abb
format: 3
evidence:
    - path: cmd/abbd/main.go
      lines: "388"
      quote: if state.Fingerprint != tree || rev == "" {
    - path: cmd/abbd/main.go
      lines: "389"
      quote: rev = gitx.Head(e.root)
code:
    cmd/abbd/main.go#quote-6baa56c0: 2fbf51b0c5fa2aae
    cmd/abbd/main.go#quote-b765a468: ac1b670f125cd245
---

`revision` stays the commit where a tree was first synced. A sync whose tree matches the stored fingerprint keeps the stored revision, and only a new tree, or a state without a revision, takes `HEAD`. Committing `.abbd/` after a sync moves `HEAD` without changing the tree, so the next sync writes the same bytes and `.abbd/` stays clean. A drifted quote dies at the first sync of a later revision, and this keeps one revision from counting as two.
