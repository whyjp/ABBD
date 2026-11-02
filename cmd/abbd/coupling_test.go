package main

import "testing"

func couplingProject(t *testing.T) (*project, string) {
	t.Helper()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\n// Post settles through billing.Charge()\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	return p, exe
}

// A coupling the script cannot see may join two units only the script
// reports, by their refs, without restating either in the supplement.
func TestASupplementCouplingNamesScriptUnitsByTheirRefs(t *testing.T) {
	p, exe := couplingProject(t)
	sup := `{"units":[],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"call",
  "evidence":[{"path":"ledger/book.go","lines":"3","quote":"billing.Charge()"}]}]}`
	p.setup(0, "synced: 2 unit(s)", "--supplement", p.file(t, "sup.json", sup), "--", exe)
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0002 ledger depends_on MOD-0001 billing via call")
}

func TestASupplementCouplingStillChecksItsEnds(t *testing.T) {
	p, exe := couplingProject(t)
	unknown := `{"units":[],"relations":[{"from":"ledger","to":"nowhere","kind":"depends_on","via":"call",
  "evidence":[{"path":"ledger/book.go","lines":"3","quote":"billing.Charge()"}]}]}`
	p.setup(1, `"nowhere"`, "--supplement", p.file(t, "unknown.json", unknown), "--", exe)
	wrongKind := `{"units":[],"relations":[{"from":"ledger","to":"billing","kind":"provides","via":"call",
  "evidence":[{"path":"ledger/book.go","lines":"3","quote":"billing.Charge()"}]}]}`
	p.setup(1, "is not a relation ABBD knows", "--supplement", p.file(t, "kind.json", wrongKind), "--", exe)
}
