# abbd for Codex

Codex 문서에는 GitHub 하위 폴더에서 플러그인을 설치하는 방법이 없다 ([build-skills](https://learn.chatgpt.com/docs/build-skills)). 그래서 문서화된 skill 경로(`.agents/skills`, 사용자 범위는 `~/.agents/skills`)에 원본 skill을 복사하는 설치 스크립트를 쓴다. 원본은 Claude 플러그인과 같은 `plugins/abbd/skills/`다.

## 설치

```sh
git clone https://github.com/whyjp/abbd && cd abbd
go install ./cmd/abbd                                     # 또는 go install github.com/whyjp/abbd/cmd/abbd@latest
sh plugins/install.sh --host codex /path/to/project       # 프로젝트 범위: .agents/skills + .codex/config.toml
sh plugins/install.sh --host codex --scope user           # 사용자 범위: ~/.agents/skills + ~/.codex/config.toml
# Windows: powershell -ExecutionPolicy Bypass -File plugins/install.ps1 -HostName codex -Project C:\work\project
```

설치 스크립트는 abbd skill 디렉터리만 바꾼다. `config.toml`이 없을 때만 새로 쓰고, 이미 있으면 아래 블록을 출력해 직접 합치게 한다. MCP를 수동으로 붙이려면 `codex mcp add abbd -- abbd mcp`도 된다 ([MCP](https://learn.chatgpt.com/docs/extend/mcp.md?surface=cli)).

```toml
[mcp_servers.abbd]
command = "abbd"
args = ["mcp"]
```

## AGENTS.md 스니펫

Codex에는 세션 시작 훅으로 skill 진입을 강제하는 장치가 없다. 프로젝트 `AGENTS.md`에 다음을 넣는다.

```markdown
## abbd

이 저장소는 abbd 턴 루프를 쓴다.

- 모든 요청은 abbd-session-bootstrap skill로 시작한다. 먼저 `.abbd/BLUEPRINT.md`를 읽고, 첫 명령은 `abbd status --json`이다.
- 컨텍스트는 BLUEPRINT.md와 `abbd run`이 주는 `read_first`의 md에서 가져온다. `.abbd/changes`, `requirements`, `decisions`, `verification`, `generated`와 state·candidates·learning·bootstrap-report 파일은 이력이므로 열지 않는다. `.abbd/.ignore`가 ripgrep 검색에서 빼지만 직접 읽기를 막는 훅은 없으니 이 지시를 지킨다. 이력이 필요하면 `abbd inspect ID`, `abbd decisions QUERY`, `abbd turn show`, `abbd review CHG-x`를 쓴다.
- 사용자가 프로젝트 전체 규칙을 말하면 `abbd intent rule --claim "<사용자의 말>"`로 기록한다(turn 없이도 된다). `run`의 `standing_rules`는 새 경로에도 적용되므로 새 경로마다 대조하고 테스트를 넣는다.
- 코드를 바꾸는 요청은 첫 편집 전에 `abbd run "<사용자의 말 그대로>"`로 turn을 열고, abbd 스킬 순서(session-bootstrap → … → reflect-and-correct)를 따른다. Codex에는 편집을 기록하는 훅이 없어서, turn 밖에서 고친 파일은 `reconcile`이 검증 없이 외부 변경으로 흡수한다. 사용자 메시지를 남기는 훅도 없으므로 `run`에 사용자의 말을 넘긴다.
- 필수 검증이 PASSED가 아니면 `abbd sync`를 하지 않는다. sync가 종료 코드 2를 내면 완료가 아니다. maps/graph/HTML이 CURRENT가 아니면 완료라고 보고하지 않는다.
- 등급 B 질문은 먼저 `abbd decide`로 기록하고, 결과가 `ask: true`일 때만 일반 텍스트로 묻는다. 사용자의 답은 CLI에서 `--user`를 붙여 기록하고, 에이전트 자신의 선택에는 붙이지 않는다. MCP 도구는 에이전트 통로라 사용자 결정을 기록하지 못한다. 턴당 3개 한도와 이미 답한 질문의 재사용은 하네스가 처리한다. bootstrap 중에는 묻지 않는다.
- turn이 끝나기 전에 이번 turn의 모델 호출 수를 `abbd turn note --model-calls N`으로 보고한다.
- 등급 C 행동(공유 브랜치 push, 배포, 외부 리소스 삭제, 비밀정보 변경)은 명시적 승인 없이 하지 않는다. `abbd verify`, `abbd check`, `abbd decide --tier C`의 종료 코드 2는 오류가 아니라 정지다. 결과를 읽는다.
- sync 결과에 `review`가 있으면 구조를 채팅에 풀어 쓰지 않고 `abbd serve`의 리뷰 화면 링크와 대기 카드 수만 알린다. 카드는 사용자가 결정한다.
```

## 호스트 차이

- 서브에이전트는 `spawn_agent`이고 `~/.codex/config.toml`의 `[features] multi_agent = true`가 필요하다. 없으면 후보 비교와 다중 관점 검토가 순차 한 번으로 줄어든다.
- 구조화된 선택지 질문 도구가 없다. change-intent의 질문은 텍스트로 묻는다.
- Claude Code 플러그인의 훅(SessionStart, UserPromptSubmit, PreToolUse 이력 읽기 거부, PostToolUse 편집 기록, Stop)이 없다. 위 AGENTS.md 스니펫이 그 지시를 대신하지만 엔진이 확인하지는 못한다. 특히 에이전트의 편집이 기록되지 않으므로 turn을 먼저 여는 순서는 에이전트가 지킨다.
- MCP 서버(`abbd mcp`)는 에이전트가 부르는 통로다. `decide_structural_change`는 B·C 카드를, `answer_decision`은 기존 ADR을 결정하지 못한다. 사용자의 결정은 `abbd serve`의 리뷰 화면, 결정 파일(`abbd review --apply FILE`), 사용자의 말을 그대로 `--user`와 함께 적은 CLI로 기록한다.
