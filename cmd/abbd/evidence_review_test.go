package main

import (
	"strings"
	"testing"
)

// POSIX `for NAME do ...` has no `in` and no `;` before do, so the whole loop
// is one segment that simpleCommand drops as a header.
func TestGuardReadsTheBodyOfAForLoopWithoutIn(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	for _, c := range []string{
		`set -- 1; for x do abbd approve K; done`,
		`set -- 1; for x do cp evil .abbd/intended.yaml; done`,
		`set -- 1; for x do rm -rf .abbd; done`,
		`bash -c 'for x do abbd approve K; done' _ 1`,
		`if true; then for x do abbd confirm MOD-0001.depends_on.MOD-0002; done; fi`,
	} {
		p.wantDenied(t, true, p.bash(c))
	}
}

// Line drift within the slack is never written back, so two small edits
// above a confirmed coupling's quote kill it and block with a grade C row
// although the code still holds the quote.
func TestEvidenceFollowsAQuoteThatDriftsAcrossSyncs(t *testing.T) {
	p, exe := semanticProject(t)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	p.want(0, "aligned", "align", "--all")
	p.want(0, "approved new-relation:MOD-0002.consumes.DATA-0001", "confirm", "MOD-0002.consumes.DATA-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.want(0, "off", "mode", "off", "--session", "s1")

	p.code("ledger/book.go", "package ledger\n\n// a\n// b\n// c\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	_, out := p.cmd("", "mode", "on", "--session", "s1")
	if strings.Contains(out, "removed-relation") {
		t.Fatalf("3 lines of drift is within the slack:\n%s", out)
	}
	p.cmd("", "mode", "off", "--session", "s1")
	p.code("ledger/book.go", "package ledger\n\n// a\n// b\n// c\n// d\n// e\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	_, out = p.cmd("", "mode", "on", "--session", "s1")
	if strings.Contains(out, "removed-relation:MOD-0002.consumes.DATA-0001") {
		t.Errorf("the quote is still in ledger/book.go, 2 lines from where the last sync found it, yet:\n%s", out)
	}
}

// An agent's proposal replaces a human-confirmed relation once its evidence
// is dead, erasing the confirmation instead of refusing.
func TestProposeKeepsAHumansConfirmation(t *testing.T) {
	p, exe := semanticProject(t)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	p.want(0, "aligned", "align", "--all")
	p.want(0, "approved", "confirm", "MOD-0002.consumes.DATA-0001")
	p.want(0, "active", "mode", "on", "--session", "s1")
	p.want(0, "off", "mode", "off", "--session", "s1")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.cmd("", "mode", "on", "--session", "s1")
	code, out := p.cmd("", "propose", "relation", "MOD-0002", "consumes", "DATA-0001", "--via", "data", "--evidence", "ledger/book.go:3:func Post")
	md := p.read("architecture/MOD-0002.md")
	if code == 0 && !strings.Contains(md, "confirmation: CONFIRMED") {
		t.Errorf("propose replaced the human's CONFIRMED relation (exit %d: %s):\n%s", code, out, md)
	}
}
