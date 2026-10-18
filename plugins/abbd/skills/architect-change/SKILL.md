---
name: architect-change
description: Designs a change before code is written. Picks the owning module and checks its dependency policy, sketches types, signatures, and contract changes with unimplemented bodies, compares structurally different candidates by the structural change cards each would raise, screens them against ABBD's structure smells, and writes a ChangePlan with expected cards and a verification contract per intent item. Use after change-intent for new features, cross-module or interface changes, and performance work; skip for local bug fixes and doc-only edits.
---

# architect-change

턴 루프의 4단계(Design) 중 설계 쪽을 맡는다. 코드보다 타입과 경계를 먼저 정한다. 영향 범위는 impact-review가 맡고, 두 결과가 합쳐져 ChangePlan이 된다.

## 언제 쓰나

- 새 기능, 교차 모듈 API 변경, 외부 계약 변경, 성능 개선.
- 함수 경계를 넘는 코드를 새로 만들 때.
- 단순 함수 버그, 문서만 갱신, 이름 바꾸기 수준의 리팩터링이면 건너뛴다.

## 입력

- understand-project의 현재 상태 요약. 비어 있으면 먼저 그 skill을 돌린다. 빈 저장소(greenfield)일 때만 생략한다.
- change-intent의 IntentDelta. WorkflowPlan에 `rework`가 있으면 그 항목이 이번 설계의 첫 요구다.

## 절차

### 1. 책임 모듈 정하기

```sh
abbd inspect MOD-002
abbd graph neighbors MOD-002 --depth 1
```

- IntentDelta의 각 항목을 소유할 MOD-*를 정한다. 기존 모듈의 responsibility에 맞으면 그 모듈에 둔다.
- `inspect` 결과의 `dependency_policy`(allowed, forbidden)와 `dependency_violations`를 본다. 새 의존이 금지 목록에 있거나 허용 목록 밖이면 sync가 `forbidden_dependency` 카드(등급 B)를 만든다. 설계를 바꾸거나, 정책 변경 자체를 change-intent의 등급 B 질문으로 넘긴다.
- 기존 모듈의 책임이나 의존 정책을 바꾸면 `intent module --id`로 기록한다. 에이전트 판단이므로 origin INFERRED, confirmation UNCONFIRMED다. CONFIRMED 책임은 엔진이 INFERRED로 덮지 못하게 막는다. 막히면 decide로 묻는다.
- 새 모듈이 필요하면 디렉터리 경로로 책임을 기록한다. MOD ID는 sync가 새 패키지를 발견할 때 붙고, 기록한 책임이 그 모듈로 옮겨진다. sync는 `module_added` 카드(등급 B)를 만든다.

```sh
abbd intent module --id MOD-002 --allow MOD-005 --forbid MOD-003
abbd intent module --path internal/retrypolicy --responsibility "재시도 정책을 소유한다"
```

### 2. 타입과 경계 스케치

구현 전에 다음만 쓴다. 본문은 "not implemented"로 둔다.

- 호출하는 쪽 코드 한두 줄. 타입과 시그니처는 이 사용 예에서 끌어낸다.
- 데이터 타입. 상태마다 다른 타입이나 열거형을 써서 같이 있으면 안 되는 필드 조합을 만들 수 없게 한다.
- 공개 함수·메서드 시그니처와 오류를 돌려주는 방식.
- 계약(IF/EVT/DATA, 설정 키, 배포 단위)의 변경점. 각 변경에 2절의 카드 kind를 붙인다(impact-review 2절 표).
- 입력 검증 위치. CLI, 설정, 네트워크, 외부 API에서 받은 값은 그 진입점에서 파싱하고, 안쪽 함수는 파싱된 타입만 받는다.

### 3. 후보 비교 (비단순 작업만)

교차 모듈 변경이나 선례 없는 설계는 구조가 다른 후보를 둘 이상 만든다. 같은 모듈 배치 안에서 함수 이름이나 인자 순서만 다른 것은 후보로 치지 않는다. 후보끼리는 다음으로 비교한다.

