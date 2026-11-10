# ABBD

ABBD를 처음부터 다시 짓는 중이다. 설계는 `basedoc/ABBD-재설계.html`에 있다.

ABBD는 병합된 코드의 아키텍처 토폴로지를 최종 상태로 `.abbd/`에 보관하고, 사람과 에이전트가 그것을 묻게 하는 능력이다. 구현에는 끼어들지 않는다. 분석, 설계, 결정, 구현은 쓰는 중인 모드(poteto-mode나 사용자의 모드)와 그 스킬들, 그리고 대화와 PR 속의 사람이 맡는다. 구성은 셋이다.

- `/abbd` 스킬: 작업 전에 청사진을 읽고 `abbd explain`, `abbd path`, `abbd impact`로 묻는다. 읽는 이가 찾은 결합과 사람이 말한 것을 `abbd record`로 남긴다. 결합마다 무엇이 건너가는지 짧은 구절(`--phrase`)을 붙이고, 청사진은 그것을 선 옆에 쓴다. 작업 뒤에 `abbd sync`를 돌리고 `abbd diff`를 PR에 싣는다. 그림을 그리는 방식을 바꿀 때는 앞뒤로 `abbd lint`를 돌려 `docs/blueprint-svg.md`의 그리기 기준에 대 보고, 교차에 초과 꺾임과 계단 4배를 더한 합이 줄고, T자(tee)와 초과 꺾임이 늘지 않고, 하드 위반이 `.abbd/lint-debt.yaml`의 부채를 넘지 않고, 배치 부채가 2% 넘게 늘지 않을 때만 바꾼 것을 남긴다.
- `/abbd-setup` 스킬: 코드만 있는 저장소에 /abbd의 리더로 코드를 읽어 추출 스크립트를 만들고 `abbd setup`을 돌린다. 마지막에 `abbd install`이 프로젝트의 `CLAUDE.md`에 짧은 상시 지시 블록을 넣는다. 처음 커밋한 상태가 기준선이다.
- `abbd` CLI: 추출, 근거 확인, ID 유지, 저장, 질의, 차이 계산, 그림 측정, `.abbd/blueprint/index.html` 렌더링만 하는 레버다. 판단하거나 막거나 묻지 않는다.

호스트는 Claude Code 하나다. 이전 구현(v1.5 Phase 1)은 태그 `abbd-v1.5-phase1`에 남아 있다. 이전 PRD는 `basedoc/`에 있다.

## 설치

pstack은 필요 없다. pstack 0.15.36이 설치되어 있으면 /abbd의 리더가 읽기 엔진으로 쓸 수 있다. CLI는 `go build -o bin/abbd ./cmd/abbd`로 만들고 PATH에 둔다.

저장소 루트가 Claude Code 마켓플레이스다.

```
/plugin marketplace add <이 저장소의 경로>
/plugin install abbd@abbd
```

코드만 있는 저장소에서는 `/abbd:abbd-setup`을 돌린다. 그 뒤 `CLAUDE.md`의 블록이 어떤 어시스턴트에게든 grep보다 청사진을 먼저 묻게 하고, `/abbd:abbd`가 묻고 기록하고 차이를 싣는 방법을 준다.

## 훅과 CI

`abbd status --check`는 상태, 청사진, 모듈 설명 중 하나가 STALE이거나 근거가 drifted면 1로 끝난다. 모두 같은 PR에서 바로잡을 수 있는 것이다. dead 근거는 보고만 하고 실패시키지 않는다. 이미 모든 뷰에서 빠졌고, 지우려면 `abbd retract`뿐이기 때문이다. 상태나 청사진이 UNKNOWN이어도 1로 끝나고 원인을 말한다. git 작업 트리가 아니거나 git이 지문을 못 뜨면 상태가 UNKNOWN이고, `graph.json`이 없으면 청사진이 UNKNOWN이다. 모듈이 사라졌거나 최상위가 아닌 설명(orphaned)은 보고만 하고 건너뛴다. 상태는 추적하지 않는 파일까지 지문에 넣으므로, 빌드가 파일을 쓰기 전 깨끗한 체크아웃에서 돌린다. lefthook과 GitHub Actions 예시는 `plugins/abbd/skills/abbd/SKILL.md`의 "Gate a hook or CI"에 있다.
