---
unit: DATA-0006
slot: entries.high-water
origin: INFERRED
digest: 4448970d95a74be5
format: 3
evidence:
    - path: internal/identity/identity.go
      lines: "75"
      quote: // highWater holds the largest number ever minted per kind, so deleted IDs are never reused.
code:
    internal/identity/identity.go#quote-ba3ad463: 8a98f6d508acc78b
reread:
    by: pstack poteto-mode 0.15.36
    "on": "2026-11-13"
    moved:
        - internal/identity/identity.go (the lines around it changed)
---

The largest number ever minted per kind, so an ID a deleted unit held is never given to another.
