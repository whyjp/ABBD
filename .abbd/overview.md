---
format: 1
zones:
    - name: abbd 스킬
      units:
        - FEAT-0001
        - FEAT-0002
      evidence:
        - path: plugins/abbd/skills/abbd-setup/SKILL.md
          lines: "8"
          quote: Setup reads code with the abbd skill's reader
      code:
        plugins/abbd/skills/abbd-setup/SKILL.md#quote-e504c6fb: 6263b86408a1a5a3
    - name: 읽는 쪽
      units:
        - MOD-0017
        - MOD-0013
        - MOD-0022
        - MOD-0056
        - MOD-0057
      evidence:
        - path: internal/gitx/gitx.go
          lines: "1"
          quote: // Package gitx reads the few git facts ABBD needs.
        - path: internal/owners/owners.go
          lines: "1"
          quote: // Package owners reads who owns which code from a repository's CODEOWNERS.
      code:
        internal/gitx/gitx.go#quote-57ce0d67: 2fb4c125587d4786
        internal/owners/owners.go#quote-c278b53e: 49816dc251f8d21d
    - name: 토폴로지 규칙
      units:
        - MOD-0026
        - MOD-0009
        - MOD-0054
        - MOD-0047
      evidence:
        - path: internal/topology/impact.go
          lines: "1"
          quote: '// Package topology answers questions about the stored topology: one unit at a'
      code:
        internal/topology/impact.go#quote-aae46dc8: b632aab8d7ef2476
---

에이전트는 /abbd와 /abbd-setup 스킬로 abbd CLI를 부른다. sync는 추출을 읽고 ID를 맞추고 인용을 검사해 store에 쓴 뒤 screen이 블루프린트를 그린다. internal/model은 CLI와 대부분의 internal 패키지가 쓰는 어휘라 선을 그리지 않았다.
