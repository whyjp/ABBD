---
unit: IF-0001
slot: entries.json
origin: INFERRED
digest: 99615a53ca1fde3c
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: "49"
      quote: if e.asJSON {
code:
    cmd/abbd/diff.go#quote-698196f9: 7532039e363702bf
reread:
    by: concept-orch (Claude Opus 5.5)
    "on": "2026-11-10"
    moved:
        - cmd/abbd/diff.go (the lines around it changed)
---

Prints the same categories as JSON, with the base revision, for a tool rather than a reader.
