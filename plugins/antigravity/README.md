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
이 워크스페이스는 abbd 턴 루프를 쓴다. 코드를 바꾸는 요청은 abbd-session-bootstrap skill로 시작하고 `abbd run "REQUEST"`로 turn을 연다.
필수 검증이 PASSED가 아니면 `abbd sync`를 하지 않는다. sync가 종료 코드 2를 내면 완료가 아니다.
등급 B 질문은 `abbd decide`가 `ask: true`를 줄 때만 묻는다. 등급 C 행동은 명시적 승인 없이 하지 않는다.
sync 결과에 `review`가 있으면 `abbd serve`의 리뷰 화면 링크와 대기 카드 수만 알리고, 카드는 사용자가 결정한다.
```
