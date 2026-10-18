# Third-party notices

## 복사한 파일과 번역

이 저장소는 pstack, Graphify, GitNexus, CodeGraphContext의 파일, 코드, 원문 문장을 복사하지 않았다. 아래 프로젝트에서는 아이디어와 작동 원리만 가져왔다. 앞으로 실제 파일이나 표현을 재사용하면 이 파일에 출처 링크, 재사용 범위, 원본 저작권 표시와 라이선스 전문을 추가한다.

pstack에 대해서는 정정이 있다. 이전 판의 skill 일부는 pstack 문단을 한국어로 거의 그대로 옮긴 번역이었다. 해당 부분은 impact-review의 원칙·"한 가지 사실" 5단계·검색이 놓치는 경로 목록·출력 절(blast-radius에서), reflect-and-correct의 4단계 교정 순서·예외 규칙·규칙 표(correct에서), architect-change의 후보 비교 기준 일부와 구현 중 이탈 규칙(architect에서), implement-and-prove와 change-intent의 몇 문장(principle-test-behavior-not-implementation, principle-attack-the-premise, principle-boundary-discipline, poteto-mode의 질문 분류에서)이었고, 분량은 약 60~70줄이었다. 이번 판에서 이 부분을 지우고 ABBD 엔진이 실제로 계산하는 것(impact 보고서 필드, 계약 diff, 구조 변경 카드와 등급, `learn --kind` 순서, verify의 실패 출력과 Evidence)에서 다시 썼다. 문단 단위 번역은 남아 있지 않고, 문장 단위 번안이 16곳(약 18~20줄) 남아 있다. 모두 아래 "가져온 아이디어"의 범위 안에 있다. 생각 자체가 같은 곳은 아래 "가져온 아이디어"에 적었다.

라이선스 표기는 PRD v1.3(02장, 02A장, 18장)이 확인한 내용을 따른다. 이 문서는 법률 자문이 아니다. 배포 방식을 정할 때 각 원본의 라이선스를 다시 확인한다.

## 참고한 프로젝트

### pstack

- 라이선스: MIT License, Copyright (c) 2026 Lauren Tan.
- 가져온 아이디어(문단은 가져오지 않음. 문장 단위 번안은 위 정정에 적었다). 어느 skill이 그 생각을 쓰는지 함께 적는다.
  - poteto-mode의 작업 유형별 경로 선택과, 실행해서 알 수 있는 사실은 묻지 않는다는 원칙: session-bootstrap, change-intent.
  - how/why의 "무엇을 하나"와 "왜 이런 모양인가" 분리, 주장마다 근거를 밝히고 찾아본 곳을 적는 습관: understand-project. 근거 수준 다섯 가지(실행 확인, 코드 확인, 기록 확인, 추정, 모름)는 ABBD의 origin 축에 맞춰 새로 정했다.
  - architect의 타입·경계 먼저, 호출하는 쪽 사용 예에서 타입 끌어내기, 구조가 다른 후보 둘 이상, 구현 중 계획 이탈을 신호로 다루기: architect-change. 비교 기준, 구조 냄새 목록, ChangePlan 템플릿은 ABBD의 구조 변경 카드와 의존 정책에서 새로 만들었다.
  - blast-radius의 "diff 밖에서 깨지는 것"을 찾고 안전 주장을 실행으로 뒷받침하기: impact-review. 단계와 목록은 엔진 출력(contracts impact, impact의 unknowns, 카드 종류)과 계약 종류별 표로 대체했다.
  - correct의 "같은 실수는 구조로 막고, 문서는 마지막"과 새 검사가 과거 실수에 실패하는지 확인하기: reflect-and-correct. 교정 순서는 ABBD의 `learn --kind` 값(PRD 02장이 정한 순서)을 따른다.
  - reflect의 교훈 환류, principle-prove-it-works의 실패 내용 확인, principle-sequence-verifiable-units의 작은 검증 단위, poteto-mode bug-fix playbook의 실패 테스트 먼저와 단위 테스트 통과가 버그 부재가 아니라는 구분: reflect-and-correct, implement-and-prove.
