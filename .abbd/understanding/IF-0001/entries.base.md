---
unit: IF-0001
slot: entries.base
origin: INFERRED
digest: ba8d1bba11ee2603
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: "19"
      quote: if base, err = gitx.MergeBase(e.root); err != nil {
code:
    cmd/abbd/diff.go#quote-92dcb906: b91f5247373b4ffa
---

The revision to compare with. Left out, diff takes the merge base with the default branch, so a PR shows only its own change.
