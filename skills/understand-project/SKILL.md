---
name: understand-project
description: Builds an evidence-linked picture of the current project before a change. Answers how the relevant code works (from source, LSP, graph) and why it is shaped that way (from decisions, git history, user-stated intent), keeping observed facts separate from inferred purpose. Use after session-bootstrap for any request that touches existing code, and for read-only "how does X work" or "why is X like this" questions.
---

# understand-project

턴 루프의 2단계(Understand current)를 맡는다. "어떻게 동작하나(how)"와 "왜 이렇게 생겼나(why)"를 분리해서 답한다. 산출물은 근거 링크가 붙은 현재 상태 요약이다.

## 언제 쓰나

- 기존 코드를 건드리는 모든 요청에서 session-bootstrap 다음.
- 코드 변경 없는 설명 요청. 이때 turn은 열지 않는다.

## 입력

- WorkflowPlan의 관련 ID(FEAT-*, MOD-*, IF-*, TEST-*)와 요청 원문.
- ProjectStateSnapshot의 최신성. STALE이면 그래프·맵보다 소스를 우선한다.

## 원칙

- 기존 맵을 무조건 믿지 않는다. 맵의 주장은 최신 코드와 비교해 확인한다.
- 그래프 전체를 컨텍스트에 넣지 않는다. 필요한 관계만 bounded query로 펼친다.
- 코드에서 관찰한 동작은 의도의 증거가 아니다. 코드가 무엇을 하는지는 확정할 수 있어도, 왜 그렇게 하도록 의도했는지는 확정할 수 없다.

## 조회 수단 고르기 (Graph-on-demand)

| 상황 | 쓰는 것 |
|---|---|
| 국소·저위험 변경 | LSP와 해당 소스, 관련 단위 테스트. 그래프 조회는 생략해도 된다 |
| 교차 모듈 API 변경 | Architecture Map, 호출자, 계약 영향 |
| 외부 계약 변경 | 외부 계약 뷰, 소비자, 호환성 정책 |
| 그래프가 STALE이거나 UNKNOWN | LSP, 원본 소스, 테스트. 불확실성을 보고에 표시한다 |
| 반복되는 변경 영역 | 저장된 그래프, 결정 이력, 과거 증거 |

그래프 대신 소스로 폴백했으면 그 이유를 한 줄로 남긴다(예: "graph STALE, 소스 직접 확인").

## 절차

### 1. How: 무엇을 하나

```sh
myharness inspect FEAT-001 --json      # 기능 / 모듈 / 계약 / 테스트의 refs와 evidence
myharness graph neighbors MOD-002 --depth 1
myharness graph trace --from FEAT-001 --to IF-002
myharness contracts list --module MOD-002
```

- `inspect` 결과의 파일 경로와 라인을 직접 열어 읽는다. 결과만 믿고 넘어가지 않는다.
- graph 답에는 `freshness`가 붙는다. STALE이면 위 표대로 소스로 확인한다. 정적 그래프에 경로가 없다는 답(`found: false`)은 관계가 없다는 뜻이 아니다.
- turn이 열려 있으면 inspect, graph, contracts, impact 호출은 하네스가 `graph_queries`로 센다. 턴 예산(`project.yaml`의 `budgets`)을 넘기지 않게 필요한 ID만 조회한다.
- 진입점, 데이터 흐름, 소유 모듈, 외부 계약(IF/EVT/DATA, 설정 키, 배포 단위)을 정리한다.
- 정적 분석으로 알 수 없는 동적 디스패치, 런타임에 조립되는 토픽·라우트는 "없음"이 아니라 UNKNOWN으로 적는다.

### 2. Why: 왜 이런 모양인가

다음 순서로 근거를 찾는다.

1. 해당 FEAT-*.intent와 관련 ADR-*(`myharness decisions FEAT-001`처럼 ID나 낱말로 검색). origin이 USER_STATED 또는 ADOPTED면 의도의 근거로 쓴다. 답한 ADR(`answered: true`)은 사용자가 이미 정한 것이다.
2. confirmation이 CONFIRMED인 불변조건.
3. Git 이력. `git log --follow -- PATH`, `git blame -L a,b PATH`로 변경 커밋과 PR 설명을 읽는다.
4. 코드의 방어 로직(재시도, 타임아웃, 중복 제거)은 과거 장애의 흔적일 수 있다. 근거를 찾으면 링크하고, 못 찾으면 추정이라고 적는다.

origin이 INFERRED ∧ UNCONFIRMED인 purpose는 "추정"으로만 인용한다. 이번 변경 결과가 그 purpose에 의존하면 change-intent에서 지연 확인 대상으로 넘긴다.

### 3. 맵과 코드의 차이

맵이 말하는 것과 코드가 하는 것이 다르면 차이 목록을 만든다. 이 차이는 이 단계에서 고치지 않는다. 변경 범위에 들어오면 sync-knowledge에서 반영하고, 범위 밖이면 보고에만 남긴다.

## 출력: 현재 상태 요약

각 주장 옆에 근거와 축 상태를 붙인다.

```
- MOD-002는 heartbeat-events 토픽을 소비한다. [service/heartbeat/consumer.go:42, origin EXTRACTED]
- 중복 수신은 저장하지 않는다. [FEAT-001 invariant, USER_STATED, verification PASSED (TEST-019)]
- 재시도 횟수 3의 이유는 찾지 못했다. [추정, 근거 없음]
- 그래프 STALE, 호출자 목록은 소스 검색으로 확인함.
```

- 근거 없는 문장은 쓰지 않거나 "추정"이라고 적는다.
- 읽기 전용 질문이었다면 이 요약이 최종 답이다. turn을 열지 않았으므로 동기화는 필요 없다.

다음 단계는 change-intent다. 단순 버그처럼 의도 변화가 없으면 implement-and-prove로 바로 간다.

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
