---
question: How does a charge reach the ledger?
format: 1
steps:
    - unit: MOD-0001
      does: Charge() inserts the charge row
      calls:
        - DATA-0001
      evidence:
        - path: billing/charge.go
          lines: "3"
          symbol: Charge
          quote: INSERT INTO charges
      writer: human
      source: conversation 2026-11-03
      code:
        billing/charge.go#quote-1a2b3c4d: 0123456789abcdef
    - unit: MOD-0002
      does: Post() reads the charge into the book
      note: reads every charge, no cache
      fails: stops when the book is closed
      evidence:
        - path: ledger/book.go
          lines: 3-4
          quote: SELECT amount FROM charges
      writer: reader
      code:
        ledger/book.go#quote-5e6f7a8b: fedcba9876543210
retired:
    - unit: MOD-0001
      does: retries a failed insert
      fails: gives up after three tries
      evidence:
        - path: billing/retry.go
          lines: "3"
          quote: const tries = 3
      code:
        billing/retry.go#quote-9c0d1e2f: 00112233aabbccdd
---

From the insert to the book.
