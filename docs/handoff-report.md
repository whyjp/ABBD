# 인계 보고 (PRD 16장 템플릿) · v1.5 기준

2026-10-18 · 저장소 `github.com/whyjp/ABBD` (main) · PRD `basedoc/abbd_prd_v1_5.html`

## [Goal]

ABBD (Active Blueprint-Based Development). 에이전트가 하네스가 만들고 유지하는 설계도(Blueprint)를 기준으로 개발하고, 그 설계도가 검증된 변경과 함께 갱신된다. 이번 범위는 Phase 1과 v1.4 리뷰 화면, v1.5 이름 확정이다.

## [Bootstrap Modes]

모드는 greenfield, code-only, adopt 세 가지다. 어느 모드도 질문하지 않는다. 자세한 동작은 `README.md`와 `docs/cli.md`에 있다. adopt는 의미 필드만 ADOPTED로 가져오고 구조는 코드에서 다시 추출한다.

## [Contract View]

Go AST 기준으로 다음을 추출한다.

- HTTP 라우트는 IF, Kafka 토픽은 EVT, SQL 테이블은 DATA, 환경 변수는 CFG, main 패키지는 CLI(IF)다.
- Dockerfile, compose, k8s는 배포 단위가 된다.
- Python/JS/TS는 정규식 분석기로 처리한다.
- 런타임에 조립되는 토픽은 UNKNOWN으로 남긴다. 실서비스 계약은 UNVERIFIABLE과 이유를 함께 적는다.
- `NewWriter` 같은 생성자는 메시징 패키지에서 불렀을 때만 토픽으로 본다(벤치마크 중 발견한 오탐).

## [Approval Tiers / Review Surface]

- **등급 A/B/C 판정.** 등급은 구조 변경(StructuralDelta) 항목마다 따로 매긴다. 표는 PRD 08B를 따른다. 외부 소비자가 있는 계약의 추가·삭제·breaking 변경은 C다.
- **사본 기반 sync.** sync는 턴을 SSOT 사본에 먼저 적용한다. 승인되지 않은 C 항목이 있으면 사본을 버린다. 그래서 승인 전에는 설계도에 반영되지 않는다.
- **결정의 범위.** 결정은 자기가 본 변경에만 유효하다. 같은 턴 안에서 같은 항목이 다시 바뀌면 새 카드가 생긴다.
- **결정 통로.** `abbd serve`(loopback 전용, 세션 토큰, Host 검사), 결정 파일(`abbd review --apply`), MCP(`decide_structural_change`, `answer_decision`), CLI(`review --decide`, `decide --adr`)가 있다. 어느 통로든 같은 레코드를 쓴다.
- **거부 처리.** 거부와 되돌리기 요청은 다음 `abbd run`의 IntentDelta(`rework`)로 정확히 한 번 들어간다. C 카드가 모두 거부된 턴은 다음 run이 닫는다.
- **화면.** 토폴로지는 SSOT에서 결정적으로 렌더링한다(SVG, 시스템/모듈/기능 경로). 배치는 `generated/layout.json`에 고정된다.
- **진입 강제.** 플러그인 SessionStart가 모든 코드 변경에 턴을 의무로 명시한다. Stop 훅은 설계도가 작업 트리와 맞지 않으면 세션을 한 번 되돌린다.

## [pstack Source Review]

설치본 pstack 0.9.93의 아이디어만 빌려 왔다. 별도 모델(fable)의 원본 비교에서 초기 스킬 일부(약 60~70줄)가 pstack 문장을 가깝게 번역한 수준이라는 지적이 나왔다. 그 구간은 ABBD 엔진 동작 기준으로 다시 썼다. 경위는 `THIRD_PARTY_NOTICES.md`에 있다. 같은 비교에서 엔진이 강제하는 규칙은 54개(그중 33개는 테스트로 고정), 산문뿐인 규칙은 26개였다. 산문 규칙 일부는 이후 명령이나 테스트로 옮겼다.

## [Proposed Architecture]

```
cmd/abbd → internal/cli ─┐
internal/mcp ────────────┼→ internal/workflow   턴 엔진, 게이트, 결정, Project Intelligence API
internal/serve ──────────┘     ├ bootstrap · intent · impact · verify · policy
                               ├ knowledge (EXTRACTED 갱신 + Publish)
                               └ review (StructuralDelta, layout, SVG 렌더)
                                    └ analysis · identity · ssot · graph · html
plugins/abbd   Claude Code 플러그인 (skills 8, .mcp.json, SessionStart/Stop 훅)
plugins/install.sh|ps1  Codex · Antigravity 설치
benchmark/     부록 A 실행기, 숨김 테스트, 기준 구현
```

## [Open Decisions] (기본값으로 진행함)

| 결정 | 진행한 기본값 | 바꾸려면 |
|---|---|---|
| 그래프 저장소 | `generated/graph.json` | 커지면 SQLite 어댑터 |
| 최신성 | git HEAD + 파일별 fingerprint (대형 저장소 비용은 측정하지 않음) | git 기반 증분 |
| Go 모듈 경로 | `github.com/whyjp/abbd` (GitHub 저장소 이름은 ABBD) | 대문자 경로로 바꾸면 `go install` 문구도 바꾼다 |
| 배포 | `fmh-phase1-draft`를 main에 직접 push (사용자가 push를 지시함, PR 리뷰 없음) | 이후 작업은 PR로 받는다 |
| Q20 / Q21 | 서버 SVG + 최소 JS / A 등급은 카드 없이 적용 목록 | `internal/review/render.go` |
| Stop 훅 강도 | 한 번만 되돌림 (두 번째 stop은 통과) | `plugins/abbd/scripts/stop-gate.sh` |

## [Evidence]

- `go vet ./...`, `go test ./...`: 단위 테스트와 `tests/`의 E2E, 수용 기준(AC-BOOT, AC-REV-01~06), 회귀 테스트. 리뷰가 찾은 결함마다 고정 테스트가 있고, 대부분은 수정 전 커밋에서 실패하는 것을 확인했다.
- `claude plugin validate`가 통과한다. 격리된 설정에서 GitHub 마켓플레이스로 설치되고, GitHub에서 `go install`도 된다. 둘 다 a743a65 시점에 확인했고, 이후 커밋에서는 다시 확인하지 않았다.
- 리뷰 화면은 `abbd serve`와 Playwright로 시각 확인하고 승인 클릭까지 해 봤다(1d947f5 시점).
- 벤치마크 파일럿 결과는 `docs/benchmark-pilot.md`에 있다.

## [Next Step]

1. **천장을 깨는 벤치마크 과제.** 이전 결정과 충돌하는 요청, 외부 소비자 계약 변경, 여러 턴 전 불변조건을 깨기 쉬운 리팩터링이 필요하다. 3회 반복해 분산을 재고, 두 arm을 같은 커밋에서 돌린다.
2. **멀티 호스트(FR-14).** Codex와 Antigravity에서 설치 스크립트 실행을 넘어, 실제 에이전트로 code-only 시나리오를 끝까지 돌린다.
3. **PriorArtMatrix(02A).** Graphify, GitNexus, CodeGraphContext의 원문과 라이선스를 직접 연다.
4. **serve 동시 쓰기.** serve와 CLI가 동시에 SSOT를 쓸 때 잠금이 없다(리뷰에서 추정으로 지적, 미재현).
5. **T3 사용자 수준 실행 계층(driver)** 은 아직 없다.
