---
unit: IF-0001
slot: entries.check
origin: INFERRED
digest: 628e543d2b271d31
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: "44"
      quote: if e.check && d.Boundary() {
code:
    cmd/abbd/diff.go#quote-2469385d: 18cc88c2808b81f0
---

Exits 1 when the change reaches the boundary between modules: a contract signature or entry changed, a contract came or went, or a relation between top-level modules appeared or disappeared. It decides nothing itself. A CI job reads the exit to label the PR.
