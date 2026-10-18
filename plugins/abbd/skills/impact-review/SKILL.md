---
name: impact-review
description: Finds what a change can break outside its diff. Reads the engine's contract diff and impact report first, predicts the structural change cards (tiers A/B/C) the change will raise on the review screen, works the report's unknowns by contract kind, backs the claim the plan relies on with a recorded verify run, and stops on tier C. Use before implementing any change that crosses a module or contract, and again on the real diff before sync.
---

# impact-review

턴 루프의 4단계(Design) 중 영향 범위를 맡는다. 구현 뒤에는 실제 diff로 한 번 더 돈다. 결과는 ChangePlan의 영향 절, 예상 구조 변경 카드, verify 범위가 된다.

## 언제 쓰나

- 구현 전. 모듈이나 계약을 넘는 변경, 리팩터링, 요청이 기존 코드에 대해 무언가를 단정할 때("X를 public으로", "X는 이미 Y를 처리한다"). 설계를 그 단정 위에 올리기 전에 엔진 결과와 소스로 맞는지 본다.
- 구현 후, sync 전. 실제 diff 기준으로 계약 diff와 구조 변경을 다시 본다.
- 단순 함수 버그는 관련 feature/contract와 직접 호출자만 본다.

## 엔진이 계산하는 것과 에이전트가 할 일

엔진은 호출자, 의존 모듈, 영향 기능, 그 테스트, 계약 diff와 등급을 이미 계산한다. 그 목록을 다시 손으로 만들지 않는다. 에이전트의 몫은 세 가지다.

1. 엔진 결과의 `unknowns`와, 분석기가 간선으로 만들지 못한 연결을 계약 종류별로 확인한다.
2. 이 변경이 리뷰 화면에 올릴 구조 변경 카드를 미리 알고, 등급 C 카드가 생길 변경이면 구현 전에 멈춘다.
3. 계획이 기대는 "이래서 안전하다"는 주장을 실행 결과로 뒷받침하고, 그 실행을 Evidence로 남긴다.

## 절차

### 1. 계약 diff와 영향 보고서

```sh
abbd contracts impact --json
abbd impact --json
abbd contracts list --module MOD-002
```

- `contracts impact`는 작업 트리와 마지막 동기화를 비교해 계약마다 `change`(added, removed, modified), `breaking`, `tier`, `reason`을 주고 `highest_tier`를 붙인다. 구현 전에는 비어 있는 것이 정상이다.
- `impact`는 contracts, modules, dependents, features, tests, unknowns와 가장 높은 등급을 준다.
- 각 계약의 compatibility와 external_consumers를 `abbd inspect IF-x`로 본다. external_consumers가 INFERRED ∧ UNCONFIRMED이거나 "미상"이면 그 값이 등급을 B에서 C로 바꿀 수 있다. 이번 변경이 그 계약을 건드리면 change-intent의 지연 확인 대상이다.

### 2. 예상 구조 변경 카드

sync는 턴 시작 시점의 구조와 결과 구조를 비교해 구조 변경(StructuralDelta)을 계산하고, 항목마다 카드를 만든다. 모듈 안쪽 코드만 바뀐 턴은 카드가 0건이고 리뷰 화면 알림도 없다. ChangePlan에 예상 카드를 적어 두면, 계획에 없던 카드가 receipt에 나왔을 때 계획이 틀렸다는 신호가 된다.

| kind | 생기는 경우 | 등급 |
|---|---|---|
| `dependency_added`, `dependency_removed` | 허용 범위 안에서 모듈 의존 간선이 생기거나 사라짐 | A |
| `feature_path_changed` | 기능이 지나는 모듈·계약 경로가 바뀜 | A |
| `module_added`, `module_removed`, `module_split`, `module_merged` | 모듈 구성이 바뀜 | B |
| `forbidden_dependency` | `dependency_policy`가 금지하거나 허용 목록에 없는 의존 | B |
| `contract_added`, `contract_removed` | 외부 계약이 생기거나 사라짐 | B, 외부 소비자가 있으면 C |
| `contract_compat_changed` | selector·방향·호환성 정책·외부 소비자 값이 바뀜 | B, breaking이고 외부 소비자가 있으면 C |
| `ownership_moved` | 계약이나 기능의 소유 모듈이 바뀜 | B |
| `deploy_mapping_changed` | 모듈과 배포 단위의 매핑이 바뀜 | B |

- A 카드는 적용된 상태로 남고 사람은 되돌리기를 요청할 수 있다.
- B 카드는 기본값으로 적용되고, sync가 카드마다 UNCONFIRMED ADR(기본값 `approve`)을 남긴다. 턴은 NEEDS_USER로 끝난다.
- C 카드는 승인 전까지 Blueprint에 반영되지 않는다. turn 안의 sync는 종료 코드 2로 멈추고 `reason`에 카드 ID를 적는다. turn 밖의 `sync`와 `reconcile`(`run`이 먼저 하는 재조정 포함)도 그 변경을 Blueprint에 넣지 않고 CHG를 BLOCKED로 남긴다(종료 코드 2).
- 카드 결정은 사람이 리뷰 화면에서 한다(sync-knowledge 4절). 에이전트는 카드를 승인하지 않는다.

### 3. 계약 종류별로 unknowns 확인

`impact`의 `unknowns`와 `graph trace`의 `found: false`는 "관계 없음"이 아니다. 분석기는 소스에 글자로 적힌 심볼·호출·바인딩만 간선으로 만든다. 계약 종류마다 그 밖에 있는 것을 확인한다.

