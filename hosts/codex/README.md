# Codex adapter

## skill 설치

Codex는 저장소의 `.agents/skills/<name>/SKILL.md`에서 프로젝트 skill을 찾는다. 설치 전에 현재 Codex 문서에서 경로를 확인한다. `skills/` 아래 여덟 개 디렉터리를 복사한다. 다시 실행하면 같은 디렉터리만 덮어쓴다.

```sh
mkdir -p <project>/.agents/skills
for d in skills/*/; do
  [ -f "$d/SKILL.md" ] || continue
  rm -rf "<project>/.agents/skills/$(basename "$d")"
  cp -R "${d%/}" "<project>/.agents/skills/"
done
```

## MCP 서버

`~/.codex/config.toml`에 추가한다.

```toml
[mcp_servers.myharness]
command = "myharness"
args = ["mcp"]
```

## AGENTS.md 스니펫

Codex에는 세션 시작 훅으로 skill 진입을 강제하는 장치가 기본으로 없다. 프로젝트 `AGENTS.md`에 다음을 넣어 매 요청이 session-bootstrap으로 시작하게 한다.

```markdown
## For My Harness

이 저장소는 For My Harness 턴 루프를 쓴다.

- 모든 요청은 session-bootstrap skill로 시작한다. 첫 명령은 `myharness status --json`이다.
- 코드를 바꾸는 요청은 `myharness run "REQUEST"`로 turn을 열고, skills/README.md의 순서를 따른다.
- 필수 검증이 PASSED가 아니면 `myharness sync`를 하지 않는다. sync가 종료 코드 2를 내면 완료가 아니다. maps/graph/HTML이 CURRENT가 아니면 완료라고 보고하지 않는다.
- 등급 B 질문은 먼저 `myharness decide`로 기록하고, 결과가 `ask: true`일 때만 일반 텍스트로 묻는다. 턴당 3개 한도와 이미 답한 질문의 재사용은 하네스가 처리한다. bootstrap 중에는 묻지 않는다.
- turn이 끝나기 전에 이번 turn의 모델 호출 수를 `myharness turn note --model-calls N`으로 보고한다.
- 등급 C 행동(공유 브랜치 push, 배포, 외부 리소스 삭제, 비밀정보 변경)은 명시적 승인 없이 하지 않는다.
```

## 호스트 차이

- 서브에이전트는 `spawn_agent`이고 `~/.codex/config.toml`의 `[features] multi_agent = true`가 필요하다. 없으면 후보 비교와 다중 관점 검토가 순차 한 번으로 줄어든다.
- 구조화된 선택지 질문 도구가 없다. change-intent의 질문은 텍스트로 묻는다.
