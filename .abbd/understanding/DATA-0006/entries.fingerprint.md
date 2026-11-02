---
unit: DATA-0006
slot: entries.fingerprint
origin: INFERRED
digest: 6b2ee24f5bdac057
format: 3
evidence:
    - path: internal/store/store.go
      lines: "26"
      quote: // State is the sync watermark. Fingerprint is a git tree of the working tree at sync time.
    - path: internal/screen/screen.go
      lines: "106"
      quote: 'case in.Tree != in.Synced.Fingerprint:'
code:
    internal/screen/screen.go#quote-3c3de2d3: 0301285aafec1aeb
    internal/store/store.go#quote-a5f8c5d5: 4f00e18df3e46c9b
---

A git tree of the working tree at the last sync, with .abbd left out. status calls the state STALE once the tree no longer matches it, and the next sync diffs it against the tree now to follow renamed files.
