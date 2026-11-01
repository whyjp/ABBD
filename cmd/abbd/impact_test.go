package main

import (
	"strings"
	"testing"
)

// The impact follows a reader's coupling while its evidence lives, and only
// reads: it writes nothing.
func TestImpactFollowsLiveCouplingsAndWritesNothing(t *testing.T) {
	p := semanticProject(t)
	before := snapshot(t, p.root)
	out := p.want(0, "direct MOD-0001 billing: seed", "impact", "MOD-0001")
	for _, want := range []string{
		"direct DATA-0001 charges table: inside a seed",
		"direct MOD-0002 ledger: consumes DATA-0001 (inferred, via data)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if !equal(before, snapshot(t, p.root)) {
		t.Fatalf("impact must not write .abbd/")
	}
	p.want(1, "is not a unit of the state", "impact", "MOD-0001,MOD-0099")

	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.want(0, "", "sync")
	contains(t, p.want(0, "seed", "impact", "MOD-0001"), "direct MOD-0002 ledger: consumes DATA-0001 (inferred, via data, drifted)")
	p.laterRevision()
	p.want(0, "", "sync")
	if out := p.want(0, "seed", "impact", "MOD-0001"); strings.Contains(out, "MOD-0002") {
		t.Fatalf("a coupling whose evidence died leads nowhere:\n%s", out)
	}
}

// A file a module and a unit inside it both anchor seeds them both, so the
// impact reaches what the module depends on, however the two rank.
func TestImpactFromAFileSeedsEveryUnitAnchoredThere(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"rates","kind":"DATA","name":"rates file","parent":"billing","anchors":[{"path":"billing/charge.go"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
],"relations":[
 {"from":"billing","to":"ledger","kind":"depends_on"}
]}`)
	p.setup(0, "synced: 3 unit(s)", "--", exe)

	out := p.want(0, "", "impact", "billing/charge.go")
	for _, want := range []string{": seed\n", "billing: seed", "rates file: seed", "ledger: depended on by"} {
		if !strings.Contains(out, want) {
			t.Fatalf("impact from billing/charge.go must seed the module and its inside unit and reach ledger; missing %q:\n%s", want, out)
		}
	}
	out = p.want(0, "", "impact", "billing/charge.go, ledger/book.go")
	if !strings.Contains(out, "billing: seed") || !strings.Contains(out, "ledger: seed") {
		t.Fatalf("each comma-separated query seeds its own units:\n%s", out)
	}
}
