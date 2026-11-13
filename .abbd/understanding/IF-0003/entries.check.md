---
unit: IF-0003
slot: entries.check
origin: INFERRED
reader: pstack poteto-mode 0.15.36
digest: 6583b22a1a8b470b
format: 3
evidence:
    - path: cmd/abbd/evidence.go
      lines: "262"
      quote: if e.check && len(behind) > 0 {
code:
    cmd/abbd/evidence.go#quote-c251be8f: fc6eb9a1e10f2883
---

Exits 1 while anything status reports can still be brought current in the same PR: a STALE or UNKNOWN state or blueprint, a STALE module text, note or figure part, a drifted evidence item, or a dangling phrase note whose edge no longer shows. Parent and condition conflicts it prints, and they do not fail it.
