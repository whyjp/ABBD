---
name: implement-and-prove
description: Implements a planned change in small units that each end in a check, then proves it with real execution at the right verification tier (T0 to T3) through the verify command. For bugs, records a failing-first verify run whose failure output shows the bug before fixing; reads failures by their got/want output rather than the assertion line; keeps what ran and what could not run as separate lists; and never lets a failed check reach sync. Use after the ChangePlan is ready, or directly after understand-project for a simple local bug fix.
---

# implement-and-prove

턴 루프의 5단계(Implement)와 6단계(Prove)를 맡는다. 산출물은 diff, 단위별 증거, EvidenceReport다.

## 언제 쓰나

- ChangePlan과 검증 계약이 정해진 뒤.
- 단순 함수 버그는 understand-project 다음 바로 여기로 온다. 순서는 실패 먼저, 수정, 관련 테스트다.

## 입력

- ChangePlan(타입·시그니처, 예상 카드, 검증 계약)과 impact-review의 verify 범위.
- WorkflowPlan의 검증 수준과 턴 예산(`.abbd/project.yaml`의 `budgets`).

## 검증 수준

| 수준 | 대상 | 최소 증거 |
|---|---|---|
| T0 | 문서·표현만 변경 | 링크, 스키마, 생성물 일치(`abbd check`) |
| T1 | 국소 코드 수정 | 고친 경로를 지나는 테스트 + 타입/컴파일 확인 |
| T2 | 교차 모듈 기능 | 관련 통합 테스트 + 경계/호환성 검사 |
| T3 | 고위험 상태·동시성·데이터 | 장애 주입, 확대된 회귀 범위, 사용자 수준 실행 증거 |

T1의 PASSED는 테스트가 지나간 분기가 기대대로 동작한다는 증거다. 같은 버그가 다른 입력이나 다른 진입점에서 남아 있지 않다는 증거는 아니다. 버그 보고가 HTTP 응답이나 CLI 출력처럼 사용자가 보는 곳에서 나왔으면, 같은 곳에서 증상이 사라졌는지도 확인한다.

수준을 낮추면 이유를 EvidenceReport의 "수준 조정"에 적는다.

## 절차

### 1. 버그면 실패부터 기록

수정 전에 버그를 드러내는 테스트를 쓰고 verify로 돌린다.

```sh
abbd verify --cmd "go test ./internal/heartbeat -run TestDuplicateIsNotStoredTwice"
```

- 종료 코드 2와 `overall: FAILED`가 나와야 한다. 이 실행의 evidence ID가 재현의 증거다.
- 실패가 버그 때문인지 출력으로 확인한다(3절). 컴파일 오류, 다른 테스트의 panic, 빠진 fixture, `no test files`는 재현이 아니다. 테스트를 고쳐 다시 돌린다.
- 재현 테스트가 FAILED를 내지 못하면 코드를 고치지 않는다. 재현 실패를 보고하고, 입력과 중간 값을 찍는 테스트를 더 만들어 다시 시도한다.
- 수정 뒤 같은 명령을 다시 돌려 PASSED를 받는다. EvidenceReport에는 두 evidence ID를 순서대로 적는다.

### 2. 작은 단위로 나누기

ChangePlan을 각각 검사 하나로 끝나는 단위로 나눈다. 앞 단위의 검사가 통과해야 다음 단위로 간다.

- 타입과 시그니처를 먼저 넣고 빌드가 되는지 본다.
- 동작 하나를 넣고 그 동작의 테스트를 돌린다.
- 외부 계약 변경은 마지막 단위로 두고 호환성 검사와 함께 확인한다.
- 단위가 끝날 때마다 ChangePlan의 예상 카드 밖 변경이 생기지 않았는지 본다(architect-change "구현 중 계획에서 벗어날 때").

복잡한 변경은 격리된 git worktree나 서브에이전트에서 진행해도 된다. 서브에이전트가 쓴 diff는 직접 검토한다.

### 3. 실패 출력 읽기

verify 결과에는 실패한 테스트마다 `failures[]`가 있고, 각 항목은 `test`, 그 테스트의 `output`, `evidence`를 담는다.

- 보고와 판단에는 `output`에서 실제 값과 기대 값이 함께 찍힌 줄을 인용한다. 테스트 소스의 단언문을 인용하지 않는다. 단언문은 무엇을 기대했는지만 말하고, 무엇이 나왔는지는 출력만 말한다.
- 실패 종류를 먼저 나눈다. 값 불일치(got/want), 예상 못 한 오류나 panic, 시간 초과, 빈 결과. 값 불일치가 아니면 테스트가 확인하려던 동작까지 가지 못한 것이다.
- 실패 출력에 got/want가 없어 비교할 수 없으면, 실패 메시지에 실제 값을 찍도록 테스트를 고친 뒤 다시 돌린다.
- `output`이 비어 있으면 `.abbd/verification/runs/`의 해당 evidence에서 `output_tail`을 본다.

