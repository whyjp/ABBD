# 인계 보고 (PRD 16장 템플릿) · v1.5 + 밀도 2차 보강

2026-10-18 · 저장소 `github.com/whyjp/ABBD` (main) · PRD `basedoc/abbd_prd_v1_5.html`

## [Goal]

ABBD (Active Blueprint-Based Development). 에이전트는 하네스가 만들고 유지하는 설계도(Blueprint)를 기준으로 개발한다. 설계도는 검증된 변경과 함께 갱신된다. 이번 범위는 Phase 1, v1.4 리뷰 화면, v1.5 이름 확정, 그리고 pstack 대비 밀도 검증 2차에서 나온 우회 경로 보강이다.

## [작업 이력]

| 단계 | 내용 | 커밋 |
|---|---|---|
| v1.3 | Go CLI `abbd`, md 원본 + YAML frontmatter SSOT, 세 축 출처(origin/verification/confirmation), anchor 기반 안정 ID, run → verify → sync 턴 엔진, 완료 게이트 | 초기 커밋들 |
| v1.4 | StructuralDelta 카드(A/B/C), 항목별 승인·거부, `abbd serve` 리뷰 화면, 결정 파일, 고정 배치(`layout.json`), 결정적 SVG 렌더, AC-REV-01~06 | 1d947f5 전후 |
| v1.5 | 이름 ABBD, `.cccv`/`.myharness` 자동 이전, Claude 플러그인 패키징, Codex·Antigravity 설치 스크립트 | a743a65 전후 |
| 밀도 1차 보강 | 산문 규칙을 엔진으로: `--user` 결정 주체, 실패 재현 먼저, T3 driver, 수준 하향 사유, 범위 밖 의미 재작성 금지, 손편집 탐지 | 1aaeb80 |
| 설계도 우선 | `BLUEPRINT.md`, `.abbd/.ignore`, history-guard, `read_first`, 상시 규칙 | d0a1f01, 86fe734 |
| 사용자 원문 | UserPromptSubmit 훅이 사용자 메시지를 그대로 REQ로, 규칙 문장마다 상시 규칙 하나, 중복 제거 | dd066fa |
| 밀도 2차 보강 | 아래 [Density] B1~B9 수정 | 019fead 이후 |

## [Bootstrap Modes]

모드는 greenfield, code-only, adopt 세 가지다. 어느 모드도 질문하지 않는다. 자세한 동작은 `README.md`와 `docs/cli.md`에 있다. adopt는 의미 필드만 ADOPTED로 가져오고 구조는 코드에서 다시 추출한다.

## [Contract View]

Go AST 기준으로 다음을 추출한다.

- HTTP 라우트는 IF, Kafka 토픽은 EVT, SQL 테이블은 DATA, 환경 변수는 CFG, main 패키지는 CLI(IF)다.
- Dockerfile, compose, k8s는 배포 단위가 된다.
- Python/JS/TS는 정규식 분석기로 처리한다.
- 런타임에 조립되는 토픽은 UNKNOWN으로 남긴다. 실서비스 계약은 UNVERIFIABLE과 이유를 함께 적는다.

## [Approval Tiers / Review Surface]

- **등급 판정.** 구조 변경(StructuralDelta) 항목마다 PRD 08B 표로 매긴다. 외부 소비자가 있는 계약의 추가·삭제·breaking 변경은 C다.
- **C는 승인 전 반영되지 않는다.** 턴 안의 sync는 SSOT 사본에 먼저 적용하고, 승인되지 않은 C가 있으면 사본을 버린다. 2차 보강 후에는 턴 밖 경로(`abbd reconcile`, `abbd run`의 암묵 재조정)도 같다. 외부 C 변경은 BLOCKED 기록으로 남고 종료 코드 2를 낸다.
- **결정의 범위.** 결정은 자기가 본 변경에만 유효하다. 같은 항목이 다시 바뀌면 새 카드가 생긴다.
- **결정 통로.** 사용자 통로는 `abbd serve`(loopback 전용, 세션 토큰, Host 검사), 결정 파일(`abbd review --apply`), 사용자가 직접 치는 `--user` CLI다. MCP는 에이전트 통로다. MCP와 `--user` 없는 CLI로는 B/C 카드를 결정할 수 없고, 답은 에이전트의 INFERRED 선택으로 기록된다.
- **거부 처리.** 거부는 다음 `abbd run`의 `rework`로 정확히 한 번 들어간다. C 카드가 모두 거부된 턴은 다음 run이 닫고, 그 턴의 파일은 재조정하지 않고 새 턴의 작업으로 넘긴다.

