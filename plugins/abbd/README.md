# abbd Agent Skills

호스트 중립 Agent Skills다. [Agent Skills 사양](https://agentskills.io/specification)을 따른다. 디렉터리 하나에 `SKILL.md` 하나, frontmatter의 `name`은 디렉터리 이름과 같다. 호스트별 설치 경로와 기능 차이는 [`../README.md`](../README.md)에 있다.

모든 skill은 하네스 코어 CLI를 호출한다. 명령, 플래그, 종료 코드의 원본은 [`../../docs/cli.md`](../../docs/cli.md)다. 같은 기능은 `abbd mcp`의 MCP 도구로도 쓸 수 있다. MCP 서버는 에이전트가 부르는 통로라 사용자 결정을 기록하지 못한다(아래 "엔진이 막는 것").


## 설치 (Claude Code)

```text
/plugin install abbd --marketplace whyjp/ABBD
```

2.1.275보다 오래된 Claude Code에서는 `/plugin marketplace add whyjp/ABBD` 다음에 `/plugin install abbd@abbd`를 실행한다. 스킬과 MCP 서버가 부르는 CLI는 따로 설치한다: `go install github.com/whyjp/abbd/cmd/abbd@latest`.

## 턴 단계와 skill

| 턴 단계 (PRD 04장) | skill | 주요 CLI 명령 | 산출물 / 게이트 |
|---|---|---|---|
| 0. Session bootstrap | session-bootstrap | `.abbd/BLUEPRINT.md`, `abbd status`, `abbd init [--preview]`, `abbd bootstrap`, `abbd reconcile`, `abbd review`, `abbd intent rule --claim TEXT` | ProjectStateSnapshot, 지난 턴의 결정 대기 카드 수, 사용자의 standing rule |
| 1. Route | session-bootstrap | `abbd run "REQUEST" [--type T] [--tier T0..T3 [--reason TEXT]]`, `abbd plan "REQUEST"` | WorkflowPlan + 검증 수준 + `read_first` + `standing_rules`(`plan`도 둘을 준다). `run`은 bootstrap과 외부 변경 재조정을 먼저 하고, 에이전트가 고친 파일은 재조정하지 않고 turn에 넣고, 거부된 카드를 `rework`로 넘긴다. 수준을 낮추려면 `--reason`이 필요하다 |
| 2. Understand current | understand-project | `abbd inspect ID`, `abbd graph neighbors ID`, `abbd graph trace`, `abbd contracts list`, `abbd decisions [QUERY]` | 근거 링크 있는 현재 상태 |
| 3. Resolve intent | change-intent | `abbd intent rule`, `abbd intent delta`, `abbd intent feature`, `abbd intent invariant`, `abbd intent module`, `abbd intent contract`, `abbd decide`, `abbd decide --adr ADR-x --answer KEY --user`, `abbd decisions`, `abbd confirm`, `abbd review CHG-x` | IntentDelta(rework 먼저, 새 경로에 걸리는 standing rule) / 등급 B 질문(턴당 3개, 답한 질문은 재사용) |
| 4. Design | architect-change | `abbd inspect ID`, `abbd graph neighbors ID`, `abbd intent module --id/--path`, `abbd verification-plan` | ChangePlan(예상 구조 변경 카드 포함) + 검증 계약 |
| 4. Impact | impact-review | `abbd contracts impact [--diff]`, `abbd impact`, `abbd policy classify`, `abbd decide --tier C` | ChangePlan의 영향 범위, 예상 카드와 등급, 등급 C 정지 기록(종료 코드 2) |
| 5. Implement / 6. Prove | implement-and-prove | `abbd verification-plan`, `abbd verify`, `abbd verify --no-repro --reason TEXT`, `abbd driver set ID --run CMD --expect TEXT`, `abbd verify --drive`, `abbd verify --unverifiable "CHECK" --reason TEXT`, `abbd turn note --model-calls N` | diff + EvidenceReport(재현 실패 → 수정 후 통과, T3 사용자 수준 실행, 실행함 / 실행 못함). verify FAILED는 종료 코드 2 |
| 7. Synchronize | sync-knowledge | `abbd intent contract`, `abbd intent module`, `abbd sync [--commit]`, `abbd check`, `abbd render`, `abbd status`, `abbd serve`, `abbd review --decide ... --user` | ConsistencyReport, ChangeReceipt, 새 BLUEPRINT.md, 구조 변경이 있으면 리뷰 화면 링크. sync·check 종료 코드 2는 게이트 거부 |
| 8. Review / ship, 9. Learn | reflect-and-correct | `abbd turn show`, `abbd review CHG-x`, `abbd policy classify`, `abbd turn abort [--reason TEXT]`, `abbd learn --from-receipts`, `abbd learn --kind K --proposal TEXT`, `abbd learn --preference key=value` | 검토된 ChangeReceipt, `.abbd/learning.yaml`의 교정 제안 |

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

## 컨텍스트

에이전트의 컨텍스트는 `.abbd/BLUEPRINT.md` 한 장에서 시작한다. standing rule, 모듈, 계약, 기능, 불변조건, 열린 결정이 지금 값으로 있고 publish마다 다시 만들어진다. 그다음 `abbd run`의 `read_first`가 고른 md 원본을 읽는다. 요청마다 쌓이는 이력(`.abbd/changes`, `requirements`, `decisions`, `verification`, `generated`, state·candidates·learning·bootstrap-report 파일)은 컨텍스트가 아니다. 엔진이 쓰는 `.abbd/.ignore`가 ripgrep 기반 검색에서 빼고, 이 플러그인의 PreToolUse 훅(`scripts/history-guard.sh`)이 이력 경로를 여는 흔한 읽기(Read·Grep·Glob, `cat`, `cd .abbd`, `.abbd` 아래 와일드카드, `find`, 이력 경로의 `git show`·`git log`)를 거부한다. 모든 우회를 막는 장치는 아니다. 이력은 `abbd inspect`, `abbd decisions`, `abbd turn show`, `abbd review CHG-x`로 묻는다.

사용자가 프로젝트 전체에 건 규칙은 `abbd intent rule --claim`으로 standing rule이 되고, 새 경로에도 적용된다. `intent rule`은 turn 없이도 동작하고 같은 규칙을 두 번 만들지 않는다. `run`은 사용자의 메시지에서 범위를 넓히는 말과 의무를 나타내는 말이 함께 있는 문장(또는 "everywhere", "from now on" 같은 강한 범위의 문장)을 문장마다 규칙으로 저장한다. 이 규칙은 UNCONFIRMED이고 BLUEPRINT.md에 "user's words, detected as a rule, unconfirmed"로 표시된다.

이 플러그인의 PostToolUse 훅(`scripts/note-edit.sh`)은 에이전트가 Edit·Write·MultiEdit·NotebookEdit로 고친 파일을 기록한다. 그 파일은 외부 변경이 아니므로 `reconcile`과 turn 없는 `sync`가 거부하고, 다음 `abbd run`이 turn으로 가져가 verify와 sync를 거치게 한다. 셸 명령으로 고친 파일은 기록되지 않는다.

## 엔진이 막는 것

skill 문장 대신 엔진이 거부하는 규칙이다. skill은 해당 명령과 게이트만 가리킨다.

| 규칙 | 막는 곳 |
|---|---|
| `--user` 없는 답은 사용자 결정이 아니다. 등급 C를 풀지 못한다. `--adr` 답은 `--user`가 있어야 하고, 카드의 ADR이면 카드 결정으로 기록된다 | `decide`, `review --decide` |
| MCP는 에이전트 통로다. B·C 카드와 기존 ADR을 결정하지 못하고, MCP 질문에 붙인 답은 에이전트의 선택이다 | MCP `decide_structural_change`, `answer_decision`, `ask_user_decision` |
| 버그 수정 턴은 수정 전 작업 트리에서 실패한 테스트가 있는 FAILED verify(또는 사용자 수준 FAILED)가 있어야 한다. `verify --no-repro --reason`(10자 이상)으로 면제 | sync |
| T3 턴은 사용자 수준 실행이 있어야 한다(앞선 turn에 등록한 driver의 `verify --drive` PASSED, 또는 `verify --unverifiable user-level --reason`이면 NEEDS_USER). 같은 turn에 등록한 driver는 치지 않는다. driver 명령이 진입점을 지나는지는 검사하지 않는다 | sync |
| 승인 안 된 등급 C 구조 변경은 Blueprint에 들어가지 않는다. turn 밖의 변경도 같다 | sync, reconcile, `run`의 재조정 |
| 에이전트가 turn 없이 고친 파일은 외부 변경으로 흡수되지 않는다 | reconcile, turn 없는 sync |
| verify 명령이 등급 C 행동(push, publish, apply, deploy 등)이면 실행하지 않는다 | verify |
| 라우팅된 수준보다 낮추려면 이유가 있어야 한다 | `run --tier` |
| 이번 턴 범위 밖 항목에 INFERRED 의미를 쓰지 않는다 | `intent` |
| `.abbd/` md의 엔진 영역을 손으로 고치지 않는다. 손편집 기록은 `check --accept-edits`까지 남는다 | check |
| 예산 초과(모델 호출, 그래프 조회, 테스트 실행)는 경고로 남는다 | verify, sync |

## 리뷰 화면

구조 변경(모듈, 의존, 계약, 소유, 배포 단위, 기능 경로)은 sync가 카드로 만들어 리뷰 화면에 올린다. 사람은 `abbd serve`가 띄운 화면에서 카드마다 승인·거부·사유를 남긴다. skill은 채팅에 구조를 풀어 쓰지 않고 화면 링크와 대기 카드 수만 전하며, 카드를 대신 결정하지 않는다. 거부된 카드는 다음 `abbd run`의 `rework`로 돌아온다. 모듈 안쪽 코드만 바뀐 턴은 카드가 없고 화면 알림도 없다.

## 공통 어휘

각 `SKILL.md` 끝의 "공통 어휘" 절은 모든 skill에서 글자 그대로 같다. 원본 정의는 PRD 06장(origin / verification / confirmation 세 축), 08B장(리뷰 화면), 10장(승인 등급 A / B / C)이다. 한 skill에서 고치면 나머지 일곱 개도 같이 고친다.

## 출처

pstack(MIT, Copyright (c) 2026 Lauren Tan)의 poteto-mode 라우팅, how/why, architect, blast-radius, correct, reflect의 아이디어를 가져왔다. 파일과 원문 문장은 복사하지 않았다. 이전 판에는 pstack 문단을 한국어로 옮긴 부분이 있었고, 이번 판에서 ABBD 엔진의 동작에서 다시 썼다. 문단 단위 번역은 없고, 문장 단위 번안이 16곳 남아 있다. 무엇을 가져왔는지는 [`../../THIRD_PARTY_NOTICES.md`](../../THIRD_PARTY_NOTICES.md)에 있다.
