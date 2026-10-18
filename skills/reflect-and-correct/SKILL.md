---
name: reflect-and-correct
description: Ends a For My Harness turn. Reviews the ChangeReceipt against the code and doc diff, reports cost, side effects, pending tier C items, and residual risk, then turns repeated mistakes into enforced constraints in the order architecture, types, lint, tests, docs, proving each new check fails on a real past mistake. Use after sync-knowledge on every turn for the review, and for the learning part when the user corrects the same mistake again or a turn repeats a known failure.
---

# reflect-and-correct

턴 루프의 8단계(Review / ship)와 9단계(Learn)를 맡는다. 산출물은 검토된 ChangeReceipt와, 필요할 때 규칙/Skill 변경 제안이다.

## 언제 쓰나

- 검토(8단계): sync-knowledge가 끝난 모든 턴. 작은 턴은 receipt 확인과 보고만 한다.
- 교정(9단계): 사용자가 같은 문제를 다시 지적했을 때, 같은 종류의 실패가 두 번째로 나왔을 때. 한 번뿐인 실수는 교훈으로 치지 않는다.

## 입력

- ChangeReceipt, ConsistencyReport, EvidenceReport.
- 이번 세션의 사용자 교정, 최근 커밋·revert, 리뷰 코멘트.

## 1. ChangeReceipt 검토

```sh
myharness turn show --json
```

receipt가 다음 항목을 모두 담고 있는지, 각 항목이 실제 diff와 맞는지 확인한다.

| 항목 | 확인할 것 |
|---|---|
| 목적 | 무엇을 바꾸려 했는가. IntentDelta와 일치하는가 |
| 변경 | 코드/테스트/SSOT의 핵심 diff. 빠진 파일이 없는가 |
| 영향 | 관련 Feature/Module/Interface. impact-review 결과와 일치하는가 |
| 증거 | 실행한 검증과 결과, 실행 못한 항목과 이유 |
| 동기화 | map / graph / html revision 상태. 모두 CURRENT인가 |
| 결정 | 등급 A 자동 적용 / 등급 B 기본값 진행(UNCONFIRMED ADR) / 등급 C 대기 |
| 의도 충실도 | Requested Delta 항목 수 / 테스트 PASSED로 덮인 수 / 사용자 CONFIRMED 수 |
| 계측 | `telemetry`의 model_calls(호스트 보고, `turn note`), graph_queries, test_runs, questions_asked, duration_sec(하네스가 셈) |
| 후속 | 잔여 리스크, 권장 다음 작업, 새로 쌓인 UNCONFIRMED 수 |

코드/문서 diff는 직접 읽는다. 서브에이전트가 쓴 부분도 마찬가지다. 같은 맥락에서 쓴 사람이 스스로 승인하지 않는다. 독립 리뷰가 필요한 변경인데 리뷰어를 돌릴 수 없으면 그 사실을 보고한다.

## 2. 반영과 보고

- 등급 A 행동(로컬 검증, 맵 동기화, 설정에 따라 로컬 commit)은 진행하고 보고한다. 로컬 commit의 등급은 `myharness policy classify --action local-commit`으로 확인한다. 등급 A이면 `myharness sync --commit`이 sync와 함께 commit을 만든다.
- 등급 C 행동(push, 배포 등)은 실행하지 않는다. 준비된 변경과 승인 요청을 보고한다.
- 사용자 보고는 receipt의 요지로 쓴다. 목적, 바뀐 것, 검증 결과(실행함/실행 못함), 동기화 상태, 기본값으로 진행한 결정과 뒤집는 방법, 등급 C 대기 항목, 새 UNCONFIRMED 수.
- 등급 C 대기 항목은 receipt의 `blocked`와 `myharness decisions`의 답 없는 등급 C ADR로 확인한다.
- 턴을 버려야 하면 `myharness turn abort --reason TEXT`를 쓴다. 그 turn이 만든 초안 FEAT는 지워지고, CHG는 BLOCKED 기록으로 남는다. 작업 트리의 코드는 되돌리지 않으므로, 버린 코드가 남아 있으면 다음 `run`이 외부 변경으로 재조정한다.

여기까지 끝나야 턴이 COMPLETED다.

## 3. 반복 실수 찾기 (교정이 필요할 때만)

- 사용자 교정, revert, 리뷰 코멘트, 우회 로직 주석을 읽고 실수를 종류별로 묶는다.
- 같은 종류가 두 번 나와야 교정 대상이다.
- 각 종류에 실제 사례(커밋, 파일, 대화의 지적)를 붙인다.

## 4. 가장 높은 단계에서 막기

다음 순서로, 가능한 가장 앞 단계에서 해결한다. 뒤 단계를 고르면 앞 단계가 왜 안 되는지 적는다.

