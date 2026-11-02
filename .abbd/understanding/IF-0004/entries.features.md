---
unit: IF-0004
slot: entries.features
origin: INFERRED
digest: cf869a8bfe36898d
format: 3
evidence:
    - path: cmd/abbd/impact.go
      lines: "35"
      quote: if e.features {
code:
    cmd/abbd/impact.go#quote-528242a6: 34e9bc1b9fe25bb0
---

Groups what the change reaches by feature: each page or skill it reaches, its reached parts under it, and the reached contracts it uses with the entries it names. Units no feature holds follow under no feature.
