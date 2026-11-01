package main

import (
	"strings"
	"testing"
)

func activeSemanticProject(t *testing.T) *project {
	t.Helper()
	p, exe := semanticProject(t)
	p.setup(0, "bootstrapped", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	p.want(0, "aligned", "align", "--all")
	p.want(0, "active", "mode", "on", "--session", "s1")
	return p
}

// An unconfirmed coupling never gates, but a turn that touches one end must
// read the other.
func TestImpactFollowsLiveSemanticCouplings(t *testing.T) {
	p := activeSemanticProject(t)
	out := p.want(0, "OPEN", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "rename the amount column")
	for _, want := range []string{
		"impact DATA-0001: provided by MOD-0001 (semantic, via data) (computed, direct)",
		"impact MOD-0002: consumes DATA-0001 (semantic, via data) (computed, direct)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("the impact should follow the live couplings and say they are semantic; missing %q:\n%s", want, out)
		}
	}
	p.want(0, "CLOSED", "turn", "end", "--session", "s1")

	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.want(0, "active", "mode", "on", "--session", "s1")
	out = p.want(0, "OPEN", "turn", "start", "--session", "s1", "--seeds", "MOD-0001", "--request", "x")
	if strings.Contains(out, "MOD-0002") {
		t.Fatalf("a coupling whose evidence is gone leads nowhere:\n%s", out)
	}
}

// A seed's change most likely breaks where it meets what it depends on, so
// those units are direct even though nothing depends on the seed.
func TestASeedsOwnDependenciesAreDirect(t *testing.T) {
	p := activeTurnProject(t, topoConsumed)
	out := p.start("MOD-0003")
	if !strings.Contains(out, "impact IF-0001: consumed by MOD-0003 (computed, direct)") {
		t.Fatalf("the contract the seed consumes should be direct impact:\n%s", out)
	}
	if strings.Contains(out, "MOD-0001") {
		t.Fatalf("what the seed uses is read where they meet, not followed further:\n%s", out)
	}
}
