---
unit: IF-0001
slot: entries.base
origin: INFERRED
digest: 1bbedeb29c78e674
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: "27"
      quote: if base, err = gitx.MergeBase(e.root); err != nil {
code:
    cmd/abbd/diff.go#quote-92dcb906: f47b2475a2822773
reread:
    by: concept-orch (Claude Opus 5.5)
    "on": "2026-11-10"
    moved:
        - cmd/abbd/diff.go (the lines around it changed)
---

The revision to compare with. Left out, diff takes the merge base with the default branch, so a PR shows only its own change.
