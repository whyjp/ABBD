---
name: session-bootstrap
description: Opens every ABBD turn. Checks whether project knowledge exists, runs Zero-Map Bootstrap without asking questions when it does not, reconciles external code changes, reports map/graph/HTML freshness, then routes the request to a work type and verification tier. Use at the start of every session and every user request in a repository that has a .abbd/ directory or should get one, before reading code to change it.
---

# session-bootstrap

턴 루프의 0단계(Session bootstrap)와 1단계(Route)를 맡는다. 모든 사용자 요청은 이 skill로 시작한다. 산출물은 ProjectStateSnapshot과 WorkflowPlan이다.

## 언제 쓰나

- 세션의 첫 요청, 그리고 이후 모든 요청의 시작.
- 다른 skill을 실행하기 전. 이 skill을 건너뛰면 STALE 맵을 최신으로 믿고 작업하게 된다.

## 입력

- 사용자 요청 원문.
- 프로젝트 루트. 다른 디렉터리면 모든 명령에 `--root DIR`를 붙인다.

## 절차

### 0. 컨텍스트는 지금의 Blueprint에서

`.abbd/BLUEPRINT.md`를 먼저 읽는다. standing rule, 모듈, 계약, 기능, 불변조건, 열린 결정이 지금 유효한 값으로 한 장에 있고, publish마다 다시 만들어진다. 그다음 `abbd run`이 주는 `read_first`의 md 원본만 읽는다. turn을 열지 않는 질문이면 `abbd plan`을 쓴다. `plan`도 `read_first`와 `standing_rules`를 준다. `matches`에 나온 ID는 `abbd inspect ID`로 본다.

- `.abbd/changes`, `requirements`, `decisions`, `verification`, `generated`와 state·candidates·learning·bootstrap-report 파일은 요청마다 쌓인 이력이다. 지난 의도가 지금 의도에 섞이므로 훑어보지 않는다. `.abbd/.ignore`가 ripgrep 검색에서 뺀다. Claude Code 플러그인의 PreToolUse 훅(history-guard.sh)은 이력 경로를 여는 흔한 읽기를 거부한다(Read·Grep·Glob, `cat`, `cd .abbd`, `.abbd` 아래 와일드카드, `find`, 이력 경로의 `git show`·`git log`). 모든 우회를 막는 장치는 아니므로 이 규칙은 훅이 없어도 지킨다.
- 이력이 필요하면 묻는다. 항목 하나는 `abbd inspect ID`, 결정은 `abbd decisions QUERY`, 지난 턴은 `abbd turn show`나 `abbd review CHG-x`.
- 사용자가 프로젝트 전체에 대한 규칙을 말하면 그 말 그대로 기록한다. `run`은 사용자의 메시지를 문장마다 본다. 범위를 넓히는 말("항상", "모든", "앞으로", "always", "every")과 의무를 나타내는 말("해야", "하지 않는다", "금지", "must", "never")이 함께 있는 문장, 또는 "어디서든", "everywhere", "from now on"처럼 범위가 강한 말이 있는 문장을 standing rule로 저장한다.
- 이렇게 저장된 규칙의 문장은 사용자의 말이지만, 그것을 규칙으로 읽은 것은 엔진의 추정이다. 그래서 UNCONFIRMED이고, BLUEPRINT.md에는 "user's words, detected as a rule, unconfirmed"로 표시된다. `run` 결과의 `standing_rules`를 확인한다. 사용자가 규칙으로 말했는데 빠졌으면 직접 기록하고, 규칙이 아닌 문장이 잡혔으면 사용자 보고에 적는다.
- `intent rule`은 turn 없이도 동작한다. 질문만 하는 요청에서 나온 규칙도 기록할 수 있다. 거의 같은 말의 규칙이 이미 있으면 새로 만들지 않고 기존 규칙을 돌려준다.

```sh
abbd intent rule --claim "모든 API는 인증을 거친다"   # 사용자의 문장, USER_STATED, 프로젝트 범위
abbd intent rule --req REQ-3 --retire                  # 사용자가 규칙을 거둘 때
```

### 1. 현재 상태 읽기

```sh
abbd status --json
```

결과에서 revision, fingerprint, maps/graph/HTML 각각의 최신성(CURRENT / STALE / DRAFT / UNKNOWN), 진행 중인 turn, 대기 중인 외부 변경을 읽는다. 이것이 ProjectStateSnapshot이다.

지난 턴의 구조 변경 카드가 결정을 기다리는지는 `abbd review --json`의 `open`으로 본다. 0이 아니면 사용자 보고에 대기 수와 리뷰 화면 링크(sync-knowledge 4절)만 적는다. 카드를 대신 결정하지 않는다.

