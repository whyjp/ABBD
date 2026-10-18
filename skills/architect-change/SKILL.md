---
name: architect-change
description: Designs a change before code is written. Picks the owning module, checks its dependency policy, sketches types, signatures, and interface boundaries with unimplemented bodies, compares structurally different candidates for non-trivial work, and writes a ChangePlan with a verification contract per intent item. Use after change-intent for new features, cross-module or interface changes, and performance work; skip for local bug fixes and doc-only edits.
---

# architect-change

턴 루프의 4단계(Design) 중 설계 쪽을 맡는다. 코드보다 타입과 경계를 먼저 정한다. 영향 범위 분석은 impact-review가 맡고, 두 결과가 합쳐져 ChangePlan이 된다.

## 언제 쓰나

- 새 기능, 교차 모듈 API 변경, 외부 계약 변경, 성능 개선.
- 함수 경계를 넘는 코드를 새로 만들 때.
- 단순 함수 버그, 문서만 갱신, 이름 바꾸기 수준의 리팩터링이면 건너뛴다.

## 입력

- understand-project의 현재 상태 요약. 비어 있으면 먼저 그 skill을 돌린다. 빈 저장소(greenfield)일 때만 생략할 수 있다.
- change-intent의 IntentDelta.

## 절차

### 1. 책임 모듈 정하기

- IntentDelta의 각 항목을 소유할 MOD-*를 정한다. 기존 모듈의 responsibility에 맞으면 그 모듈에 둔다.
- 그 모듈의 `dependency_policy`(allowed, forbidden)를 확인한다. 새 의존이 forbidden에 걸리면 설계를 바꾸거나, 정책 변경 자체를 등급 B 질문으로 넘긴다.
- 새 모듈이 필요하면 responsibility 초안을 쓴다. 이것은 에이전트 판단이므로 origin INFERRED, confirmation UNCONFIRMED다.

```sh
myharness inspect MOD-002
myharness graph neighbors MOD-002 --depth 1
```

### 2. 타입과 경계 스케치

구현 전에 다음만 쓴다. 본문은 "not implemented"로 둔다.

- 데이터 타입. 잘못된 상태가 표현되지 않게 만든다(상태별 타입, 열거형, 생성자 검증).
- 공개 함수·메서드 시그니처. 오류를 어떻게 돌려주는지 포함한다.
- 모듈 간 인터페이스(IF-*)와 외부 계약(EVT-*, DATA-*, 설정 키, 배포 단위)의 변경점.
- 외부 입력 검증은 경계(CLI, 설정, 네트워크, 외부 API)에 모으고 내부 로직은 순수 함수로 둔다.

### 3. 후보 비교 (비단순 작업만)

교차 모듈 변경이나 선례 없는 설계는 구조가 다른 후보를 최소 둘 만든다. 같은 구조 안의 세부 차이는 후보로 치지 않는다.

비교 기준:

- 공개 인터페이스가 더 작고 더 많은 복잡도를 숨기는가.
- 한 파일만 보고 맞아 보이는 변경이 저장소 전체에서도 맞는가. 다음 기여자는 연 파일만 보고 가장 가까운 예를 베끼는 에이전트라고 가정한다.
- 기존 불변조건(preserved_invariants)을 타입이나 구조로 지킬 수 있는가.
- 외부 계약의 호환성을 유지하는가. 유지하지 못하면 impact-review의 등급 판정으로 넘긴다.

선택 이유를 두세 문장으로 남긴다. 사용자 가치 판단이 필요한 선택이면 change-intent의 decide로 등급 B 질문을 만든다(턴당 3개 한도에 포함).

### 4. 검증 계약

IntentDelta의 각 항목마다 무엇이 그것을 증명하는지 정한다.

| 항목 | 증명 방법 | 검증 수준 |
|---|---|---|
| dynamic retry policy via config | 설정 변경 후 재시도 횟수가 바뀌는 통합 테스트 | T2 |
| FEAT-001 중복 저장 금지 유지 | TEST-019 재실행 | T2 |

- 증명은 사용자가 관찰할 수 있는 동작을 단언한다. 내부 구현을 단언하는 테스트는 리팩터링마다 깨진다.
- 실행할 수 없는 증명(실서비스 토픽 등)은 미리 UNVERIFIABLE 후보로 표시하고 이유를 적는다.

## 출력: ChangePlan

```
ChangePlan (CHG-024)
- 책임 모듈: MOD-002 (dependency_policy 위반 없음)
- 타입/시그니처: RetryPolicy, PolicySource.Load() (RetryPolicy, error)
- 인터페이스 변경: IF-002 동작 변경 없음. 설정 키 heartbeat.retry.max 추가
- 선택한 후보와 이유: ...
- 검증 계약: 위 표
- 영향 범위: impact-review 결과를 붙인다
```

## 구현 중 이탈 처리

ChangePlan이 구현의 계약이다. 구현 중 시그니처에 없던 파라미터가 필요해지면 이유를 따진다. 계획이 틀렸는지, 요구를 놓쳤는지, 구현이 범위를 넘는지 판단한다. 계획을 고쳐야 하면 ChangePlan을 먼저 고치고 다음 단위로 간다. 구현에 맞추려고 계획을 사후에 고쳐 쓰지 않는다.

다음 단계는 impact-review다.

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
