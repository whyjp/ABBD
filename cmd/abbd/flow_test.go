package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chargeFlow is how a charge reaches the ledger: billing writes it, retries
// it, and the ledger reads it. Its steps cite the code each runs.
const chargeFlow = `---
question: How does a charge reach the ledger?
steps:
  - unit: MOD-0001
    does: Charge() inserts the charge row
    calls: [DATA-0001]
    evidence:
      - {path: billing/charge.go, lines: "3", quote: "INSERT INTO charges"}
  - unit: MOD-0001
    does: retries a failed insert
    fails: gives up after three tries
    evidence:
      - {path: billing/retry.go, lines: "3", quote: "const tries = 3"}
  - unit: MOD-0002
    does: Post() reads the charge into the book
    note: reads every charge, no cache
    calls: [DATA-0001]
    evidence:
      - {path: ledger/book.go, lines: "3-4", quote: "SELECT amount FROM charges"}
---

From the insert to the book.
`

func (p *project) flow(code int, contains, draft string, extra ...string) string {
	p.t.Helper()
	return p.want(code, contains, append([]string{"flow", "charge", p.file(p.t, "flow.md", draft)}, extra...)...)
}

// A flow is stored with each step stamped on the code it cites, and the page
// draws it as numbered steps with the step's unit, note, failure and calls.
func TestAFlowIsStoredAndDrawnInOrder(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored flow charge, 3 steps, INFERRED", chargeFlow)
	stored := p.read("flows/charge.md")
	for _, want := range []string{"question: How does a charge reach the ledger?", "writer: reader", "origin: INFERRED", "code:", "From the insert to the book."} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored flow must hold %q:\n%s", want, stored)
		}
	}
	p.want(0, "flows: 1 flow(s), 3 step(s), 0 STALE", "status", "--check")
	page := p.read("blueprint/index.html")
	for _, want := range []string{`id="flow-charge"`, `id="flow-charge.3"`, "Post() reads the charge into the book", "gives up after three tries", "→ charges table", `href="#m-MOD-0002"`, `<a href="#flow-charge">How does a charge reach the ledger?</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page must draw %q", want)
		}
	}
	if out := p.want(0, "question: How does a charge reach the ledger?", "flow", "charge"); !strings.Contains(out, "const tries = 3") || strings.Contains(out, "code:") {
		t.Errorf("abbd flow KEY prints the stored flow as a draft, without what abbd sets:\n%s", out)
	}
}

// An edit near one step's quote stales that step alone, and the check fails
// until the flow is read again; the same steps on the moved code are refused.
func TestAFlowStepGoesStaleOnItsOwn(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	out := p.want(1, "1 flow step(s) STALE", "status", "--check")
	if !strings.Contains(out, "stale: flow charge step 2 (MOD-0001): billing/retry.go (the quote is gone") || strings.Contains(out, "step 1 (") || strings.Contains(out, "step 3 (") {
		t.Errorf("only step 2 cites the changed code:\n%s", out)
	}
	p.flow(1, "the code does not hold", chargeFlow)
	p.flow(0, "stored", strings.Replace(chargeFlow, "const tries = 3", "const tries = 5", 1))
	p.want(0, "flows: 1 flow(s), 3 step(s), 0 STALE", "status", "--check")

	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}")+"// a new last line\n")
	p.code("billing/retry.go", strings.Replace(retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"), "// line 4\n", "var backoff = 2\n", 1))
	p.resync()
	p.want(1, "flow charge step 2", "status", "--check")
	p.flow(1, "read it again", strings.Replace(chargeFlow, "const tries = 3", "const tries = 5", 1))
}

// A draft enters only as an ordered walk through the state: a question, two
// steps or more, quotes the code holds once, units the state holds, and
// calls that ride a stored relation.
func TestAFlowDraftIsCheckedBeforeItIsStored(t *testing.T) {
	p := notesProject(t)
	p.code("billing/dup.go", "package billing\n// retry\n// retry\n")
	for _, c := range []struct{ from, to, want string }{
		{"question: How does a charge reach the ledger?", "question: ''", "one question"},
		{"  - unit: MOD-0002", "  - unit: MOD-0009", "MOD-0009 is no longer in the state"},
		{"calls: [DATA-0001]\n    evidence:\n      - {path: billing/charge.go", "calls: [MOD-0002]\n    evidence:\n      - {path: billing/charge.go", "no live relation joins MOD-0001 to MOD-0002"},
		{"const tries = 3", "const tries = 4", "the code does not hold"},
		{`{path: billing/retry.go, lines: "3", quote: "const tries = 3"}`, `{path: billing/dup.go, lines: "2", quote: "retry"}`, "more than once"},
		{"    does: retries a failed insert\n", "    does: ''\n", "says what it does"},
	} {
		draft := strings.Replace(chargeFlow, c.from, c.to, 1)
		if draft == chargeFlow {
			t.Fatalf("the case %q changes nothing", c.want)
		}
		p.flow(1, c.want, draft)
	}
	one := chargeFlow[:strings.Index(chargeFlow, "  - unit: MOD-0001\n    does: retries")] + "---\n"
	p.flow(1, "two steps or more", one)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "flows", "charge.md")); err == nil {
		t.Error("a refused draft must store nothing")
	}
}

// A human's flow stays until its code changes: a reader may not replace a
// current one, and replacing a stale one drops the human's word.
func TestAReaderReplacesAHumansFlowOnlyOnceItIsStale(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored flow charge, 3 steps, USER_STATED", chargeFlow, "--source", "conversation 2026-11-03")
	p.flow(1, "only a human replaces it", strings.Replace(chargeFlow, "reads every charge", "reads each charge", 1))
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	p.flow(0, "dropped the flow the human stated (conversation 2026-11-03)", strings.Replace(chargeFlow, "const tries = 3", "const tries = 5", 1))
	if stored := p.read("flows/charge.md"); !strings.Contains(stored, "writer: reader") || strings.Contains(stored, "source:") {
		t.Errorf("the reader's flow must replace the human's:\n%s", stored)
	}
}

// A flow whose unit left the state fails the check, since storing the flow
// again brings it current.
func TestAFlowOnAUnitThatLeftFailsTheCheck(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	p.extract(`{"units":[{"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}]}`)
	p.resync()
	p.want(1, "MOD-0002 is no longer in the state", "status", "--check")
}