- 링크:
  - R01 README: https://github.com/cursor/plugins/blob/main/pstack/README.md
  - R02 poteto-mode: https://github.com/cursor/plugins/blob/main/pstack/skills/poteto-mode/SKILL.md
  - R03 Create Verification Skill: https://github.com/cursor/plugins/blob/main/pstack/skills/create-verification-skill/SKILL.md
  - R04 Maintain Verification Skill: https://github.com/cursor/plugins/blob/main/pstack/skills/maintain-verification-skill/SKILL.md
  - R05 Verify and Ship Guide: https://github.com/cursor/plugins/blob/main/pstack/docs/guide/06-verify-and-ship.md
  - R06 How: https://github.com/cursor/plugins/blob/main/pstack/skills/how/SKILL.md
  - R07 Why: https://github.com/cursor/plugins/blob/main/pstack/skills/why/SKILL.md
  - R08 Architect: https://github.com/cursor/plugins/blob/main/pstack/skills/architect/SKILL.md
  - R09 Blast Radius: https://github.com/cursor/plugins/blob/main/pstack/skills/blast-radius/SKILL.md
  - R10 Correct: https://github.com/cursor/plugins/blob/main/pstack/skills/correct/SKILL.md
  - R11 Reflect: https://github.com/cursor/plugins/blob/main/pstack/skills/reflect/SKILL.md
  - R12 Evaluation playbook: https://github.com/cursor/plugins/blob/main/pstack/skills/poteto-mode/playbooks/eval.md
  - R13 Poteto Help: https://github.com/cursor/plugins/blob/main/pstack/skills/poteto-help/SKILL.md
  - R14 LICENSE: https://github.com/cursor/plugins/blob/main/pstack/LICENSE

### pstack 포트

호스트 어댑터 범위를 정할 때 Cursor 전용 기능을 다른 호스트로 옮긴 사례로 참고했다.

- R38 pstack-claude (Claude Code / Codex / OpenCode 포트): https://github.com/michael-denyer/pstack-claude
- R39 t3-pstack (T3 Code 포트): https://github.com/chhoumann/t3-pstack

### Graphify

- 라이선스: 공개 저장소에 Apache-2.0 `LICENSE`와 MIT `LICENSE-MIT`가 함께 있다. 저장소 전체를 MIT로 간주하지 않는다. 파일을 차용하게 되면 해당 파일의 적용 조건과 NOTICE를 확인한다.
- 참고한 아이디어: 코드·문서·설정을 잇는 단일 그래프, 관계별 EXTRACTED/INFERRED 근거 구별, HTML Explorer와 MCP 산출, 초기 스캔과 증분 갱신 분리.
- 링크:
  - R30 저장소/README: https://github.com/Graphify-Labs/graphify
  - R31 Apache-2.0 LICENSE: https://github.com/Graphify-Labs/graphify/blob/v8/LICENSE
  - R32 LICENSE-MIT: https://github.com/Graphify-Labs/graphify/blob/v8/LICENSE-MIT

### GitNexus

- 라이선스: PolyForm Noncommercial 1.0.0. 코드를 빌려오지 않았다.
- 참고한 아이디어: 심볼 context, 호출 impact, trace, Git diff 기반 변경 감지, bounded 조회와 작업 실행 루프의 연결.
- 링크:
  - R33 저장소/README: https://github.com/abhigyanpatwari/GitNexus
  - R34 LICENSE: https://github.com/abhigyanpatwari/GitNexus/blob/main/LICENSE

### CodeGraphContext

- 라이선스: MIT.
- 참고한 아이디어: 언어별 분석기 분리, SCIP 기반 참조 탐색, 저장 엔진 교체 가능성, CLI와 MCP의 동일한 의미.
- 링크:
  - R35 저장소/README: https://github.com/CodeGraphContext/CodeGraphContext
  - R36 LICENSE: https://github.com/CodeGraphContext/CodeGraphContext/blob/main/LICENSE

## Go 의존성

`go.mod`의 유일한 외부 의존성은 다음 하나다.

- gopkg.in/yaml.v3 v3.0.1: MIT License와 Apache License 2.0. https://github.com/go-yaml/yaml/tree/v3.0.1

`go.sum`에 있는 gopkg.in/check.v1 줄은 yaml.v3의 테스트 의존성 `go.mod` 해시다. 빌드되는 바이너리에는 들어가지 않는다.