1. 아키텍처: 상태마다 소유자 하나, 작업마다 지원 경로 하나. 잘못된 import가 실패하도록 내부를 숨긴다. 손으로 맞추는 목록을 단일 원본으로 바꾼다. 에이전트가 베낄 만한 옛 방식과 죽은 코드를 지운다.
2. 타입: 잘못된 상태를 쓸 수 없게 만든다.
3. 린트/CI 검사: 오류 메시지가 대신 쓸 파일·타입·함수를 알려 준다. 이미 흔한 패턴이면 새로 늘어날 때만 실패시킨다.
4. 테스트: 동작이 깨지면 실패하는 테스트. 동작이 깨져도 통과하는 테스트는 고치거나 지운다.
5. 문서/에이전트 규칙: 판단이 필요한 경우에만, 마지막에. 문서는 어겨도 아무것도 실패하지 않는다.

"한 번 더 주의하라"는 문장만 추가하는 것은 교정이 아니다.

고른 단계를 제안으로 기록한다. `--kind`는 위 순서의 architecture, type, lint, test, doc, skill 중 하나다. 진행 중인 turn이 있으면 그 CHG의 next_steps에도 붙는다. 기록은 `.myharness/learning.yaml`에 `status: proposed`로 쌓인다.

```sh
myharness learn --kind lint --proposal "재시도 횟수 상수 하드코딩을 금지하고 RetryPolicy를 쓰게 한다"
```

## 5. 증명

- 새 검사마다 실제 과거 실수에 대해 실패하는지 돌려서 확인한다. 실패하지 않으면 그 검사는 막지 못한다.
- 로컬과 CI에서 같은 명령을 쓴다.
- 예외는 해당 줄에 이유, 만료일, 사람의 승인을 함께 적는다.
- 교정 변경도 일반 턴과 같다. implement-and-prove로 검증하고 sync-knowledge로 동기화한다.

## 6. Skill과 정책 변경

- 반복되는 사용자 선호는 사용자가 동의하면 `myharness learn --preference key=value`로 `project.yaml`의 `preferences`에 쓴다. 한 번 답한 질문은 ADR로 남아 있고 decide가 `reused`로 돌려주므로 다시 묻지 않는다.
- 스킬 변경 제안은 `myharness learn --kind skill --proposal TEXT`로 남긴다.
- 이 skills/ 디렉터리나 전역 Skill을 바꾸는 제안은 다른 프로젝트에도 영향을 준다. 로컬에서 평가한 뒤 사용자 승인을 받고 적용한다. 자동 적용하지 않는다.
- 규칙마다 무엇이 그것을 강제하는지 짝지은 표를 프로젝트의 에이전트 지침 파일에 유지한다. 규칙이 이미 있는데 강제 수단이 없어서 반복됐다면, 같은 변경에서 더 높은 단계로 올린다. 실수가 더 이상 불가능해지면 규칙을 지운다.

## 출력

- 검토된 ChangeReceipt 요지와 사용자 보고.
- 교정을 했다면: 실수 종류, 증거, 고른 단계와 더 높은 단계가 안 된 이유, 새 검사가 과거 실수에 실패한 실행 결과.
- Skill/정책 변경 제안 목록(적용 전, 승인 대기).

## 공통 어휘 (PRD 06장·10장, 모든 skill에서 동일)

- origin: USER_STATED | ADOPTED | EXTRACTED | OBSERVED | INFERRED. 모든 사실·간선에 필수다. "왜" 필드(purpose, responsibility, rationale)에는 USER_STATED, ADOPTED, INFERRED만 쓴다. EXTRACTED와 OBSERVED는 "무엇을 하는가"에만 쓴다.
- verification: NOT_RUN | PASSED | FAILED | UNVERIFIABLE. 동작·계약·불변조건에 적용한다. PASSED에는 evidence 링크가 있어야 하고, UNVERIFIABLE에는 이유를 적는다.
- confirmation: UNCONFIRMED | CONFIRMED | REJECTED. 의미 필드에 적용한다. INFERRED는 사용자 CONFIRMED로만 승격된다. 테스트 PASSED는 verification만 바꾸고 origin은 바꾸지 않는다. purpose는 어떤 증거로도 자동 승격되지 않는다.
- 최신성은 별도 축이다: CURRENT | STALE | DRAFT | UNKNOWN.
- 승인 등급 A: 자동 실행하고 ChangeReceipt에 사후 보고한다. 읽기·탐색, 가역적 코드/테스트 수정, 맵 동기화, 그래프·HTML 재생성, bootstrap 후보 생성, 로컬 검증.
- 승인 등급 B: 선택지 2~3개, 추천 기본값, 각 선택이 바꾸는 범위를 제시한다. 같은 턴 안에 답이 없으면 기본값으로 진행하고 UNCONFIRMED ADR을 남긴다(상태 NEEDS_USER). 턴당 질문은 3개까지다.
- 승인 등급 C: 명시적 승인 없이는 실행하지 않는다. 공유 브랜치 push/force-push, 배포, 외부 리소스 삭제, 비밀정보·권한 변경, external_consumers: true인 인터페이스의 breaking change, 실서비스 데이터 변경. 가역적 준비까지만 하고 BLOCKED로 보고한다.
