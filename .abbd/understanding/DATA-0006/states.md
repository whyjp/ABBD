---
unit: DATA-0006
slot: states
origin: INFERRED
digest: 7d29c1fe03f0ab7b
format: 3
evidence:
    - path: internal/screen/screen.go
      lines: "96"
      quote: 'case in.Tree == "" || in.Synced.Fingerprint == "":'
    - path: internal/screen/screen.go
      lines: "98"
      quote: 'case in.Synced.Merged || in.Tree != in.Synced.Fingerprint:'
    - path: internal/store/store.go
      lines: "274"
      quote: if conflictMarker(l) {
code:
    internal/screen/screen.go#quote-48139f5a: c60ba95ebc6da4c9
    internal/screen/screen.go#quote-ca8afc4b: 6dbc405e08760638
    internal/store/store.go#quote-6ed82a6b: b06055957d82a9ba
---

| state | when | what brings it current |
|---|---|---|
| CURRENT | the tree's fingerprint now equals `fingerprint` | nothing |
| STALE | the fingerprints differ: code changed since the sync | `abbd sync` |
| STALE | a merge left both sides' lines in the file, by `merge=union` or as conflict markers | `abbd sync` on the merged tree |
| UNKNOWN | no git work tree, git failed to fingerprint, or the state has no `fingerprint` | run in a git checkout; a sync in one writes the fingerprint |

`status --check` exits 1 on STALE and on UNKNOWN, and names the cause of an UNKNOWN, because only a sync fixes STALE.
