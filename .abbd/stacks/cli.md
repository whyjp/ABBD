---
question: What are the layers of the abbd CLI?
format: 1
layers:
    - name: commands
      role: parses one command line, runs its verb, and renders the blueprint after a write
      units:
        - MOD-0001
      evidence:
        - path: cmd/abbd/main.go
          lines: "219"
          quote: var commands = map[string]command{
        - path: cmd/abbd/main.go
          lines: "303"
          quote: if err := e.publish(); err != nil {
      writer: reader
      code:
        cmd/abbd/main.go#quote-16c07118: 2f0d969b30da3d70
        cmd/abbd/main.go#quote-e69b8656: 16fa2f8eb12c79c6
    - name: blueprint rendering
      role: derives the pages and graph.json a human reads, and never writes the topology
      units:
        - MOD-0034
      evidence:
        - path: internal/screen/screen.go
          lines: "1"
          quote: '// Package screen renders the blueprint a human reads: the overview of the'
      writer: reader
      code:
        internal/screen/screen.go#quote-9b5341ae: c693966fc05454b1
    - name: store
      role: reads and writes the .abbd directory, the markdown every view derives from
      units:
        - MOD-0041
      evidence:
        - path: internal/store/store.go
          lines: "1"
          quote: // Package store reads and writes the .abbd directory.
      writer: reader
      code:
        internal/store/store.go#quote-346a4ce1: 3af914b66ef05333
    - name: topology rules
      role: keeps unit IDs, folds the supplement in, groups crowded levels, answers queries
      units:
        - MOD-0026
        - MOD-0009
        - MOD-0054
        - MOD-0047
      evidence:
        - path: internal/identity/identity.go
          lines: "1"
          quote: // Package identity keeps design-unit IDs stable across re-extraction.
        - path: internal/bootstrap/overlay.go
          lines: "1"
          quote: // Package bootstrap folds the semantic supplement, what pstack's readers
        - path: internal/group/group.go
          lines: "1"
          quote: // Package group places a crowded level in the directories the code has.
        - path: internal/topology/impact.go
          lines: "1"
          quote: '// Package topology answers questions about the stored topology: one unit at a'
      writer: reader
      code:
        internal/bootstrap/overlay.go#quote-3231e532: e7e7de76993d8f28
        internal/group/group.go#quote-6eaa7829: 1c38cfa4b8c7bd28
        internal/identity/identity.go#quote-79789f59: 3aa5c56106c47baf
        internal/topology/impact.go#quote-aae46dc8: b632aab8d7ef2476
    - name: readers
      role: read the extraction output, quoted code, git and module texts into facts
      units:
        - MOD-0017
        - MOD-0013
        - MOD-0022
        - MOD-0056
      evidence:
        - path: internal/extract/extract.go
          lines: "2"
          quote: // It is the boundary where untrusted model output becomes typed units.
        - path: internal/evidence/evidence.go
          lines: "1"
          quote: // Package evidence checks, deterministically, that the code still holds the
        - path: internal/gitx/gitx.go
          lines: "1"
          quote: // Package gitx reads the few git facts ABBD needs.
        - path: internal/narrative/narrative.go
          lines: "1"
          quote: '// Package narrative reads a module''s text as a human would: in sections,'
      writer: reader
      code:
        internal/evidence/evidence.go#quote-e3dbd856: 887edccfe9ccaf83
        internal/extract/extract.go#quote-4b621c5c: ba7a321b50b9feb1
        internal/gitx/gitx.go#quote-57ce0d67: 2fb4c125587d4786
        internal/narrative/narrative.go#quote-ed17a1ca: 22c9ad30bb9a8168
    - name: vocabulary
      role: names the units, relations, claims and figures every layer above passes around
      units:
        - MOD-0030
      evidence:
        - path: internal/model/model.go
          lines: "175"
          quote: type Unit struct {
      writer: reader
      code:
        internal/model/model.go#quote-44d7f48b: 17dfd679365e70ca
---

Each layer imports only the layers below it. The commands wire every layer together, and the vocabulary imports nothing of abbd's own.
