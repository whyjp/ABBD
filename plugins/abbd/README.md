# abbd Agent Skills

호스트 중립 Agent Skills다. [Agent Skills 사양](https://agentskills.io/specification)을 따른다. 디렉터리 하나에 `SKILL.md` 하나, frontmatter의 `name`은 디렉터리 이름과 같다. 호스트별 설치 경로와 기능 차이는 [`../README.md`](../README.md)에 있다.

모든 skill은 하네스 코어 CLI를 호출한다. 명령과 플래그의 원본은 [`../docs/cli.md`](../docs/cli.md)다. 같은 기능은 `abbd mcp`의 MCP 도구로도 쓸 수 있다.


## 설치 (Claude Code)

```text
/plugin install abbd --marketplace whyjp/ABBD
```

2.1.275보다 오래된 Claude Code에서는 `/plugin marketplace add whyjp/ABBD` 다음에 `/plugin install abbd@abbd`를 실행한다. 스킬과 MCP 서버가 부르는 CLI는 따로 설치한다: `go install github.com/whyjp/abbd/cmd/abbd@latest`.

## 턴 단계와 skill

| 턴 단계 (PRD 04장) | skill | 주요 CLI 명령 | 산출물 / 게이트 |
|---|---|---|---|
| 0. Session bootstrap | session-bootstrap | `abbd status`, `abbd init [--preview]`, `abbd bootstrap`, `abbd reconcile` | ProjectStateSnapshot |
| 1. Route | session-bootstrap | `abbd run "REQUEST" [--type T] [--tier T0..T3]`, `abbd plan "REQUEST"` | WorkflowPlan + 검증 수준. `run`은 bootstrap과 외부 변경 재조정을 먼저 한다 |
| 2. Understand current | understand-project | `abbd inspect ID`, `abbd graph neighbors ID`, `abbd graph trace`, `abbd contracts list`, `abbd decisions [QUERY]` | 근거 링크 있는 현재 상태 |
| 3. Resolve intent | change-intent | `abbd intent delta`, `abbd intent feature`, `abbd intent invariant`, `abbd intent module`, `abbd intent contract`, `abbd decide`, `abbd decisions`, `abbd confirm` | IntentDelta / 등급 B 질문(턴당 3개, 답한 질문은 재사용) |
| 4. Design | architect-change | `abbd inspect ID`, `abbd graph neighbors ID`, `abbd intent module`, `abbd verification-plan` | ChangePlan + 검증 계약 |
| 4. Impact | impact-review | `abbd contracts impact [--diff]`, `abbd impact`, `abbd policy classify`, `abbd decide --tier C` | ChangePlan의 영향 범위, 등급 판정, 등급 C 정지 기록 |
| 5. Implement / 6. Prove | implement-and-prove | `abbd verification-plan`, `abbd verify`, `abbd verify --unverifiable "CHECK" --reason TEXT`, `abbd turn note --model-calls N` | diff + EvidenceReport(실행함 / 실행 못함) |
| 7. Synchronize | sync-knowledge | `abbd intent contract`, `abbd intent module`, `abbd sync [--commit]`, `abbd check`, `abbd render`, `abbd status` | ConsistencyReport, ChangeReceipt. sync 종료 코드 2는 게이트 거부 |
| 8. Review / ship, 9. Learn | reflect-and-correct | `abbd turn show`, `abbd policy classify`, `abbd turn abort [--reason TEXT]`, `abbd learn --kind K --proposal TEXT`, `abbd learn --preference key=value` | 검토된 ChangeReceipt, `.abbd/learning.yaml`의 교정 제안 |

계측은 하네스가 센다. 그래프 조회(inspect, contracts, impact, graph), 테스트 실행(verify), 질문 수(decide), 소요 시간(sync)은 자동이고, 모델 호출 수만 호스트가 `abbd turn note --model-calls N`으로 보고한다.

## 실행 순서

기본 순서:

```
session-bootstrap → understand-project → change-intent → architect-change
  → impact-review → implement-and-prove → (impact-review on the real diff)
  → sync-knowledge → reflect-and-correct
```

작은 작업일수록 가볍게 돈다. 단계의 행위는 지키되 모든 단계에 별도 에이전트나 도구 호출을 둘 필요는 없다.

- 단순 함수 버그: session-bootstrap → understand-project → implement-and-prove → sync-knowledge → reflect-and-correct(receipt 확인만).
- 문서만 갱신: session-bootstrap(`run --type docs`) → sync-knowledge(문서 수정, 의미 필드는 intent 명령, sync, check).
- 코드 변경 없는 질문: session-bootstrap(`plan`, turn 열지 않음) → understand-project.

어느 경로든 두 게이트는 생략하지 않는다. 필수 검증이 PASSED가 아니면 sync하지 않고, maps/graph/HTML이 CURRENT가 아니면 완료라고 보고하지 않는다.

## 공통 어휘

각 `SKILL.md` 끝의 "공통 어휘" 절은 모든 skill에서 글자 그대로 같다. 원본 정의는 PRD 06장(origin / verification / confirmation 세 축)과 10장(승인 등급 A / B / C)이다. 한 skill에서 고치면 나머지 일곱 개도 같이 고친다.

## 출처

pstack(MIT, Copyright (c) 2026 Lauren Tan)의 poteto-mode 라우팅, how/why, architect, blast-radius, correct, reflect의 아이디어를 참고해 독립적으로 썼다. pstack 파일이나 문장을 복사하지 않았다. 자세한 내용은 [`../THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md)에 있다.