| 계약 | 분석기가 보는 것 | 따로 확인할 것 |
|---|---|---|
| IF (HTTP·RPC) | 라우트 등록 문자열, 핸들러 | 응답 본문 필드 이름과 타입, 상태 코드, 저장소 밖 클라이언트 |
| EVT (토픽·큐) | 상수로 적힌 토픽 이름, produce/consume 호출 | 런타임에 조립되는 토픽 이름, 메시지 스키마, 다른 서비스의 소비자 |
| DATA (테이블·스키마) | 마이그레이션 파일, SQL 문자열 | 컬럼을 읽는 다른 언어·배치 작업, ORM 태그, 백필 필요 여부 |
| 설정 키 | 코드에 적힌 키 이름 | 배포 설정의 기본값, 같은 키를 읽는 다른 모듈 |
| 배포 단위 | Dockerfile, 진입점 | 실행 인자, 헬스 체크 경로 |

확인하지 못한 연결은 보고에 UNKNOWN으로 남기고, 그 경로를 덮는 테스트를 verify 범위에 넣는다. 덮을 테스트도 없으면 `abbd verify --unverifiable "CHECK" --reason TEXT` 후보로 둔다.

### 4. 안전 주장을 실행으로 뒷받침

계획이 "이 변경은 X 때문에 안전하다"에 기대면, 그 X를 하나의 문장으로 적고 understand-project의 근거 수준(실행 확인, 코드 확인, 기록 확인, 추정, 모름)으로 표시한다. 주장이 여럿이면, 틀렸을 때 다시 열리는 위험이 가장 많은 주장부터 확인한다.

계획이 기대는 주장은 실행 확인까지 올린다. 실제 코드를 부르는 작은 테스트나 명령을 만들고 verify로 돌린다.

```sh
abbd verify --cmd "go test ./internal/api -run TestPostMessageKeepsResponseShape"
```

- 결과의 evidence ID가 그 주장의 근거다. 보고에는 ID와 출력 한 줄을 붙인다.
- 실행 확인까지 못 갔으면 도달한 수준을 그대로 적고 "미증명"이라고 쓴다.
- 이 테스트는 버리지 않고 verify 범위에 남긴다.

### 5. 승인 등급 판정

변경이나 후속 행동이 외부 효과를 가지면 엔진에 등급을 묻는다. 정지 목록을 외워 판단하지 않는다.

```sh
abbd policy classify --action push
abbd policy classify --action local-commit
```

모르는 action이면 오류에 엔진이 아는 action 목록이 나온다.

verify는 돌릴 명령도 검사한다. 명령 자리에 맞춘 정규식(`git ... push`, `docker push`, `npm publish`, `kubectl apply|delete|rollout|scale|patch`, `terraform apply|destroy`, `helm install|upgrade|uninstall|rollback`, `rm -rf /`, `drop table|database`, 명령 맨 앞의 `deploy` 스크립트)에 걸리면 등급 C로 보고 실행하지 않는다. `./internal/deploy/...` 같은 경로 안의 낱말은 걸리지 않는다. 이 목록 밖의 외부 효과는 잡지 못하므로, 그런 명령은 verify에 넣기 전에 그 행동의 등급을 `policy classify --action`으로 묻는다.

등급 C 행동이 계획 자체에 들어 있으면(배포, 외부 리소스 삭제, 외부 소비자가 있는 계약의 breaking change) 구현 전에 정지를 기록한다.

```sh
abbd decide --tier C --question "IF-003 POST /messages 경로를 /v2/messages로 바꿔도 되나?" --subject IF-003
```

- 종료 코드는 2이고 stderr에 `BLOCKED`가 찍힌다. 결과는 출력되며 `blocked: true`, `ask: true`이고 CHG의 `blocked`에 들어간다.
- 정지 상태를 다시 보려면 같은 decide를 반복하지 않고 `abbd decide --adr ADR-x`(답 없이)나 `abbd decisions IF-003`을 쓴다.
- 가역적 준비(로컬 코드, 마이그레이션 초안, 테스트)까지만 하고 BLOCKED로 보고한다. 보고에는 승인 요청과 준비된 변경을 함께 적는다.
- 승인은 사용자가 한다. sync는 리뷰 화면의 C 카드 승인과, 사용자가 채팅에서 명시적으로 승인한 뒤의 `--answer approve --user` 기록만 인정한다. `--user` 없는 approve는 에이전트의 선택이라 C 정지를 풀지 못한다.
- decide를 잊어도 sync가 C 카드에서 멈추므로 C 변경이 반영되는 일은 없다. 다만 구현 전에 멈추면 버릴 작업이 줄어든다.

### 6. verify 범위

`impact`의 `tests`, 3절의 unknowns를 덮는 테스트, 4절의 증명 테스트, 새 경로에 걸리는 standing rule의 테스트(change-intent 0절)를 합친다. `abbd verification-plan`과 대조하고, 그보다 좁히면 이유를 적는다. 위험 점수가 낮다는 이유로 영향 기능의 회귀 테스트를 빼지 않는다.

## 출력

ChangePlan의 영향 절에 다음을 쓴다.

```
영향 (CHG-024)
- 계약 diff: IF-002 modified, breaking false, 등급 B (contracts impact)
- 예상 카드: contract_compat_changed IF-002 (B), dependency_added MOD-002->MOD-005 (A)
- 영향 기능과 테스트: FEAT-001 → TEST-019, TEST-021
- UNKNOWN: heartbeat-events 토픽의 저장소 밖 소비자. TEST-030으로 메시지 스키마 고정
- 안전 주장: "응답 본문 필드는 그대로다" 실행 확인 (run-2026-10-18-004)
- 등급 C: 없음
- verify 범위: go test ./internal/api/... ./internal/heartbeat/...
```

구현 후 다시 돌았으면 실제 카드와 예상 카드를 나란히 적는다. 다음 단계는 implement-and-prove이고, 구현 후 재확인이었으면 sync-knowledge다.

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
