# 첫 회차 결과 보고 (PRD 16장 템플릿)

작성 2026-10-18 · 브랜치 `fmh-phase1-draft` · 범위 Phase 1 초안 (HITL 없이 문서 충실 구현)

## [Goal]

내 개발 방식에 맞춘 개인용 하네스다. 준비물 없이 시작하고, 표준 구조(SSOT Architecture Map + Feature Map, md 원본 + 생성 HTML)를 하네스가 직접 만들고 유지한다. 이번 회차의 목표는 PRD 14장 Phase 1 완료 기준이다. code-only E2E를 통과하고, greenfield를 합격 테스트로 통과하고, 사람이 문서를 쓰지 않으며, bootstrap 질문은 0회여야 한다.

## [Bootstrap Modes]

- **greenfield.** 소스가 없으면 자동 선택된다. 빈 지식으로 시작하고, 첫 `myharness run` 요청이 REQ와 FEAT 초안(USER_STATED)이 된다.
- **code-only.** 소스가 있으면 자동 선택된다. Discover → Observe(`--verify-available`일 때만 실행, 아니면 NOT_RUN으로 명시) → Infer(진입점마다 INFERRED FEAT 후보) → Seed → BootstrapReport.
- **adopt.** `--from` 문서(md/html 표, yaml/json)에서 의미 필드만 ADOPTED로 가져오고 구조는 코드에서 다시 추출한다. 충돌과 매칭 실패는 보고서에 남는다. 입력이 깨져도 기본 경로는 계속된다.
- 세 모드 모두 입력을 읽지 않는다. 불확실한 항목은 확인 대기 목록(`confirm-queue.html`, `myharness confirm --list`)에 올라간다.

## [Contract View]

Phase 1 추출 범위는 Go AST 기준이다 (`internal/analysis`).

- HTTP 라우트(`HandleFunc`, `GET/POST/...`, Go 1.22 패턴)는 IF.
- Kafka 토픽(`Topic` 필드·상수, reader/writer 생성자, 방향은 이름으로 판정)은 EVT.
- SQL 마이그레이션 `CREATE TABLE`은 DATA. 소유 모듈은 테이블명을 참조하는 Go 코드다.
- `os.Getenv`/`LookupEnv`는 CFG. main 패키지는 CLI 명령(IF)이다.
- Dockerfile, compose, k8s Deployment는 서빙 구조(배포 단위 → 모듈)다.
- Python/JS/TS는 정규식 분석기로 라우트와 심볼만 다룬다.
- 런타임에 조립되는 토픽은 `origin: UNKNOWN`으로 남기고, 없다고 단정하지 않는다.
- 브로커·DB·환경 변수 계약은 로컬 실행으로 확인할 수 없어 `UNVERIFIABLE`과 이유를 함께 적는다.

## [Approval Tiers]

- **A.** 읽기, 로컬 수정, 맵 동기화, 렌더, 로컬 검증, 로컬 commit(`project.yaml`로 변경 가능)은 자동이다.
- **B.** `myharness decide`는 선택지 2~3개와 기본값을 받는다. 턴당 3회까지만 묻고, 넘치면 묻지 않고 기본값으로 진행한다. 같은 질문은 ADR에서 찾아 다시 묻지 않는다. 답이 없으면 UNCONFIRMED ADR이 남고 턴은 NEEDS_USER로 끝난다. sync는 소비자 미상 계약의 breaking change를 자동으로 B로 기록한다.
- **C.** external_consumers: true 계약의 breaking change가 있으면 sync가 BLOCKED + exit 2로 멈춘다. `decide --tier C --subject ID --answer approve`가 있어야 진행한다. push/deploy 등은 `myharness policy classify`가 C로 분류한다.
- **지연 확인.** bootstrap 항목은 묻지 않는다. `run`이 이번 턴의 대상 중 의미가 미확인인 항목을 `confirm_if_purpose_used`로 알려 주고, 결과가 그 의미에 의존할 때만 에이전트가 등급 B로 묻는다.

## [pstack Source Review]

이 세션에서 설치본 pstack 0.9.93(pstack-claude 포트)의 poteto-mode와 원칙 스킬은 직접 읽었다. how, why, architect, blast-radius, correct, reflect의 로컬 원문은 스킬 작성 위임 에이전트에게 참고하라고 지시했다. 차용은 아이디어 수준이고, 파일과 문장은 복사하지 않았다 (`THIRD_PARTY_NOTICES.md`). 재사용한 절차는 작업 유형별 라우팅, 구현 전 타입·경계 설계, 증거 중심 영향 분석, 교정 순서(아키텍처 → 타입 → 린트 → 테스트 → 문서), 검증 가능한 작은 단위다. 재구현한 엔진은 턴 상태 기계, 완료 게이트, SSOT 동기화, 앵커 ID, 계약 추출이다.

