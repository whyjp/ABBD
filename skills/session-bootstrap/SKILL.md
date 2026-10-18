---
name: session-bootstrap
description: Opens every For My Harness turn. Checks whether project knowledge exists, runs Zero-Map Bootstrap without asking questions when it does not, reconciles external code changes, reports map/graph/HTML freshness, then routes the request to a work type and verification tier. Use at the start of every session and every user request in a repository that has a .cccv/ directory or should get one, before reading code to change it.
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

### 1. 현재 상태 읽기

```sh
cccv status --json
```

결과에서 revision, fingerprint, maps/graph/HTML 각각의 최신성(CURRENT / STALE / DRAFT / UNKNOWN), 진행 중인 turn, 대기 중인 외부 변경을 읽는다. 이것이 ProjectStateSnapshot이다.

### 2. 지식이 없으면 bootstrap

`.cccv/`가 없으면 질문 없이 바로 초기화한다. 맵이 없는 것은 오류가 아니라 첫 턴의 정상 상태다.

```sh
cccv init                         # 모드 자동 감지
cccv init --mode code-only        # 코드만 있는 저장소
cccv init --mode greenfield       # 빈 저장소. 첫 요청이 요구사항이 된다
cccv init --mode adopt --from PATH   # 비슷한 기존 문서가 있을 때
```

- 무엇이 생길지 먼저 보려면 `--preview`를 붙인다. BootstrapReport만 출력하고 `.cccv/`를 쓰지 않는다.
- 빌드·테스트를 돌릴 수 있는 환경이면 `--verify-available`을 붙여 OBSERVED 근거를 확보한다.
- `.cccv/`가 이미 있는데 `--mode`와 `--from` 없이 `init`을 부르면 bootstrap 대신 `status` 결과를 돌려준다.
- bootstrap 중에는 사용자에게 아무것도 묻지 않는다. 의도·책임·호환 정책이 불확실한 항목은 INFERRED ∧ UNCONFIRMED로 남고 `generated/confirm-queue.html`에 올라간다.
- adopt 입력이 실패해도 기본 bootstrap 경로는 그대로 진행한다.
- 다시 스캔해야 하면 `cccv bootstrap --mode M --scan`을 쓴다. 앵커 2개 이상이 일치하는 항목은 기존 안정 ID와 확인된 의도를 유지한다.

사용자에게는 BootstrapReport의 요지만 알린다. 생성된 Feature/Module 수, UNCONFIRMED 항목 수, 검증 범위, Explorer 경로(`cccv open-docs`).

### 3. 외부 변경 재조정

turn을 열 요청이면 이 단계를 따로 하지 않는다. `cccv run`이 대기 중인 외부 변경을 먼저 재조정하고, 결과를 WorkflowPlan의 `reconciled`에 담는다. turn을 열지 않는 질문인데 status가 대기 중인 외부 변경(`pending_changes`)을 보고하면 직접 실행한다.

```sh
cccv reconcile
```

- `intent_source: external`인 CHG가 생기고 EXTRACTED 사실(심볼·호출·바인딩)만 갱신된다.
- purpose, responsibility, invariant 같은 의미 필드는 건드리지 않는다. 질문도 하지 않는다.
- 외부 변경이 CONFIRMED 계약이나 확정된 기능과 충돌하면 reconcile이 등급 B ADR을 기본값 `accept`로 남기고 상태를 NEEDS_USER로 둔다. 결과의 `conflicts`가 비어 있지 않으면 사용자 보고에 그 ADR을 적는다.
- 진행 중인 turn이 있으면 reconcile은 거부된다. 그 turn의 변경은 `cccv sync`가 처리한다.

### 4. STALE 처리

맵·그래프·HTML 중 하나라도 STALE이면 그 사실을 사용자 보고에 적는다. 영향도 질의는 해도 되지만 결과를 "STALE 기준"으로 표시하고, 판단은 최신 소스로 다시 확인한다.

### 5. Route

작업을 시작하려면 turn을 연다.

```sh
cccv run "REQUEST"
cccv run "REQUEST" --type bug-fix --tier T1   # 라우팅을 바로잡을 때만
```

`run`은 `.cccv/`가 없으면 질문 없이 bootstrap부터 하고(WorkflowPlan의 `bootstrap`), 외부 변경을 재조정한 뒤 라우팅, 컨텍스트 선택, 의도 해석, 초안 CHG 작성을 하고 WorkflowPlan을 출력한다. WorkflowPlan에는 CHG ID, 작업 타입, 검증 수준, 단계, 컨텍스트 전략과 이유, 턴 예산이 있다. 새 기능이면 `new_feature`에 초안 FEAT ID가 있다.

- `--type`은 docs, bug-fix, perf, interface-change, refactor, feature 중 하나다. CLI는 다른 값도 받지만 단계 목록이 바뀌지 않으므로 이 여섯 개만 쓴다.
- `--tier`는 T0~T3다. 요청에 재시도·동시성·중복 같은 위험어가 있으면 `run`이 T3로 올린다. 낮출 때는 이유를 EvidenceReport에 적는다.
- `ambiguous: true`이면 change-intent에서 등급 B 질문을 남긴다.
- 진행 중인 turn이 있으면 `run`은 거부된다. 이어서 쓸지 판단하고, 버릴 때만 `cccv turn abort --reason TEXT`를 쓴다.

코드를 바꾸지 않는 질문("X는 어떻게 동작해?", "이렇게 바꾸면 영향이 어디까지야?")이면 turn을 열지 않는다. 아무것도 쓰지 않는 dry run을 쓴다.

```sh
cccv plan "REQUEST"
```

## 작업 타입별 경로

WorkflowPlan의 작업 타입에 맞춰 다음 skill을 고른다. 작은 작업일수록 가볍게 돈다. 단계의 행위는 지키되 모든 단계에 별도 에이전트나 도구 호출을 둘 필요는 없다.

| 작업 타입 | 경로 | 기본 검증 수준 |
|---|---|---|
| 문서만 갱신 | 원본 md 식별 → 수정 → sync-knowledge | T0 |
| 단순 함수 버그 | understand-project(LSP·관련 feature/contract만) → implement-and-prove(재현 → 수정 → 관련 테스트) → sync-knowledge | T1 |
| 리팩터링 | understand-project → impact-review → implement-and-prove → sync-knowledge | T1 또는 T2 |
| 새 기능 | understand-project → change-intent → architect-change → impact-review → implement-and-prove → sync-knowledge → reflect-and-correct | T1 또는 T2 |
| 교차 모듈 API 변경 | 전체 루프. 외부 계약 영향부터 본다 | T2 |
| 성능 개선 | understand-project(기준 환경·불변조건) → architect-change → implement-and-prove(동일 조건 실측 비교) → sync-knowledge | T2 |
| 상태·동시성·데이터 고위험 | 전체 루프 + 확대된 회귀 범위 | T3 |

단순 버그에서는 다중 후보 설계와 다중 모델 검토를 생략한다. 턴당 예산(모델 호출·그래프 조회·테스트 실행 상한)은 `.cccv/project.yaml`을 따른다.

## 게이트와 출력

- ProjectStateSnapshot: revision, 최신성 4종, 외부 변경 처리 결과, UNCONFIRMED 수.
- WorkflowPlan: 작업 타입, 난도/위험도, 검증 수준 T0~T3, 실행할 skill 순서.
- 상태 전이: DISCOVERING에서 시작한다. bootstrap이나 reconcile이 실패하면 BLOCKED로 보고하고 원인을 적는다.

다음 단계는 understand-project다.

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
