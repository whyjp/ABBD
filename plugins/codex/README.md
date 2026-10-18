# cccv for Codex

Codex 문서에는 GitHub 하위 폴더에서 플러그인을 설치하는 방법이 없다 ([build-skills](https://learn.chatgpt.com/docs/build-skills)). 그래서 문서화된 skill 경로(`.agents/skills`, 사용자 범위는 `~/.agents/skills`)에 원본 skill을 복사하는 설치 스크립트를 쓴다. 원본은 Claude 플러그인과 같은 `plugins/cccv/skills/`다.

## 설치

```sh
git clone https://github.com/whyjp/cccv && cd cccv
go install ./cmd/cccv                                     # 또는 go install github.com/whyjp/cccv/cmd/cccv@latest
sh plugins/install.sh --host codex /path/to/project       # 프로젝트 범위: .agents/skills + .codex/config.toml
sh plugins/install.sh --host codex --scope user           # 사용자 범위: ~/.agents/skills + ~/.codex/config.toml
# Windows: powershell -ExecutionPolicy Bypass -File plugins/install.ps1 -HostName codex -Project C:\work\project
```

설치 스크립트는 cccv skill 디렉터리만 바꾼다. `config.toml`이 없을 때만 새로 쓰고, 이미 있으면 아래 블록을 출력해 직접 합치게 한다. MCP를 수동으로 붙이려면 `codex mcp add cccv -- cccv mcp`도 된다 ([MCP](https://learn.chatgpt.com/docs/extend/mcp.md?surface=cli)).

```toml
[mcp_servers.cccv]
command = "cccv"
args = ["mcp"]
```

## AGENTS.md 스니펫

Codex에는 세션 시작 훅으로 skill 진입을 강제하는 장치가 없다. 프로젝트 `AGENTS.md`에 다음을 넣는다.

```markdown
## cccv

이 저장소는 cccv 턴 루프를 쓴다.

- 모든 요청은 session-bootstrap skill로 시작한다. 첫 명령은 `cccv status --json`이다.
- 코드를 바꾸는 요청은 `cccv run "REQUEST"`로 turn을 열고, cccv 스킬 순서(session-bootstrap → … → reflect-and-correct)를 따른다.
- 필수 검증이 PASSED가 아니면 `cccv sync`를 하지 않는다. sync가 종료 코드 2를 내면 완료가 아니다. maps/graph/HTML이 CURRENT가 아니면 완료라고 보고하지 않는다.
- 등급 B 질문은 먼저 `cccv decide`로 기록하고, 결과가 `ask: true`일 때만 일반 텍스트로 묻는다. 턴당 3개 한도와 이미 답한 질문의 재사용은 하네스가 처리한다. bootstrap 중에는 묻지 않는다.
- turn이 끝나기 전에 이번 turn의 모델 호출 수를 `cccv turn note --model-calls N`으로 보고한다.
- 등급 C 행동(공유 브랜치 push, 배포, 외부 리소스 삭제, 비밀정보 변경)은 명시적 승인 없이 하지 않는다.
```

## 호스트 차이

- 서브에이전트는 `spawn_agent`이고 `~/.codex/config.toml`의 `[features] multi_agent = true`가 필요하다. 없으면 후보 비교와 다중 관점 검토가 순차 한 번으로 줄어든다.
- 구조화된 선택지 질문 도구가 없다. change-intent의 질문은 텍스트로 묻는다.
