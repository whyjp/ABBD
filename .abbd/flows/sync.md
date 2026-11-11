---
question: What does abbd sync do, in order?
format: 1
steps:
    - unit: MOD-0041
      does: LoadProject() reads project.yaml → extraction argv, name, purpose, overview_cut
      fails: stops when project.yaml names no script; run abbd setup first
      calls:
        - CFG-0002
      evidence:
        - path: internal/store/store.go
          lines: "565"
          quote: return p, readYAML(s.path("project.yaml"), &p)
        - path: cmd/abbd/main.go
          lines: "456"
          quote: p, err := e.st.LoadProject()
        - path: cmd/abbd/main.go
          lines: "460"
          quote: if len(p.Extract) == 0 {
      writer: reader
      code:
        cmd/abbd/main.go#quote-db012aa7: b732db54bf14e694
        cmd/abbd/main.go#quote-f3e597fc: fc7bb3d5697db4c4
        internal/store/store.go#quote-ac7ade60: 48cd87ae44eebe3c
    - unit: MOD-0004
      does: runScript(argv) → the script's stdout, run in the project root
      fails: stops when the script exits non-zero, and prints its stderr
      calls:
        - DATA-0002
      evidence:
        - path: cmd/abbd/main.go
          lines: "463"
          quote: out, err := e.runScript(p.Extract)
        - path: cmd/abbd/main.go
          lines: "589"
          symbol: runScript
          quote: cmd := exec.Command(argv[0], argv[1:]...)
      writer: reader
      code:
        cmd/abbd/main.go#quote-414b351c: 1890665b94e73ff6
        cmd/abbd/main.go#quote-c1c55757: 417fc75a7dfd695c
    - unit: MOD-0017
      does: Parse(stdout) → extract.Result, the units by ref with anchors and relations
      calls:
        - DATA-0002
      evidence:
        - path: internal/extract/extract.go
          lines: "110"
          quote: func Parse(r io.Reader) (Result, error) { return parse(r, false) }
        - path: cmd/abbd/main.go
          lines: "467"
          quote: ex, err := extract.Parse(bytes.NewReader(out))
      writer: reader
      code:
        cmd/abbd/main.go#quote-2a172499: f449d858035def28
        internal/extract/extract.go#quote-1772af40: c0ff7246a9ff04f7
    - unit: MOD-0041
      does: Writable(), LoadCandidates(), LoadState() → stored units, candidates, last state
      note: LoadState reads a state a merge left with both sides' lines as one, marked Merged
      fails: Writable refuses a state with a unit file in the old format; run abbd migrate
      calls:
        - DATA-0006
      evidence:
        - path: internal/store/store.go
          lines: "86"
          quote: return nil, ErrOldFormat
        - path: internal/store/store.go
          lines: "321"
          quote: func (s Store) LoadCandidates() ([]identity.Candidate, error) {
        - path: internal/store/store.go
          lines: "336"
          quote: func (s Store) LoadState() (State, error) {
        - path: cmd/abbd/main.go
          lines: "529"
          quote: stored, err := st.Writable()
      writer: reader
      code:
        cmd/abbd/main.go#quote-3ccd4c44: 62a3553d47a88124
        internal/store/store.go#quote-2f954852: ef700c613d1f871f
        internal/store/store.go#quote-6d304a0c: 2b2495bfd99ecd31
        internal/store/store.go#quote-a6d8aa94: abdb9585b345b697
    - unit: MOD-0022
      does: Snapshot(root, .abbd) → the working tree's fingerprint
      note: .abbd/ is left out, so committing the state leaves the tree unchanged
      evidence:
        - path: internal/gitx/gitx.go
          lines: "95"
          quote: func Snapshot(dir, exclude string) (string, error) {
        - path: cmd/abbd/main.go
          lines: "541"
          quote: tree, err := gitx.Snapshot(e.root, store.Dir)
      writer: reader
      code:
        cmd/abbd/main.go#quote-e3be779d: 1b1b4070e5cecefa
        internal/gitx/gitx.go#quote-f84a71b6: 1cfe55877cfb5787
    - unit: MOD-0022
      does: Renames(last fingerprint, tree) → files moved since the last sync
      fails: warns and finds no moves when the last snapshot is gone
      evidence:
        - path: internal/gitx/gitx.go
          lines: "304"
          quote: func Renames(dir, oldTree, newTree string)
        - path: cmd/abbd/main.go
          lines: "550"
          quote: last sync snapshot %s is gone
      writer: reader
      code:
        cmd/abbd/main.go#quote-09640d26: 029b0529c0a085e3
        internal/gitx/gitx.go#quote-fe8cda5b: 42469c96965b6ec8
    - unit: MOD-0022
      does: Head() → the revision, taken only when the tree changed
      note: a sync of an unchanged tree keeps the stored revision
      evidence:
        - path: internal/gitx/gitx.go
          lines: "31"
          quote: func Head(dir string) string {
        - path: cmd/abbd/main.go
          lines: "560"
          quote: if state.Fingerprint != tree || rev == "" {
      writer: reader
      code:
        cmd/abbd/main.go#quote-6baa56c0: 2fbf51b0c5fa2aae
        internal/gitx/gitx.go#quote-bf242a07: b58a57cef6252aa3
    - unit: MOD-0026
      does: Reconcile(stored, renames, extraction, prior) → units with their IDs kept
      fails: refuses an extraction with no units while units are stored
      calls:
        - DATA-0003
      evidence:
        - path: internal/identity/identity.go
          lines: "73"
          quote: func Reconcile(stored []model.Unit
        - path: cmd/abbd/main.go
          lines: "554"
          quote: the extraction reported no units; refusing to mark every unit absent
      writer: reader
      code:
        cmd/abbd/main.go#quote-5664e499: 9d734f0de284ca33
        internal/identity/identity.go#quote-215518d5: ffa9923d84229d05
    - unit: MOD-0013
      does: Apply(units, rev) → every quote live, drifted or dead
      note: a drifted quote dies at the first sync of a later revision, and only then does a crowded level fall back to directories
      evidence:
        - path: internal/evidence/evidence.go
          lines: "227"
          quote: func (c *Checker) Apply(units []model.Unit, rev string) []model.Unit {
        - path: cmd/abbd/main.go
          lines: "564"
          quote: res.Units = evidence.New(e.root).Apply(res.Units, rev)
      writer: reader
      code:
        cmd/abbd/main.go#quote-46d5e4b3: f362c61d73a69f5d
        internal/evidence/evidence.go#quote-b16aa3be: b7eeba037dd92cf2
    - unit: MOD-0054
      does: Fallback(units) → a crowded level placed in the directories its code lives in
      note: then each module sentence no stamp covers is stamped with its code, before the units are saved
      evidence:
        - path: internal/group/group.go
          lines: "36"
          quote: func Fallback(units []model.Unit, hw map[model.Kind]int)
        - path: cmd/abbd/main.go
          lines: "565"
          quote: res.Units, res.HighWater = group.Fallback(res.Units, res.HighWater)
      writer: reader
      code:
        cmd/abbd/main.go#quote-752e5b50: abe058985fc558c4
        internal/group/group.go#quote-f1fc9398: 7665af6fd8209a95
    - unit: MOD-0041
      does: SaveUnits(), SaveCandidates() → the unit files and candidates.yaml
      calls:
        - DATA-0004
      evidence:
        - path: internal/store/store.go
          lines: "301"
          quote: func (s Store) SaveUnits(units []model.Unit) error {
        - path: internal/store/store.go
          lines: "326"
          quote: func (s Store) SaveCandidates(cs []identity.Candidate) error {
      writer: reader
      code:
        internal/store/store.go#quote-bace279f: 7b2259bf39672043
        internal/store/store.go#quote-d57cbb81: 85dc2f778cc996e7
    - unit: MOD-0041
      does: UpgradeUnderstandings(), then SaveState() writes revision and fingerprint
      calls:
        - DATA-0006
      evidence:
        - path: internal/store/understanding.go
          lines: "541"
          quote: func (s Store) UpgradeUnderstandings(units []model.Unit)
        - path: internal/store/store.go
          lines: "411"
          quote: func (s Store) SaveState(st State) error {
      writer: reader
      code:
        internal/store/store.go#quote-7d54ae9a: b356d62f32e8f6d1
        internal/store/understanding.go#quote-4f816bc6: d2d35eb41237bf09
    - unit: MOD-0007
      does: publish() → every page and graph.json, after a command writes, or saves part of what it then refuses
      note: then records in state.yaml the inputs hash it rendered from and the hash of the files it wrote
      calls:
        - DATA-0005
      evidence:
        - path: cmd/abbd/main.go
          lines: "427"
          quote: if err := e.publish(); err != nil {
        - path: cmd/abbd/screen.go
          lines: "27"
          quote: files, err := screen.Files(screen.Build(in))
        - path: cmd/abbd/screen.go
          lines: "31"
          quote: if err := e.st.SaveBlueprint(files); err != nil {
        - path: cmd/abbd/screen.go
          lines: "34"
          quote: in.Synced.Blueprint, in.Synced.Rendered = in.Inputs, store.HashRender(files)
      writer: reader
      code:
        cmd/abbd/main.go#quote-e69b8656: f18ecffa6b9f3e6e
        cmd/abbd/screen.go#quote-18c7c622: 8d8ed08440cabc5e
        cmd/abbd/screen.go#quote-061a5173: 91f35d6e30a925d0
        cmd/abbd/screen.go#quote-f313962c: 4a93f263f38fbe1b
retired:
    - unit: MOD-0007
      does: publish() → every page and graph.json, after every command that writes
      note: then records in state.yaml the inputs hash it rendered from and the hash of the files it wrote
      evidence:
        - path: cmd/abbd/main.go
          lines: "276"
          quote: if err := e.publish(); err != nil {
        - path: cmd/abbd/screen.go
          lines: "27"
          quote: files, err := screen.Files(screen.Build(in))
        - path: cmd/abbd/screen.go
          lines: "31"
          quote: if err := e.st.SaveBlueprint(files); err != nil {
        - path: cmd/abbd/screen.go
          lines: "34"
          quote: in.Synced.Blueprint, in.Synced.Rendered = in.Inputs, store.HashRender(files)
      code:
        cmd/abbd/main.go#quote-e69b8656: 16fa2f8eb12c79c6
        cmd/abbd/screen.go#quote-18c7c622: 8d8ed08440cabc5e
        cmd/abbd/screen.go#quote-061a5173: 91f35d6e30a925d0
        cmd/abbd/screen.go#quote-f313962c: 4a93f263f38fbe1b
    - unit: MOD-0041
      does: UpgradeUnderstandings(), SaveState() → state.yaml with revision and fingerprint
      evidence:
        - path: internal/store/understanding.go
          lines: "524"
          quote: func (s Store) UpgradeUnderstandings(units []model.Unit)
        - path: internal/store/store.go
          lines: "383"
          quote: func (s Store) SaveState(st State) error {
      code:
        internal/store/store.go#quote-7d54ae9a: 81704cd8150204d2
        internal/store/understanding.go#quote-4f816bc6: d2d35eb41237bf09
    - unit: MOD-0013
      does: Apply(units, rev) → every quote live, drifted or dead
      note: a drifted quote dies at the first sync of a later revision
      evidence:
        - path: internal/evidence/evidence.go
          lines: "227"
          quote: func (c *Checker) Apply(units []model.Unit, rev string) []model.Unit {
        - path: cmd/abbd/main.go
          lines: "490"
          quote: res.Units = evidence.New(e.root).Apply(res.Units, rev)
      code:
        cmd/abbd/main.go#quote-46d5e4b3: 078d5df8a2df1ea6
        internal/evidence/evidence.go#quote-b16aa3be: b7eeba037dd92cf2
    - unit: MOD-0054
      does: Fallback(units) → a crowded level placed in the directories its code lives in
      evidence:
        - path: internal/group/group.go
          lines: "36"
          quote: func Fallback(units []model.Unit, hw map[model.Kind]int)
        - path: cmd/abbd/main.go
          lines: "491"
          quote: res.Units, res.HighWater = group.Fallback(res.Units, res.HighWater)
      code:
        cmd/abbd/main.go#quote-752e5b50: 34b797b724d0cb4a
        internal/group/group.go#quote-f1fc9398: 7665af6fd8209a95
    - unit: MOD-0041
      does: LoadProject() reads project.yaml → the registered extraction argv
      fails: stops when project.yaml names no script; run abbd setup first
      evidence:
        - path: internal/store/store.go
          lines: "523"
          quote: return p, readYAML(s.path("project.yaml"), &p)
        - path: cmd/abbd/main.go
          lines: "428"
          quote: p, err := e.st.LoadProject()
        - path: cmd/abbd/main.go
          lines: "432"
          quote: if len(p.Extract) == 0 {
      code:
        cmd/abbd/main.go#quote-db012aa7: b732db54bf14e694
        cmd/abbd/main.go#quote-f3e597fc: fc7bb3d5697db4c4
        internal/store/store.go#quote-ac7ade60: 5d4ca50e7b137b28
    - unit: MOD-0041
      does: LoadProject() reads project.yaml → the extraction argv, and the project name and purpose
      fails: stops when project.yaml names no script; run abbd setup first
      evidence:
        - path: internal/store/store.go
          lines: "562"
          quote: return p, readYAML(s.path("project.yaml"), &p)
        - path: cmd/abbd/main.go
          lines: "456"
          quote: p, err := e.st.LoadProject()
        - path: cmd/abbd/main.go
          lines: "460"
          quote: if len(p.Extract) == 0 {
      code:
        cmd/abbd/main.go#quote-db012aa7: b732db54bf14e694
        cmd/abbd/main.go#quote-f3e597fc: fc7bb3d5697db4c4
        internal/store/store.go#quote-ac7ade60: b76daadf56ef2bcd
---

What one `abbd sync` does, from the registered script to the rendered blueprint. Everything after the extraction runs inside `syncActual`, and publishing runs in `run` after any command that writes.
