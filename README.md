# ABBD

ABBD를 처음부터 다시 짓는 중이다. 설계는 `basedoc/ABBD-재설계.html`에 있다.

ABBD는 pstack 위에 얹는 설계도(토폴로지) 층이다. 작업 절차와 검증은 pstack에 맡기고, ABBD는 사람이 승인한 의도된 토폴로지, 코드에서 뽑은 실제 토폴로지, 둘의 어긋남, 완료 게이트만 맡는다. 호스트는 Claude Code 하나다.

이전 구현(v1.5 Phase 1)은 태그 `abbd-v1.5-phase1`에 남아 있다. 이전 PRD는 `basedoc/`에 있다.
