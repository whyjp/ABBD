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
| T3 | 고위험 상태·동시성·데이터 | 장애 주입, 확대된 회귀 범위, 사용자 수준 실행 증거(6절) |

T1의 PASSED는 테스트가 지나간 분기가 기대대로 동작한다는 증거다. 같은 버그가 다른 입력이나 다른 진입점에서 남아 있지 않다는 증거는 아니다. 버그 보고가 HTTP 응답이나 CLI 출력처럼 사용자가 보는 곳에서 나왔으면, 같은 곳에서 증상이 사라졌는지도 확인한다.

라우팅된 수준을 낮추는 길은 `abbd run "REQUEST" --tier T1 --reason TEXT` 하나다. 이유 없는 하향은 `run`이 거부하고, 이유는 receipt에 남는다.

## 절차

### 1. 버그면 실패부터 기록

수정 전에 버그를 드러내는 테스트를 쓰고 verify로 돌린다. 버그 수정 턴(`--type bug-fix`)은 수정 전 작업 트리에서 FAILED가 난 verify 없이는 sync가 거부한다. 고친 뒤의 PASSED만으로는 그 테스트가 버그를 잡는지 알 수 없기 때문이다.

엔진이 재현으로 치는 것은 둘이다. 최종 PASSED와 다른 작업 트리(fingerprint)에서 FAILED가 난 테스트가 최종 작업 트리에서 같은 이름으로 PASSED인 경우, 또는 앞선 요청에서 등록한 driver(`verify --drive`)가 그렇게 FAILED 뒤 PASSED인 경우다. 재현 뒤 지운 실패 테스트는 재현이 아니다. 실패한 테스트 없이 명령만 실패한 실행(`verify --cmd false` 같은)은 재현이 아니다.

```sh
abbd verify --cmd "go test ./internal/heartbeat -run TestDuplicateIsNotStoredTwice"
```

- 종료 코드 2와 `overall: FAILED`가 나와야 한다. 이 실행의 evidence ID가 재현의 증거다.
- 재현이 정말 불가능하면(환경 의존, 이미 사라진 데이터) `abbd verify --no-repro --reason TEXT`로 이유를 남긴다. 열린 turn이 있어야 하고, 사유는 10자 이상이어야 한다.
- 엔진은 실패한 테스트가 있는지까지만 본다. 그 실패가 버그 때문인지는 보지 않는다. 출력으로 직접 확인한다(3절). 컴파일 오류, 다른 테스트의 panic, 빠진 fixture, `no test files`는 재현이 아니다. 테스트를 고쳐 다시 돌린다.
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
- `output`이 비어 있으면 그 테스트만 `abbd verify --cmd`로 다시 돌린다. `.abbd/verification/`은 이력이라 직접 열지 않는다.

### 4. 테스트 작성 기준

- 테스트는 계약(IF/EVT/DATA, 설정 키)이나 공개 함수의 결과를 단언한다. 비공개 함수의 호출 횟수나 내부 구조체 필드를 단언하면 `--type refactor` 턴에서 동작이 그대로인데도 FAILED가 나온다.
- 실패를 통과로 만들려고 기대 값을 고치지 않는다. 확정된 Feature 계약을 실패한 동작으로 덮어쓰지 않는다.
- 불변조건을 지키는 테스트는 `abbd intent invariant --feature FEAT-x --claim C --test TEST-x`로 잇는다.
- `standing_rules`의 규칙이 새 경로에 걸리면 그 새 경로에서 규칙을 확인하는 테스트를 쓴다(architect-change 검증 계약). 기존 경로의 테스트로 대신하지 않는다.

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

### 6. 사용자 수준 실행 (T3)

T3 턴은 사용자 수준 실행(driver의 PASSED) 없이는 sync가 거부한다. 단위 테스트는 부품을 증명할 뿐 사용자가 보는 동작을 증명하지 않기 때문이다.

```sh
abbd driver set FEAT-005 --run "go run ./cmd/worker --once" --expect "stored 1"
abbd driver list
abbd verify --drive
abbd verify --unverifiable user-level --reason "이 환경에 브로커 없음"   # 돌릴 수 없을 때만
```

