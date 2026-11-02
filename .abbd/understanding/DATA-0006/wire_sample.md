---
unit: DATA-0006
slot: wire_sample
origin: INFERRED
digest: 2b610a88e5ce15ad
format: 3
evidence:
    - path: internal/store/store.go
      lines: "32"
      quote: Revision    string             `yaml:"revision"`
    - path: internal/store/store.go
      lines: "34"
      quote: Blueprint   string             `yaml:"blueprint"`
    - path: internal/store/store.go
      lines: "35"
      quote: Rendered    string             `yaml:"rendered"`
code:
    internal/store/store.go#quote-8f961298: c94e417b4c816d29
    internal/store/store.go#quote-3610ea4c: 974357860e038ae2
    internal/store/store.go#quote-28231f2c: 9a93eb5cc6adc5e8
---

```yaml
revision: c22dff8fda6f6527d729bb64ff835d127c25b551
fingerprint: f47f6bda02522b323f9cf97e3c542806dfeecb9e
blueprint: 0d528176012c93f1bef2f78f8238d4ffce4d7ffc56161a283ba9708bdb14f1b1
rendered: 5137c5b3144d6ac0485ad7d17c08bf1ac091040b35af61ba8d52746e056d8b8f
high_water:
    MOD: 56
    IF: 22
```
