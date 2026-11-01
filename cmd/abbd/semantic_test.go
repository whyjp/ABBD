package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The ledger reads the charges table billing writes. No import shows it, so
// only a reader finds it, and only the quotes keep it alive.
const chargesTable = `{"units":[
 {"key":"t","kind":"DATA","name":"charges table","parent":"b",
  "evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
],"relations":[
 {"from":"b","to":"t","kind":"provides","via":"data","evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},
 {"from":"l","to":"t","kind":"consumes","via":"data","evidence":[{"path":"ledger/book.go","lines":"3-4","quote":"SELECT amount FROM charges"}]}
]}`

const ledgerTopo = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
]}`

func (p *project) code(rel, content string) { write(p.t, filepath.Join(p.root, rel), content) }

func semanticProject(t *testing.T) (*project, string) {
	t.Helper()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	return p, exe
}

func TestASupplementUnitOutsideTheScriptNeedsEvidence(t *testing.T) {
	p, exe := semanticProject(t)
	bare := strings.Replace(chargesTable, `
  "evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},`, `"anchors":[{"path":"db/schema.sql"}]},`, 1)
	p.setup(1, "cite the code each one rests on", "--supplement", p.file(t, "bare.json", bare), "--", exe)

	p.setup(0, "bootstrapped: 3 design unit candidate(s)", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	md := p.read("architecture/DATA-0001.md")
	for _, want := range []string{"value: charges table\n    origin: INFERRED", "parent: MOD-0001", "quote: INSERT INTO charges", "path: billing/charge.go"} {
		if !strings.Contains(md, want) {
			t.Fatalf("a unit only the reader found should be stored with its evidence and parent; missing %q:\n%s", want, md)
		}
	}
	q := p.want(0, "MOD-0002.consumes.DATA-0001", "confirm", "--list")
	if !strings.Contains(q, `INFERRED via data: ledger/book.go:3-4 \"SELECT amount FROM charges\"`) {
		t.Fatalf("the queue should show how the coupling arises and the code it rests on:\n%s", q)
	}
	p.want(0, "aligned", "align", "--all")
	p.want(0, "active", "mode", "on", "--session", "s1")
	if !strings.Contains(p.read("architecture/DATA-0001.md"), "charges table") || strings.Contains(p.read("architecture/DATA-0001.md"), "absent") {
		t.Fatalf("re-extraction by the script must keep a semantic unit whose evidence is live")
	}
}

func TestDeletingTheEvidenceRemovesTheCoupling(t *testing.T) {
	p, exe := semanticProject(t)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	p.want(0, "aligned", "align", "--all")
	p.want(0, "approved new-relation:MOD-0002.consumes.DATA-0001", "confirm", "MOD-0002.consumes.DATA-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.want(0, "off", "mode", "off", "--session", "s1")

	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	out := p.want(2, "BLOCKED removed-relation:MOD-0002.consumes.DATA-0001", "mode", "on", "--session", "s1")
	if strings.Contains(out, "provides.DATA-0001") || strings.Contains(out, "unit-removed") {
		t.Fatalf("only the coupling whose evidence went away is removed:\n%s", out)
	}
	if !strings.Contains(p.read("architecture/MOD-0002.md"), "dead: true") {
		t.Fatalf("the dead coupling stays on record, marked dead:\n%s", p.read("architecture/MOD-0002.md"))
	}

	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	out = p.want(2, "BLOCKED unit-removed:DATA-0001", "mode", "on", "--session", "s1")
	if !strings.Contains(p.read("architecture/DATA-0001.md"), "absent: true") {
		t.Fatalf("a semantic unit whose evidence is gone is absent:\n%s", out)
	}
}

func TestAnUnconfirmedCouplingWithDeadEvidenceLeavesTheQueue(t *testing.T) {
	p, exe := semanticProject(t)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.want(2, "bootstrapped", "mode", "on", "--session", "s1")
	if q := p.want(0, "MOD-0001.provides.DATA-0001", "confirm", "--list"); strings.Contains(q, "MOD-0002.consumes") {
		t.Fatalf("a guess whose evidence is gone has nothing left to confirm:\n%s", q)
	}
}

func TestRelationKindsMustFitTheUnitsTheyJoin(t *testing.T) {
	p, exe := semanticProject(t)
	modProvidesMod := `{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
],"relations":[{"from":"b","to":"l","kind":"provides"}]}`
	p.setup(1, "MOD provides MOD is not a relation ABBD knows", "--supplement", p.file(t, "sup.json", modProvidesMod), "--", exe)
	badVia := strings.Replace(chargesTable, `"via":"data"`, `"via":"telepathy"`, 1)
	p.setup(1, `unknown via "telepathy"`, "--supplement", p.file(t, "sup.json", badVia), "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "architecture")); err == nil {
		t.Fatalf("a refused supplement must write nothing")
	}
	p.setup(0, "bootstrapped", "--", exe)
	p.want(0, "aligned", "align", "--all")
	p.want(1, "MOD consumes MOD is not a relation ABBD knows", "couple", "MOD-0002.consumes.MOD-0001")
}
