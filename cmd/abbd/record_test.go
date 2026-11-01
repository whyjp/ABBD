package main

import (
	"os/exec"
	"strings"
	"testing"
)

// sync converges: run on the same tree it rewrites nothing, and after the
// state is committed with the code it leaves the commit clean.
func TestSyncIsIdempotent(t *testing.T) {
	p := semanticProject(t)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "code and state")
	first := p.want(0, "MOD-0001 matched", "sync")
	before := snapshot(t, p.root)
	if again := p.want(0, "", "sync"); again != first {
		t.Fatalf("two syncs of one tree print different results:\n%s\n---\n%s", first, again)
	}
	if !equal(before, snapshot(t, p.root)) {
		t.Fatalf("a second sync of the same tree rewrote .abbd/")
	}
	out, err := exec.Command("git", "-C", p.root, "status", "--porcelain").CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Fatalf("sync after committing .abbd/ must leave the commit clean, got %v:\n%s", err, out)
	}
}

func TestSyncNeedsARegisteredScript(t *testing.T) {
	p, _ := bare(t)
	p.want(1, "run abbd setup", "sync")
}

// A reader's coupling enters only with a live quote; a human's coupling
// enters with where they said it, and a reader's finding never replaces it.
func TestRecordRelationNeedsEvidenceOrASource(t *testing.T) {
	p := semanticProject(t)
	p.want(1, "cite at least one --evidence", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call")
	p.want(1, "the code does not hold", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Refund()")
	if strings.Contains(p.read("architecture/MOD-0002.md"), "to: MOD-0001") {
		t.Fatalf("a refused finding must write nothing:\n%s", p.read("architecture/MOD-0002.md"))
	}

	p.want(0, "recorded MOD-0002.depends_on.MOD-0001, INFERRED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()")
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "to: MOD-0001\n      kind: depends_on\n      origin: INFERRED\n      via: call\n      evidence:") {
		t.Fatalf("the finding should be stored INFERRED with its via and evidence:\n%s", md)
	}

	pr := "https://github.com/o/r/pull/7"
	p.want(0, "USER_STATED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--source", pr)
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "origin: USER_STATED\n      source: "+pr) {
		t.Fatalf("a stated coupling carries its source:\n%s", md)
	}
	p.want(1, "is USER_STATED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()")
	p.want(1, "MOD consumes MOD is not a relation ABBD knows", "record", "relation", "MOD-0002", "consumes", "MOD-0001", "--source", pr)
}

func TestRecordUnitNeedsLiveEvidence(t *testing.T) {
	p := semanticProject(t)
	p.want(1, "cite at least one --evidence", "record", "unit", "DATA", "ledger query", "--parent", "MOD-0002")
	p.want(1, "is not a module", "record", "unit", "DATA", "ledger query", "--parent", "DATA-0001", "--evidence", "ledger/book.go:3:SELECT amount")
	p.want(0, "recorded DATA-0002 ledger query, INFERRED", "record", "unit", "DATA", "ledger query", "--parent", "MOD-0002", "--evidence", "ledger/book.go:3:SELECT amount")
	if md := p.read("architecture/DATA-0002.md"); !strings.Contains(md, "parent: MOD-0002") || !strings.Contains(md, "quote: SELECT amount") {
		t.Fatalf("the unit should be stored with its parent and evidence:\n%s", md)
	}
	p.want(0, "", "sync")
	if md := p.read("architecture/DATA-0002.md"); strings.Contains(md, "absent") {
		t.Fatalf("a sync by the script keeps a recorded unit whose evidence is live:\n%s", md)
	}
}

// What a human states needs where they said it, and outlives a later setup
// whose supplement guesses otherwise.
func TestRecordFactNeedsASourceAndOutlivesTheSupplement(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "synced", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	p.want(1, "--source", "record", "fact", "MOD-0002.purpose", "--value", "posts journal entries")
	p.want(1, "is not ID.purpose or ID.responsibility", "record", "fact", "MOD-0002.signature", "--value", "x", "--source", "conversation 2026-11-01")
	p.want(0, "recorded MOD-0002.purpose, USER_STATED", "record", "fact", "MOD-0002.purpose", "--value", "posts journal entries", "--source", "conversation 2026-11-01")
	stated := "value: posts journal entries\n    origin: USER_STATED\n    source: conversation 2026-11-01\n"
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, stated) {
		t.Fatalf("the stated purpose should replace the guess with its source:\n%s", md)
	}
	p.setup(0, "synced", "--supplement", p.file(t, "sup2.json", supplement))
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, stated) {
		t.Fatalf("a later supplement must not replace what a human stated:\n%s", md)
	}
	p.want(0, "relations: 2 EXTRACTED, 0 USER_STATED, 1 INFERRED, 0 dead", "status")
}

// An inside unit a reader found shared one anchor with a script unit and was
// folded into it: the reader's unit vanished and setup exited 0.
func TestSetupRefusesASupplementUnitThatWouldMergeIntoAScriptUnit(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.extract(`{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing"},{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"r","kind":"DATA","name":"charge record","anchors":[{"path":"billing/charge.go"}]}
]}`)
	inside := `{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"t","kind":"DATA","name":"charges table","parent":"b","evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]}
]}`
	out := p.setup(1, `"charges table"`, "--supplement", p.file(t, "sup.json", inside), "--", exe)
	if !strings.Contains(out, `"charge record"`) {
		t.Fatalf("the refusal should name both units:\n%s", out)
	}
}
