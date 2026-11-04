---
unit: IF-0003
slot: entries.check
origin: INFERRED
digest: f094983865ac4943
format: 3
evidence:
    - path: cmd/abbd/evidence.go
      lines: "203"
      quote: if e.check && len(behind) > 0 {
code:
    cmd/abbd/evidence.go#quote-c251be8f: 613e5842a473e7a6
---

Exits 1 while anything status reports can still be brought current in the same PR: a STALE state, blueprint, text or note, a drifted evidence item, or a dangling phrase note whose edge no longer shows.
