# Host adapters

하네스 코어는 CLI(`cccv`)와 같은 API를 내는 MCP 서버(`cccv mcp`)다. 어떤 호스트에서도 CLI와 검증 명령은 그대로 돈다. 어댑터가 조절하는 것은 다섯 가지뿐이다. skill 발견 경로, 서브에이전트, 사용자에게 묻기, 모델 선택, diff/도구 접근.

호스트에만 있는 기능을 공통 프로토콜의 필수 기능으로 만들지 않는다. skills/의 본문은 호스트 도구 이름을 쓰지 않고 CLI 명령만 쓴다.

## 기능 표

| | Claude Code | Codex | Cursor | OpenCode | 일반 CLI (호스트 없음) |
|---|---|---|---|---|---|
| 프로젝트 skill 경로 | `.claude/skills/<name>/SKILL.md` | `.agents/skills/<name>/SKILL.md` | `.cursor/skills/<name>/SKILL.md` | `.opencode/skills/<name>/SKILL.md` | 없음. 사람이나 스크립트가 SKILL.md를 읽고 명령을 실행한다 |
| 상시 지침 파일 | `CLAUDE.md` | `AGENTS.md` | `.cursor/rules/`, `AGENTS.md` | `AGENTS.md` | 없음 |
| MCP 등록 | 프로젝트 `.mcp.json` | `~/.codex/config.toml`의 `[mcp_servers.*]` | `.cursor/mcp.json` | `opencode.json`의 `mcp` | 해당 없음. CLI 직접 호출 |
| 서브에이전트 | 있음 (`Agent` 도구, `.claude/agents/`) | 있음 (`spawn_agent`, `multi_agent` 기능 필요) | 있음 | 있음 | 없음. 단계를 순차 실행 |
| 사용자에게 묻기 | `AskUserQuestion` (구조화 선택지) | 일반 텍스트 질문 | 일반 텍스트 질문 | 일반 텍스트 질문 | 터미널 프롬프트, 또는 `decide --answer`로 사후 기록 |
| 모델 선택 | 서브에이전트별 `model` 지정 | 설정 파일과 `spawn_agent` 옵션 | 호스트 모델 선택기 | 에이전트별 `model` 설정 | 해당 없음 |
| 이 저장소의 어댑터 | [`claude-code/`](claude-code/) | [`codex/`](codex/) | 아직 없음 | 아직 없음 | 필요 없음 |

Claude Code 행은 이 저장소에서 쓰는 설정을 기준으로 했다. Codex, Cursor, OpenCode의 경로와 설정 파일은 각 호스트 문서의 현재 판과 대조해야 한다. 호스트가 자주 바뀌므로 어댑터를 추가할 때 그 호스트의 문서를 다시 읽고 이 표를 고친다.

## 등급 B 질문의 호스트 차이

change-intent skill은 질문을 먼저 `cccv decide`로 기록하고, 결과가 `ask: true`일 때만 사용자에게 묻는다. 호스트에 구조화된 질문 도구가 있으면 그것으로 보여 주고, 없으면 일반 텍스트로 묻는다. 어느 쪽이든 답은 같은 질문에 `--answer KEY`를 붙인 decide로 기록하고, 같은 턴 안에 답이 없으면 기본값으로 진행한다. 턴당 3개 한도와 답한 질문의 재사용은 하네스가 처리하므로 호스트 기능이 달라도 같다.

## 계측의 호스트 차이

그래프 조회, 테스트 실행, 질문 수, 소요 시간은 하네스가 센다. 모델 호출 수만 하네스가 볼 수 없으므로 호스트나 에이전트가 `cccv turn note --model-calls N`으로 보고한다. 호스트가 그 수를 알려 주지 않으면 보고하지 않고, receipt의 `model_calls_note`가 보고되지 않았음을 남긴다.

## 서브에이전트가 없을 때

architect-change의 후보 비교, impact-review의 다중 관점 검토는 순차 한 번으로 줄어든다. 줄었다는 사실을 ChangeReceipt 보고에 적는다. 독립 리뷰가 필요한 변경에서 리뷰어를 돌릴 수 없으면 자기 검토로 대신하지 않고, 리뷰가 빠졌다고 보고한다.

## 설치

- Claude Code: `sh hosts/claude-code/install.sh <project>` 또는 `powershell -ExecutionPolicy Bypass -File hosts/claude-code/install.ps1 -Project <project>`. 기본 실행 정책이 서명 없는 스크립트를 막는 Windows에서는 `-ExecutionPolicy Bypass`가 필요하다. 두 스크립트 모두 하네스 skill 디렉터리만 교체하고 다른 skill은 건드리지 않는다. `.mcp.json`은 쓰지 않고 붙여 넣을 스니펫만 출력한다.
- Codex: [`codex/README.md`](codex/README.md).
- 그 밖의 호스트: `skills/` 아래 여덟 개 디렉터리를 그 호스트의 skill 경로로 복사하고, MCP 서버 명령을 `cccv mcp`로 등록한다.