## [History Ignore / Reference Drive]

PRD는 요청마다 쌓이는 이력이라 지난 내역이 지금의 컨텍스트에 섞이면 노이즈가 된다. 그래서 에이전트를 이력에서 떼어 내고 현재 설계도로 몰아 준다. 장치는 다섯 겹이다.

| 장치 | 하는 일 | 검증 |
|---|---|---|
| `.abbd/BLUEPRINT.md` | 현재 상시 규칙, 모듈, 계약, 기능, 열린 결정을 한 장에. publish 때마다 다시 쓴다 | `tests/blueprint_test.go` |
| `.abbd/.ignore` | `changes/`, `requirements/`, `decisions/`, `verification/runs/`, `generated/`를 rg·검색 도구에서 제외 | rg로 실제 제외 확인하는 테스트 |
| PreToolUse `history-guard.sh` | Read/Grep/Glob/Bash/PowerShell로 이력 경로를 읽으면 거부하고 BLUEPRINT와 `abbd inspect`로 안내 | 우회 변형 8종 거부, 정상 읽기 7종 허용 (아래) |
| `abbd run`/`plan`의 `read_first`, `standing_rules` | 이번 턴에 먼저 읽을 현재 파일과 지켜야 할 규칙을 결과에 넣는다. 이력 경로는 절대 넣지 않는다 | `tests/blueprint_test.go` |
| SessionStart 훅 | 세션 시작마다 턴 의무, BLUEPRINT, 상시 규칙, 상태를 주입한다 | `tests/surface_test.go` |

history-guard는 밀도 2차에서 `cd .abbd && cat changes/…`, `cat .abbd/ch*/…`, `find .abbd … -exec cat`, `git show HEAD:.abbd/changes/…`, `git log -p -- .abbd/changes`, `rtk git show …`, Glob `.abbd/*/*.md` 일곱 가지로 뚫렸다. 지금은 이력 경로·`.abbd/` 바로 아래 와일드카드·`.abbd`로 cd를 모두 거부하고, 현재 설계도 경로를 이름으로 대지 않은 Bash의 `.abbd` 언급도 거부한다. 단일 `abbd …` 명령과 `git add/commit/status` 류만 통과시킨다. 이 판단은 정규식이라 보안 경계는 아니다. 컨텍스트 위생 장치다. pstack에는 같은 종류의 장치가 없다. pstack의 PreToolUse는 Copilot 모델 선택용이고, `.audit` 기록은 커밋하지 않는 규칙만 있다.

## [Density] pstack 대비 밀도 검증

독립 평가자(별도 세션)가 두 소스를 원문 그대로 비교했다. pstack 내용은 복사하지 않았다.

| 항목 | 1차 (v1.3) | 2차 (86fe734) |
|---|---|---|
| 엔진이 강제하고 테스트로 고정한 규칙 | 33 | 64 |
| 엔진이 강제하지만 테스트 없음 | 21 | 30 |
| 산문뿐인 규칙 | 26 | 30 |
| "pstack보다 얇다" 판정 쌍 | 3 | 0 |
| pstack 번역 분량 | 문단 단위 60~70줄 | 문장 단위 16곳 |
| skill과 엔진 불일치 | 5 | 1차 5건 해결, 새로 13건 |
| 강제처럼 보이지만 우회되는 규칙 | 조사 안 함 | 9건 (B1~B9) |

2차 판정은 "구조가 산문을 대체한다는 주장은 3분의 2쯤 참"이었다. 남은 3분의 1은 턴 밖 경로와 자기 증명이었다. 019fead 이후 보강은 다음과 같다.

