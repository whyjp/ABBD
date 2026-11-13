package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBriefIsTheSkeletonAndReadingListOfAModule(t *testing.T) {
	p := semanticProject(t)
	before := snapshot(t, p.root)
	out := p.want(0, "## Read first", "brief", "MOD-0001")
	for _, want := range []string{"## Overview\n", "## Key Concepts\n", "- TODO **charges table**", "DATA-0001", "### Files, entry first", "- `billing/charge.go` (", "### Boxes that say only their kind"} {
		if !strings.Contains(out, want) {
			t.Errorf("brief does not hold %q:\n%s", want, out)
		}
	}
	if !equal(before, snapshot(t, p.root)) {
		t.Error("brief wrote to .abbd; it only reads")
	}
	p.want(1, "is not a module or feature", "brief", "DATA-0001")

	p.want(1, "line 3 still says \"TODO write this last", "understand", "MOD-0001", p.file(t, "brief.md", out))
	_, rest, _ := strings.Cut(out, "## Read first")
	p.want(1, `"Read first" goes to nowhere`, "understand", "MOD-0001", p.file(t, "brief.md", "## Overview\n\nbilling.\n\n## Read first"+rest))
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() { _ = 1 }\n")
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling writes the charges table.\n"))
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() { _ = 2 }\n")
	p.want(0, "### Moved since the stored text\n\n- billing/charge.go", "brief", "MOD-0001")
}

func TestUnderstandRefusesWhatTheBriefLeftAndSectionsItDoesNotHold(t *testing.T) {
	p := semanticProject(t)
	for text, want := range map[string]string{
		"## Overview\n\nbilling charges.\n\n## Key Concepts\n\n- TODO **charges table**: DATA DATA-0001\n": `line 7 still says "- TODO **charges table**: DATA DATA-0001"`,
		"## Overview\n\nbilling charges.\n\n## How It Works\n\nfirst it charges.\n":                        `"How It Works" goes to an ordered flow, abbd flow KEY FILE`,
		"# Billing\n\nbilling charges.\n": `"Billing" goes to a ### heading inside Overview or Key Concepts`,
	} {
		p.want(1, want, "understand", "MOD-0001", p.file(t, "how.md", text))
	}
	p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling charges.\n\n### Retries\n\nnone.\n\n## Key Concepts\n\n- **TODO list** (`billing/charge.go`): not a TODO line, the charges table.\n"))
}

func TestUnderstandReportsUnnamedInsideUnitsAndOtherUnitsFiles(t *testing.T) {
	p := semanticProject(t)
	out := p.want(0, "unmentioned: MOD-0001's text never names DATA-0001 charges table", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling charges cards and posts to `ledger/book.go`, beside `billing/charge.go` and `owner/repo`.\n"))
	if !strings.Contains(out, "names: MOD-0001's text names ledger/book.go, code of MOD-0002") {
		t.Errorf("understand did not report a path of another module:\n%s", out)
	}
	if strings.Contains(out, "names billing/charge.go") || strings.Contains(out, "owner/repo") {
		t.Errorf("understand reported its own code or a path no unit holds:\n%s", out)
	}
	out = p.want(0, "unmentioned: 1 line(s); abbd status --verbose lists them", "status", "--check")
	if !strings.Contains(out, "names: 1 line(s)") {
		t.Errorf("status did not fold the names: report:\n%s", out)
	}
	p.want(0, "names: MOD-0001's text names ledger/book.go", "status", "--verbose")

	out = p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling writes the Charges Table.\n"), "--reader", "  abbd reader ")
	if strings.Contains(out, "unmentioned:") || strings.Contains(out, "names:") {
		t.Errorf("understand reported a text that names its units and no other code:\n%s", out)
	}
	if !strings.Contains(p.read("understanding/MOD-0001.md"), "reader: abbd reader\n") {
		t.Errorf("--reader was not stored trimmed:\n%s", p.read("understanding/MOD-0001.md"))
	}
}

// Lint and the brief name the boxes that say only their kind, and the
// brief drafts each one's responsibility from the comment at its anchors.
func TestLintAndBriefNameTheBoxesThatSayOnlyTheirKind(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "lint: described: ", "lint")
	p.want(0, `"undescribed": [`, "lint", "--json")
	p.code("billing/charge.go", "package billing\n\nfunc Charge() { _ = 0 }\n")
	p.want(0, "MOD-0001 billing: no comment at its anchors says what it is for; read `billing/charge.go`", "brief", "MOD-0001")
	p.code("billing/charge.go", "package billing\n\n// Charge bills a card once. It never retries.\nfunc Charge() { _ = 1 }\n")
	p.want(0, "\"Charge bills a card once.\" (from `billing/charge.go`)", "brief", "MOD-0001")
}

