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
	t.Parallel()
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "synced: 3 unit(s)", "--", exe)
	if !strings.Contains(p.read("project.yaml"), filepath.Base(exe)) {
		t.Fatalf("setup should record the extraction script:\n%s", p.read("project.yaml"))
	}
	if md := p.read("architecture/MOD-0001.md"); !strings.Contains(md, "takes payments\n    writer: supplement") || strings.Contains(md, "confirmation") {
		t.Fatalf("a purpose the script guessed is INFERRED and waits for nobody:\n%s", md)
	}
}

func TestSupplementAddsInferredFactsThatSurviveReextraction(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "3 matched by the supplement", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	md := p.read("architecture/MOD-0002.md")
	if !strings.Contains(md, "keeps the books\n    writer: supplement") || !strings.Contains(md, "to: IF-0001") || !strings.Contains(md, "quote: func Serve()") {
		t.Fatalf("the supplement should add an inferred purpose and a coupling with its evidence:\n%s", md)
	}
	p.want(0, "MOD-0002 matched", "sync", "--verbose")
	if !strings.Contains(p.read("architecture/MOD-0002.md"), "kind: consumes") {
		t.Fatalf("re-extraction by the script must keep a coupling the script cannot see")
	}
}

func TestASupplementCouplingNeedsEvidence(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(codeOnly)
	bare := strings.Replace(supplement, `,"via":"call","evidence":[{"path":"api/http.go","quote":"func Serve()"}]`, "", 1)
	p.setup(1, "l -consumes-> a cites no evidence", "--supplement", p.file(t, "sup.json", bare), "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must write nothing")
	}
}

func TestSupplementNamingUnknownCodeIsRefused(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(codeOnly)
	bad := `{"units":[{"key":"x","kind":"MOD","name":"ghost","anchors":[{"path":"ghost/x.go","symbol":"X"}],"purpose":"haunts"}]}`
	p.setup(1, "fix the script", "--supplement", p.file(t, "sup.json", bad), "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd")); err == nil {
		t.Fatalf("a refused setup must write nothing")
	}
}

func TestSetupAgainOnUnchangedCodeRewritesNothing(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	p, exe := bare(t)
	p.extract(codeOnly)
	write(t, filepath.Join(p.root, ".abbd", "extractor", "supplement.json"), supplement)
	p.setup(0, "3 matched by the supplement", "--supplement", ".abbd/extractor/supplement.json", "--", exe)
}

func TestCorrectedSupplementWithdrawsEarlierGuesses(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// A setup's supplement owns only its own claims. Re-running setup never
// withdraws what a reader recorded with abbd record. It withdraws a
// supplement unit it no longer reports, with a reason, unless a human stated
// something about it, which would otherwise sit where no command reaches it.
func TestSetupKeepsReaderFindingsAndAUnitAHumanStatedAbout(t *testing.T) {
	t.Parallel()
	src := "conversation 2026-11-01"
	p := semanticProject(t)
	p.want(0, "recorded DATA-0002", "record", "unit", "DATA", "ledger query", "--parent", "MOD-0002", "--evidence", "ledger/book.go:3:SELECT amount")
	p.want(0, "INFERRED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:6:func Post()")
	out := p.setup(0, "synced", "--supplement", p.file(t, "sup2.json", ledgerTopo))
	if md := p.read("architecture/DATA-0002.md"); strings.Contains(md, "absent") {
		t.Errorf("setup withdrew a unit a reader recorded:\n%s\n%s", out, md)
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0002 ledger depends_on MOD-0001 billing via call")
	if md := p.read("architecture/DATA-0001.md"); !strings.Contains(md, "absent_reason: withdrawn") {
		t.Errorf("a supplement unit the supplement no longer reports is withdrawn with a reason:\n%s", md)
	}

	p = semanticProject(t)
	p.want(0, "USER_STATED", "record", "fact", "DATA-0001.purpose", "--value", "the books read it", "--source", src)
	out = p.setup(0, "synced", "--supplement", p.file(t, "sup3.json", ledgerTopo))
	if !strings.Contains(out, "DATA-0001 kept") || strings.Contains(p.read("architecture/DATA-0001.md"), "absent") {
		t.Errorf("setup should keep DATA-0001, which a human stated something about, and say so:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "DATA-0001"), "purpose: the books read it (USER_STATED, "+src+")")
}

// A script's or a reader's sentence about a module is stamped with the
// module's code, and its page marks it STALE once that code changes.
func TestASentenceGoesStaleWithItsCode(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.extract(codeOnly)
	p.setup(0, "synced", "--", exe)
	if md := p.read("architecture/MOD-0001.md"); !strings.Contains(md, "takes payments\n    writer: supplement\n    digest: ") {
		t.Fatalf("the purpose should be stamped with its module's code:\n%s", md)
	}
	stale := `왜 있는가</strong> takes payments <span class="pill`
	if page := p.read("blueprint/MOD-0001.html"); !strings.Contains(page, stale) || strings.Contains(page, "이 문장을 쓴 뒤") {
		t.Fatalf("a fresh sentence reads without STALE")
	}
	p.code("billing/charge.go", "package billing\n\nfunc Charge() { retry() }\n")
	p.want(0, "", "sync")
	if page := p.read("blueprint/MOD-0001.html"); !strings.Contains(page, "이 문장을 쓴 뒤 이 단위의 코드가 바뀌었습니다") {
		t.Errorf("a sentence whose module's code changed should read STALE")
	}
}

// A reader who reads a module again says its one line with the text, and
// the line reads fresh against the code it was read with.
func TestAReaderRewritesTheSentenceWithTheText(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.extract(codeOnly)
	p.setup(0, "synced", "--", exe)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() { retry() }\n")
	p.want(0, "", "sync")
	text := p.file(t, "t.md", "### Overview\nbilling charges cards and retries.\n")
	p.want(0, `responsibility of MOD-0001 is now "charges cards, retrying a failure"`, "understand", "MOD-0001", text, "--sentence", "charges cards, retrying a failure")
	p.want(0, "", "sync")
	_, role, _ := strings.Cut(p.read("blueprint/MOD-0001.html"), "<strong>맡은 것</strong>")
	role, _, _ = strings.Cut(role, "</p>")
	if !strings.Contains(role, "charges cards, retrying a failure") || strings.Contains(role, "STALE") {
		t.Errorf("the rewritten sentence should read fresh: %s", role)
	}
}

// A person's responsibility outranks a reader's, so --sentence leaves it.
func TestASentenceLeavesAPersonsLine(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.extract(codeOnly)
	p.setup(0, "synced", "--", exe)
	p.want(0, "", "record", "fact", "MOD-0001.responsibility", "--value", "charges cards", "--source", "pr 3")
	text := p.file(t, "t.md", "### Overview\nbilling charges.\n")
	p.want(0, "stays as a person stated it", "understand", "MOD-0001", text, "--sentence", "takes money")
	if md := p.read("architecture/MOD-0001.md"); !strings.Contains(md, "charges cards") || strings.Contains(md, "takes money") {
		t.Errorf("the person's line should stay:\n%s", md)
	}
	p.want(1, "--sentence goes with", "understand", "MOD-0001", "--restamp", "--sentence", "x")
}
