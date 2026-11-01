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
	if out := p.want(0, "seed", "impact", "MOD-0001"); strings.Contains(out, "MOD-0002") {
		t.Fatalf("a coupling whose evidence is gone leads nowhere:\n%s", out)
	}
}