const hypothesisTopo = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"ChargeCard"}]},
 {"key":"charger","kind":"MOD","name":"charger","parent":"billing","anchors":[{"path":"billing/charger.go","symbol":"Run"}]},
 {"key":"web","kind":"MOD","name":"web","anchors":[{"path":"web/a.go"}]}
]}`

const hypothesisSup = `{"units":[
 {"key":"c","kind":"MOD","name":"charger","anchors":[{"path":"billing/charger.go","symbol":"Run"}],"responsibility":"Charges every card twice"}
]}`

// The brief hands the reader hypotheses, not prose: no TODO carries a code
// span or a stored sentence, a stored line setup wrote without evidence says
// so, anchors are files to start from, the names other modules use most are
// candidates, and the Overview asks for no negative.
func TestBriefOffersHypothesesNotProse(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\ntype Ledger struct{}\n\nfunc ChargeCard() {}\n")
	p.code("billing/charger.go", "package billing\n\nfunc Run() { ChargeCard() }\n")
	p.code("web/a.go", "package web\n\nvar _ = billing.ChargeCard\n")
	p.code("web/b.go", "package web\n\nvar _ = billing.ChargeCard\nvar _ billing.Ledger\n")
	git(t, p.root, "add", "-A")
	p.extract(hypothesisTopo)
	p.setup(0, "synced", "--supplement", p.file(t, "sup.json", hypothesisSup), "--", exe)
	out := p.want(0, "## Read first", "brief", "MOD-0001")
	for _, want := range []string{
		"- TODO **charger**: MOD MOD-0002\n",
		"### Files to start from and stored lines, by unit",
		"- **charger** MOD-0002: start from `billing/charger.go`; stored line, unverified (setup wrote it without evidence): confirm or drop: \"Charges every card twice\"",
		"### Names other modules use most",
		"- `ChargeCard` in `billing/charge.go`, named in 2 file(s) outside the module",
		"- `Ledger` in `billing/charge.go`, named in 1 file(s) outside the module",
		"the text says nothing about this brief",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("brief does not hold %q:\n%s", want, out)
		}
	}
	skeleton, _, _ := strings.Cut(out, "## Read first")
	for _, not := range []string{"does not do", "`", "Charges every card twice", "Write all prose in"} {
		if strings.Contains(skeleton, not) {
			t.Errorf("the skeleton holds %q:\n%s", not, skeleton)
		}
	}
	if strings.Contains(out, "Write all prose in") {
		t.Errorf("brief names a language project.yaml does not set:\n%s", out)
	}

	cfg := filepath.Join(p.root, ".abbd", "project.yaml")
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	write(t, cfg, string(b)+"language: ko\n")
	p.want(0, "## Read first\n\nWrite all prose in ko; keep code identifiers, paths and quotes verbatim.\n", "brief", "MOD-0001")
}

// A text that leaves out a name other modules use most is not asked for it:
// those names are candidates the reader may skip without a word.
func TestUnderstandDoesNotAskForTheNamesOtherModulesUse(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\ntype Ledger struct{}\n\nfunc ChargeCard() {}\n")
	p.code("billing/charger.go", "package billing\n\nfunc Run() { ChargeCard() }\n")
	p.code("web/a.go", "package web\n\nvar _ = billing.ChargeCard\nvar _ billing.Ledger\n")
	git(t, p.root, "add", "-A")
	p.extract(hypothesisTopo)
	p.setup(0, "synced", "--", exe)
	p.want(0, "`ChargeCard` in `billing/charge.go`", "brief", "MOD-0001")
	out := p.want(0, "understood", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling charges cards.\n\n## Key Concepts\n\n- **charger** (`billing/charger.go`): runs a charge.\n"))
	if strings.Contains(out, "unmentioned:") {
		t.Errorf("understand asked for a name the brief only offers:\n%s", out)
	}
}
