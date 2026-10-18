---
name: sync-knowledge
description: Closes the synchronization gate of a For My Harness turn. After verification passes, runs sync to update Feature Map, Architecture Map (including the external contract view), decisions, graph, and HTML at one revision, then check and status to prove nothing is STALE. Agent-written semantic fields stay origin INFERRED and purpose is never promoted from tests. Use after implement-and-prove reports all required checks PASSED, and for doc-only turns; never report a turn done before this gate passes.
---

# sync-knowledge

턴 루프의 7단계(Synchronize)를 맡는다. 코드 수정 완료와 맵 동기화는 같은 완료 단위다. 산출물은 ConsistencyReport와, sync가 쓰는 ChangeReceipt다.

## 언제 쓰나

- implement-and-prove가 필수 검증을 모두 PASSED로 보고한 뒤.
- 문서만 갱신하는 턴(T0).
- "맵은 나중에 업데이트"라고 남기고 끝내고 싶어질 때. 그렇게 끝내지 않는다.

## 입력

- EvidenceReport. FAILED가 하나라도 남아 있으면 이 skill을 시작하지 않는다.
- IntentDelta와 ChangePlan.

## 절차

### 1. 의미 필드 작성

sync 전에 이번 변경으로 바뀐 의미를 기록한다. 기계적 층(심볼, 간선, HTML 렌더)은 하네스가 결정적으로 갱신한다. 의미 층(purpose, responsibility, invariant)은 에이전트가 쓴다.

```sh
myharness intent feature --id FEAT-001 --name "Heartbeat ingestion" \
  --purpose "재시도 정책을 설정으로 바꿀 수 있는 heartbeat 저장" --origin INFERRED
myharness intent invariant --feature FEAT-001 \
  --claim "재시도 정책이 바뀌어도 중복 저장되지 않는다" --origin INFERRED --test TEST-019
```

규칙:

- IntentDelta가 바꾸지 않은 의미 필드는 다시 쓰지 않는다. 기존 USER_STATED·CONFIRMED 문장을 에이전트 문장으로 덮으면 확정 의도가 추정으로 내려간다.
- 에이전트가 쓴 의미 필드는 반드시 origin INFERRED다. 사용자가 이번 턴에 말한 문장만 USER_STATED로 쓴다.
- 테스트 PASSED는 verification만 PASSED로 바꾼다. origin과 confirmation은 그대로다.
- purpose는 테스트, 커버리지, 실행 결과 어떤 것으로도 승격하지 않는다. CONFIRMED는 사용자 확인(`myharness confirm`)으로만 생긴다.
- 확정된 계약을 실패한 동작으로 덮어쓰지 않는다.
- 외부 계약(IF/EVT/DATA, 설정 키, 배포 단위)을 바꿨으면 Architecture Map의 외부 계약 뷰를 같은 턴에 갱신한다. 실행 검증하지 못한 계약은 verification UNVERIFIABLE과 이유를 남긴다.

### 2. sync 게이트

```sh
myharness sync
```

- sync는 PASSED 검증 뒤에만 동작한다. maps, graph, HTML을 한 revision으로 갱신하고 ChangeReceipt를 쓴다.
- 실패하면 산출물은 DRAFT로 남는다. 실패 원인을 고치고 다시 sync한다. 같은 명령을 다시 돌려도 같은 상태로 수렴한다.

### 3. 정합성 확인

```sh
myharness check --json
myharness status --json
```

- check는 스키마, provenance 규칙, 참조 무결성, md/html/graph revision 일치를 본다.
- status에서 maps, graph, HTML이 모두 CURRENT인지 확인한다.
- HTML만 STALE이면 `myharness render`로 다시 만들고 check를 다시 돌린다.
- md 원본과 HTML의 revision이 다르면 STALE이다. HTML을 손으로 고치지 않는다. 원본 md를 고치고 render한다.

### 4. 문서만 갱신하는 턴

1. 고칠 대상이 원본 SSOT(`.myharness/features/`, `.myharness/architecture/`, `.myharness/decisions/`의 md)인지 생성물(`generated/`)인지 구분한다. 생성물은 직접 고치지 않는다.
2. 원본 md를 고친다.
3. `myharness render`, `myharness check`를 돌린다.
4. 고친 문장이 코드·계약의 증거와 맞는지 확인한다. 증거가 없으면 INFERRED로 둔다.

## 완료 금지 조건

다음 중 하나라도 해당하면 턴을 완료라고 보고하지 않는다.

- maps, graph, HTML 중 하나라도 STALE 또는 DRAFT.
- check가 불일치를 보고함.
- sync가 실패함.
- 필수 검증이 FAILED이거나 PASSED 증거 없이 NOT_RUN.

이때 상태는 BLOCKED이고, 보고에는 무엇이 STALE인지, 원인, 재시도 방법을 적는다. 최신성을 거짓으로 주장하지 않는다.

## 출력: ConsistencyReport

```
ConsistencyReport (CHG-024)
- result_revision: a1b2c3d
- sync_state: feature_map CURRENT, architecture_map CURRENT, graph CURRENT, html CURRENT
- check: 불일치 0
- 갱신한 ID: FEAT-001, MOD-002, IF-002
- 새 의미 필드: 2건, origin INFERRED, confirmation UNCONFIRMED
- 외부 계약 뷰: 설정 키 heartbeat.retry.max 추가, verification PASSED
```

- 상태 전이: SYNCHRONIZING에서 COMPLETED 직전까지 간다. COMPLETED 보고는 reflect-and-correct의 ChangeReceipt 검토 뒤에 한다.

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
