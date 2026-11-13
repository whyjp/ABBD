---
unit: IF-0001
slot: entries.json
origin: INFERRED
reader: pstack poteto-mode 0.15.36
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

같은 범주를 기준 리비전과 함께 JSON으로 찍고, `words`도 담는다. `words`는 기준 이후 생기거나 사라지거나 바뀐 모듈 글, slot 노트, flow step마다 항목 하나이고, 각 항목은 키, 종류, 유닛, 변경, 그리고 잃은 문장과 얻은 문장을 개수 제한 없이 모두 담는다. 사람이 아니라 도구가 읽는 출력이다.