### 2. 지식이 없으면 bootstrap

`.abbd/`가 없으면 질문 없이 바로 초기화한다. 맵이 없는 것은 오류가 아니라 첫 턴의 정상 상태다.

```sh
abbd init                         # 모드 자동 감지
abbd init --mode code-only        # 코드만 있는 저장소
abbd init --mode greenfield       # 빈 저장소. 첫 요청이 요구사항이 된다
abbd init --mode adopt --from PATH   # 비슷한 기존 문서가 있을 때
```

- 무엇이 생길지 먼저 보려면 `--preview`를 붙인다. BootstrapReport만 출력하고 `.abbd/`를 쓰지 않는다.
- 빌드·테스트를 돌릴 수 있는 환경이면 `--verify-available`을 붙여 OBSERVED 근거를 확보한다.
- `.abbd/`가 이미 있는데 `--mode`와 `--from` 없이 `init`을 부르면 bootstrap 대신 `status` 결과를 돌려준다.
- bootstrap 중에는 사용자에게 아무것도 묻지 않는다. 의도·책임·호환 정책이 불확실한 항목은 INFERRED ∧ UNCONFIRMED로 남고 `generated/confirm-queue.html`에 올라간다.
- adopt 입력이 실패해도 기본 bootstrap 경로는 그대로 진행한다.
- 다시 스캔해야 하면 `abbd bootstrap --mode M --scan`을 쓴다. 앵커 2개 이상이 일치하는 항목은 기존 안정 ID와 확인된 의도를 유지한다.

사용자에게는 BootstrapReport의 요지만 알린다. 생성된 Feature/Module 수, UNCONFIRMED 항목 수, 검증 범위, Explorer 경로(`abbd open-docs`).

### 3. 외부 변경 재조정

turn을 열 요청이면 이 단계를 따로 하지 않는다. `abbd run`이 대기 중인 외부 변경을 먼저 재조정하고, 결과를 WorkflowPlan의 `reconciled`에 담는다. turn을 열지 않는 질문인데 status가 대기 중인 외부 변경(`pending_changes`)을 보고하면 직접 실행한다.

```sh
abbd reconcile
```

- `intent_source: external`인 CHG가 생기고 EXTRACTED 사실(심볼·호출·바인딩)만 갱신된다.
- purpose, responsibility, invariant 같은 의미 필드는 건드리지 않는다. 질문도 하지 않는다.
- 외부 변경이 CONFIRMED 계약이나 확정된 기능과 충돌하면 reconcile이 등급 B ADR을 기본값 `accept`로 남기고 상태를 NEEDS_USER로 둔다. 결과의 `conflicts`가 비어 있지 않으면 사용자 보고에 그 ADR을 적는다.
- 진행 중인 turn이 있으면 reconcile은 거부된다. 그 turn의 변경은 `abbd sync`가 처리한다.
- 에이전트가 Edit·Write로 고친 파일은 외부 변경이 아니다. Claude Code 플러그인의 PostToolUse 훅(note-edit.sh)이 그 경로를 기록하고, reconcile은 그 파일이 있으면 거부한다("edited by the agent without a turn"). 다음 `abbd run`이 그 파일을 turn으로 가져가므로 verify와 sync를 거친다. 셸 명령으로 고친 파일은 기록되지 않는다. 그래서 코드를 고치기 전에 turn을 연다.
- 외부 변경에 승인되지 않은 등급 C 구조 변경이 있으면 reconcile은 그 변경을 Blueprint에 넣지 않는다. CHG를 BLOCKED로 남기고 종료 코드 2로 끝난다. 이 상태에서는 `run`도 거부된다. 리뷰 화면 링크를 사용자에게 주고, 사용자가 결정한 뒤 다시 한다.

### 4. STALE 처리

맵·그래프·HTML 중 하나라도 STALE이면 그 사실을 사용자 보고에 적는다. 영향도 질의는 해도 되지만 결과를 "STALE 기준"으로 표시하고, 판단은 최신 소스로 다시 확인한다.

### 5. Route

작업을 시작하려면 turn을 연다.

```sh
abbd run "REQUEST"
abbd run "REQUEST" --type bug-fix --tier T1 --reason TEXT   # 라우팅을 바로잡을 때만
```

