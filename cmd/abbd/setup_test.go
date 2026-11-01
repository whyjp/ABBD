package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const codeOnly = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],"purpose":"takes payments"},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[
 {"from":"billing","to":"api","kind":"provides"},
 {"from":"billing","to":"ledger","kind":"depends_on"}
]}`

// The swarm sees that ledger calls the http api; no import shows it.
const supplement = `{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],"responsibility":"owns the charges table"},
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}],"purpose":"keeps the books"},
 {"key":"a","kind":"IF","name":"http api","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[
 {"from":"l","to":"a","kind":"consumes","via":"call","evidence":[{"path":"api/http.go","quote":"func Serve()"}]}
]}`

func TestSetupRegistersTheScriptAndSyncs(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "synced: 3 unit(s)", "--", exe)
	if !strings.Contains(p.read("project.yaml"), filepath.Base(exe)) {
		t.Fatalf("setup should record the extraction script:\n%s", p.read("project.yaml"))
	}
	if md := p.read("architecture/MOD-0001.md"); !strings.Contains(md, "takes payments\n    origin: INFERRED") || strings.Contains(md, "confirmation") {
		t.Fatalf("a purpose the script guessed is INFERRED and waits for nobody:\n%s", md)
	}
}

func TestSupplementAddsInferredFactsThatSurviveReextraction(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "3 matched by the supplement", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	md := p.read("architecture/MOD-0002.md")
	if !strings.Contains(md, "keeps the books\n    origin: INFERRED") || !strings.Contains(md, "to: IF-0001") || !strings.Contains(md, "quote: func Serve()") {
		t.Fatalf("the supplement should add an inferred purpose and a coupling with its evidence:\n%s", md)
	}
	p.want(0, "MOD-0002 matched", "sync")
	if !strings.Contains(p.read("architecture/MOD-0002.md"), "kind: consumes") {
		t.Fatalf("re-extraction by the script must keep a coupling the script cannot see")
	}
}

func TestASupplementCouplingNeedsEvidence(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	bare := strings.Replace(supplement, `,"via":"call","evidence":[{"path":"api/http.go","quote":"func Serve()"}]`, "", 1)
	p.setup(1, "l -consumes-> a cites no evidence", "--supplement", p.file(t, "sup.json", bare), "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must write nothing")
	}
}

func TestSupplementNamingUnknownCodeIsRefused(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	bad := `{"units":[{"key":"x","kind":"MOD","name":"ghost","anchors":[{"path":"ghost/x.go","symbol":"X"}],"purpose":"haunts"}]}`
	p.setup(1, "fix the script", "--supplement", p.file(t, "sup.json", bad), "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must write nothing")
	}
}

func TestSetupAgainOnUnchangedCodeRewritesNothing(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "synced", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	first := snapshot(t, p.root)
	p.setup(0, "synced", "--supplement", p.file(t, "sup2.json", supplement))
	if again := snapshot(t, p.root); !equal(first, again) {
		t.Fatalf("setup again on unchanged code must rewrite nothing")
	}
}

func TestNondeterministicScriptIsRefused(t *testing.T) {
	p, exe := bare(t)
	p.extract(strings.Replace(codeOnly, `"name":"ledger"`, `"name":"ledger {{RUN}}"`, 1))
	p.setup(1, "not deterministic", "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must not record the script or any unit")
	}
}

func TestScriptThatDependsOnTheHashSeedIsRefused(t *testing.T) {
	t.Setenv("PYTHONHASHSEED", "0")
	p, exe := bare(t)
	p.extract(strings.Replace(codeOnly, `"name":"ledger"`, `"name":"ledger{{PYTHONHASHSEED}}"`, 1))
	p.setup(1, "PYTHONHASHSEED", "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must write nothing")
	}
}

func TestSetupResolvesFilesAgainstTheRoot(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	write(t, filepath.Join(p.root, ".abbd", "extractor", "supplement.json"), supplement)
	p.setup(0, "3 matched by the supplement", "--supplement", ".abbd/extractor/supplement.json", "--", exe)
}

func TestCorrectedSupplementWithdrawsEarlierGuesses(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	corrected := `{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}],"responsibility":"owns the refunds table"}
]}`
	p.setup(0, "", "--supplement", p.file(t, "sup2.json", corrected))
	if md := p.read("architecture/MOD-0001.md"); !strings.Contains(md, "owns the refunds table") {
		t.Fatalf("the corrected guess should replace the old one:\n%s", md)
	}
	if md := p.read("architecture/MOD-0002.md"); strings.Contains(md, "consumes") || strings.Contains(md, "keeps the books") {
		t.Fatalf("withdrawn guesses must leave the unit:\n%s", md)
	}
}

// A project commits .abbd/ with its code. Syncing again afterwards must leave
// that commit clean, or every commit would ask for another.
func TestCommittedStateStaysCleanOnResync(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "synced", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "state")
	p.want(0, "matched", "sync")
	out, err := exec.Command("git", "-C", p.root, "status", "--porcelain").CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Fatalf("a resync after committing .abbd/ should change nothing git sees, got %v:\n%s", err, out)
	}
}

// A setup's supplement owns the guesses, so it withdraws a recorded unit it no
// longer reports. A unit a human stated something about stays: a stated fact,
// a stated relation of its own, or a stated relation pointing at it would
// otherwise sit where explain and record cannot reach it.
func TestSetupKeepsARecordedUnitAHumanStatedAbout(t *testing.T) {
	p := semanticProject(t)
	src := "conversation 2026-11-01"
	p.want(0, "recorded DATA-0002", "record", "unit", "DATA", "ledger query", "--parent", "MOD-0002", "--evidence", "ledger/book.go:3:SELECT amount")
	p.want(0, "USER_STATED", "record", "fact", "DATA-0002.purpose", "--value", "reads charges for the books", "--source", src)
	p.want(0, "recorded DATA-0003", "record", "unit", "DATA", "ledger rows", "--parent", "MOD-0002", "--evidence", "ledger/book.go:4:FROM charges")
	p.want(0, "USER_STATED", "record", "relation", "MOD-0001", "consumes", "DATA-0003", "--source", src)
	p.want(0, "recorded DATA-0004", "record", "unit", "DATA", "charge call", "--parent", "MOD-0001", "--evidence", "billing/charge.go:4:func Charge")
	p.want(0, "USER_STATED", "record", "relation", "DATA-0004", "depends_on", "MOD-0002", "--source", src)
	p.want(0, "recorded DATA-0005", "record", "unit", "DATA", "unsaid", "--parent", "MOD-0002", "--evidence", "ledger/book.go:6:func Post")

	out := p.setup(0, "synced", "--supplement", p.file(t, "sup2.json", chargesTable))
	for _, id := range []string{"DATA-0002", "DATA-0003", "DATA-0004"} {
		if !strings.Contains(out, id+" kept") || strings.Contains(p.read("architecture/"+id+".md"), "absent: true") {
			t.Errorf("setup should keep %s, which a human stated something about, and say so:\n%s", id, out)
		}
	}
	if !strings.Contains(p.read("architecture/DATA-0005.md"), "absent: true") {
		t.Errorf("a recorded unit no human stated anything about is withdrawn with the supplement's guesses")
	}
	contains(t, p.want(0, "", "explain", "DATA-0002"), "purpose: reads charges for the books (USER_STATED, "+src+")")
	contains(t, p.want(0, "", "explain", "MOD-0001"), "DATA-0003 ledger rows (stated)")
	contains(t, p.want(0, "", "explain", "DATA-0004"), "MOD-0002 ledger (stated)")
}
