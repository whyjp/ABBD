---
unit: DATA-0006
slot: entries.fingerprint
origin: INFERRED
digest: 185ec73ac501cb3a
format: 3
evidence:
    - path: internal/store/store.go
      lines: "27"
      quote: // Fingerprint is a git tree of the working tree at sync time; Blueprint is
    - path: internal/screen/screen.go
      lines: "98"
      quote: 'case in.Synced.Merged || in.Tree != in.Synced.Fingerprint:'
code:
    internal/screen/screen.go#quote-48139f5a: c60ba95ebc6da4c9
    internal/store/store.go#quote-d6dd7f11: 8b8b1d7c367cb00c
---

A git tree of the working tree at the last sync, with .abbd left out. status calls the state STALE once the tree no longer matches it, and the next sync diffs it against the tree now to follow renamed files.
