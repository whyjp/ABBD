---
name: implement-and-prove
description: Implements a planned change in small units that each end in a check, then proves it with real execution at the right verification tier (T0 to T3) through the verify command. Records what ran and what could not run as separate lists, reproduces bugs before fixing, and never lets a failed check reach sync. Use after the ChangePlan is ready, or directly after understand-project for a simple local bug fix.
---

# implement-and-prove

턴 루프의 5단계(Implement)와 6단계(Prove)를 맡는다. 산출물은 diff, 단위별 증거, EvidenceReport다.

## 언제 쓰나

- ChangePlan과 검증 계약이 정해진 뒤.
- 단순 함수 버그는 understand-project 다음 바로 여기로 온다. 이때 순서는 재현, 수정, 관련 테스트다.

## 입력

- ChangePlan(타입·시그니처, 검증 계약)과 impact-review의 verify 범위.
- WorkflowPlan의 검증 수준과 턴 예산(`.myharness/project.yaml`).

## 검증 수준

| 수준 | 대상 | 최소 증거 |
|---|---|---|
| T0 | 문서·표현만 변경 | 링크, 스키마, 생성물 일치(`myharness check`) |
| T1 | 국소 코드 수정 | 단위 테스트 + 타입/컴파일 확인 |
| T2 | 교차 모듈 기능 | 관련 통합 테스트 + 경계/호환성 검사 |
| T3 | 고위험 상태·동시성·데이터 | 장애 주입, 확대된 회귀 범위, 사용자 수준 실행 증거 |

프로젝트 설정과 사용자 선호로 기준을 조정할 수 있다. 수준을 낮출 때는 이유를 EvidenceReport에 적는다.

## 절차

### 1. 버그면 먼저 재현

수정 전에 실패를 재현하는 테스트나 명령을 만든다. 재현하지 못하면 원인을 추측해서 고치지 않는다. 재현 실패를 보고하고, 증상에서 시스템이 처음 잘못되는 지점을 계속 추적한다.

### 2. 작은 단위로 나누기

ChangePlan을 각각 검사 하나로 끝나는 단위로 나눈다. 앞 단위의 검사가 통과해야 다음 단위로 간다.

- 타입과 시그니처를 먼저 넣고 컴파일이 되는지 본다.
- 동작 하나를 넣고 그 동작의 테스트를 돌린다.
- 외부 계약 변경은 마지막 단위로 두고 호환성 검사와 함께 확인한다.

복잡한 변경은 격리된 git worktree나 서브에이전트에서 진행해도 된다. 서브에이전트가 쓴 diff는 직접 검토한다.

### 3. 테스트 작성 기준

- 사용자가 관찰할 수 있는 동작을 단언한다. 동작이 깨지면 실패하고 내부만 바뀌면 통과해야 한다.
- 실패 동작을 정상으로 만들기 위해 기대값을 고치지 않는다. 확정된 Feature 계약을 실패한 동작으로 덮어쓰지 않는다.

### 4. verify로 증거 남기기

```sh
myharness verify --affected                   # impact 기준 영향 범위
myharness verify --all                        # 전체 회귀가 필요할 때(T3 등)
myharness verify --cmd "go test ./service/heartbeat/..."
```

- verify는 명령, revision, 통과/실패를 Evidence로 저장하고 turn을 갱신한다.
- 검증 계약의 각 항목이 어느 Evidence로 덮였는지 연결한다.
- 증거 없이 PASSED라고 쓰지 않는다. "컴파일된다"는 T1 이상의 증거가 아니다.

### 5. "실행함"과 "실행 못함"을 나눈다

EvidenceReport에는 두 목록을 따로 둔다.

- 실행함: 명령, 결과(PASSED / FAILED), evidence ID.
- 실행 못함: 항목, 이유(환경 없음, 실서비스 의존, 권한 없음 등). 이 항목의 verification은 UNVERIFIABLE이고, 이유가 없으면 NOT_RUN으로 남는다.

실행 못한 검증을 실행한 것처럼 요약하지 않는다.

### 6. 실패하면

- verify가 FAILED면 수정하고 다시 돌린다. 이 상태에서는 sync-knowledge로 가지 않는다.
- 같은 전제를 공유하는 수정이 두 번 이상 같은 검사에서 실패하면, 그 전제를 적고 전제 자체를 직접 확인한다.
- 계속 실패하면 상태를 BLOCKED로 보고한다. 작업 중 만든 설계 delta는 DRAFT로 남고, 확정 의도처럼 보이게 하지 않는다.

### 7. 예산과 계측

T0/T1 작업은 경량 루프로 돈다. 모델 호출, 그래프 조회, 테스트 실행 각각의 상한은 `project.yaml`을 따른다. 호스트가 모델 호출 수를 알려 주면 기록한다.

```sh
myharness turn note --model-calls 7
```

## 게이트와 출력

EvidenceReport:

```
EvidenceReport (CHG-024, 검증 수준 T2)
- 실행함: go test ./service/heartbeat/... PASSED (run-2026-10-18-001)
- 실행함: TEST-019 PASSED (run-2026-10-18-002)
- 실행 못함: 운영 heartbeat-events 토픽 소비 확인. 실서비스 의존, UNVERIFIABLE
- 검증 계약 대응: dynamic retry → run-...-001, 중복 저장 금지 유지 → run-...-002
- 수준 조정: 없음
```

- 상태 전이: IMPLEMENTING에서 VERIFYING으로 간다. 필수 검증이 모두 PASSED일 때만 sync-knowledge로 넘어간다.
- 구현 후에는 impact-review를 실제 diff 기준으로 한 번 더 돌린다(교차 모듈 변경일 때).

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
