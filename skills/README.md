# For My Harness Agent Skills

호스트 중립 Agent Skills다. [Agent Skills 사양](https://agentskills.io/specification)을 따른다. 디렉터리 하나에 `SKILL.md` 하나, frontmatter의 `name`은 디렉터리 이름과 같다. 호스트별 설치 경로와 기능 차이는 [`../hosts/README.md`](../hosts/README.md)에 있다.

모든 skill은 하네스 코어 CLI를 호출한다. 명령과 플래그의 원본은 [`../docs/cli.md`](../docs/cli.md)다. 같은 기능은 `myharness mcp`의 MCP 도구로도 쓸 수 있다.

## 턴 단계와 skill

| 턴 단계 (PRD 04장) | skill | 주요 CLI 명령 | 산출물 / 게이트 |
|---|---|---|---|
| 0. Session bootstrap | session-bootstrap | `myharness status`, `myharness init`, `myharness bootstrap`, `myharness reconcile` | ProjectStateSnapshot |
| 1. Route | session-bootstrap | `myharness run "REQUEST"`, `myharness plan "REQUEST"` | WorkflowPlan + 검증 수준 |
| 2. Understand current | understand-project | `myharness inspect ID`, `myharness graph neighbors ID`, `myharness graph trace`, `myharness contracts list` | 근거 링크 있는 현재 상태 |
| 3. Resolve intent | change-intent | `myharness decide`, `myharness confirm`, `myharness intent feature`, `myharness intent invariant` | IntentDelta / 등급 B 질문 |
| 4. Design | architect-change | `myharness inspect ID`, `myharness graph neighbors ID` | ChangePlan + 검증 계약 |
| 4. Impact | impact-review | `myharness contracts impact`, `myharness impact`, `myharness policy classify` | ChangePlan의 영향 범위, 등급 판정 |
| 5. Implement / 6. Prove | implement-and-prove | `myharness verify`, `myharness turn note` | diff + EvidenceReport |
| 7. Synchronize | sync-knowledge | `myharness sync`, `myharness check`, `myharness render`, `myharness status` | ConsistencyReport, ChangeReceipt |
| 8. Review / ship, 9. Learn | reflect-and-correct | `myharness turn show`, `myharness policy classify`, `myharness turn abort` | 검토된 ChangeReceipt, 규칙/Skill 변경 제안 |

## 실행 순서

기본 순서:

```
session-bootstrap → understand-project → change-intent → architect-change
  → impact-review → implement-and-prove → (impact-review on the real diff)
  → sync-knowledge → reflect-and-correct
```

작은 작업일수록 가볍게 돈다. 단계의 행위는 지키되 모든 단계에 별도 에이전트나 도구 호출을 둘 필요는 없다.

- 단순 함수 버그: session-bootstrap → understand-project → implement-and-prove → sync-knowledge → reflect-and-correct(receipt 확인만).
- 문서만 갱신: session-bootstrap → sync-knowledge(원본 md 수정, render, check).
- 코드 변경 없는 질문: session-bootstrap(`plan`, turn 열지 않음) → understand-project.

어느 경로든 두 게이트는 생략하지 않는다. 필수 검증이 PASSED가 아니면 sync하지 않고, maps/graph/HTML이 CURRENT가 아니면 완료라고 보고하지 않는다.

## 공통 어휘

각 `SKILL.md` 끝의 "공통 어휘" 절은 모든 skill에서 글자 그대로 같다. 원본 정의는 PRD 06장(origin / verification / confirmation 세 축)과 10장(승인 등급 A / B / C)이다. 한 skill에서 고치면 나머지 일곱 개도 같이 고친다.

## 출처

pstack(MIT, Copyright (c) 2026 Lauren Tan)의 poteto-mode 라우팅, how/why, architect, blast-radius, correct, reflect의 아이디어를 참고해 독립적으로 썼다. pstack 파일이나 문장을 복사하지 않았다. 자세한 내용은 [`../THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md)에 있다.
