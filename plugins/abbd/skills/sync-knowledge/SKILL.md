---
name: sync-knowledge
description: Closes the synchronization gate of a ABBD turn. After verification passes, runs sync to update Feature Map, Architecture Map (including the external contract view), decisions, graph, HTML, and the review screen at one revision, then hands the user the review screen link when the turn has structural change cards, without describing the structure in chat or deciding cards for the user. Agent-written semantic fields stay origin INFERRED and purpose is never promoted from tests. Use after implement-and-prove reports all required checks PASSED, and for doc-only turns; never report a turn done before this gate passes.
---

# sync-knowledge

턴 루프의 7단계(Synchronize)를 맡는다. 코드 수정 완료와 맵 동기화는 같은 완료 단위다. 산출물은 ConsistencyReport와, sync가 쓰는 ChangeReceipt다.

## 언제 쓰나

- implement-and-prove가 필수 검증을 모두 PASSED로 보고한 뒤.
- 문서만 갱신하는 턴(T0).
- "맵은 나중에 업데이트"라고 남기고 끝내고 싶어질 때. 그렇게 끝내지 않는다.

## 입력

- EvidenceReport. FAILED가 하나라도 남아 있으면 이 skill을 시작하지 않는다.
- IntentDelta와 ChangePlan(예상 구조 변경 카드 포함).

## 절차

### 1. 의미 필드 작성

sync 전에 이번 변경으로 바뀐 의미를 기록한다. 기계적 층(심볼, 간선, HTML, 리뷰 화면)은 엔진이 결정적으로 갱신한다. 의미 층(purpose, responsibility, invariant)은 에이전트가 쓴다.

```sh
abbd intent feature --id FEAT-001 --name "Heartbeat ingestion" \
  --purpose "재시도 정책을 설정으로 바꿀 수 있는 heartbeat 저장" --origin INFERRED
abbd intent invariant --feature FEAT-001 \
  --claim "재시도 정책이 바뀌어도 중복 저장되지 않는다" --origin INFERRED --test TEST-019
```

- IntentDelta가 바꾸지 않은 의미 필드는 다시 쓰지 않는다. 엔진은 CONFIRMED 필드만 지키고 INFERRED 필드는 덮어쓰게 둔다.
- 에이전트가 쓴 의미 필드는 origin INFERRED다. 사용자가 이번 턴에 말한 문장만 USER_STATED로 쓴다. 테스트 PASSED는 verification만 바꾸고, CONFIRMED는 사용자 확인(`abbd confirm`이나 리뷰 화면의 카드 승인)으로만 생긴다.
- 외부 계약을 바꿨으면 호환성 정책과 외부 소비자 여부를 `intent contract`로, 모듈 책임이나 의존 정책을 `intent module`로 기록한다. 이번 턴이 만든 모듈은 `intent module --path DIR`로 쓴다. selector와 위치 같은 EXTRACTED 사실은 sync가 갱신한다.
- `.abbd/` 아래 md를 손으로 고치지 않는다. intent, confirm, decide, review 명령이 origin과 confirmation 규칙을 지킨다.

```sh
abbd intent contract --id IF-002 --compatibility "필드 추가만 허용"
abbd intent module --id MOD-002 --responsibility "heartbeat 수신과 저장"
```

### 2. sync 게이트

```sh
abbd sync
```

sync는 현재 작업 트리 그대로에서 마지막 verify가 PASSED일 때만 완료한다. 문서 파일만 바뀐 turn은 T0로 보고 `abbd check`를 증거로 기록한다. 완료하면 maps, graph, HTML, 리뷰 화면(`generated/review.html`)을 한 revision으로 갱신하고 ChangeReceipt를 쓴 뒤 check를 스스로 돌린다. 같은 명령을 다시 돌려도 같은 상태로 수렴한다.

종료 코드 0은 완료, 2는 게이트 거부(산출물은 DRAFT, turn은 열린 채), 1은 오류다. 2이면 결과의 `reason`을 읽는다.

| `reason` | 할 일 |
|---|---|
| 검증 NOT_RUN / FAILED / UNVERIFIABLE | implement-and-prove로 돌아가 verify |
| 검증 이후 작업 트리가 바뀜 | 다시 verify |
| 등급 C: 명시적 승인 필요, 뒤에 카드 ID(`CHG-x#n kind subject`) | 상태 BLOCKED. 승인 전에는 Blueprint에 반영되지 않는다. 4절대로 사용자에게 화면을 주고, 승인된 뒤 다시 sync |
| T0 검사 실패, 스키마/계약 검사 실패, 동기화 후 산출물 불일치 | 원인을 고치고 다시 sync |

- 결과에 `review`가 있으면 이번 턴에 구조 변경 카드가 있다는 뜻이다. 4절로 간다. 없으면 구조 변경이 0건이고, 리뷰 화면을 언급하지 않는다.
- 등급 B 카드마다 sync가 UNCONFIRMED ADR(기본값 `approve`)을 남긴다. 답 없는 등급 B ADR이 하나라도 있으면 receipt의 상태는 NEEDS_USER다.
- `abbd sync --commit`은 완료 뒤 로컬 commit을 하나 만든다(`CHG-x: 요청`). `project.yaml`의 local-commit이 등급 A가 아니면 commit하지 않는다. 이때도 sync는 완료(종료 코드 0)이고, `reason`이 "commit 안 함"으로 시작하며 `commit`이 비어 있다. push는 하지 않는다.
- 진행 중인 turn이 없으면 sync는 외부 변경을 재조정하거나 생성물만 다시 만든다.

### 3. 정합성 확인

```sh
abbd check --json
abbd status --json
```

