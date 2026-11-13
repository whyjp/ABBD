---
unit: IF-0001
slot: entries.json
origin: INFERRED
digest: d56d9f7b07d75819
format: 3
evidence:
    - path: cmd/abbd/diff.go
      lines: "66"
      quote: Words []wordChange `json:"words"`
    - path: cmd/abbd/words.go
      lines: "44"
      quote: Lost   []string `json:"lost"`
code:
    cmd/abbd/diff.go#quote-04e8f4bf: bf0cb96264136bd2
    cmd/abbd/words.go#quote-b0e0124e: afb8df8f6c45f674
---

Prints the same categories as JSON, with the base revision and `words`: one entry per module text, slot note or flow step that came, went or changed since the base, with its key, kind, unit, change and every sentence it lost and gained, uncapped, for a tool rather than a reader.
