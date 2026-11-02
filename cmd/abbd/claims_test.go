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
	if md := p.read("architecture/DATA-0001.md"); !strings.Contains(md, "supplement_ref: t") {
		t.Fatalf("a paired supplement unit keeps the supplement's ref:\n%s", md)
	}

	p.resync()
	contains(t, p.want(0, "", "diff", "--base", "HEAD"), "no topology change")
	p.setup(0, "synced", "--supplement", ".abbd/extractor/supplement.json")
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0002 ledger depends_on MOD-0001 billing via call")
	contains(t, p.want(0, "", "explain", "DATA-0001"), "inside MOD-0001 billing")
}

// Retracting your own claim converges: once it is gone, retracting it again
// says so and succeeds, for an edge and for a parent.
func TestRetractingYourOwnClaimTwiceSucceeds(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "INFERRED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()")
	p.want(0, "", "record", "parent", "MOD-0002", "MOD-0001", "--evidence", "ledger/book.go:6:func Post()")
	for _, target := range []string{"MOD-0002.depends_on.MOD-0001", "MOD-0002.parent"} {
		p.want(0, "retracted "+target, "retract", target, "--reason", "wrong")
		before := units(t, p.root)
		p.want(0, "already retracted", "retract", target, "--reason", "wrong")
		if !equal(before, units(t, p.root)) {
			t.Fatalf("a second retract of %s rewrote the state", target)
		}
	}
}

const ownedTable = `{"units":[
 {"key":"t","kind":"DATA","name":"charges table","parent":"b",
  "parent_evidence":[{"path":"billing/owner.go","lines":"2","quote":"owns the charges table"}],
  "evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
]}`

// Re-running setup restates the supplement's claims. An item that died
// stays dead, so the parent it held up does not come back.
func TestSetupAgainKeepsADeadSupplementParentDead(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("billing/owner.go", "package billing\n// owns the charges table\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	sup := p.file(t, "sup.json", ownedTable)
	p.setup(0, "synced", "--supplement", sup, "--", exe)
	contains(t, p.want(0, "", "explain", "DATA-0001"), "inside MOD-0001 billing")
	p.code("billing/owner.go", "package billing\n// something else\n")
	p.resync()
	p.laterRevision()
	p.resync()
	p.setup(0, "synced", "--supplement", sup)
	if out := p.want(0, "", "explain", "DATA-0001"); strings.Contains(out, "inside MOD-0001") {
		t.Fatalf("setup revived a parent whose evidence died:\n%s", out)
	}
	if md := p.read("architecture/DATA-0001.md"); !strings.Contains(md, "status: dead") {
		t.Fatalf("the restated item lost its dead status:\n%s", md)
	}
}

// An item is the same code whatever symbol the supplement names with it, so
// a supplement that adds a symbol to a dead quote keeps it dead.
func TestSetupWithASymbolAddedKeepsADeadItemDead(t *testing.T) {
	p := semanticProject(t)
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.resync()
	p.laterRevision()
	p.resync()
	contains(t, p.want(0, "", "status"), "evidence: 0 drifted, 1 dead")
	symbol := strings.Replace(chargesTable, `{"path":"ledger/book.go","lines":"3-4"`, `{"path":"ledger/book.go","symbol":"Post","lines":"3-4"`, 1)
	p.setup(0, "synced", "--supplement", p.file(t, "symbol.json", symbol))
	contains(t, p.want(0, "", "status"), "evidence: 0 drifted, 1 dead")
}

func oldState(t *testing.T, p *project, exe string, files map[string]string) {
	t.Helper()
	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), "extract:\n    - "+exe+"\n")
	for name, content := range files {
		write(t, filepath.Join(p.root, ".abbd", name), content)
	}
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "old state")
	write(t, filepath.Join(p.root, ".abbd", "extractor", "supplement.json"), chargesTable)
}

