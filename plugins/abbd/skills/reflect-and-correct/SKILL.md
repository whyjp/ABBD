---
name: reflect-and-correct
description: Ends a ABBD turn. Reviews the ChangeReceipt (including its structural change cards) against the code and doc diff, reports cost, side effects, pending tier C items, and residual risk, then turns mistakes that recur across receipts, rejected review cards, failed runs, and user corrections into enforced constraints, choosing the earliest learn kind that can stop them and proving each new check with a recorded failing verify run. Use after sync-knowledge on every turn for the review, and for the learning part when the same kind of mistake shows up a second time.
---

# reflect-and-correct

턴 루프의 8단계(Review / ship)와 9단계(Learn)를 맡는다. 산출물은 검토된 ChangeReceipt와, 필요할 때 `.abbd/learning.yaml`의 교정 제안이다.

## 언제 쓰나

- 검토(8단계): sync-knowledge가 끝난 모든 턴. 작은 턴은 receipt 확인과 보고만 한다.
- 교정(9단계): 같은 종류의 실수가 두 번째로 나왔을 때. 두 번째인지는 아래 2절의 기록으로 판단한다.

## 1. ChangeReceipt 검토

```sh
abbd turn show --json
abbd review CHG-024 --json
```

receipt의 각 항목이 실제 diff와 맞는지 확인한다. 코드와 문서 diff는 직접 읽는다. 서브에이전트가 쓴 부분도 같다.

| 항목 | receipt 필드 | 확인할 것 |
|---|---|---|
| 목적 | `request_summary`, `intent_delta` | IntentDelta와 실제 변경이 같은 일을 하는가 |
| 변경 | `files_changed` | 빠지거나 의도하지 않은 파일이 없는가 |
| 영향 | `impacted` | impact-review의 영향 기능과 같은가 |
| 증거 | `verification`, `verification_evidence` | 실행한 것과 실행 못 한 것(UNVERIFIABLE 이유)이 나뉘어 있는가 |
| 동기화 | `sync_state` | 넷 다 CURRENT인가 |
| 구조 변경 | `structural_deltas` | ChangePlan의 예상 카드와 같은가. 등급별 상태(applied, default_applied, pending, approved, rejected) |
| 결정 | `decisions`, `blocked` | 기본값으로 진행한 B, 대기 중인 C |
| 의도 충실도 | `intent_fidelity` | 요청 항목 수, 테스트로 덮인 수, 사용자 CONFIRMED 수 |
| 계측 | `telemetry` | model_calls(호스트 보고), graph_queries, test_runs, questions_asked, duration_sec. 예산 초과 경고는 `risks`에 있다 |
| 후속 | `risks`, `next_steps`, `new_unconfirmed` | 잔여 위험과 다음 작업 |

구조 변경 카드의 승인·거부는 사람이 리뷰 화면에서 한다. 이 skill은 카드를 검토해 보고할 뿐 결정하지 않는다. 변경이 독립 리뷰를 받아야 하는데 리뷰어를 돌릴 수 없으면, 그 사실을 보고에 적는다.

## 2. 반영과 보고

- 로컬 commit 등급은 `abbd policy classify --action local-commit`으로 본다. 등급 A이면 `abbd sync --commit`이 sync와 함께 commit을 만든다. 거부되면 sync는 완료된 채 결과의 `reason`이 "commit 안 함"으로 시작하고 `commit`이 비어 있다.
- 등급 C 행동(push, 배포 등)은 실행하지 않는다. 준비된 변경과 승인 요청을 보고한다. 대기 항목은 receipt의 `blocked`, `abbd review`의 pending C 카드, `abbd decisions`의 답 없는 등급 C ADR로 확인한다.
- 사용자 보고는 receipt 요지로 쓴다. 목적, 바뀐 것, 검증(실행함 / 실행 못함), 동기화 상태, 기본값으로 진행한 결정과 뒤집는 방법, 등급 C 대기, 새 UNCONFIRMED 수. 구조 변경이 있으면 카드 수와 결정 대기 수, 리뷰 화면 링크만 적는다(sync-knowledge 4절).
- 턴을 버려야 하면 `abbd turn abort --reason TEXT`를 쓴다. 초안 FEAT는 지워지고 CHG는 BLOCKED 기록으로 남는다. 작업 트리의 코드는 되돌리지 않는다. 에이전트가 Edit·Write로 고친 파일이 남아 있으면 다음 `run`이 그 파일을 새 turn으로 가져가 verify와 sync를 거치게 한다. `reconcile`과 turn 없는 `sync`는 그 파일을 거부한다. 남기지 않을 편집은 abort 전에 되돌린다.

여기까지 끝나야 턴이 COMPLETED다.

## 3. 반복 찾기 (교정이 필요할 때만)

ABBD는 실수의 흔적을 이미 기록한다. 대화 기억 대신 이 기록에서 찾고, 엔진의 집계에서 시작한다.

```sh
abbd learn --from-receipts
```

두 턴 이상에서 반복된 문제(실패한 테스트, 거부된 카드 kind, BLOCKED 사유)를 사례 CHG ID와 먼저 시도할 구조적 교정과 함께 돌려준다. 빈 결과면 아래 기록 중 엔진이 세지 않는 것만 본다. `.abbd/changes`와 `.abbd/verification`은 이력이라 직접 열지 않고 명령으로 묻는다.