### 4. 테스트 작성 기준

- 테스트는 계약(IF/EVT/DATA, 설정 키)이나 공개 함수의 결과를 단언한다. 비공개 함수의 호출 횟수나 내부 구조체 필드를 단언하면 `--type refactor` 턴에서 동작이 그대로인데도 FAILED가 나온다.
- 실패를 통과로 만들려고 기대 값을 고치지 않는다. 확정된 Feature 계약을 실패한 동작으로 덮어쓰지 않는다.
- 불변조건을 지키는 테스트는 `abbd intent invariant --feature FEAT-x --claim C --test TEST-x`로 잇는다.

### 5. verify로 증거 남기기

```sh
abbd verification-plan                   # 엔진이 고른 명령·테스트·UNKNOWN을 먼저 본다
abbd verify --affected                   # 기본값. build 명령 + impact가 고른 테스트
abbd verify --all                        # build + 전체 test 명령
abbd verify --cmd "go test ./service/heartbeat/..." --cmd "go vet ./..."
```

- verify는 명령, revision, fingerprint, 테스트별 결과를 Evidence(`.abbd/verification/runs/`)로 저장하고 turn을 갱신한다. T2와 T3 턴은 `--affected`여도 전체 test 명령으로 넓어진다.
- 종료 코드는 PASSED면 0, FAILED면 2다. 2여도 결과가 출력되므로 `overall`, `failed_tests`, `failures`를 읽는다.
- `go test`가 `no test files`로 끝난 패키지는 PASSED로 기록되고 `reason`에 그 사실이 남는다. 그 PASSED는 동작의 증거가 아니다.
- verify 뒤에 작업 트리를 바꾸면 그 PASSED는 sync에서 무효다. 결과를 저장소 안 파일로 리다이렉트해도 작업 트리가 바뀐다. 마지막 수정 뒤에 다시 verify한다.
- 검증 계약의 각 항목을 덮은 evidence ID를 EvidenceReport에 적는다.
- "컴파일된다"는 T1 이상의 증거가 아니다.

### 6. "실행함"과 "실행 못함"을 나눈다

```sh
abbd verify --unverifiable "운영 heartbeat-events 토픽 소비" --reason "이 환경에 브로커 없음"
```

- 결과의 `could_not_run`에 들어가고 Evidence로 남는다. `test_runs`는 늘지 않는다. `--reason` 없이는 기록되지 않는다.
- UNVERIFIABLE 기록은 앞선 PASSED나 FAILED를 덮지 않는다. UNVERIFIABLE만 있으면 turn의 verification은 UNVERIFIABLE이고 sync는 통과하지 않는다. 실행 가능한 필수 검증은 따로 PASSED가 있어야 한다.
- 실행 못 한 검증을 실행한 것처럼 요약하지 않는다.

### 7. 계속 실패하면

- FAILED 상태에서는 sync-knowledge로 가지 않는다.
- 같은 테스트가 두 번 연속 FAILED이고 두 수정이 같은 가정(예: "이 함수는 한 번만 불린다")에 기댔다면, 세 번째 수정 전에 그 가정만 확인하는 작은 테스트를 `verify --cmd`로 돌린다.
- 그래도 막히면 BLOCKED로 보고한다. 작업 중 만든 설계 delta는 DRAFT로 남고, 확정 의도처럼 보이게 하지 않는다.

### 8. 예산과 계측

모델 호출, 그래프 조회, 테스트 실행 각각의 상한은 `project.yaml`의 `budgets`를 따른다. 테스트 실행이 상한을 넘으면 verify 결과의 `warnings`에 나온다. 그래프 조회와 모델 호출은 기록만 된다.

엔진이 그래프 조회(inspect, contracts, impact, graph), 테스트 실행(verify), 질문 수(decide), 소요 시간(sync)을 센다. 모델 호출 수만 호스트가 보고한다. 부를 때마다 더해진다.

```sh
abbd turn note --model-calls 7
```

## 게이트와 출력

```
EvidenceReport (CHG-024, 검증 수준 T2)
- 재현: go test ./internal/heartbeat -run TestDuplicate FAILED (run-2026-10-18-001)
  출력: heartbeat_test.go:41: stored 2 rows, want 1
- 수정 후: 같은 명령 PASSED (run-2026-10-18-003)
- 실행함: go test ./service/heartbeat/... PASSED (run-2026-10-18-004)
- 실행 못함: 운영 heartbeat-events 토픽 소비 확인. 실서비스 의존, UNVERIFIABLE
- 검증 계약 대응: dynamic retry → run-...-004, 중복 저장 금지 유지 → run-...-003
- 수준 조정: 없음
```

상태 전이는 IMPLEMENTING에서 VERIFYING이다. 필수 검증이 모두 PASSED일 때만 sync-knowledge로 넘어간다.

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