- check는 스키마, provenance 규칙, 참조 무결성, evidence 링크, md/html/graph revision 일치를 본다. 오류가 있으면 종료 코드 2로 끝나고 보고서(`errors`, `warnings`, `stale`)는 그대로 출력된다. `errors`의 각 줄을 고친다.
- status에서 maps, graph, HTML이 모두 CURRENT인지 확인한다. HTML만 STALE이면 `abbd render`로 다시 만들고 check를 다시 돌린다. HTML을 손으로 고치지 않는다.

### 4. 리뷰 화면 넘기기

구조 변경의 리뷰 대상은 채팅 답변이 아니라 리뷰 화면이다. sync 결과의 `review`에는 화면 파일 경로(`path`), 턴(`change`), 등급별 카드 수(`by_tier`), 결정 대기 수(`open`), 카드 목록(`deltas`)이 있다.

1. 사용자에게 링크와 숫자만 준다. 예: "구조 변경 3건(B 2, A 1), 결정 대기 2건. 리뷰 화면: http://localhost:7787/". 카드를 하나씩 글로 풀거나 구조를 채팅에서 설명하지 않는다. 그림과 근거는 화면에 있다.
2. 승인·거부를 바로 기록하려면 서버가 필요하다. `abbd serve`를 백그라운드로 띄우고 출력된 URL을 준다. 서버를 띄울 수 없으면 `review.path`의 HTML 파일 경로를 준다. 그 페이지는 결정 파일을 내려받게 하고, 사용자가 그 파일을 주면 `abbd review --apply FILE`로 반영한다.
3. 카드 결정은 사용자가 한다. 에이전트는 카드를 승인·거부하지 않는다. 사용자가 채팅에서 특정 카드에 대해 결정을 명시적으로 말했을 때만 그 말을 그대로 기록한다. 거부와 되돌리기 요청은 사유가 필요하다.

```sh
abbd serve
abbd review CHG-024 --decide CHG-024#2 --decision reject --reason "구버전 클라이언트 호환 유지"
```

4. 카드에 결정을 기록할 때는 `abbd review --decide`(또는 화면, MCP `decide_structural_change`)를 쓴다. 카드의 ADR에 `abbd decide --adr`로 답하면 카드 상태가 따라오지 않는다.
5. 처리하지 않은 B 카드는 기본값으로 적용된 채 남고 확인 대기 목록에 계속 보인다. 턴 완료를 미루지 않는다.
6. 거부된 카드는 다음 `abbd run`의 `rework`와 IntentDelta로 들어간다. 이번 턴에서 바로 고치지 않는다.
7. `abbd review --relayout`은 사용자가 전체 재배치를 요청했을 때만 쓴다. 평소에는 노드 위치가 revision 사이에 고정된다.

C 카드로 sync가 멈췄으면 같은 방식으로 화면을 주고 BLOCKED로 보고한다. 사용자가 화면에서 승인하거나 채팅에서 명시적으로 승인한 뒤(`abbd decide --tier C --question Q --subject IF-x --answer approve`) 다시 sync한다.

### 5. 문서만 갱신하는 턴

1. `abbd run "REQUEST" --type docs`로 turn을 연다(검증 수준 T0).
2. 고칠 대상을 구분한다. 프로젝트 문서(README, docs/ 등)는 그대로 고친다. 엔진 지식(Feature purpose, Module responsibility, 불변조건, 계약 호환성)은 `intent feature|invariant|module|contract`로 고친다. `.abbd/generated/`는 생성물이므로 고치지 않는다.
3. `abbd sync`로 닫는다. 계약 호환성을 바꿨으면 그 변경도 구조 변경 카드가 된다.
4. `abbd check`로 확인한다. 고친 문장이 코드·계약의 증거와 맞는지 확인하고, 증거가 없으면 INFERRED로 둔다.

## 완료 금지 조건

다음 중 하나라도 해당하면 턴을 완료라고 보고하지 않는다.

- maps, graph, HTML 중 하나라도 STALE 또는 DRAFT.
- check가 오류를 보고함(종료 코드 2).
- sync가 실패하거나 거부됨.
- 필수 검증이 FAILED이거나 PASSED 증거 없이 NOT_RUN.

이때 상태는 BLOCKED이고, 보고에는 무엇이 STALE인지, 원인, 재시도 방법을 적는다. 최신성을 거짓으로 주장하지 않는다. NEEDS_USER(기본값으로 진행한 B 결정이 남음)는 완료이며, 대기 결정 수와 화면 링크를 함께 보고한다.

## 출력: ConsistencyReport

```
ConsistencyReport (CHG-024)
- result_revision: a1b2c3d
- sync_state: feature_map CURRENT, architecture_map CURRENT, graph CURRENT, html CURRENT
- check: 오류 0
- 갱신한 ID: FEAT-001, MOD-002, IF-002
- 새 의미 필드: 2건, origin INFERRED, confirmation UNCONFIRMED
- 구조 변경: 3건 (A 1, B 2), 결정 대기 2건 → 리뷰 화면 링크
- 상태: NEEDS_USER
```

## 공통 어휘 (PRD 06장·08B장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
- 결정을 받는 곳: 구조 변경(모듈, 의존, 계약, 소유, 배포 단위, 기능 경로)의 등급 B·C 결정은 사람이 리뷰 화면(`abbd serve`)의 카드에서 한다. 구조와 무관한 의미 선택만 `abbd decide` 질문으로 묻는다. 채팅에는 구조를 글로 풀지 않고 화면 링크와 대기 카드 수만 남긴다.
- 종료 코드: 0 성공, 1 오류, 2 게이트 거부(sync, check 불일치, 검증 FAILED, 승인 안 된 등급 C decide). 2일 때도 결과가 출력되므로 결과 필드를 읽고 판단한다.
