# abbd for Antigravity

Antigravity는 `SKILL.md` skill을 워크스페이스 `.agents/skills/`와 전역 `~/.gemini/config/skills/`에서 읽고, MCP 서버를 `.agents/mcp_config.json` 또는 `~/.gemini/config/mcp_config.json`에서 읽는다 ([skills](https://antigravity.google/docs/skills), [mcp](https://antigravity.google/docs/mcp)). 원본은 Claude 플러그인과 같은 `plugins/abbd/skills/`다.

## 설치

```sh
git clone https://github.com/whyjp/abbd && cd abbd
go install ./cmd/abbd
sh plugins/install.sh --host antigravity /path/to/workspace     # .agents/skills + .agents/mcp_config.json
sh plugins/install.sh --host antigravity --scope user           # ~/.gemini/config/skills + ~/.gemini/config/mcp_config.json
# Windows: powershell -ExecutionPolicy Bypass -File plugins/install.ps1 -HostName antigravity -Project C:\work\workspace
```

Antigravity CLI에는 `agy plugin install <github-url | ./path>`가 있다. 저장소 하위 폴더를 지정하는 방법은 문서에서 찾지 못했으므로, 클론한 뒤 로컬 경로로 설치하는 것만 안내한다. 이 경로는 아직 이 저장소에서 실행해 보지 않았다.

```sh
agy plugin install ./plugins/abbd
```

MCP 설정 (`mcp_config.json`이 이미 있으면 설치 스크립트가 이 블록을 출력한다):

```json
{
  "mcpServers": {
    "abbd": { "command": "abbd", "args": ["mcp"] }
  }
}
```

## 상시 규칙 (선택)

세션마다 턴 루프로 들어가게 하려면 `.agents/rules/abbd.md`를 둔다. Antigravity 규칙 파일은 `trigger` frontmatter가 필요하다 ([rules](https://antigravity.google/docs/rules)).

```markdown
---
trigger: always_on
---
이 워크스페이스는 abbd 턴 루프를 쓴다. 코드를 바꾸는 요청은 abbd-session-bootstrap skill로 시작하고, 첫 편집 전에 `abbd run "<사용자의 말 그대로>"`로 turn을 연다. 편집을 기록하는 훅이 없어서 turn 밖에서 고친 파일은 `reconcile`이 검증 없이 외부 변경으로 흡수한다.
컨텍스트는 `.abbd/BLUEPRINT.md`와 `run`이 주는 `read_first`의 md에서 가져온다. `.abbd/changes`, `requirements`, `decisions`, `verification`, `generated`와 state·candidates·learning·bootstrap-report 파일은 이력이므로 열지 않는다. `.abbd/.ignore`가 ripgrep 검색에서 빼지만 직접 읽기를 막는 훅은 없으니 이 규칙을 지킨다. 이력은 `abbd inspect ID`, `abbd decisions QUERY`, `abbd turn show`, `abbd review CHG-x`로 묻는다.
사용자가 프로젝트 전체 규칙을 말하면 `abbd intent rule --claim "<사용자의 말>"`로 기록한다(turn 없이도 된다). standing rule은 새 경로에도 적용된다.
필수 검증이 PASSED가 아니면 `abbd sync`를 하지 않는다. sync가 종료 코드 2를 내면 완료가 아니다.
등급 B 질문은 `abbd decide`가 `ask: true`를 줄 때만 묻고, 사용자의 답만 CLI에서 `--user`를 붙여 기록한다. MCP 도구는 에이전트 통로라 사용자 결정을 기록하지 못한다. 등급 C 행동은 명시적 승인 없이 하지 않는다.
sync 결과에 `review`가 있으면 `abbd serve`의 리뷰 화면 링크와 대기 카드 수만 알리고, 카드는 사용자가 결정한다.
```

## 호스트 차이

- Claude Code 플러그인의 훅(SessionStart, UserPromptSubmit, PreToolUse 이력 읽기 거부, PostToolUse 편집 기록, Stop)이 없다. 위 규칙 파일이 그 지시를 대신하지만 엔진이 확인하지는 못한다. 특히 에이전트의 편집이 기록되지 않으므로 turn을 먼저 여는 순서는 에이전트가 지킨다.
- MCP 서버(`abbd mcp`)는 에이전트가 부르는 통로다. `decide_structural_change`는 B·C 카드를, `answer_decision`은 기존 ADR을 결정하지 못한다. 사용자의 결정은 `abbd serve`의 리뷰 화면, 결정 파일(`abbd review --apply FILE`), 사용자의 말을 그대로 `--user`와 함께 적은 CLI로 기록한다.