| 우회 | 2차에서 본 것 | 지금 |
|---|---|---|
| B1 턴 밖 편집 | 턴 없이 고치고 `sync`하면 외부 변경으로 흡수, 검증 0회, C도 반영 | PostToolUse `note-edit.sh`가 에이전트 편집을 기록. `reconcile`은 그 파일을 거부하고 다음 `run`이 턴에 넣는다. 외부 C는 BLOCKED |
| B2 MCP 결정 주체 | `answer_decision`이 무조건 사용자 | MCP는 에이전트 통로. B/C 카드 결정 불가 |
| B3 실패 재현 | `verify --cmd false`로 통과 | 실패한 테스트가 있는 FAILED만 인정. `--no-repro`는 10자 이상 사유 |
| B4 T3 | 같은 턴에 `echo PASS` driver로 통과 | 같은 턴에 등록한 driver는 이번 턴을 증명하지 못함. `--unverifiable user-level`은 NEEDS_USER |
| B5 손편집 | 다음 publish가 흔적을 지움 | `State.HandEdits`로 남고 `check --accept-edits`로만 지움 |
| B6 history-guard | 7종 통과 | 위 표대로 거부 |
| B7 명령 등급 | substring. `git -c x push` 실행, `go vet ./internal/deploy/...` 거부 | 명령 위치 기준 정규식 |
| B8 Stop 훅 두 번째 통과 | 설계 선택 | 유지 |
| B9 상시 규칙의 새 경로 | 검사 없음 | 상시 규칙이 있고 새 라우트에 테스트가 없으면 `risks` 경고 |

규칙 감지(M11)도 바꿨다. substring 대신 문장마다 범위어와 의무어를 함께 요구한다("everywhere" 같은 강한 범위어는 단독 인정). "절대 경로로 바꿔줘", "always_on 플래그", "모든 테스트가 통과하도록 고쳐줘"는 규칙이 되지 않는다. 자동 감지 규칙은 UNCONFIRMED로 BLUEPRINT에 표시한다. skill·문서가 엔진과 어긋난 13건(M1~M13)은 같은 커밋 묶음에서 엔진 기준으로 고쳤다. 보강 후 밀도 평가는 다시 돌리지 않았다. 위 "지금" 열은 회귀 테스트(`tests/bypass_test.go`, `tests/rules_test.go`, 단위 테스트)와 guard 15건 실행으로 확인한 것이다.

## [Benchmark]

실행기는 `benchmark/run.py`다. arm마다 `claude -p … --setting-sources project --plugin-dir <arm>`으로 격리한 세션을 열고, 숨김 테스트로 채점한다. 모델 sonnet, 과제당 예산 $4.

**v1 파일럿 (10과제, 기능만).** pstack 10/10, abbd 10/10, 회귀 0. 비용 pstack $1.46, abbd $1.30. 턴 103 대 68. 1회 실행이라 차이는 가설이다. 결과는 `docs/benchmark-results/`.

**v2 (12과제, 앞 과제에서 말한 규칙을 뒤 과제가 지키는지, 2회 반복).** 결과는 `docs/benchmark-results-v2/`.

| arm | 기능 첫 시도 | 규칙 첫 시도 | 규칙 최종 | 회귀 | 비용 USD | 턴 | 분 |
|---|---|---|---|---|---|---|---|
| 하네스 없음 | 100% | 98% | 98% | 0 | 1.57 | 103 | 5.5 |
| pstack | 100% | 98% | 98% | 0 | 2.03 | 122 | 5.8 |
| superpowers 7.0.0 | 100% | 100% | 100% | 0 | 1.86 | 124 | 5.6 |
| gstack 25dbe34 | 100% | 100% | 100% | 0 | 1.75 | 120 | 5.6 |
| abbd 86fe734 | 100% | 100% | 100% | 0 | 1.92 | 86 | 6.5 |

차이는 BodyLimit 규칙 하나다. 하네스 없음과 pstack이 2회 중 1회씩 놓쳤다. n=2라 유의하지 않다. 천장이 여전하다. 과제의 규칙이 코드 한 곳(미들웨어)에 모여 있어서 기억 없는 에이전트도 코드를 읽으면 지킨다. abbd는 규칙을 설계도에 올렸지만 T01의 규칙 하나를 놓치고 JSONErrors를 중복 저장했다. 이 둘은 dd066fa(사용자 원문 캡처, 문장별 감지, 중복 제거)에서 고쳤고 다시 측정하지는 않았다. abbd는 턴이 가장 적고(86) 시간이 가장 길다(6.5분). 시간은 매 턴 verify·sync 실행 비용으로 추정한다.

