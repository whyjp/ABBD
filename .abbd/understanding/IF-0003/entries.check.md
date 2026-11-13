---
unit: IF-0003
slot: entries.check
origin: INFERRED
reader: pstack poteto-mode 0.15.36
digest: 887d5751860f5230
format: 3
evidence:
    - path: cmd/abbd/evidence.go
      lines: "262"
      quote: if e.check && len(behind) > 0 {
    - path: cmd/abbd/evidence.go
      lines: "259"
      quote: for _, c := range append(model.ParentConflicts(in.Units), model.ConditionConflicts(in.Units)...) {
code:
    cmd/abbd/evidence.go#quote-c251be8f: fc6eb9a1e10f2883
    cmd/abbd/evidence.go#quote-fcd7c4d4: 7236e81f83631873
---

Exits 1 while anything status reports can still be brought current in the same PR: a STALE or UNKNOWN state or blueprint, a STALE module text or note, a STALE flow step, stack layer or table row, a drifted evidence item, or a dangling phrase note whose edge no longer shows. The `conflict:` lines it prints, a parent or a relation condition that two writers give differently, never fail it.