| 기록 | 묻는 명령 | 보는 것 |
|---|---|---|
| 거부된 카드 | `abbd review CHG-x`의 `deltas[].decision`, 다음 턴의 `rework` | kind와 거부 사유. 같은 kind에 같은 취지의 사유가 두 번이면 후보 |
| 실패한 검증 | `learn --from-receipts`, `abbd inspect CHG-x`의 `verification_evidence` | 같은 테스트나 같은 종류의 실패가 여러 턴에 걸쳐 나옴 |
| 버린 턴 | `learn --from-receipts`의 blocked 항목, `abbd inspect CHG-x`의 `blocked` "aborted: 사유" | 같은 이유로 버림 |
| 계획 밖 카드 | receipt의 `structural_deltas`와 그 턴의 ChangePlan | 같은 kind의 예상 못 한 카드 |
| 재조정 충돌 | `intent_source: external` CHG의 등급 B ADR | 같은 계약이 반복해서 충돌 |
| 사용자 교정 | 이번 세션의 지적, revert 커밋 | 같은 지적 |

같은 종류가 두 번 나와야 교정 대상이다. 각 종류에 실제 사례 두 개(CHG ID, evidence ID, 카드 ID, 커밋)를 붙인다.

## 4. 교정 수단 고르기

`abbd learn --kind`의 값 순서가 선호 순서다. 실수를 막을 수 있는 가장 앞의 kind를 고르고, 앞 kind를 건너뛰었으면 그 이유를 proposal 문장에 적는다.

| kind | 바꾸는 것 | 막는 방식과 ABBD에서의 확인 |
|---|---|---|
| architecture | 모듈 경계, 의존 정책, 계약 정책 | `intent module --forbid`로 금지한 의존은 inspect와 `forbidden_dependency` 카드로 드러난다. `intent contract --external-consumers true`로 표시한 계약의 breaking change는 C 카드가 되어 sync가 멈춘다 |
| type | 프로젝트 코드의 타입 | 잘못된 값이나 조합을 만들면 컴파일이 실패한다. 빌드 명령이 verify에 들어 있으므로 매 턴 확인된다 |
| lint | 프로젝트의 정적 검사나 CI 검사 | 검사 명령을 `.abbd/project.yaml`의 `verification.build_commands`에 넣으면 `abbd verify`가 범위와 상관없이 매 턴 돌린다. 오류 메시지에는 대신 쓸 함수나 파일을 적는다 |
| test | 동작 테스트 | 테스트를 `intent invariant --test TEST-x`로 불변조건에 이으면 그 테스트의 결과가 불변조건의 verification이 된다. 테스트는 그 동작을 소유한 모듈의 패키지에 두어 `verify --affected`가 고르게 한다 |
| doc | 에이전트 지침, 프로젝트 문서 | verify도 sync도 이것을 확인하지 않는다. 검사로 바꿀 수 없는 판단에만 쓴다 |
| skill | 이 skill들 | 다른 프로젝트에도 영향을 준다. 6절 |

"더 주의한다"만 적은 proposal은 교정으로 치지 않는다.

```sh
abbd learn --kind architecture --proposal "MOD-002는 MOD-003(storage 내부)을 직접 import하지 않는다. intent module --id MOD-002 --forbid MOD-003. 사례 CHG-011, CHG-019 거부 카드"
```

진행 중인 turn이 있으면 그 CHG의 next_steps에도 붙는다. 기록은 `.abbd/learning.yaml`에 `status: proposed`로 쌓인다.

## 5. 증명

- 새 검사는 과거 실수에 실패해야 한다. 과거 실수를 작업 트리에 다시 만들고(사례 커밋을 되살리거나 같은 코드를 쓴다) 검사를 verify로 돌린다. 종료 코드 2와 `failures`에 그 실수를 가리키는 출력이 나오면 증명이다. 그 evidence ID를 proposal에 적는다.
- 되살린 실수를 지우고 다시 verify해서 PASSED를 확인한다. 이 두 실행 뒤에야 교정 변경을 sync한다.
- 검사의 예외가 필요하면 코드 주석으로 끝내지 않고 `abbd decide --subject ID`로 등급 B 결정을 남긴다. 예외도 ADR로 검색된다.
- 교정 변경도 일반 턴과 같다. implement-and-prove로 검증하고 sync-knowledge로 동기화한다.

## 6. Skill과 선호

- 반복되는 사용자 선호는 사용자가 동의하면 `abbd learn --preference key=value`로 `project.yaml`의 `preferences`에 쓴다. 한 번 답한 질문은 ADR로 남아 decide가 `reused: true`로 돌려주므로 따로 기록하지 않는다.
- skill 변경 제안은 `abbd learn --kind skill --proposal TEXT`로 남긴다. 이 skill 디렉터리나 전역 skill을 바꾸는 변경은 로컬에서 평가하고 사용자 승인을 받은 뒤 적용한다. 자동 적용하지 않는다.
- 검사, 금지 의존, 타입이 생긴 뒤에는 같은 내용을 적은 doc 규칙을 지운다. 둘 다 두면 한쪽만 고쳐져 어긋난다.

## 출력

- 검토된 ChangeReceipt 요지와 사용자 보고.
- 교정을 했다면 실수 종류, 사례 ID 두 개, 고른 kind와 앞 kind가 안 된 이유, 실패한 검증과 통과한 검증의 evidence ID.
- Skill/정책 변경 제안 목록(적용 전, 승인 대기).

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
