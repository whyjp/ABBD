---
unit: DATA-0006
slot: entries.fingerprint
origin: INFERRED
digest: 578adad1d6eb57bc
format: 3
evidence:
    - path: internal/store/store.go
      lines: "33"
      quote: Fingerprint string             `yaml:"fingerprint"`
    - path: internal/screen/screen.go
      lines: "98"
      quote: 'case in.Synced.Merged || in.Tree != in.Synced.Fingerprint:'
code:
    internal/screen/screen.go#quote-48139f5a: c60ba95ebc6da4c9
    internal/store/store.go#quote-94800254: 2b82ea06d4f6ba9b
---

A git tree of the working tree at the last sync, with .abbd left out. status calls the state STALE once the tree no longer matches it, and the next sync diffs it against the tree now to follow renamed files.