**미확인.** GitHub 원문(R01-R14)과 Graphify, GitNexus, CodeGraphContext의 README·LICENSE는 이번 회차에 열어 보지 않았다. 라이선스 표기는 PRD가 적은 내용을 옮긴 것이다. PriorArtMatrix(02A)는 다음 회차 작업이다.

## [Proposed Architecture]

```
cmd/myharness → internal/cli ─┐
internal/mcp ─────────────────┴→ internal/workflow   턴 엔진 + Project Intelligence API
                                   ├ bootstrap   greenfield/code-only/adopt
                                   ├ intent      라우팅(표) + 의도 해석
                                   ├ impact      계약 먼저 → 모듈 → 역의존 → 기능 → 테스트
                                   ├ verify      실행 증거, UNVERIFIABLE 구분, redaction
                                   ├ policy      등급 표
                                   └ knowledge   EXTRACTED 층 갱신(Apply/Seed) + Publish
                                        ├ analysis  Go AST / regex / SQL / manifests
                                        ├ identity  앵커 매칭
                                        ├ ssot      md 원본 저장소
                                        ├ graph     파생 그래프 + bounded query
                                        └ html      Explorer + 확인 대기 페이지
```

## [Open Decisions] (등급 B: 아래 기본값으로 진행함)

| 결정 | 진행한 기본값 | 바꾸려면 |
|---|---|---|
| 그래프 저장소 (PRD는 SQLite 가설) | `generated/graph.json`. 의존성 하나, 재생성 가능 | 그래프가 커지면 SQLite 어댑터로 교체 |
| 최신성 기준 | git HEAD + 파일별 내용 fingerprint | 큰 저장소에서 비용이 문제면 git 기반 증분 |
| 첫 언어 / 호스트 (Q2, Q3) | Go 정적 분석 / Claude Code 어댑터 + Codex 안내 | 다른 언어 분석기 추가 |
| 로컬 commit (Q18) | A, 단 `sync --commit`을 줄 때만 | `project.yaml` `policy.local_commit_tier` |
| 의미 필드 자동 생성 문구 | `(추정) ...` 기계적 문장, INFERRED | 에이전트가 `myharness intent ...`로 INFERRED 재작성 |
| 외부 소비자 기본값 | 미상(nil) → breaking이면 B | `myharness confirm IF-x --external-consumers true` |

## [Implemented Prototype]

init → run → (에이전트 구현) → verify → sync가 code-only와 greenfield 양쪽에서 동작한다. 실패 경로(FAILED, 검증 후 편집, 등급 C)는 DRAFT로 남고 exit 2를 낸다. 외부 변경은 run이나 reconcile에서 질문 없이 재조정되고, 확정 계약과 충돌하면 등급 B ADR이 남는다.

## [Evidence]

- `go vet ./...`, `go test ./...` (단위 + `tests/` E2E)
- `tests/codeonly_test.go` `TestCodeOnlyE2E`: PRD 12장 첫 시나리오 1~7 단계와 AC-BOOT-02/03/04/06, 등급 B/C
- `tests/greenfield_test.go` `TestGreenfieldE2E` (AC-BOOT-01, 두 번째 시나리오), `TestAdoptMode` (FR-17, AC-BOOT-05)
- `tests/surface_test.go`: MCP와 CLI 일치(FR-10), 스킬이 미등록 명령을 쓰면 실패, Agent Skills frontmatter, 도메인 층 예약
- 결정 기록: `.audit/fmh-phase1.tsv` (커밋하지 않음)

## [Next Step]

1. 02A PriorArtMatrix를 만든다. R01-R14, R30, R33, R35 원문과 라이선스를 직접 연다. 실패 기준은 원문 없이 쓴 행이 하나라도 남는 것이다.
2. FR-14 멀티 호스트를 확인한다. Claude Code와 Codex에서 같은 code-only 시나리오를 스킬로 끝까지 돌린다. 지금은 설치 스크립트만 검증했다.
3. 사용자 저장소에서 cold-start grounding을 잰다. 사용자의 기존 SSOT를 정답으로 두고 bootstrap 결과를 비교한다(부록 A). 실패 기준은 모듈 recall < 0.8이다.
4. 비 Go 언어의 분석 정확도를 측정하거나, LSP/SCIP 어댑터를 붙인다 (Phase 3).
