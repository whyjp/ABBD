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
        module texts: re-stamps old-format texts whose code matches, names the rest to re-read
        project.yaml: reads the extraction argv to run the project's script
        state.yaml: reads the ID high-water mark, then writes revision and fingerprint
        unit files: rewrites every unit file from the reconciled extraction
      evidence:
        - path: cmd/abbd/main.go
          lines: "525"
          symbol: syncActual
          quote: if err := st.SaveUnits(res.Units); err != nil {
        - path: cmd/abbd/main.go
          lines: "496"
          symbol: syncActual
          quote: state, err := st.LoadState()
        - path: cmd/abbd/main.go
          lines: "542"
          symbol: syncActual
          quote: return res, st.SaveState(state)
        - path: cmd/abbd/main.go
          lines: "531"
          symbol: syncActual
          quote: current, widened, err := st.UpgradeUnderstandings(res.Units)
        - path: cmd/abbd/main.go
          lines: "415"
          quote: p, err := e.st.LoadProject()
      writer: reader
      code:
        cmd/abbd/main.go#quote-045deabc: 277a8a331c1c57f6
        cmd/abbd/main.go#quote-714f2ba0: 59f3f48639d38179
        cmd/abbd/main.go#quote-22175f6c: 98424d209f0d7558
        cmd/abbd/main.go#quote-db012aa7: b732db54bf14e694
        cmd/abbd/main.go#quote-e28fdf28: b662511d12a95a22
    - unit: MOD-0006
      cells:
        module texts: writes a reader's text with its reader, the module's digest and cited code
        unit files: writes a record's claim, fact or parent, or just a phrase note on a shown edge
      evidence:
        - path: cmd/abbd/record.go
          lines: "141"
          symbol: recordRelation
          quote: u.PutRelation(rel)
        - path: cmd/abbd/record.go
          lines: "144"
          symbol: recordRelation
          quote: u.PutNote(note)
        - path: cmd/abbd/understand.go
          lines: "86"
          symbol: understand
          quote: 'if err := e.st.SaveUnderstanding(store.Understanding{Unit: id'
      writer: reader
      code:
        cmd/abbd/record.go#quote-96b34390: cea44e852e7a7e92
        cmd/abbd/record.go#quote-5614cda7: dec0a70e1688a95f
        cmd/abbd/understand.go#quote-e0b4fa19: 747a51ddc1079f48
    - unit: MOD-0005
      cells:
        module texts: prints a text with its reader, and whether its code moved since
        unit files: reads every unit to resolve and answer a query
      evidence:
        - path: cmd/abbd/query.go
          lines: "261"
          symbol: explainHow
          quote: texts, err := e.st.LoadUnderstandings()
        - path: cmd/abbd/query.go
          lines: "271"
          symbol: explainHow
          quote: if t.Digest != e.st.ModuleDigest(units, u.ID) {
      writer: reader
      code:
        cmd/abbd/query.go#quote-1a68db9a: 68d6b606e038c5c4
        cmd/abbd/query.go#quote-0793a015: ec9cd2bd2fda6b40
    - unit: MOD-0007
      cells:
        blueprint: writes every page and graph.json
        module texts: reads the readers' texts to draw beside each module
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
        unit files: compares the stored units with the base revision's, or with the copy a write kept before the edits since the last commit
      evidence:
        - path: cmd/abbd/diff.go
          lines: "173"
          symbol: unitsAt
          quote: return e.st.UnitsAt(base)
        - path: cmd/abbd/evidence.go
          lines: "109"
          symbol: status
          quote: 'case in.Synced.Fingerprint == "":'
        - path: cmd/abbd/evidence.go
          lines: "112"
          symbol: status
          quote: switch in.BlueprintFreshness() {
      writer: reader
      code:
        cmd/abbd/diff.go#quote-d7133fe3: c0c3ba779b5a8e28
        cmd/abbd/evidence.go#quote-30c3c1df: 474a69039651a4f6
        cmd/abbd/evidence.go#quote-c6625aef: fd9b8c605a722cdf
retired:
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
      code:
        cmd/abbd/record.go#quote-96b34390: 77467f8b8d20a875
        cmd/abbd/understand.go#quote-c870eaa1: 94ae3f2c35dc5f77
    - unit: MOD-0006
      cells:
        module texts: writes how's text with the module's digest and cited code
        unit files: writes the one unit a record names, with its relation and phrase, fact or parent
      evidence:
        - path: cmd/abbd/record.go
          lines: "118"
          symbol: recordRelation
          quote: u.PutRelation(rel)
        - path: cmd/abbd/understand.go
          lines: "83"
          symbol: understand
          quote: 'if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Digest: digest'
      code:
        cmd/abbd/record.go#quote-96b34390: 5ffeca89905aa31f
        cmd/abbd/understand.go#quote-c870eaa1: 94ae3f2c35dc5f77
    - unit: MOD-0006
      cells:
        module texts: writes how's text with the module's digest and cited code
        unit files: writes the unit a record names, with its claim, phrase note, fact or parent
      evidence:
        - path: cmd/abbd/record.go
          lines: "134"
          symbol: recordRelation
          quote: u.PutRelation(rel)
        - path: cmd/abbd/record.go
          lines: "137"
          symbol: recordRelation
          quote: u.PutNote(note)
        - path: cmd/abbd/understand.go
          lines: "83"
          symbol: understand
          quote: 'if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Digest: digest'
      code:
        cmd/abbd/record.go#quote-96b34390: 7592c997356842e8
        cmd/abbd/record.go#quote-5614cda7: 06ebdcfaa1c5ed4e
        cmd/abbd/understand.go#quote-c870eaa1: 94ae3f2c35dc5f77
    - unit: MOD-0004
      cells:
        module texts: re-stamps each old-format text whose module's code still matches
        project.yaml: reads the extraction argv to run the project's script
        state.yaml: reads the ID high-water mark, then writes revision and fingerprint
        unit files: rewrites every unit file from the reconciled extraction
      evidence:
        - path: cmd/abbd/main.go
          lines: "473"
          symbol: syncActual
          quote: if err := st.SaveUnits(res.Units); err != nil {
        - path: cmd/abbd/main.go
          lines: "444"
          symbol: syncActual
          quote: state, err := st.LoadState()
        - path: cmd/abbd/main.go
          lines: "490"
          symbol: syncActual
          quote: return res, st.SaveState(state)
        - path: cmd/abbd/main.go
          lines: "479"
          symbol: syncActual
          quote: current, widened, err := st.UpgradeUnderstandings(res.Units)
        - path: cmd/abbd/main.go
          lines: "363"
          quote: p, err := e.st.LoadProject()
      code:
        cmd/abbd/main.go#quote-045deabc: 277a8a331c1c57f6
        cmd/abbd/main.go#quote-714f2ba0: 59f3f48639d38179
        cmd/abbd/main.go#quote-22175f6c: 228fd02a0d005209
        cmd/abbd/main.go#quote-db012aa7: b732db54bf14e694
        cmd/abbd/main.go#quote-e28fdf28: b662511d12a95a22
    - unit: MOD-0006
      cells:
        module texts: writes how's text with the module's digest and cited code
        unit files: writes a record's claim, fact or parent, or just a phrase note on a shown edge
      evidence:
        - path: cmd/abbd/record.go
          lines: "141"
          symbol: recordRelation
          quote: u.PutRelation(rel)
        - path: cmd/abbd/record.go
          lines: "144"
          symbol: recordRelation
          quote: u.PutNote(note)
        - path: cmd/abbd/understand.go
          lines: "83"
          symbol: understand
          quote: 'if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Digest: digest'
      code:
        cmd/abbd/record.go#quote-96b34390: cea44e852e7a7e92
        cmd/abbd/record.go#quote-5614cda7: dec0a70e1688a95f
        cmd/abbd/understand.go#quote-c870eaa1: 94ae3f2c35dc5f77
    - unit: MOD-0005
      cells:
        module texts: reads how's text and says whether the module's code moved since
        unit files: reads every unit to resolve and answer a query
      evidence:
        - path: cmd/abbd/query.go
          lines: "252"
          symbol: explainHow
          quote: texts, err := e.st.LoadUnderstandings()
        - path: cmd/abbd/query.go
          lines: "262"
          symbol: explainHow
          quote: if t.Digest != e.st.ModuleDigest(units, u.ID) {
      code:
        cmd/abbd/query.go#quote-1a68db9a: 68d6b606e038c5c4
        cmd/abbd/query.go#quote-0793a015: f3107515b0fe8519
    - unit: MOD-0005
      cells:
        module texts: reads a reader's text and says whether the module's code moved since
        unit files: reads every unit to resolve and answer a query
      evidence:
        - path: cmd/abbd/query.go
          lines: "252"
          symbol: explainHow
          quote: texts, err := e.st.LoadUnderstandings()
        - path: cmd/abbd/query.go
          lines: "262"
          symbol: explainHow
          quote: if t.Digest != e.st.ModuleDigest(units, u.ID) {
      code:
        cmd/abbd/query.go#quote-1a68db9a: 68d6b606e038c5c4
        cmd/abbd/query.go#quote-0793a015: a7577b4f52f97404
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
          lines: "109"
          symbol: status
          quote: 'case in.Synced.Fingerprint == "":'
        - path: cmd/abbd/evidence.go
          lines: "112"
          symbol: status
          quote: switch in.BlueprintFreshness() {
      code:
        cmd/abbd/diff.go#quote-c3e410f2: 2bbb4ade6758f770
        cmd/abbd/evidence.go#quote-30c3c1df: 474a69039651a4f6
        cmd/abbd/evidence.go#quote-c6625aef: fd9b8c605a722cdf
    - unit: MOD-0002
      cells:
        blueprint: says whether its files are the ones the state rendered
        state.yaml: reads the fingerprint to say whether the state is CURRENT
        unit files: compares the stored units with the base revision's, or with the copy a write kept before a session's edits
      evidence:
        - path: cmd/abbd/diff.go
          lines: "171"
          symbol: unitsAt
          quote: return e.st.UnitsAt(base)
        - path: cmd/abbd/evidence.go
          lines: "109"
          symbol: status
          quote: 'case in.Synced.Fingerprint == "":'
        - path: cmd/abbd/evidence.go
          lines: "112"
          symbol: status
          quote: switch in.BlueprintFreshness() {
      code:
        cmd/abbd/diff.go#quote-d7133fe3: 697a880350600c1c
        cmd/abbd/evidence.go#quote-30c3c1df: 474a69039651a4f6
        cmd/abbd/evidence.go#quote-c6625aef: fd9b8c605a722cdf
---

Each row is one part of the CLI and each column one file under `.abbd/`. Only sync and the record writers write the topology. Every command that writes renders the blueprint after it, so blueprint publish reads every input and writes the blueprint and the state. The query and review commands only read.
cells: 9 cell(s) say a use no stored relation carries, 0 empty cell(s) sit over a stored relation; abbd status lists them
