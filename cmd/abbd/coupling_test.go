package main

import (
	"strings"
	"testing"
)

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
	t.Parallel()
	p, exe := couplingProject(t)
	sup := `{"units":[],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"call",
  "evidence":[{"path":"ledger/book.go","lines":"3","quote":"billing.Charge()"}]}]}`
	p.setup(0, "synced: 2 unit(s)", "--supplement", p.file(t, "sup.json", sup), "--", exe)
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0002 ledger depends_on MOD-0001 billing via call")
}

func TestASupplementCouplingStillChecksItsEnds(t *testing.T) {
	t.Parallel()
	p, exe := couplingProject(t)
	unknown := `{"units":[],"relations":[{"from":"ledger","to":"nowhere","kind":"depends_on","via":"call",
  "evidence":[{"path":"ledger/book.go","lines":"3","quote":"billing.Charge()"}]}]}`
	p.setup(1, `"nowhere"`, "--supplement", p.file(t, "unknown.json", unknown), "--", exe)
	wrongKind := `{"units":[],"relations":[{"from":"ledger","to":"billing","kind":"provides","via":"call",
  "evidence":[{"path":"ledger/book.go","lines":"3","quote":"billing.Charge()"}]}]}`
	p.setup(1, "is not a relation ABBD knows", "--supplement", p.file(t, "kind.json", wrongKind), "--", exe)
}

// The supplement may restate a script unit under a ref of its own. A
// coupling from that ref to the script's ref is the unit to itself.
func TestASupplementCouplingMayNotJoinAUnitToItself(t *testing.T) {
	t.Parallel()
	p, exe := couplingProject(t)
	self := `{"units":[{"ref":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}],
  "relations":[{"from":"l","to":"ledger","kind":"depends_on","via":"call",
  "evidence":[{"path":"ledger/book.go","lines":"3","quote":"billing.Charge()"}]}]}`
	p.setup(1, `relates MOD "ledger" to itself`, "--supplement", p.file(t, "self.json", self), "--", exe)
}

// A supplement ref that names another unit than the script's same ref
// changes what every coupling by that ref joins, so setup says so.
func TestSetupWarnsWhenASupplementRefShadowsAScriptRef(t *testing.T) {
	t.Parallel()
	p, exe := couplingProject(t)
	shadow := `{"units":[{"ref":"billing","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}]}`
	out := p.setup(0, "synced: 2 unit(s)", "--supplement", p.file(t, "shadow.json", shadow), "--", exe)
	contains(t, out, `supplement ref "billing" names MOD "ledger", which shadows the script's ref "billing" for MOD "billing"`)
	same := `{"units":[{"ref":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}]}`
	if out := p.setup(0, "synced: 2 unit(s)", "--supplement", p.file(t, "same.json", same), "--", exe); strings.Contains(out, "shadows") {
		t.Errorf("a ref that restates the script's unit under the same ref shadows nothing:\n%s", out)
	}
}
