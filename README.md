# ABBD

ABBD를 처음부터 다시 짓는 중이다. 설계는 `basedoc/ABBD-재설계.html`에 있다.

ABBD는 병합된 코드의 아키텍처 토폴로지를 최종 상태로 `.abbd/`에 보관하고, 사람과 에이전트가 그것을 묻게 하는 능력이다. 구현에는 끼어들지 않는다. 분석, 설계, 결정, 구현은 쓰는 중인 모드(poteto-mode나 사용자의 모드)와 그 스킬들, 그리고 대화와 PR 속의 사람이 맡는다. 구성은 셋이다.

- `/abbd` 스킬: 작업 전에 블루프린트를 읽고 `abbd explain`, `abbd path`, `abbd impact`로 묻는다. 읽는 이가 찾은 결합과 사람이 말한 것을 `abbd record`로 남긴다. 결합마다 무엇이 건너가는지 짧은 구절(`--phrase`)을 붙이고, 블루프린트는 그것을 선 옆에 쓴다. 작업 뒤에 `abbd sync`를 돌리고 `abbd diff`를 PR에 싣는다. 그림을 그리는 방식을 바꿀 때는 앞뒤로 `abbd lint`를 돌린다. 바꾼 것을 남길지는 `docs/blueprint-svg.md`의 "What a change is weighed by"에 있는 게이트가 정한다.
- `/abbd-setup` 스킬: 코드만 있는 저장소에 /abbd의 읽기 에이전트로 코드를 읽어 추출 스크립트를 만들고 `abbd setup`을 돌린다. 마지막에 `abbd install`이 프로젝트의 `CLAUDE.md`에 짧은 상시 지시 블록을 넣는다. 처음 커밋한 상태가 기준선이다.
- `abbd` CLI: 추출, 근거 확인, ID 유지, 저장, 질의, 차이 계산, 그림 측정, `.abbd/blueprint/index.html` 렌더링만 하는 도구다. 판단하거나 막거나 묻지 않는다.

호스트는 Claude Code 하나다. 이전 구현(v1.5 Phase 1)은 태그 `abbd-v1.5-phase1`에 남아 있다. 이전 PRD는 `basedoc/`에 있다.

## 설치

pstack은 필요 없다. pstack 0.15.36이 설치되어 있으면 /abbd의 읽기 에이전트가 읽기 엔진으로 쓸 수 있다. CLI는 `go build -o bin/abbd ./cmd/abbd`로 만들고 PATH에 둔다. Windows에서는 `bin/abbd.exe`로 만든다.

CI가 검증하는 플랫폼은 Linux뿐이다. `.github/workflows/test.yml`이 Linux에서 `gofmt`, `go vet`, `go test`를 돌리고, 이 저장소의 `.abbd`를 `abbd sync`한 결과가 두 번 돌려도 바이트까지 같은지 비교한다. Windows에서는 WSL로 개발한다. Windows 네이티브와 macOS는 CI로 검증하지 않고 지원을 약속하지 않으며, 안정화 이후로 미룬다.

저장소 루트가 Claude Code 마켓플레이스다.

```
/plugin marketplace add <이 저장소의 경로>
/plugin install abbd@abbd
```

코드만 있는 저장소에서는 `/abbd:abbd-setup`을 돌린다. 그 뒤 `CLAUDE.md`의 블록이 어떤 어시스턴트에게든 grep보다 블루프린트를 먼저 묻게 하고, `/abbd:abbd`가 묻고 기록하고 차이를 싣는 방법을 준다.

## 훅과 CI

`abbd status --check`는 같은 PR에서 바로잡을 수 있는 것이 남았을 때 1로 끝난다. 상태나 블루프린트가 STALE이거나 UNKNOWN일 때, 모듈 설명, 노트, 흐름, 스택, 표, 개요 기록 중 하나가 STALE일 때, 근거가 drifted일 때가 그렇다. 무엇이 실패시키고 무엇이 보고만 되는지는 `plugins/abbd/skills/abbd/SKILL.md`의 "Gate a hook or CI"에 한 번만 적혀 있고, lefthook과 GitHub Actions 예시도 거기 있다. 상태는 추적하지 않는 파일까지 지문에 넣으므로, 빌드가 파일을 쓰기 전 깨끗한 체크아웃에서 돌린다.