| 기준 | 확인 방법 |
|---|---|
| 리뷰 부담 | 후보가 만들 구조 변경 카드의 수와 등급. B·C 카드가 적은 쪽이 사람의 결정을 덜 요구한다 |
| 의존 정책 | 금지 의존이나 허용 목록 밖 의존이 생기는가 (`inspect`의 `dependency_violations`) |
| 불변조건 | preserved_invariants를 타입이나 연결된 테스트(`intent invariant --test`)로 지킬 수 있는가, 산문으로만 남는가 |
| 계약 | external_consumers가 true이거나 미상인 계약을 breaking하게 바꾸는가. 그렇다면 C 카드 후보다 |
| 공개 범위 | 모듈 밖으로 드러나는 타입·함수 수. 적을수록 다음 변경의 카드가 줄어든다 |

후보마다 [`references/structure-smells.md`](references/structure-smells.md)의 목록을 훑고 걸린 항목을 적는다.

고른 후보와 버린 후보의 이유를 ChangePlan에 남긴다. 사용자 가치 판단이 필요한 선택이면 change-intent의 decide로 등급 B 질문을 만든다(턴당 3개 한도에 포함). 결정이 구조 변경 카드로 나올 선택(새 모듈, 계약 호환성)이면 채팅에서 따로 묻지 않고 기본안을 구현해 카드로 결정을 받는다. `--answer`는 사용자의 답만 기록한다. 엔진이 그 ADR을 USER_STATED·CONFIRMED로 저장하므로 에이전트가 고른 후보를 `--answer`로 남기지 않는다.

### 4. 검증 계약

IntentDelta의 각 항목마다 무엇이 그것을 증명하는지 정한다.

| 항목 | 증명 방법 | 검증 수준 |
|---|---|---|
| dynamic retry policy via config | 설정 변경 후 재시도 횟수가 바뀌는 통합 테스트 | T2 |
| FEAT-001 중복 저장 금지 유지 | TEST-019 재실행 | T2 |

- 증명은 계약이나 공개 함수의 결과를 단언한다.
- 실행할 수 없는 증명(실서비스 토픽 등)은 UNVERIFIABLE 후보로 표시하고 이유를 적는다. implement-and-prove가 `abbd verify --unverifiable "CHECK" --reason TEXT`로 기록한다.
- 엔진이 영향 범위에서 고른 명령은 `abbd verification-plan`으로 본다. 검증 계약이 그보다 좁으면 이유를 적는다.

## 출력: ChangePlan

[`references/changeplan-template.md`](references/changeplan-template.md)의 형식으로 쓴다. 짧은 예:

```
ChangePlan (CHG-024)
- 책임 모듈: MOD-002, 새 모듈 internal/retrypolicy (intent module --path)
- 사용 예: p, err := retrypolicy.Load(cfg); consumer.Run(ctx, p)
- 타입/시그니처: RetryPolicy, Load(Config) (RetryPolicy, error)
- 예상 카드: module_added internal/retrypolicy (B), dependency_added MOD-002->MOD-new (A)
- 계약 변경: 설정 키 heartbeat.retry.max 추가 (contract_added, B)
- 고른 후보와 이유 / 버린 후보와 이유: ...
- 검증 계약: 위 표
- 영향: impact-review 결과
```

## 구현 중 계획에서 벗어날 때

ChangePlan의 예상 카드 목록이 구현의 경계다. 구현 중 새 의존, 새 계약, 시그니처 밖 파라미터가 필요해지면 다음 단위로 가기 전에 확인한다.

1. `abbd contracts impact`와 `abbd impact`를 다시 돌려 예상 카드 밖의 변경이 생겼는지 본다.
2. 생겼으면 원인을 하나로 정한다. 계획이 요구를 놓쳤는지, 구현이 IntentDelta 밖으로 나갔는지.
3. 계획이 놓친 것이면 ChangePlan과 `abbd intent delta --add`를 먼저 고친 뒤 진행한다. 구현이 넘친 것이면 구현을 줄인다.
4. 같은 턴에서 계획 밖 카드가 두 번째로 생기면 설계가 틀린 것이다. 이 skill의 1절부터 다시 한다.

sync 뒤 receipt의 `structural_deltas`에 예상하지 않은 카드가 있으면, 그 사실을 reflect-and-correct 보고에 적는다.

다음 단계는 impact-review다.

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
