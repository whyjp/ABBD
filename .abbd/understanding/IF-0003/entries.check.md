---
unit: IF-0003
slot: entries.check
origin: INFERRED
digest: 413730ba13787a17
format: 3
evidence:
    - path: cmd/abbd/evidence.go
      lines: "179"
      quote: if e.check && len(behind) > 0 {
code:
    cmd/abbd/evidence.go#quote-c251be8f: 016b33677f56b704
---

Exits 1 while anything status reports can still be brought current in the same PR: a STALE state, blueprint, text or note, or a drifted evidence item.
