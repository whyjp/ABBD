---
format: 1
zones:
    - name: abbd 스킬
      units:
        - FEAT-0001
        - FEAT-0002
      evidence:
        - path: plugins/abbd/skills/abbd-setup/SKILL.md
          lines: "8"
          quote: Setup reads code with the abbd skill's reader
      code:
        plugins/abbd/skills/abbd-setup/SKILL.md#quote-e504c6fb: 6263b86408a1a5a3
    - name: 읽는 쪽
      units:
        - MOD-0017
        - MOD-0013
        - MOD-0022
        - MOD-0056
        - MOD-0057
      evidence:
        - path: internal/gitx/gitx.go
          lines: "1"
          quote: // Package gitx reads the few git facts ABBD needs.
        - path: internal/owners/owners.go
          lines: "1"
          quote: // Package owners reads who owns which code from a repository's CODEOWNERS.
      code:
        internal/gitx/gitx.go#quote-57ce0d67: 2fb4c125587d4786
        internal/owners/owners.go#quote-c278b53e: 49816dc251f8d21d
    - name: 토폴로지 규칙
      units:
        - MOD-0026
        - MOD-0009
        - MOD-0054
        - MOD-0047
      evidence:
        - path: internal/topology/impact.go
          lines: "1"
          quote: '// Package topology answers questions about the stored topology: one unit at a'
      code:
        internal/topology/impact.go#quote-aae46dc8: b632aab8d7ef2476
