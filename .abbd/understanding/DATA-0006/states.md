---
unit: DATA-0006
slot: states
origin: INFERRED
digest: 9233b4e791ce8514
format: 3
evidence:
    - path: internal/screen/screen.go
      lines: "102"
      quote: 'case in.Tree == "" || in.Synced.Fingerprint == "":'
    - path: internal/screen/screen.go
      lines: "104"
      quote: 'case in.Tree != in.Synced.Fingerprint:'
code:
    internal/screen/screen.go#quote-3c3de2d3: 0301285aafec1aeb
    internal/screen/screen.go#quote-ca8afc4b: 7c2bf3a7528a2410
---

| state | when | what brings it current |
|---|---|---|
| CURRENT | the tree's fingerprint now equals `fingerprint` | nothing |
| STALE | the fingerprints differ: code changed since the sync | `abbd sync` |
| UNKNOWN | no git work tree, git failed to fingerprint, or the state has no `fingerprint` | run in a git checkout; a sync in one writes the fingerprint |

`status --check` exits 1 on STALE and on UNKNOWN, and names the cause of an UNKNOWN, because only a sync fixes STALE.
