---
unit: IF-0001
slot: entries.json
origin: INFERRED
digest: 5dddefc58202a048
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: "32"
      quote: if e.asJSON {
code:
    cmd/abbd/diff.go#quote-698196f9: f06f41c4b443b16c
---

Prints the same categories as JSON, with the base revision, for a tool rather than a reader.