lines:
    - from: FEAT-0001
      to: MOD-0001
      word: abbd sync 실행
      evidence:
        - path: plugins/abbd/skills/abbd/SKILL.md
          lines: "142"
          quote: 1. Run `abbd sync` so the state and the blueprint match the changed code
      code:
        plugins/abbd/skills/abbd/SKILL.md#quote-850d8860: e7084a3719d36eec
    - from: FEAT-0002
      to: MOD-0001
      word: abbd setup 실행
      evidence:
        - path: plugins/abbd/skills/abbd-setup/SKILL.md
          lines: "49"
          quote: abbd setup --supplement .abbd/extractor/supplement.json
      code:
        plugins/abbd/skills/abbd-setup/SKILL.md#quote-a36c3d8c: 9a637622fc262c71
    - from: FEAT-0002
      to: FEAT-0001
      word: abbd 스킬의 리더
      evidence:
        - path: plugins/abbd/skills/abbd-setup/SKILL.md
          lines: "8"
          quote: Setup reads code with the abbd skill's reader
      code:
        plugins/abbd/skills/abbd-setup/SKILL.md#quote-e504c6fb: 6263b86408a1a5a3
    - from: FEAT-0001
      to: MOD-0055
      word: pstack how 탐색
      evidence:
        - path: plugins/abbd/skills/abbd/references/reader.md
          lines: "7"
          quote: you may use pstack `how` to explore a module this reader calls complex
      code:
        plugins/abbd/skills/abbd/references/reader.md#quote-f7c345a8: 05bfdef1bc5e8da7
    - from: MOD-0001
      to: MOD-0017
      word: 스크립트 출력 Parse
      evidence:
        - path: cmd/abbd/main.go
          lines: "490"
          quote: ex, err := extract.Parse(bytes.NewReader(out))
      code:
        cmd/abbd/main.go#quote-2a172499: f449d858035def28
    - from: MOD-0001
      to: MOD-0009
      word: 보충 doc Overlay
      evidence:
        - path: cmd/abbd/setup.go
          lines: "47"
          quote: if ex, f, err = bootstrap.Overlay(ex, doc); err != nil {
      code:
        cmd/abbd/setup.go#quote-8e531963: 7bfbaeac1dd5db18
    - from: MOD-0009
      to: MOD-0017
      word: extract.Result 덧입힘
      evidence:
        - path: internal/bootstrap/overlay.go
          lines: "26"
          quote: func Overlay(base, doc extract.Result) (extract.Result, Findings, error) {
      code:
        internal/bootstrap/overlay.go#quote-4f820910: 1c667ed7ec2d6f98
    - from: MOD-0001
      to: MOD-0022
      word: 작업 트리 Snapshot
      evidence:
        - path: cmd/abbd/main.go
          lines: "568"
          quote: tree, err := gitx.Snapshot(e.root, store.Dir)
      code:
        cmd/abbd/main.go#quote-e3be779d: 1b1b4070e5cecefa
    - from: MOD-0001
      to: MOD-0026
      word: 저장본과 추출 Reconcile
      evidence:
        - path: cmd/abbd/main.go
          lines: "590"
          quote: res := identity.Reconcile(stored, renames, ex, prior, state.HighWater, ws)
      code:
        cmd/abbd/main.go#quote-9f1933c8: 91cb400ac9ac34b5
    - from: MOD-0001
      to: MOD-0013
      word: 단위 인용 Apply
      evidence:
        - path: cmd/abbd/main.go
          lines: "591"
          quote: res.Units = evidence.New(e.root).Apply(res.Units, rev)
      code:
        cmd/abbd/main.go#quote-46d5e4b3: f362c61d73a69f5d
    - from: MOD-0001
      to: MOD-0054
      word: 단위 Fallback 배치
      evidence:
        - path: cmd/abbd/main.go
          lines: "592"
          quote: res.Units, res.HighWater = group.Fallback(res.Units, res.HighWater)
      code:
        cmd/abbd/main.go#quote-752e5b50: abe058985fc558c4
    - from: MOD-0001
      to: MOD-0041
      word: SaveUnits 단위 파일
      evidence:
        - path: cmd/abbd/main.go
          lines: "594"
          quote: if err := st.SaveUnits(res.Units); err != nil {
      code:
        cmd/abbd/main.go#quote-e28fdf28: fc29a42387cb7d71
    - from: MOD-0001
      to: MOD-0034
      word: 청사진 Build
      evidence:
        - path: cmd/abbd/screen.go
          lines: "27"
          quote: files, err := screen.Files(screen.Build(in))
      code:
        cmd/abbd/screen.go#quote-f313962c: 4a93f263f38fbe1b
    - from: MOD-0034
      to: MOD-0041
      word: LoadUnits 단위 읽기
      evidence:
        - path: internal/screen/screen.go
          lines: "55"
          quote: if in.Units, err = st.LoadUnits(); err != nil {
      code:
        internal/screen/screen.go#quote-fdde0a18: 2a0731b0dc6b6aa7
    - from: MOD-0001
      to: MOD-0047
      word: topology.Explain 질의
      evidence:
        - path: cmd/abbd/query.go
          lines: "92"
          quote: ex := topology.Explain(units, u.ID)
      code:
        cmd/abbd/query.go#quote-4f2bd2ae: ddf90943a65b4738
    - from: MOD-0034
      to: MOD-0047
      word: EntryUses 항목 사용처
      evidence:
        - path: internal/screen/entries.go
          lines: "55"
          quote: uses := topology.EntryUses(in.Units, u.ID)
      code:
        internal/screen/entries.go#quote-522ddad9: 181440dd213d4621
    - from: MOD-0034
      to: MOD-0057
      word: owners.Read 소유자
      evidence:
        - path: internal/screen/screen.go
          lines: "78"
          quote: in.Owners = OwnersOf(owners.Read(st.ProjectDir()), in.Units)
      code:
        internal/screen/screen.go#quote-3cef4285: aa69823742221c4f
    - from: MOD-0034
      to: MOD-0056
      word: narrative.Lines 줄
      evidence:
        - path: internal/screen/words.go
          lines: "151"
          quote: for _, l := range narrative.Lines(text) {
      code:
        internal/screen/words.go#quote-98a69d6b: 546e6d6f3a87b30a
---

에이전트는 /abbd와 /abbd-setup 스킬로 abbd CLI를 부른다. sync는 추출을 읽고 ID를 맞추고 인용을 검사해 store에 쓴 뒤 screen이 청사진을 그린다. internal/model은 CLI와 대부분의 internal 패키지가 쓰는 어휘라 선을 그리지 않았다.
