package main

import (
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

	p.want(1, "line 3 still says \"TODO what the module is responsible for", "understand", "MOD-0001", p.file(t, "brief.md", out))
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

// A text's room follows the module's tier: a small module's sections hold
// MaxSection characters and its Key Concepts bulletsSmall bullets, and a
// module past largeUnits units or largeFiles files gets more.
func TestATextsRoomFollowsTheModulesTier(t *testing.T) {
	small := tier{Chars: 2000, Bullets: 10}
	large := tier{Large: true, Chars: 3000, Bullets: 15}
	var kc strings.Builder
	kc.WriteString("## Overview\n\n" + strings.Repeat("a", 2500) + "\n\n## Key Concepts\n\n")
	for range 12 {
		kc.WriteString("- **x** (`x.go`): y.\n  - nested\n")
	}
	text := kc.String()
	if n := bullets(text); n != 12 {
		t.Fatalf("bullets = %d, want 12 top-level", n)
	}
	var out strings.Builder
	if long, _ := readText(&out, "MOD-0001", text, small); long != 2 {
		t.Errorf("a small module's text reported %d long marks, want the section and the bullets:\n%s", long, out.String())
	}
	out.Reset()
	if long, _ := readText(&out, "MOD-0001", text, large); long != 0 {
		t.Errorf("a large module's text reported %d long marks, want none:\n%s", long, out.String())
	}
	p := semanticProject(t)
	p.want(0, "### Length\n\n- small module (1 inside units, 1 files", "brief", "MOD-0001")
}