`run`은 `.abbd/`가 없으면 질문 없이 bootstrap부터 하고(WorkflowPlan의 `bootstrap`), 외부 변경을 재조정한 뒤(에이전트가 고친 파일은 재조정하지 않고 이번 turn에 넣는다) 라우팅, 컨텍스트 선택, 의도 해석, 초안 CHG 작성을 하고 WorkflowPlan을 출력한다. WorkflowPlan에는 CHG ID, 작업 타입, 검증 수준, 단계, 컨텍스트 전략과 이유, 턴 예산, `read_first`, `standing_rules`가 있다. 새 기능이면 `new_feature`에 초안 FEAT ID가 있다.

- `read_first`를 순서대로 읽는다(BLUEPRINT.md, 그다음 대상 md 원본). 이력 디렉터리는 열지 않는다(0절).
- `standing_rules`는 새 경로에도 적용된다. 이번 변경이 만드는 경로마다 규칙을 대조하는 일은 change-intent와 architect-change가 맡는다.

- `--type`은 docs, bug-fix, perf, interface-change, refactor, feature 중 하나다. 다른 값은 usage 오류(종료 코드 1)다.
- `--tier`는 T0~T3다. 요청에 재시도·동시성·중복 같은 위험어가 있으면 `run`이 T3로 올린다. 라우팅된 수준보다 낮추려면 `--tier T1 --reason TEXT`를 준다. 이유 없이 낮추면 `run`이 거부하고, 이유는 receipt에 남는다.
- `ambiguous: true`이면 change-intent에서 등급 B 질문을 남긴다.
- `rework`가 있으면 리뷰 화면에서 거부되거나 되돌리기를 요청받은 구조 변경 카드다. 줄마다 카드 ID, kind, 대상, 사용자가 적은 사유가 있고, 엔진이 이미 이번 CHG의 IntentDelta additions에 넣었다. 새 요청보다 먼저 다룬다. change-intent의 "거부된 카드 다루기"로 간다. 사용자 보고 첫머리에 rework 건수를 적는다.
- 진행 중인 turn이 있으면 `run`은 거부된다. 이어서 쓸지 판단하고, 버릴 때만 `abbd turn abort --reason TEXT`를 쓴다. abort는 작업 트리를 되돌리지 않는다. 버린 코드가 남아 있으면 다음 `run`이 그 파일을 새 turn으로 가져가므로, 남기지 않을 편집은 먼저 되돌린다.
- 지난 turn이 거부된 등급 C 카드로 멈춰 있으면 `run`이 그 turn을 닫는다. 그 turn이 바꾼 파일은 외부 변경으로 재조정되지 않고, 거부된 카드와 함께 이번 turn의 rework가 된다.

코드를 바꾸지 않는 질문("X는 어떻게 동작해?", "이렇게 바꾸면 영향이 어디까지야?")이면 turn을 열지 않는다. 아무것도 쓰지 않는 dry run을 쓴다.

```sh
abbd plan "REQUEST"
```

## 작업 타입별 경로

WorkflowPlan의 작업 타입에 맞춰 다음 skill을 고른다. `rework`가 있는 턴은 작업 타입과 상관없이 change-intent를 거친다.

| 작업 타입 | 경로 | 기본 검증 수준 |
|---|---|---|
| 문서만 갱신 | 원본 md 식별 → 수정 → sync-knowledge | T0 |
| 단순 함수 버그 | understand-project(LSP·관련 feature/contract만) → implement-and-prove(재현 → 수정 → 관련 테스트) → sync-knowledge | T1 |
| 리팩터링 | understand-project → impact-review → implement-and-prove → sync-knowledge | T1 또는 T2 |
| 새 기능 | understand-project → change-intent → architect-change → impact-review → implement-and-prove → sync-knowledge → reflect-and-correct | T1 또는 T2 |
| 교차 모듈 API 변경 | 전체 루프. 외부 계약 영향부터 본다 | T2 |
| 성능 개선 | understand-project(기준 환경·불변조건) → architect-change → implement-and-prove(동일 조건 실측 비교) → sync-knowledge | T2 |
| 상태·동시성·데이터 고위험 | 전체 루프 + 확대된 회귀 범위 | T3 |

단순 버그에서는 다중 후보 설계와 다중 모델 검토를 생략한다. 턴당 예산(모델 호출·그래프 조회·테스트 실행 상한)은 `.abbd/project.yaml`을 따른다.

## 게이트와 출력

- ProjectStateSnapshot: revision, 최신성 4종, 외부 변경 처리 결과, UNCONFIRMED 수.
- WorkflowPlan: 작업 타입, 난도/위험도, 검증 수준 T0~T3, 실행할 skill 순서, `rework` 건수, `read_first`, `standing_rules`.
- 상태 전이: DISCOVERING에서 시작한다. bootstrap이나 reconcile이 실패하면 BLOCKED로 보고하고 원인을 적는다.

다음 단계는 understand-project다.

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
