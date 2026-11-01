# ABBD

ABBD를 처음부터 다시 짓는 중이다. 설계는 `basedoc/ABBD-재설계.html`에 있다.

ABBD는 pstack을 감싸는 모드(`abbd-mode`)와 레버 CLI(`abbd`)다. 분석, 설계, 결정, 구현은 pstack과 대화와 PR 속의 사람이 맡는다. ABBD는 병합된 코드의 아키텍처 토폴로지를 최종 상태로 `.abbd/`에 보관하고, 사람이 빨리 이해하고 판단하도록 `.abbd/blueprint/index.html`로 그린다. 호스트는 Claude Code 하나다.

이전 구현(v1.5 Phase 1)은 태그 `abbd-v1.5-phase1`에 남아 있다. 이전 PRD는 `basedoc/`에 있다.

## 설치

pstack 0.15.36(`/plugin install pstack@cursor-plugins`)이 먼저 있어야 한다. CLI는 `go build -o bin/abbd ./cmd/abbd`로 만들고 PATH에 둔다.

저장소 루트가 Claude Code 마켓플레이스다.

```
/plugin marketplace add <이 저장소의 경로>
/plugin install abbd@abbd
```

코드만 있는 저장소에서는 `/abbd:abbd-setup`이 pstack으로 추출 스크립트를 만들고 `abbd setup`을 돌린다. 처음 커밋한 상태가 기준선이다. 그 뒤 `/abbd:abbd-mode`는 작업 전에 청사진과 `abbd impact`를 읽고, 끝에 `abbd sync`와 `abbd diff`로 토폴로지 차이를 PR에 싣는다.
