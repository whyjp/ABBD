# ABBD

ABBD를 처음부터 다시 짓는 중이다. 설계는 `basedoc/ABBD-재설계.html`에 있다.

ABBD는 pstack 위에 얹는 설계도(토폴로지) 층이다. 작업 절차와 검증은 pstack에 맡기고, ABBD는 사람이 승인한 의도된 토폴로지, 코드에서 뽑은 실제 토폴로지, 둘의 어긋남, 완료 게이트만 맡는다. 호스트는 Claude Code 하나다.

이전 구현(v1.5 Phase 1)은 태그 `abbd-v1.5-phase1`에 남아 있다. 이전 PRD는 `basedoc/`에 있다.

## 설치

pstack 0.15.36(`/plugin install pstack@cursor-plugins`)이 먼저 있어야 한다. CLI는 `go build -o bin/abbd ./cmd/abbd`로 만들고 PATH에 둔다.

저장소 루트가 Claude Code 마켓플레이스다.

```
/plugin marketplace add <이 저장소의 경로>
/plugin install abbd@abbd
```

코드만 있는 저장소에서는 `/abbd:abbd-setup`이 pstack으로 추출 스크립트를 만들고 `abbd setup`을 돌린다. 그 뒤 사람이 `abbd align --all`이나 `abbd align --confirm <IDS>`로 설계 단위를 확인하면 정렬이 끝난다.
