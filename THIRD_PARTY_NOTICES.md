# Third-party notices

## 복사한 파일

이 저장소는 pstack, Graphify, GitNexus, CodeGraphContext의 파일이나 코드, 문장을 복사하지 않았다. 아래 프로젝트는 아이디어와 작동 원리만 참고했고, 구현과 문서는 독립적으로 작성했다. 앞으로 실제 파일이나 표현을 재사용하면 이 파일에 출처 링크, 재사용 범위, 원본 저작권 표시와 라이선스 전문을 추가한다.

라이선스 표기는 PRD v1.3(02장, 02A장, 18장)이 확인한 내용을 따른다. 이 문서는 법률 자문이 아니다. 배포 방식을 정할 때 각 원본의 라이선스를 다시 확인한다.

## 참고한 프로젝트

### pstack

- 라이선스: MIT License, Copyright (c) 2026 Lauren Tan.
- 참고한 아이디어: poteto-mode의 작업 라우팅과 playbook 선택, how/why 조사, architect의 타입·경계 우선 설계, blast-radius의 증거 중심 영향 분석, correct의 아키텍처 → 타입 → 린트 → 테스트 → 문서 순 교정, reflect의 교훈 환류, 검증 가능한 작은 단위와 실제 실행 증명. `skills/`의 여덟 개 skill이 이 아이디어를 독립적으로 구현한다.
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
