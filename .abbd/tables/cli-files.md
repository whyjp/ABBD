---
question: Which part of the abbd CLI reads and writes which .abbd file?
format: 1
columns:
    - name: unit files
      unit: DATA-0004
    - name: state.yaml
      unit: DATA-0006
    - name: module texts
      unit: DATA-0008
    - name: blueprint
      unit: DATA-0005
    - name: project.yaml
      unit: CFG-0002
rows:
    - unit: MOD-0004
      cells:
        project.yaml: reads the extraction argv to run the project's script
        state.yaml: reads the ID high-water mark, then writes revision and fingerprint
        unit files: rewrites every unit file from the reconciled extraction
      evidence:
        - path: cmd/abbd/main.go
          lines: "452"
          symbol: syncActual
          quote: if err := st.SaveUnits(res.Units); err != nil {
        - path: cmd/abbd/main.go
          lines: "423"
          symbol: syncActual
          quote: state, err := st.LoadState()
        - path: cmd/abbd/main.go
          lines: "469"
          symbol: syncActual
          quote: return res, st.SaveState(state)
        - path: cmd/abbd/main.go
          lines: "342"
          quote: p, err := e.st.LoadProject()
      writer: reader
      code:
        cmd/abbd/main.go#quote-045deabc: 277a8a331c1c57f6
        cmd/abbd/main.go#quote-22175f6c: 228fd02a0d005209
        cmd/abbd/main.go#quote-db012aa7: b732db54bf14e694
        cmd/abbd/main.go#quote-e28fdf28: b662511d12a95a22
    - unit: MOD-0006
      cells:
        module texts: writes how's text with the module's digest and cited code
        unit files: writes the one unit a relation, fact or parent record names
      evidence:
        - path: cmd/abbd/record.go
          lines: "110"
          symbol: recordRelation
          quote: u.PutRelation(rel)
        - path: cmd/abbd/understand.go
          lines: "83"
          symbol: understand
          quote: 'if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Digest: digest'
      writer: reader
      code:
        cmd/abbd/record.go#quote-96b34390: 77467f8b8d20a875
        cmd/abbd/understand.go#quote-c870eaa1: 94ae3f2c35dc5f77
    - unit: MOD-0005
      cells:
        module texts: reads how's text and says whether the module's code moved since
        unit files: reads every unit to resolve and answer a query
      evidence:
        - path: cmd/abbd/query.go
          lines: "249"
          symbol: explainHow
          quote: texts, err := e.st.LoadUnderstandings()
        - path: cmd/abbd/query.go
          lines: "259"
          symbol: explainHow
          quote: if t.Digest != e.st.ModuleDigest(units, u.ID) {
      writer: reader
      code:
        cmd/abbd/query.go#quote-1a68db9a: 68d6b606e038c5c4
        cmd/abbd/query.go#quote-0793a015: f3107515b0fe8519
    - unit: MOD-0007
      cells:
        blueprint: writes every page and graph.json
        module texts: reads how's texts to draw beside each module
        project.yaml: reads the extraction argv the page foot names
        state.yaml: records which inputs it rendered and which files it wrote
        unit files: reads every unit a page draws
      evidence:
        - path: cmd/abbd/screen.go
          lines: "12"
          symbol: screenInput
          quote: in, err := screen.Load(e.st)
        - path: cmd/abbd/screen.go
          lines: "31"
          symbol: publish
          quote: if err := e.st.SaveBlueprint(files); err != nil {
        - path: cmd/abbd/screen.go
          lines: "34"
          symbol: publish
          quote: in.Synced.Blueprint, in.Synced.Rendered = in.Inputs, store.HashRender(files)
      writer: reader
      code:
        cmd/abbd/screen.go#quote-8debea1b: 0412b0ffcef8ceb7
        cmd/abbd/screen.go#quote-18c7c622: 8d8ed08440cabc5e
        cmd/abbd/screen.go#quote-061a5173: 91f35d6e30a925d0
    - unit: MOD-0002
      cells:
        blueprint: says whether its files are the ones the state rendered
        state.yaml: reads the fingerprint to say whether the state is CURRENT
        unit files: compares the stored units with the base revision's
      evidence:
        - path: cmd/abbd/diff.go
          lines: "23"
          symbol: diff
          quote: was, err := e.st.UnitsAt(base)
        - path: cmd/abbd/evidence.go
          lines: "103"
          symbol: status
          quote: 'case in.Synced.Fingerprint == "":'
        - path: cmd/abbd/evidence.go
          lines: "106"
          symbol: status
          quote: switch in.BlueprintFreshness() {
      writer: reader
      code:
        cmd/abbd/diff.go#quote-c3e410f2: 2bbb4ade6758f770
        cmd/abbd/evidence.go#quote-30c3c1df: 474a69039651a4f6
        cmd/abbd/evidence.go#quote-c6625aef: fd9b8c605a722cdf
---

Each row is one part of the CLI and each column one file under `.abbd/`. Only sync and the record writers write the topology. Every command that writes renders the blueprint after it, so blueprint publish reads every input and writes the blueprint and the state. The query and review commands only read.