// The supplement owns an old INFERRED parent only when it places the unit in
// the same parent. A reader's parent elsewhere stays the reader's, so setup
// does not replace it.
func TestMigrateKeepsAReadersParentTheSupplementDoesNotMake(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	tbl := strings.Replace(oldTable, "parent: MOD-0001", "parent: MOD-0002", 1)
	tbl = strings.Replace(tbl, "parent_evidence:\n    - path: billing/charge.go\n      lines: \"3\"\n      quote: INSERT INTO charges", "parent_evidence:\n    - path: ledger/book.go\n      lines: \"6\"\n      quote: func Post()", 1)
	oldState(t, p, exe, map[string]string{
		"state.yaml":                "revision: \"\"\nfingerprint: \"\"\nmd_hash: \"\"\nhigh_water:\n    DATA: 1\n    MOD: 2\n",
		"architecture/MOD-0001.md":  oldBilling,
		"architecture/MOD-0002.md":  oldLedger,
		"architecture/DATA-0001.md": tbl,
	})
	contains(t, p.want(0, "", "migrate"), "parents: 0 supplement, 1 reader")
	p.resync()
	p.setup(0, "synced", "--supplement", ".abbd/extractor/supplement.json")
	contains(t, p.want(0, "", "explain", "DATA-0001"), "inside MOD-0002 ledger")
}

const oldMoney = `---
id: MOD-0003
kind: MOD
name:
    value: money
    origin: EXTRACTED
anchors:
    - path: money/README.md
---

# money
`

// An old human parent never recorded the script's parent it overrode.
// Migrate takes it from the extraction, so the first sync after it reports
// no conflict the human never had.
func TestMigrateRecordsWhatAHumanParentOverrode(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.code("money/README.md", "# money\n")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","parent":"money","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"money","kind":"MOD","name":"money","anchors":[{"path":"money/README.md"}]}
]}`)
	oldState(t, p, exe, map[string]string{
		"state.yaml":                "revision: \"\"\nfingerprint: \"\"\nmd_hash: \"\"\nhigh_water:\n    DATA: 1\n    MOD: 3\n",
		"architecture/MOD-0001.md":  oldBilling,
		"architecture/MOD-0002.md":  strings.Replace(oldLedger, "relations:", "parent: MOD-0001\nparent_origin: USER_STATED\nparent_source: pr 7\nrelations:", 1),
		"architecture/MOD-0003.md":  oldMoney,
		"architecture/DATA-0001.md": oldTable,
	})
	p.want(0, "migrated", "migrate")
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "over: MOD-0003") {
		t.Fatalf("the human's parent records the script's parent it overrode:\n%s", md)
	}
	if out := p.resync(); strings.Contains(out, "conflict") {
		t.Fatalf("the first sync after migrate reports a conflict:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "inside MOD-0001 billing")
}

// An item's status belongs to the code it cites. When setup restates a
// claim with an item the claim did not hold but the unit already stores
// dead, the item comes in dead, not as a fresh drift.
func TestSetupTakesAnItemsStatusFromTheUnitThatStoresIt(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	gone := "    - path: billing/charge.go\n      lines: \"4\"\n      quote: DELETE FROM charges\n---"
	oldState(t, p, exe, map[string]string{
		"state.yaml":                "revision: \"\"\nfingerprint: \"\"\nmd_hash: \"\"\nhigh_water:\n    DATA: 1\n    MOD: 2\n",
		"architecture/MOD-0001.md":  oldBilling,
		"architecture/MOD-0002.md":  oldLedger,
		"architecture/DATA-0001.md": strings.Replace(oldTable, "---\n\n# charges", gone+"\n\n# charges", 1),
	})
	sup := strings.Replace(chargesTable, `"evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},`,
		`"evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"},{"path":"billing/charge.go","lines":"4","quote":"DELETE FROM charges"}]},`, 1)
	write(t, filepath.Join(p.root, ".abbd", "extractor", "supplement.json"), sup)
	contains(t, p.want(0, "", "migrate"), "1 item(s) no longer in the tree stay dead")
	p.resync()
	p.setup(0, "synced", "--supplement", ".abbd/extractor/supplement.json")
	contains(t, p.want(0, "", "status"), "evidence: 0 drifted")
}