## [Proposed Architecture]

```
cmd/abbd → internal/cli ─┐
internal/mcp ────────────┼→ internal/workflow   턴 엔진, 게이트, 결정, Project Intelligence API
internal/serve ──────────┘     ├ bootstrap · intent · impact · verify · policy
                               ├ knowledge (EXTRACTED 갱신, Publish, BLUEPRINT)
                               └ review (StructuralDelta, layout, SVG 렌더)
                                    └ analysis · identity · ssot · graph · html
plugins/abbd   Claude Code 플러그인 (skills 8, .mcp.json, 훅: SessionStart, UserPromptSubmit,
               PreToolUse history-guard, PostToolUse note-edit, Stop)
plugins/install.sh|ps1  Codex · Antigravity 설치
benchmark/     실행기, v1·v2 과제, 숨김 테스트, 기준 구현
```

## [Open Decisions] (기본값으로 진행함)

| 결정 | 진행한 기본값 | 바꾸려면 |
|---|---|---|
| 그래프 저장소 | `generated/graph.json` | 커지면 SQLite 어댑터 |
| 최신성 | git HEAD + 파일별 fingerprint (대형 저장소 비용은 측정 안 함) | git 기반 증분 |
| Go 모듈 경로 | `github.com/whyjp/abbd` (저장소 이름은 ABBD) | 바꾸면 `go install` 문구도 바꾼다 |
| 배포 | main에 직접 push (사용자 지시, PR 리뷰 없음) | 이후 작업은 PR로 |
| Q20 / Q21 | 서버 SVG + 최소 JS / A 등급은 카드 없이 적용 목록 | `internal/review/render.go` |
| Stop 훅 강도 | 한 번만 되돌림 (두 번째 stop은 통과) | `plugins/abbd/scripts/stop-gate.sh` |
| MCP 결정 주체 | 에이전트 | 사용자 확인이 붙은 MCP 클라이언트가 생기면 별도 채널로 |

## [Evidence]

- `go vet ./...`, `go test ./...` 종료 코드 0 (019fead 이후 작업 트리). E2E, AC-BOOT, AC-REV-01~06, 회귀, 우회 고정 테스트를 포함한다.
- history-guard 15건(거부 8, 허용 7) 직접 실행 통과.
- `claude plugin validate`, GitHub 마켓플레이스 설치, `go install`은 a743a65 시점에 확인했다. 리뷰 화면 Playwright 확인은 1d947f5 시점이다.
- 벤치마크 원자료는 `docs/benchmark-results/`(v1), `docs/benchmark-results-v2/`(v2)에 있다.

## [Next Step]

1. **천장을 깨는 벤치마크 v3.** 규칙이 코드 한 곳에 모이지 않는 과제(경로마다 따로 구현되는 불변조건, 이전 결정과 충돌하는 요청, 외부 소비자 계약 변경). 3회 이상 반복하고 모든 arm을 같은 커밋에서 돌린다. dd066fa 이후 abbd를 다시 잰다.
2. **밀도 3차.** 보강 후 상태로 독립 평가를 다시 돌린다. 산문뿐인 규칙 30개 중 엔진으로 옮길 것을 고른다.
3. **Bash 편집 추적.** PostToolUse는 Edit/Write 도구만 본다. `sed -i`나 리다이렉션 편집은 외부 변경으로 보인다. Codex·Antigravity에는 이 훅이 없다.
4. **멀티 호스트(FR-14).** Codex와 Antigravity에서 실제 에이전트로 code-only 시나리오를 끝까지 돌린다.
5. **PriorArtMatrix(02A).** Graphify, GitNexus, CodeGraphContext의 원문과 라이선스를 직접 연다.
6. **serve 동시 쓰기.** serve와 CLI가 동시에 SSOT를 쓸 때 잠금이 없다(리뷰 추정, 미재현).
7. **driver와 진입점 연결.** `driver set --run`은 아무 명령이나 받는다. 진입점을 실제로 건드리는지는 검사하지 않는다.
