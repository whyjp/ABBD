---
unit: DATA-0006
slot: entries.high-water
origin: INFERRED
digest: 11f2a81d4640046d
format: 3
evidence:
    - path: internal/identity/identity.go
      lines: "72"
      quote: // highWater holds the largest number ever minted per kind, so deleted IDs are never reused.
code:
    internal/identity/identity.go#quote-ba3ad463: f490d67546a68484
---

The largest number ever minted per kind, so an ID a deleted unit held is never given to another.
