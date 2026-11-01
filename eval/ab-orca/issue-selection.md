# 과제 고르기 (2026-11-01, Orca main 90a5374)

`gh issue list -R stablyai/orca --state open --limit 80`에서 시작했다. 이 저장소는 이슈가 열리면 몇 시간 안에 PR이 붙는 경우가 많다. 그래서 후보마다 이슈 번호로 PR을 검색하고, 이슈의 `closedByPullRequestsReferences`를 보고, 원격 브랜치 15,659개의 이름을 키워드로 걸렀다.

## 고른 둘

| 이슈 | 제목 | 왜 | 닿는 모듈 (코드 grep) |
|---|---|---|---|
| #36652 (2026-10-31) | Configurable separator between branch prefix and worktree name | 이슈가 함수 이름(`computeBranchName`, `getBranchPrefixIssue`, `getConfiguredBranchPrefix`, `normalizeBranchPrefix`)과 기본값(`/`)까지 적어 명세가 분명하다. 설정 하나와 그 소비자라 작다. | shared(`src/shared/branch-prefix.ts`), desktop main(`src/main/ipc/worktree-branch-name.ts`, `src/main/agent-hooks/first-work-branch-rename.ts`), renderer(`components/settings/BranchPrefixFeedback.tsx`). 프로세스 경계 둘 |
| #36697 (2026-10-31) | Expose agent and workspace status colors as plugin theme tokens | 토큰 8개의 이름과 기본값 규칙(지금 값이 fallback)이 적혀 있다. 작다. | renderer(`assets/main.css`), shared 플러그인 매니페스트/스키마(`src/shared/plugins/...`), desktop main 플러그인 테마 등록(`src/main/plugins/contributions/plugin-theme-registries.ts`), 문서(`docs/reference/plugin-theme-tokens.md`) |

## 구현이 없다는 확인

- `gh pr list --state all --search 36652` 0건, `--search 36697` 0건. `branchPrefixSeparator`, `branch prefix separator`, `branch prefix dash`, `theme tokens agent status colors`, `workspace-status theme token` 검색에서 관련 PR 없음(나온 것은 다른 주제: #36881 사이드바 강조색, #26455 사용량 막대 색 등).
- 두 이슈 모두 `closedByPullRequestsReferences` 비어 있음, 댓글 0.
- 원격 브랜치 이름 중 `separator|theme-token|status-color|branch-prefix`에 걸린 것: `brennankbenson/orca-branch-prefix(-expanded)`(2026-05-22, GitHub 사용자 이름 접두사, 구분자 아님), `brennanb2025/agent-map-status-color`(2026-08-13, 대시보드 질문 상태 색, 테마 토큰 아님). `gh api compare`로 커밋 제목을 확인했다.

## 뺀 후보

| 이슈 | 이유 |
|---|---|
| #36942 모바일 세션 머리글 | PR #36945가 열려 있음 |
| #36745 모바일 사용량 막대 | PR #36746 |
| #36908 모바일 계정 | PR #36755 |
| #36632 Relay 리스너 | PR #36640 |
| #36995 Copilot 사용량 | #6892의 중복, PR #18210, #25911 |
| #36749 채팅 모델 세션 한정 | 해법이 "예를 들어" 둘로 열려 있어 결과 비교가 제품 결정에 묶임 |
| #36669 마지막 호스트 기억 | 중간 크기 이상, 우선순위 규칙이 길어 2회 반복 예산에 큼 |
