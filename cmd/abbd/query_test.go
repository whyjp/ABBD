package main

import (
	"strings"
	"testing"
)

func contains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q:\n%s", w, out)
		}
	}
}

// A module's explanation names the units inside it, the couplings a reader
// guessed with the code they rest on, what a human stated, and how's text,
// marked STALE once the module's code moves on. It writes nothing.
func TestExplainShowsAModuleAtAGlance(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "USER_STATED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--source", "conversation 2026-11-01")
	before := snapshot(t, p.root)
	out := p.want(0, "MOD-0001 billing (MOD, EXTRACTED)", "explain", "MOD-0001")
	contains(t, out,
		"inside:\n  DATA-0001 charges table (DATA)\n",
		"used by:\n  MOD-0002 ledger depends_on (stated)\n",
		"inferred couplings:\n",
		"  MOD-0001 billing provides DATA-0001 charges table via data\n    billing/charge.go:3 \"INSERT INTO charges\"\n",
		"  MOD-0002 ledger consumes DATA-0001 charges table via data\n    ledger/book.go:3-4 \"SELECT amount FROM charges\"\n",
		"how: none; abbd understand MOD-0001 FILE",
	)
	if !equal(before, snapshot(t, p.root)) {
		t.Fatalf("explain must not write .abbd/")
	}

	text := p.file(t, "how.md", "### Overview\nbilling writes each charge to the charges table.\n")
	p.want(0, "understood", "understand", "MOD-0001", text)
	contains(t, p.want(0, "", "explain", "billing"), "how (INFERRED, CURRENT):\n  ### Overview\n  billing writes each charge to the charges table.\n")
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge(currency string) {}\n")
	p.resync()
	contains(t, p.want(0, "", "explain", "MOD-0001"), "how (INFERRED, STALE, the code changed since it was written):")
}

// A unit inside a module names its module and the evidence it rests on.
func TestExplainResolvesNamesAndShowsAnInsideUnit(t *testing.T) {
	p := semanticProject(t)
	contains(t, p.want(0, "", "explain", "charges table"),
		"DATA-0001 charges table (DATA, INFERRED)\ninside MOD-0001 billing\n",
		"evidence: billing/charge.go:3 \"INSERT INTO charges\"\n",
	)
	p.want(1, "close: DATA-0001 charges table", "explain", "charges")
	p.want(1, `"MOD-0099" is not a unit of the state`, "explain", "MOD-0099")
}

// The path is the fewest hops between two units, each with how it arises,
// and ends with the least certain origin on it.
func TestPathNamesEachHopAndTheWeakestOrigin(t *testing.T) {
	p := semanticProject(t)
	contains(t, p.want(0, "", "path", "ledger", "MOD-0001"),
		"MOD-0002 ledger\n  -consumes-> DATA-0001 charges table (inferred, via data)\n  <-provides- MOD-0001 billing (inferred, via data)\n2 hop(s), weakest origin INFERRED\n")

	p.want(0, "USER_STATED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--source", "conversation 2026-11-01")
	contains(t, p.want(0, "", "path", "MOD-0001", "MOD-0002"),
		"MOD-0001 billing\n  <-depends_on- MOD-0002 ledger (stated)\n1 hop(s), weakest origin USER_STATED\n")

	p.code("island/x.go", "package island\n\nfunc X() {}\n")
	p.extract(strings.Replace(ledgerTopo, "\n]}", `,
 {"key":"island","kind":"MOD","name":"island","anchors":[{"path":"island/x.go","symbol":"X"}]}]}`, 1))
	p.resync()
	p.want(0, "no path from MOD-0002 ledger to MOD-0003 island", "path", "MOD-0002", "island")
	p.want(1, "are the same unit", "path", "ledger", "MOD-0002")
}
