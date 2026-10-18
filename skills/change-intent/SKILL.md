---
name: change-intent
description: Turns a user request plus the current project state into an IntentDelta. Separates current intent, the requested delta, and the resulting intent; asks at most three tier B questions through the decide command (never during bootstrap) and lazily confirms inferred items only when this turn first touches them and the result depends on their purpose. Use after understand-project whenever a request adds, changes, or removes behavior, or conflicts with a confirmed contract.
---

# change-intent

턴 루프의 3단계(Resolve intent)를 맡는다. 산출물은 IntentDelta와, 필요할 때만 던지는 등급 B 질문이다.

## 언제 쓰나

- 요청이 기능의 동작·계약·불변조건을 추가·변경·삭제할 때.
- 요청이 CONFIRMED 계약과 충돌하거나, 해석이 둘 이상일 때.
- reconcile에서 외부 변경이 CONFIRMED 계약과 충돌한다고 넘어왔을 때.
- 의도 변화가 없는 단순 버그 수정이나 문서 갱신이면 건너뛴다.

## 입력

- understand-project의 현재 상태 요약.
- `myharness run`이 만든 초안 CHG와 WorkflowPlan.

## 세 가지 의도를 섞지 않는다

| 이름 | 뜻 | 기록 위치 |
|---|---|---|
| Current Intent | 지금 유효한 기능 의미·제약·과거 결정 | FEAT-*.intent, ADR-* |
| Requested Intent Delta | 이번 요청이 의도에 가하는 변화 | CHG-*.intent_delta |
| Resulting Intent | 검증 성공 후 유효한 동작 계약 | FEAT-*.intent (sync 후) |

같은 문장을 세 곳에 중복 저장하지 않는다. CHG와 ADR은 FEAT의 의도를 ID로 참조한다.

## 절차

### 1. IntentDelta 초안

다음 세 목록을 쓴다.

- additions: 새로 생기는 동작·계약.
- changes/removals: 바뀌거나 사라지는 동작·계약. 각각 영향받는 FEAT/IF ID를 붙인다.
- preserved_invariants: 이번 변경이 지켜야 하는 기존 불변조건. CONFIRMED 또는 USER_STATED인 것만 확정 제약으로 쓴다.

### 2. 질문 전에 스스로 확인

질문하기 전에 다음 순서로 조사한다. 이미 답이 있으면 묻지 않는다.

1. 사용자 요청 원문을 다시 읽는다.
2. 관련 FEAT/MOD/IF와 ADR-*을 조회한다(`myharness inspect ID`).
3. 코드와 테스트를 확인한다.
4. Git 이력을 확인한다.
5. 같은 질문에 답한 ADR이 있는지 확인한다. 있으면 그 답을 따른다.
6. 기본값을 정한다.

### 3. 등급 B 질문이 필요한지 판단

다음 중 하나일 때만 묻는다.

- 요청 해석이 둘 이상이고 결과 기능이 실질적으로 달라진다.
- 변경이 CONFIRMED 계약·불변조건과 충돌한다.
- 외부 인터페이스의 호환성 정책을 건드린다.
- 데이터 보존, 비용, 의존성 추가 같은 가치 판단이다.
- 외부 변경이 CONFIRMED 계약과 명백히 충돌한다(reconcile에서 넘어온 경우).

bootstrap 중에는 절대 묻지 않는다. 동작으로 관찰해서 알 수 있는 사실(실행 결과, 성능, 출력)은 질문하지 말고 직접 실행해서 확인한다.

### 4. decide로 묻기

```sh
myharness decide \
  --question "재시도 정책을 런타임에 바꿀 때 진행 중인 메시지에도 적용할까?" \
  --option "next=다음 메시지부터=진행 중 메시지는 기존 정책 유지" \
  --option "all=즉시 전체 적용=진행 중 메시지도 재계산, IF-002 동작 변경" \
  --default next \
  --subject FEAT-001 --tier B
```

- 선택지는 2~3개다. 각 선택지의 effect에 그 선택이 바꾸는 범위를 적는다.
- 질문 본문이나 기본값 effect에 "답하지 않아도 진행되는 범위"를 적는다.
- 사용자가 답하면 `--answer KEY`를 붙여 다시 기록한다. 답이 없으면 기본값으로 진행하고 ADR은 origin INFERRED, confirmation UNCONFIRMED로 남는다. 턴 상태는 NEEDS_USER다.
- 턴당 질문은 최대 3개다. 넘치면 결과를 가장 크게 바꾸는 것부터 묻고, 나머지는 질문 없이 기본값으로 decide를 기록한다.
- 호스트에 구조화된 질문 도구가 있으면 그것으로 사용자에게 보여 주고, 결과는 반드시 decide로 기록한다.

### 5. 지연 확인 (lazy confirmation)

INFERRED ∧ UNCONFIRMED 항목은 다음 두 조건이 모두 맞을 때만 묻는다.

1. 이번 턴이 그 항목을 처음 건드린다.
2. 이번 변경의 결과가 그 항목의 purpose 또는 responsibility에 의존한다.

건드리지 않는 추정은 묻지 않는다. 확인은 decide의 등급 B 질문으로 하고, 사용자가 답하면 해당 항목에 반영한다.

```sh
myharness confirm --list
myharness confirm FEAT-003 --purpose "heartbeat를 검증해 저장한다"      # CONFIRMED
myharness confirm FEAT-003 --reject --value "heartbeat를 집계만 한다"    # REJECTED, 대체 값 보존
myharness confirm --merge CAND-7                                        # 앵커 1개 일치 병합 후보
```

사용자가 명시적으로 요청하지 않는 한, 이번 턴과 무관한 확인 대기 목록을 한꺼번에 처리하지 않는다.

### 6. 의도 기록

```sh
myharness intent feature --name "dynamic retry policy" \
  --purpose "재시도 정책을 설정으로 바꿀 수 있다" --origin USER_STATED \
  --id FEAT-001 --anchor symbol:go:service/heartbeat.Consumer.Run
myharness intent invariant --feature FEAT-001 \
  --claim "중복 수신 시 중복 저장되지 않는다" --origin USER_STATED --test TEST-019
```

- 사용자가 말한 것은 USER_STATED, 에이전트가 해석한 것은 INFERRED다.
- 새 기능은 `--id`를 생략해 새 ID를 받는다. 앵커는 symbol, path, test, entry 중 2종 이상을 준다.

## 게이트와 출력

- IntentDelta: additions, changes/removals, preserved_invariants, 관련 ID, 각 항목의 origin.
- 질문 기록: 묻은 질문과 ADR ID, 답 여부, 기본값으로 진행한 항목.
- 상태 전이: INTENT_RESOLVED. 기본값으로 진행한 등급 B 결정이 있으면 NEEDS_USER를 함께 보고한다.

다음 단계는 architect-change와 impact-review다.

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
