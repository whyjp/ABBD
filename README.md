# ABBD — Active Blueprint-Based Development

ABBD PRD v1.5(`basedoc/abbd_prd_v1_5.html`, 이전 판은 같은 폴더)의 Phase 1 구현이다. 에이전트가 하네스가 만들고 유지하는 설계도(Blueprint)를 기준으로 개발하고, 그 설계도가 검증된 변경과 함께 자동으로 갱신(Active)된다. 설계도는 SSOT Architecture Map, Feature Map, 외부 계약 뷰, 리뷰 화면의 토폴로지다. 매 턴 구현, 실행 검증, 문서·그래프 갱신을 하나의 완료 단위로 끝낸다. 빈 저장소든 코드만 있는 저장소든, 그 구조는 하네스가 직접 만든다.

구성은 두 층이다.

- **Core CLI `abbd`** (Go, 의존성은 `gopkg.in/yaml.v3` 하나). 상태 전이와 완료 게이트는 모델의 기억이 아니라 이 CLI가 소유한다. `abbd mcp`는 같은 기능을 MCP stdio 서버로 노출한다.
- **호스트 중립 Agent Skills** (`plugins/abbd/skills/`). 턴 루프의 단계마다 하나씩, 모두 8개다. 각 단계에서 어떤 CLI 명령을 부르는지 적혀 있다. Claude Code 플러그인으로 GitHub에서 바로 설치하고, Codex와 Antigravity는 같은 원본을 설치 스크립트로 받는다 (`plugins/README.md`).

## 빠른 시작

```sh
go build -o bin/abbd ./cmd/abbd

# code-only: 코드만 있는 저장소. 질문 없이 맵·그래프·HTML을 만든다
cd your-repo
abbd init . --mode code-only --verify-available
abbd status
abbd confirm --list            # 추정 항목. 틈날 때 정리한다

# 개발 턴
abbd run "메시지 개수를 돌려주는 GET /messages/count API 추가"
#   ... 에이전트가 코드와 테스트를 고친다 ...
abbd verify                    # 영향 범위 검증. 증거를 저장한다
abbd sync                      # PASSED일 때만 완료. 아니면 DRAFT로 남고 exit 2
abbd open-docs                 # .abbd/generated/explorer.html
```

설치:

```sh
go install github.com/whyjp/abbd/cmd/abbd@latest
# Claude Code (플러그인: 스킬 8개 + MCP 서버 + SessionStart 훅)
/plugin marketplace add whyjp/ABBD
/plugin install abbd@abbd
# Codex / Antigravity (같은 스킬 원본을 호스트 경로로 복사, MCP 설정 생성)
sh plugins/install.sh --host codex /path/to/project
sh plugins/install.sh --host antigravity /path/to/workspace
```

전체 명령은 `docs/cli.md`와 `abbd help`에 있다.

## 대상 프로젝트에 생기는 것

```
.abbd/
├── project.yaml            언어, 검증 명령, 승인 정책, 턴당 예산, 선호
├── architecture/MOD-*.md   모듈 + 외부 계약 뷰(IF/EVT/DATA/CFG) + 서빙 구조
├── features/FEAT-*.md      기능: 목적(why) / 관찰(what) / 진입점 / 검증
├── requirements/REQ-*.md   사용자 요청 원문 (USER_STATED)
├── decisions/ADR-*.md      등급 B/C 결정 (기본값 진행 기록 포함)
├── changes/CHG-*.md        턴 상태 + IntentDelta + ChangeReceipt + 계측
├── verification/           TEST-*.md, runs/run-*.yaml (실행 증거)
├── candidates.yaml         앵커 1개만 일치한 병합 후보
├── state.yaml              sync watermark (revision, fingerprint, md hash)
├── bootstrap-report.md     BootstrapReport
└── generated/              graph.json, explorer.html, confirm-queue.html (재생성 가능)
```

Markdown이 원본이다. YAML frontmatter가 구조화 데이터이고, 자동 요약 블록 아래의 `## Notes`는 사용자 영역이라 하네스가 덮어쓰지 않는다. 그래프와 HTML은 md에서 파생되며 같은 watermark를 갖는다. watermark가 어긋나면 `status`가 STALE을 보고한다 (FR-29).

## PRD 핵심 규칙이 코드에서 강제되는 곳

| PRD 규칙 | 강제 지점 |
|---|---|
| 세 축 근거 어휘, "why" 필드는 USER_STATED/ADOPTED/INFERRED만 (06장) | `internal/model/provenance.go` `NewSemantic`, `abbd check` |
| purpose는 사용자 확인으로만 승격, 테스트는 verification만 바꿈 | `internal/verify` `ApplyResults`, `workflow.Confirm` |
| 앵커 ≥2 일치는 같은 ID, 1개는 병합 후보, git rename 반영 (FR-27) | `internal/identity`, `internal/knowledge/apply.go` |
| bootstrap은 묻지 않는다 (AC-BOOT-06) | `internal/bootstrap`는 입력을 읽지 않는다. `decide`는 턴 밖에서 거부된다 |
| 검증 실패나 최신성 미달이면 완료 금지 (07장) | `workflow.Sync`: 현재 작업 트리에서 PASSED가 아니면 DRAFT + exit 2. 게시 후 다시 읽어 전부 CURRENT일 때만 완료 |
| 등급 A/B/C, 턴당 질문 ≤3, 답한 질문 재질문 금지 (10장) | `internal/policy`, `workflow.Decide`, sync의 계약 게이트 |
| 외부 계약 뷰는 Phase 1의 1급 항목 (08A) | `internal/analysis` 추출기, `MOD-*.external_contracts` |
| 외부 변경은 EXTRACTED만 갱신, 질문 없음 (03장 Reconcile) | `workflow.reconcile` |
| Graph-on-demand와 폴백 이유 기록 (FR-21) | `workflow.contextStrategy` → `CHG.plan.context_*` |
| 도메인 층은 ID 접두사만 예약 (08A) | `model.ReservedKinds`, `abbd domain`은 reserved |

## 검증

```sh
go vet ./...
go test ./...      # 단위 + E2E: code-only, greenfield, adopt, MCP, 스킬-CLI 일치
```

`tests/codeonly_test.go`는 PRD 12장 첫 번째 E2E 시나리오(Phase 1 완료 기준)를 그대로 실행한다. 맵 없는 저장소 bootstrap, 재실행 시 ID 보존, 실패한 턴의 DRAFT, 검증 후 편집 차단, 완료된 턴의 동기화, 다른 세션의 재사용, 외부 파일 이동 재조정, 확정 계약과 충돌하는 외부 변경(등급 B), external consumer가 있는 계약의 breaking change(등급 C 차단 후 승인)까지 다룬다. `tests/greenfield_test.go`는 빈 디렉터리에서 시작하는 두 턴과 adopt를 다룬다.

현재 상태와 열린 결정은 `docs/handoff-report.md`에 있다.