- driver는 진입점(FEAT 또는 IF)마다 `project.yaml`에 남고 다음 턴에도 쓰인다. 명령이 성공하고 출력에 `--expect` 문자열이 있어야 PASSED다.
- `--drive`는 이번 턴이 겨냥하는 ID(WorkflowPlan의 대상, 새 기능이면 `new_feature`)의 driver만 돌린다. "no driver covers this turn"이 나오면 그 ID에 driver를 등록한다.
- `--run`은 어떤 명령이든 받는다. 엔진은 그 명령이 진입점을 실제로 지나는지 확인하지 않는다. 그래서 driver 명령은 사용자가 닿는 방식(CLI 실행, HTTP 요청, 워커 1회 실행)으로 그 진입점을 돌려야 한다. `echo`처럼 기대 문자열만 찍는 명령은 증거가 아니다.
- 같은 turn에서 등록하거나 바꾼 driver의 PASSED는 그 turn의 T3 게이트를 통과시키지 못한다. 이번 turn을 증명할 기준을 이번 turn에 맞춰 만들 수 있기 때문이다. receipt `risks`에 그 사실이 남는다. 중단(abort)한 turn에서 등록한 driver, turn을 열기 전이라도 같은 사용자 메시지에 등록한 driver도 같다. driver는 앞선 turn에서 등록하고 그 turn을 sync로 완료해 둔다. push·deploy·publish처럼 바깥에 효과가 있는 명령(등급 C)은 driver로 등록되지 않는다.
- 이번 turn에 돌릴 driver가 없으면 `--unverifiable user-level --reason TEXT`로 남긴다. CHECK는 글자 그대로 `user-level`이어야 이 게이트에 쓰인다. sync는 완료하지만 receipt 상태는 NEEDS_USER이고 `risks`에 사유가 남는다.

### 7. "실행함"과 "실행 못함"을 나눈다

```sh
abbd verify --unverifiable "운영 heartbeat-events 토픽 소비" --reason "이 환경에 브로커 없음"
```

- 결과의 `could_not_run`에 들어가고 Evidence로 남는다. `test_runs`는 늘지 않는다. `--reason` 없이는 기록되지 않는다.
- UNVERIFIABLE 기록은 앞선 PASSED나 FAILED를 덮지 않는다. UNVERIFIABLE만 있으면 turn의 verification은 UNVERIFIABLE이고 sync는 통과하지 않는다. 실행 가능한 필수 검증은 따로 PASSED가 있어야 한다.
- 실행 못 한 검증을 실행한 것처럼 요약하지 않는다.

### 8. 계속 실패하면

- FAILED 상태에서는 sync-knowledge로 가지 않는다.
- 같은 테스트가 두 번 연속 FAILED이고 두 수정이 같은 가정(예: "이 함수는 한 번만 불린다")에 기댔다면, 세 번째 수정 전에 그 가정만 확인하는 작은 테스트를 `verify --cmd`로 돌린다.
- 그래도 막히면 BLOCKED로 보고한다. 작업 중 만든 설계 delta는 DRAFT로 남고, 확정 의도처럼 보이게 하지 않는다.

### 9. 예산과 계측

모델 호출, 그래프 조회, 테스트 실행 각각의 상한은 `project.yaml`의 `budgets`를 따른다. 테스트 실행이 상한을 넘으면 verify 결과의 `warnings`에, 모델 호출과 그래프 조회가 넘으면 sync가 receipt의 `risks`에 경고를 남긴다. 막지는 않는다.

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
- 사용자 수준: driver FEAT-005 PASSED (run-2026-10-18-005)
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
- 결정 주체: `decide --answer`, `decide --adr`, `review --decide`에서 `--user`는 사용자가 직접 한 답이라는 표시다. 없으면 에이전트의 INFERRED 선택으로 남거나 거부되고, 등급 C 정지를 풀지 못한다. 에이전트 자신의 선택에는 `--user`를 붙이지 않는다. 플러그인의 MCP 서버는 에이전트가 부르는 통로라 `--user`가 없다. MCP로는 B·C 카드와 기존 ADR을 결정하지 못하고, MCP 질문에 붙인 답은 에이전트의 선택으로 남는다. 사용자의 통로는 `abbd serve`의 리뷰 화면, 결정 파일(`review --apply`), 사용자의 말을 그대로 `--user`와 함께 적은 CLI 세 가지다.
- 컨텍스트: 지금 유효한 설계는 `.abbd/BLUEPRINT.md`(standing rule, 모듈, 계약, 기능, 불변조건, 열린 결정)와 `abbd run`의 `read_first`에 있다. `.abbd/changes`, `requirements`, `decisions`, `verification`, `generated`와 state·candidates·learning·bootstrap-report 파일은 이력이므로 컨텍스트로 읽지 않고 `abbd inspect`, `abbd decisions`, `abbd turn show`, `abbd review CHG-x`로 묻는다. standing rule은 새 경로에도 적용된다.
- 종료 코드: 0 성공, 1 오류, 2 게이트 거부(sync, check 불일치, 검증 FAILED, 승인 안 된 등급 C decide). 2일 때도 결과가 출력되므로 결과 필드를 읽고 판단한다.
