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
	t.Parallel()
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
	t.Parallel()
	p := semanticProject(t)
	contains(t, p.want(0, "", "explain", "charges table"),
		"DATA-0001 charges table (DATA, INFERRED)\ninside MOD-0001 billing\n",
		"evidence: billing/charge.go:3 \"INSERT INTO charges\"\n",
	)
	p.want(1, `"MOD-0099" is not a unit of the state`, "explain", "MOD-0099")
}

// An agent asks with what it has in hand: a file it is about to change, a
// symbol, or a few words. explain names the units that match and why, best
// first, then explains the best. Whatever names the unit itself comes first.
func TestExplainAcceptsAFileASymbolOrAPhrase(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	contains(t, p.want(0, "", "explain", "billing/charge.go"),
		"\"billing/charge.go\" is not a unit; it matches, best first:\n"+
			"  DATA-0001 charges table: anchor billing/charge.go\n"+
			"  MOD-0001 billing: anchor billing/charge.go\n"+
			"\nDATA-0001 charges table (DATA, INFERRED)\n")
	contains(t, p.want(0, "", "explain", `.\ledger\book.go:6`), "  MOD-0002 ledger: anchor ledger/book.go\n\nMOD-0002 ledger (MOD, EXTRACTED)\n")
	contains(t, p.want(0, "", "explain", "book"), "  MOD-0002 ledger: anchor ledger/book.go\n")
	contains(t, p.want(0, "", "explain", "Post"), "  MOD-0002 ledger: symbol Post at ledger/book.go\n")
	contains(t, p.want(0, "", "explain", "charges"), "  DATA-0001 charges table: name has \"charges\"\n", "\nDATA-0001 charges table (DATA, INFERRED)\n")
	contains(t, p.want(0, "", "explain", "select amount"), "  MOD-0002 ledger: quote has \"select\", \"amount\"\n")
	contains(t, p.want(0, "", "explain", "ledger"), "MOD-0002 ledger (MOD, EXTRACTED)\n")
	if out := p.want(0, "", "explain", "ledger"); strings.Contains(out, "is not a unit") {
		t.Fatalf("a unit's own name explains it without a match list:\n%s", out)
	}
	p.want(1, `"zebra" names no unit, file, symbol or phrase of the state; nearest: MOD-0002 ledger`, "explain", "zebra")
}

// A file no anchor names is found through the directory anchor that holds
// it, innermost module first.
func TestExplainFindsAFileThroughItsDirectoryAnchor(t *testing.T) {
	t.Parallel()
	p, out := nestedProject(t, `{"units":[]}`)
	if p.setupCode(out) != 0 {
		t.Fatalf("setup:\n%s", out)
	}
	p.code("app/sub/y.py", "pass\n")
	contains(t, p.want(0, "", "explain", "y"),
		"  MOD-0002 app/sub: anchor app/sub holds app/sub/y.py\n  MOD-0001 app: anchor app/sub holds app/sub/y.py\n")
}

// path and impact take the same inputs, and say what each one matched.
func TestPathAndImpactAcceptFilesAndPhrases(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	contains(t, p.want(0, "", "path", "ledger/book.go", "charges"),
		"\"ledger/book.go\" is not a unit; it matches, best first:\n  MOD-0002 ledger",
		"MOD-0002 ledger\n  -consumes-> DATA-0001 charges table (inferred, via data)\n1 hop(s)")
	contains(t, p.want(0, "", "impact", "ledger/book.go"), "direct MOD-0002 ledger: seed\n")
}

// The path is the fewest hops between two units, each with how it arises,
// and ends with the least certain origin on it.
func TestPathNamesEachHopAndTheWeakestOrigin(t *testing.T) {
	t.Parallel()
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

	p.want(0, "USER_STATED", "record", "relation", "MOD-0003", "consumes", "DATA-0001", "--source", "conversation 2026-11-01")
	contains(t, p.want(0, "", "path", "island", "ledger"),
		"MOD-0003 island\n  -consumes-> DATA-0001 charges table (stated)\n  <-provides- MOD-0001 billing (inferred, via data)\n  <-depends_on- MOD-0002 ledger (stated)\n3 hop(s)")
}

// Two units that only consume the same contract are not joined by it, and
// the path says so where they meet.
func TestPathMarksTwoConsumersThatOnlyShareAContract(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.code("island/x.go", "package island\n\nfunc X() {}\n")
	p.extract(strings.Replace(ledgerTopo, "\n]}", `,
 {"key":"island","kind":"MOD","name":"island","anchors":[{"path":"island/x.go","symbol":"X"}]}]}`, 1))
	p.resync()
	p.want(0, "USER_STATED", "record", "relation", "MOD-0003", "consumes", "DATA-0001", "--source", "conversation 2026-11-01")
	contains(t, p.want(0, "", "path", "island", "ledger"),
		"MOD-0003 island\n  -consumes-> DATA-0001 charges table (stated)\n  <-consumes- MOD-0002 ledger (inferred, via data)\n"+
			"      both consume DATA-0001 charges table, which MOD-0001 billing provides; no relation joins MOD-0003 and MOD-0002\n")
}

// diff counts an inside unit's relation to another module as its module's;
// explain of the module lists it too, marked with the inside unit it comes from.
func TestExplainModuleListsItsInsideUnitsLinksToOtherModules(t *testing.T) {
	t.Parallel()
	p, base := diffProject(t)
	p.want(0, "+ relation MOD-0001 depends_on MOD-0004", "diff", "--base", base)
	contains(t, p.want(0, "MOD-0001 billing", "explain", "MOD-0001"),
		"depends on:\n  MOD-0004 notify (extracted) via inside MOD-0003 charger\n")
	contains(t, p.want(0, "", "explain", "MOD-0004"),
		"used by:\n  MOD-0003 charger depends_on (extracted)\n")
}
