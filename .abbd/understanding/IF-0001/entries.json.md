---
unit: IF-0001
slot: entries.json
origin: INFERRED
digest: 7e0f5a4c8876d4b5
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: 56-61
      quote: Words []string `json:"words"`
code:
    cmd/abbd/diff.go#quote-8d8181f6: 3e7346802750bd2b
---

Prints the same categories as JSON, with the base revision and the written sentences that changed (`words`, null when none), for a tool rather than a reader.
