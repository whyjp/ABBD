---
unit: DATA-0006
slot: wire_sample
origin: INFERRED
digest: 63f5b605ba049532
format: 3
evidence:
    - path: internal/store/store.go
      lines: "30"
      quote: Revision    string             `yaml:"revision"`
    - path: internal/store/store.go
      lines: "31"
      quote: Fingerprint string             `yaml:"fingerprint"`
    - path: internal/store/store.go
      lines: "32"
      quote: Blueprint   string             `yaml:"blueprint"`
code:
    internal/store/store.go#quote-8f961298: b1907888710405e0
    internal/store/store.go#quote-3610ea4c: fdd1bd3cba782385
    internal/store/store.go#quote-94800254: 0fd7e896a2c7c61b
---

```yaml
revision: 8b87be3ab0e29685c841c03ec6bd0a41436e5e57
fingerprint: 7e33d18f48894c23e02284444c4c748686cb9bae
blueprint: ebf9bba1c302f60f8d90ae5470879cdc5e3583debc067bbdece4bd7093bd4a21
high_water:
    MOD: 56
    IF: 22
```
