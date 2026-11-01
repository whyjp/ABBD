package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Retract is one step: it removes the writer's own claim and hides the lower
// INFERRED claims on the target, bound to their content. A second run changes
// nothing, a re-run of setup with the same claim keeps it hidden, and a claim
// whose content changed shows again.
func TestRetractIsOneIdempotentStepBoundToTheClaimsContent(t *testing.T) {
	p := semanticProject(t)
	edge := "MOD-0002.consumes.DATA-0001"
	p.want(1, "--reason", "retract", edge)
	p.want(0, "retracted "+edge, "retract", edge, "--reason", "the ledger reads a view")
	before := units(t, p.root)
	p.want(0, "already retracted", "retract", edge, "--reason", "again")
	if !equal(before, units(t, p.root)) {
		t.Fatalf("a second retract rewrote the state")
	}
	if out := p.want(0, "", "explain", "MOD-0002"); strings.Contains(out, "DATA-0001") {
		t.Fatalf("a retracted coupling still shows:\n%s", out)
	}
	contains(t, p.want(0, "", "status"), "retracted: MOD-0002 consumes.DATA-0001 by a reader: the ledger reads a view")

	p.setup(0, "synced", "--supplement", p.file(t, "same.json", chargesTable))
	if out := p.want(0, "", "explain", "MOD-0002"); strings.Contains(out, "DATA-0001") {
		t.Fatalf("setup with the same claim brought the retracted coupling back:\n%s", out)
	}
	changed := strings.Replace(chargesTable, `"kind":"consumes","via":"data"`, `"kind":"consumes","via":"convention"`, 1)
	p.setup(0, "synced", "--supplement", p.file(t, "changed.json", changed))
	contains(t, p.want(0, "", "explain", "MOD-0002"), "consumes DATA-0001 charges table via convention")

	p.want(1, "is the script's", "retract", "MOD-0001", "--reason", "no such module")
	p.want(0, "INFERRED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()")
	p.want(0, "retracted", "retract", "MOD-0002.depends_on.MOD-0001", "--reason", "a wrong reading")
	if md := p.read("architecture/MOD-0002.md"); strings.Contains(md, "to: MOD-0001") {
		t.Fatalf("retracting a reader's own claim removes it:\n%s", md)
	}
	p.want(0, "retracted DATA-0001", "retract", "DATA-0001", "--reason", "not a table", "--source", "conversation 2026-11-02")
	if md := p.read("architecture/DATA-0001.md"); !strings.Contains(md, "absent_reason: retracted") {
		t.Fatalf("a retracted unit is absent with its reason:\n%s", md)
	}
}

// Idempotence across the writers: sync, setup, sync, setup mints no ID after
// the first pair, and the second pair changes no byte.
func TestSyncAndSetupTwiceMintNoIDsAndChangeNothing(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "INFERRED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()")
	sup := p.file(t, "sup.json", chargesTable)
	p.resync()
	p.setup(0, "synced", "--supplement", sup)
	first := snapshot(t, p.root)
	out := p.resync() + p.setup(0, "synced", "--supplement", sup)
	if strings.Contains(out, " new\n") {
		t.Fatalf("a repeated sync and setup minted an ID:\n%s", out)
	}
	if !equal(first, snapshot(t, p.root)) {
		t.Fatalf("a repeated sync and setup changed the state")
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0002 ledger depends_on MOD-0001 billing via call")
}

const oldBilling = `---
id: MOD-0001
kind: MOD
name:
    value: billing
    origin: EXTRACTED
anchors:
    - path: billing/charge.go
      symbol: Charge
relations:
    - to: DATA-0001
      kind: provides
      origin: INFERRED
      via: data
      evidence:
        - path: billing/charge.go
          lines: "3"
          quote: INSERT INTO charges
---

# billing
`

const oldLedger = `---
id: MOD-0002
kind: MOD
name:
    value: ledger
    origin: EXTRACTED
anchors:
    - path: ledger/book.go
      symbol: Post
relations:
    - to: DATA-0001
      kind: consumes
      origin: INFERRED
      via: data
      evidence:
        - path: ledger/book.go
          lines: 3-4
          quote: SELECT amount FROM charges
    - to: MOD-0001
      kind: depends_on
      origin: INFERRED
      via: call
      evidence:
        - path: ledger/book.go
          lines: "6"
          quote: func Post()
---

# ledger
`

const oldTable = `---
id: DATA-0001
kind: DATA
name:
    value: charges table
    origin: INFERRED
anchors:
    - path: billing/charge.go
parent: MOD-0001
parent_origin: INFERRED
parent_evidence:
    - path: billing/charge.go
      lines: "3"
      quote: INSERT INTO charges
evidence:
    - path: billing/charge.go
      lines: "3"
      quote: INSERT INTO charges
---

# charges table
`

// A state written before writers existed stays readable for good, because
// abbd diff reads committed states. Commands that write refuse it, and
// abbd migrate converts it once: a claim the supplement makes is the
// supplement's, one it does not make is a reader's, and the graph is the
// same after as before.
func TestMigrateTellsAReadersClaimsFromTheSupplements(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), "extract:\n    - "+exe+"\n")
	write(t, filepath.Join(p.root, ".abbd", "state.yaml"), "revision: \"\"\nfingerprint: \"\"\nmd_hash: \"\"\nhigh_water:\n    DATA: 1\n    MOD: 2\n")
	write(t, filepath.Join(p.root, ".abbd", "architecture", "MOD-0001.md"), oldBilling)
	write(t, filepath.Join(p.root, ".abbd", "architecture", "MOD-0002.md"), oldLedger)
	write(t, filepath.Join(p.root, ".abbd", "architecture", "DATA-0001.md"), oldTable)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "old state")

	contains(t, p.want(0, "", "status"), "relations: 0 EXTRACTED, 0 USER_STATED, 3 INFERRED, 0 dead")
	p.want(1, "run abbd migrate", "sync")
	p.want(1, "run abbd migrate", "record", "relation", "MOD-0001", "depends_on", "MOD-0002", "--source", "pr")
	p.want(1, "no supplement at", "migrate")

	write(t, filepath.Join(p.root, ".abbd", "extractor", "supplement.json"), chargesTable)
	out := p.want(0, "migrated 3 unit file(s)", "migrate")
	contains(t, out, "relations: 2 supplement, 1 reader", "units: 1 supplement, 0 reader", "parents: 1 supplement, 0 reader")
	p.want(0, "already in the current format", "migrate")
	if md := p.read("architecture/DATA-0001.md"); !strings.Contains(md, "supplement_key: t") {
		t.Fatalf("a paired supplement unit keeps the supplement's key:\n%s", md)
	}

	p.resync()
	contains(t, p.want(0, "", "diff", "--base", "HEAD"), "no topology change")
	p.setup(0, "synced", "--supplement", ".abbd/extractor/supplement.json")
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0002 ledger depends_on MOD-0001 billing via call")
	contains(t, p.want(0, "", "explain", "DATA-0001"), "inside MOD-0001 billing")
}
