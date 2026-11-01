# ABBD

ABBD를 처음부터 다시 짓는 중이다. 설계는 `basedoc/ABBD-재설계.html`에 있다.

ABBD는 병합된 코드의 아키텍처 토폴로지를 최종 상태로 `.abbd/`에 보관하고, 사람과 에이전트가 그것을 묻게 하는 능력이다. 구현에는 끼어들지 않는다. 분석, 설계, 결정, 구현은 쓰는 중인 모드(poteto-mode나 사용자의 모드)와 pstack, 그리고 대화와 PR 속의 사람이 맡는다. 구성은 셋이다.

- `/abbd` 스킬: 작업 전에 청사진을 읽고 `abbd explain`, `abbd path`, `abbd impact`로 묻는다. 읽는 이가 찾은 결합과 사람이 말한 것을 `abbd record`로 남긴다. 작업 뒤에 `abbd sync`를 돌리고 `abbd diff`를 PR에 싣는다.
- `/abbd-setup` 스킬: 코드만 있는 저장소에 pstack으로 추출 스크립트를 만들고 `abbd setup`을 돌린다. 마지막에 `abbd install`이 프로젝트의 `CLAUDE.md`에 짧은 상시 지시 블록을 넣는다. 처음 커밋한 상태가 기준선이다.
- `abbd` CLI: 추출, 근거 확인, ID 유지, 저장, 질의, 차이 계산, `.abbd/blueprint/index.html` 렌더링만 하는 레버다. 판단하거나 막거나 묻지 않는다.

호스트는 Claude Code 하나다. 이전 구현(v1.5 Phase 1)은 태그 `abbd-v1.5-phase1`에 남아 있다. 이전 PRD는 `basedoc/`에 있다.

## 설치

pstack 0.15.36(`/plugin install pstack@cursor-plugins`)이 먼저 있어야 한다. CLI는 `go build -o bin/abbd ./cmd/abbd`로 만들고 PATH에 둔다.

저장소 루트가 Claude Code 마켓플레이스다.

```
/plugin marketplace add <이 저장소의 경로>
/plugin install abbd@abbd
```

코드만 있는 저장소에서는 `/abbd:abbd-setup`을 돌린다. 그 뒤 `CLAUDE.md`의 블록이 어떤 어시스턴트에게든 grep보다 청사진을 먼저 묻게 하고, `/abbd:abbd`가 묻고 기록하고 차이를 싣는 방법을 준다.
