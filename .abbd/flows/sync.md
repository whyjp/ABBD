---
question: What does abbd sync do, in order?
writer: reader
origin: INFERRED
format: 1
steps:
    - unit: MOD-0041
      does: LoadProject() → the registered extraction argv
      fails: stops when project.yaml names no script; run abbd setup first
      calls:
        - CFG-0002
      evidence:
        - path: cmd/abbd/main.go
          lines: "298"
          quote: p, err := e.st.LoadProject()
        - path: cmd/abbd/main.go
          lines: "302"
          quote: if len(p.Extract) == 0 {
      code:
        cmd/abbd/main.go#quote-db012aa7: b732db54bf14e694
        cmd/abbd/main.go#quote-f3e597fc: fc7bb3d5697db4c4
    - unit: MOD-0004
      does: runScript(argv) → the script's stdout, run in the project root
      fails: stops when the script exits non-zero, and prints its stderr
      calls:
        - DATA-0002
      evidence:
        - path: cmd/abbd/main.go
          lines: "305"
          quote: out, err := e.runScript(p.Extract)
        - path: cmd/abbd/main.go
          lines: "430"
          quote: cmd := exec.Command(argv[0], argv[1:]...)
      code:
        cmd/abbd/main.go#quote-414b351c: 1890665b94e73ff6
        cmd/abbd/main.go#quote-c1c55757: 417fc75a7dfd695c
    - unit: MOD-0017
      does: Parse(stdout) → extract.Result with units, anchors and relations
      calls:
        - DATA-0002
      evidence:
        - path: cmd/abbd/main.go
          lines: "309"
          quote: ex, err := extract.Parse(bytes.NewReader(out))
      code:
        cmd/abbd/main.go#quote-2a172499: f449d858035def28
    - unit: MOD-0041
      does: Writable(), LoadCandidates(), LoadState() → stored units, candidates, last state
      calls:
        - DATA-0006
      evidence:
        - path: cmd/abbd/main.go
          lines: "367"
          quote: stored, err := st.Writable()
        - path: cmd/abbd/main.go
          lines: "371"
          quote: prior, err := st.LoadCandidates()
        - path: cmd/abbd/main.go
          lines: "375"
          quote: state, err := st.LoadState()
      code:
        cmd/abbd/main.go#quote-3ccd4c44: 62a3553d47a88124
        cmd/abbd/main.go#quote-045deabc: 277a8a331c1c57f6
        cmd/abbd/main.go#quote-dc6ae1c1: 3b6467a9e34767f2
    - unit: MOD-0022
      does: Snapshot(root, .abbd) → the working tree's fingerprint
      note: .abbd/ is left out, so committing the state leaves the tree unchanged
      evidence:
        - path: cmd/abbd/main.go
          lines: "379"
          quote: tree, err := gitx.Snapshot(e.root, store.Dir)
      code:
        cmd/abbd/main.go#quote-e3be779d: 1b1b4070e5cecefa
    - unit: MOD-0022
      does: Renames(last fingerprint, tree) → files moved since the last sync
      fails: warns and finds no moves when the last snapshot is gone
      evidence:
        - path: cmd/abbd/main.go
          lines: "383"
          quote: renames, ok, err := gitx.Renames(e.root, state.Fingerprint, tree)
        - path: cmd/abbd/main.go
          lines: "388"
          quote: last sync snapshot %s is gone
      code:
        cmd/abbd/main.go#quote-1e0ec249: 1ec3453f70fd9c06
        cmd/abbd/main.go#quote-09640d26: 029b0529c0a085e3
    - unit: MOD-0022
      does: Head() → the revision, taken only when the tree changed
      note: a sync of an unchanged tree keeps the stored revision
      evidence:
        - path: cmd/abbd/main.go
          lines: "398"
          quote: if state.Fingerprint != tree || rev == "" {
        - path: cmd/abbd/main.go
          lines: "399"
          quote: rev = gitx.Head(e.root)
      code:
        cmd/abbd/main.go#quote-6baa56c0: 2fbf51b0c5fa2aae
        cmd/abbd/main.go#quote-b765a468: ac1b670f125cd245
    - unit: MOD-0026
      does: Reconcile(stored, renames, extraction, prior) → units with their IDs kept
      fails: refuses an extraction with no units while units are stored
      calls:
        - DATA-0003
      evidence:
        - path: cmd/abbd/main.go
          lines: "392"
          quote: the extraction reported no units; refusing to mark every unit absent
        - path: cmd/abbd/main.go
          lines: "401"
          quote: res := identity.Reconcile(stored, renames, ex, prior, state.HighWater, ws)
      code:
        cmd/abbd/main.go#quote-9f1933c8: 3fce21bb2d49dd49
        cmd/abbd/main.go#quote-5664e499: 9d734f0de284ca33
    - unit: MOD-0013
      does: Apply(units, rev) → every quote live, drifted or dead
      note: a drifted quote dies at the first sync of a later revision
      evidence:
        - path: cmd/abbd/main.go
          lines: "402"
          quote: res.Units = evidence.New(e.root).Apply(res.Units, rev)
      code:
        cmd/abbd/main.go#quote-46d5e4b3: 078d5df8a2df1ea6
    - unit: MOD-0054
      does: Fallback(units) → a crowded level placed in the directories its code lives in
      evidence:
        - path: cmd/abbd/main.go
          lines: "403"
          quote: res.Units, res.HighWater = group.Fallback(res.Units, res.HighWater)
      code:
        cmd/abbd/main.go#quote-752e5b50: 34b797b724d0cb4a
    - unit: MOD-0041
      does: SaveUnits(), SaveCandidates() → the unit files and candidates.yaml
      calls:
        - DATA-0004
      evidence:
        - path: cmd/abbd/main.go
          lines: "404"
          quote: if err := st.SaveUnits(res.Units); err != nil {
        - path: cmd/abbd/main.go
          lines: "407"
          quote: if err := st.SaveCandidates(res.Candidates); err != nil {
      code:
        cmd/abbd/main.go#quote-bd410926: 960bdd53722ae760
        cmd/abbd/main.go#quote-e28fdf28: b662511d12a95a22
    - unit: MOD-0041
      does: UpgradeUnderstandings(), MDHash(), SaveState() → state.yaml
      calls:
        - DATA-0006
      evidence:
        - path: cmd/abbd/main.go
          lines: "410"
          quote: current, widened, err := st.UpgradeUnderstandings(res.Units)
        - path: cmd/abbd/main.go
          lines: "425"
          quote: return res, st.SaveState(state)
      code:
        cmd/abbd/main.go#quote-714f2ba0: 59f3f48639d38179
        cmd/abbd/main.go#quote-22175f6c: b135ec77fedd7778
    - unit: MOD-0007
      does: publish() → index.html and graph.json, after every command that writes
      note: graph.json carries the watermark and is written last
      calls:
        - DATA-0005
      evidence:
        - path: cmd/abbd/main.go
          lines: "272"
          quote: if err := e.publish(); err != nil {
        - path: cmd/abbd/screen.go
          lines: "27"
          quote: files, err := screen.Files(screen.Build(in))
      code:
        cmd/abbd/main.go#quote-e69b8656: 16fa2f8eb12c79c6
        cmd/abbd/screen.go#quote-f313962c: b96f7157ff80607b
---

What one `abbd sync` does, from the registered script to the rendered blueprint. Everything after the extraction runs inside `syncActual`, and publishing runs in `run` after any command that writes.
