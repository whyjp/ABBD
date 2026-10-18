---
name: impact-review
description: Finds what a change can break outside its diff. Checks external contracts first (interfaces, topics, schemas, config keys, deploy units), then dependent modules, features, callers, and tests; marks unanalyzable relations UNKNOWN; proves the one fact the change is safe because of by running code; and classifies actions into approval tiers, stopping on tier C. Use before implementing any change that crosses a module or contract, and again on the real diff before sync.
---

# impact-review

턴 루프의 4단계(Design) 중 영향 범위(blast radius) 쪽을 맡는다. 구현 뒤에는 실제 diff로 한 번 더 돈다. 결과는 ChangePlan의 영향 범위 절과 verify에 넘길 테스트 범위가 된다.

## 언제 쓰나

- 구현 전: 모듈·계약을 넘는 변경, 리팩터링, 요청이 기존 코드에 대해 단정하는 경우("X를 public으로", "X는 이미 Y를 처리한다"). 그 단정은 가설이고, 소비자 조사가 설계를 결정한다.
- 구현 후: sync 전에 실제 diff 기준으로 영향 기능·계약을 빠짐없이 다시 확인한다.
- 단순 함수 버그는 관련 feature/contract와 직접 호출자만 보면 된다.

## 원칙

호출자 목록을 뽑는 것이 일이 아니다. 그것은 검색 한 번이면 된다. 일은 검색에 안 잡히는 파손을 찾는 것이다. 그럴듯한 보고서는 사실이든 아니든 그럴듯하게 읽힌다. 그래서 보고서 대신 증명을 남긴다.

## 절차

### 1. 외부 계약부터

호출 그래프보다 먼저 본다.

```sh
myharness contracts impact --json
myharness contracts list --module MOD-002
```

- 변경이 건드리는 IF-*, EVT-*, DATA-*, 설정 키, 배포 단위를 나열한다.
- 각 계약의 compatibility 정책과 external_consumers 값을 확인한다.
- 호환성 정책을 건드리면 등급 B다. change-intent의 decide로 넘긴다.
- external_consumers: true인 계약의 breaking change는 등급 C다. 아래 4단계에서 멈춘다.
- external_consumers 값이 INFERRED ∧ UNCONFIRMED이면 지연 확인 조건에 해당하므로 change-intent에서 묻는다.

### 2. 모듈, 기능, 테스트

```sh
myharness impact --json
myharness graph neighbors IF-002 --depth 2
myharness graph trace --from FEAT-003 --to IF-002
```

`impact`는 contracts, modules, features, tests, unknowns 순으로 돌려준다. 이어서 다음을 확인한다.

- 영향 기능마다 그 기능의 불변조건과 VERIFIED_BY 테스트.
- 호출자. 그래프가 STALE이면 소스 검색과 LSP로 다시 찾는다.
- 검색이 놓치는 경로: 직렬화된 JSON 필드, DB 컬럼, 와이어 포맷, 같은 바이트를 읽는 다른 언어, 기능 플래그, 런타임에 조립되는 토픽 이름, 동적 플러그인.
- 정적으로 알 수 없는 관계는 "관계 없음"이 아니라 UNKNOWN 또는 possible로 적고, 테스트로 보완할 대상에 넣는다.

### 3. 안전의 근거가 되는 한 가지 사실

위험해 보이는 변경 대부분은 한 가지 사실 덕분에 안전하다. 그 사실을 찾고 얼마나 확실한지 적는다.

1. 말로만 주장함. 단독으로는 가치가 없다.
2. 실제 `file:line`을 가리킴.
3. 실패 경로를 단계별로 따라가 도달하지 않음을 보임.
4. 실제 코드를 호출하는 스크립트나 테스트를 돌림. 틀리면 크게 실패한다.
5. 실행 중인 앱에서 재현함.

싸게 갈 수 있는 데까지 내려간다. 보통 4단계는 같은 라이브러리를 import해 걱정되는 함수를 직접 부르는 작은 스크립트 하나다. 증명하지 못했으면 "미증명"이라고 적는다.

### 4. 승인 등급 판정

변경이나 후속 행동이 외부 효과를 가지면 등급을 확인한다.

```sh
myharness policy classify --action push
myharness policy classify --action deploy
myharness policy classify --action delete-external
myharness policy classify --action secret
myharness policy classify --action local-commit
```

등급 C 정지 목록:

- 공유 브랜치 push, force-push
- 배포
- 외부 리소스 삭제
- 비밀정보·권한 변경
- external_consumers: true인 인터페이스의 breaking change
- 실서비스 데이터 변경

등급 C이면 명시적 승인 없이 실행하지 않는다. 가역적 준비(로컬 브랜치의 코드, 마이그레이션 초안, 테스트)까지만 하고 상태를 BLOCKED로 보고한다. 보고에는 승인 요청과 준비된 변경을 함께 적는다.

### 5. 테스트 범위 결정

영향 기능의 VERIFIED_BY 테스트와 UNKNOWN 경로를 덮는 테스트를 합쳐 verify 범위를 정한다. 영향 위험 점수가 낮다는 이유로 영향 기능의 회귀 테스트를 빼지 않는다. 놓친 영향(Impact Recall 저하)이 가장 비싼 실패다.

## 출력

- 무엇이 바뀌나: diff에 드러나지 않는 동작 변화 포함.
- 안전의 근거: 사실 한 가지, 도달한 확실성 단계, 증명(실행 결과). 못 했으면 "미증명".
- 위험: 각 위험의 파손 방식, `file:line`, 가능성과 비용, 확인 방법.
- 확인 후 제외한 것: 확인한 항목과 안전한 이유.
- UNKNOWN 목록.
- 등급 판정: B로 넘긴 항목, C로 멈춘 항목.
- verify 범위: implement-and-prove에 넘길 테스트·명령.

다음 단계는 implement-and-prove다. 구현 후 실행했다면 sync-knowledge로 간다.

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
