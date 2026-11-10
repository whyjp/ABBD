---
unit: IF-0001
slot: entries.check
origin: INFERRED
digest: ae464dc9ef43a33f
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: "73"
      quote: if e.check && d.Boundary() {
code:
    cmd/abbd/diff.go#quote-2469385d: f366a2c3527bc805
reread:
    by: concept-orch (Claude Opus 5.5)
    "on": "2026-11-10"
    moved:
        - cmd/abbd/diff.go (the lines around it changed)
---

Exits 1 when the change reaches the boundary between modules: a contract signature or entry changed, a contract came or went, or a relation between top-level modules appeared or disappeared. It decides nothing itself. A CI job reads the exit to label the PR.
