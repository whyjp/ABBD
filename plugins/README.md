# abbd 패키지

스킬 원본은 `plugins/abbd/skills/` 한 곳이다. 호스트별 패키지는 이 원본을 그대로 서빙하거나 복사할 뿐, 사본을 따로 고치지 않는다. 모든 호스트에서 스킬이 부르는 것은 CLI `abbd`와 같은 API를 내는 MCP 서버 `abbd mcp`다. 바이너리는 `go install github.com/whyjp/abbd/cmd/abbd@latest`로 따로 설치한다.

| 호스트 | 패키지 | 설치 | 스킬 경로 | MCP |
|---|---|---|---|---|
| Claude Code | [`abbd/`](abbd/) 플러그인 + 루트 `.claude-plugin/marketplace.json` | `/plugin marketplace add whyjp/ABBD` → `/plugin install abbd@abbd` | 플러그인 `skills/` (`/abbd:<skill>`) | 플러그인 `.mcp.json` |
| Codex | [`codex/`](codex/) | `plugins/install.sh --host codex` | `.agents/skills`, `~/.agents/skills` | `.codex/config.toml`, `~/.codex/config.toml` |
| Antigravity | [`antigravity/`](antigravity/) | `plugins/install.sh --host antigravity` 또는 `agy plugin install ./plugins/abbd` | `.agents/skills`, `~/.gemini/config/skills` | `.agents/mcp_config.json`, `~/.gemini/config/mcp_config.json` |
| Claude Code (플러그인 없이) | 같은 원본 | `plugins/install.sh --host claude-code` | `.claude/skills` | 프로젝트 `.mcp.json` |

플러그인이 없는 호스트(Codex, Antigravity, 플러그인 없이 쓰는 Claude Code)에는 네임스페이스가 없어서 설치 스크립트가 스킬 이름에 `abbd-` 접두사를 붙인다 (`abbd-session-bootstrap` 등, PRD v1.5 CR-13). Claude Code 플러그인에서는 플러그인 이름이 접두사 역할을 해서 `/abbd:session-bootstrap`이 된다.

경로의 근거는 code.claude.com plugins 문서, learn.chatgpt.com Codex 문서, antigravity.google 문서다. 호스트가 자주 바뀌므로 설치 전에 그 호스트의 현재 문서와 대조한다.

Claude Code 플러그인은 SessionStart 훅(`abbd/hooks/hooks.json`)으로 세션마다 abbd 상태 몇 줄과 진입 skill을 알려 준다. 이 훅은 `sh`로 실행되므로 Windows에서는 Git Bash 같은 sh가 PATH에 있어야 한다. Codex와 Antigravity에는 같은 역할의 훅이 없어서 `AGENTS.md`나 규칙 파일 스니펫으로 대신한다(각 README).

## 등급 B 질문의 호스트 차이

change-intent skill은 질문을 먼저 `abbd decide`로 기록하고, 결과가 `ask: true`일 때만 사용자에게 묻는다. 호스트에 구조화된 질문 도구가 있으면 그것으로 보여 주고, 없으면 일반 텍스트로 묻는다. 어느 쪽이든 답은 `abbd decide --adr ADR-x --answer KEY`(또는 같은 질문에 `--answer KEY`)로 기록하고, 같은 턴 안에 답이 없으면 기본값으로 진행한다. 턴당 3개 한도와 답한 질문의 재사용은 하네스가 처리하므로 호스트 기능이 달라도 같다.

## 리뷰 화면의 호스트 차이

구조 변경 카드의 결정은 호스트와 상관없이 `abbd serve`의 리뷰 화면이 기본 경로다. 서버를 띄울 수 없는 호스트에서는 `generated/review.html` 파일을 열어 결정 파일을 내려받고 `abbd review --apply FILE`로 반영한다. MCP를 쓰는 호스트는 `list_review_cards`로 카드 요약과 화면 경로를 받고, 사용자가 호스트 화면(MCP elicitation 등)에서 고른 결정을 `decide_structural_change`로 기록할 수 있다. 어느 통로든 같은 결정 레코드가 남는다. 그림은 리뷰 화면에만 있으므로 채팅에는 링크와 대기 카드 수만 보인다.

## 계측의 호스트 차이

그래프 조회, 테스트 실행, 질문 수, 소요 시간은 하네스가 센다. 모델 호출 수만 하네스가 볼 수 없으므로 호스트나 에이전트가 `abbd turn note --model-calls N`으로 보고한다. 호스트가 그 수를 알려 주지 않으면 보고하지 않고, receipt의 `model_calls_note`가 보고되지 않았음을 남긴다.

## 서브에이전트가 없을 때

architect-change의 후보 비교, impact-review의 다중 관점 검토는 순차 한 번으로 줄어든다. 줄었다는 사실을 ChangeReceipt 보고에 적는다. 독립 리뷰가 필요한 변경에서 리뷰어를 돌릴 수 없으면 자기 검토로 대신하지 않고, 리뷰가 빠졌다고 보고한다.

