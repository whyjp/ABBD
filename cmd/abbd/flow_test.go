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
      - {path: billing/charge.go, lines: "3", symbol: Charge, quote: "INSERT INTO charges"}
  - unit: MOD-0001
    does: retries a failed insert
    fails: gives up after three tries
    evidence:
      - {path: billing/retry.go, lines: "3", quote: "const tries = 3"}
      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}
  - unit: MOD-0002
    does: Post() reads the charge into the book
    note: reads every charge, no cache
    calls: [DATA-0001]
    evidence:
      - {path: ledger/book.go, lines: "3-4", quote: "SELECT amount FROM charges"}
      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}
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
	for _, want := range []string{"question: How does a charge reach the ledger?", "writer: reader", "code:", "From the insert to the book."} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored flow must hold %q:\n%s", want, stored)
		}
	}
	p.want(0, "flows: 1 flow(s), 3 step(s), 0 STALE", "status", "--check")
	page := p.blueprint()
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
	if !strings.Contains(out, "read that code again and store the flow with abbd flow charge FILE") || strings.Contains(out, "to drop it") {
		t.Errorf("status must say to read the code and store the flow again, not to delete the flow to clear a STALE step:\n%s", out)
	}
	p.flow(1, "the code does not hold", chargeFlow)
	requoted := strings.Replace(chargeFlow, "const tries = 3", "const tries = 5", 1)
	p.flow(1, "step 2 says what a STALE step said", requoted)
	fiveTries := strings.Replace(requoted, "gives up after three tries", "gives up after five tries", 1)
	p.flow(0, "stored", fiveTries)
	p.want(0, "flows: 1 flow(s), 3 step(s), 0 STALE", "status", "--check")

	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}")+"// a new last line\n")
	p.code("billing/retry.go", strings.Replace(retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"), "// line 4\n", "var backoff = 2\n", 1))
	p.resync()
	p.want(1, "flow charge step 2", "status", "--check")
	p.flow(1, "read it again", fiveTries)
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
		{"      - {path: billing/retry.go, lines: \"3\", quote: \"const tries = 3\"}\n      - {path: billing/charge.go, lines: \"4\", symbol: Charge, quote: \"func Charge() {}\"}", `      - {path: ledger/book.go, lines: "6", quote: "func Post() {}"}`, "none of the step's quotes is in the code of MOD-0001"},
	} {
		draft := strings.Replace(chargeFlow, c.from, c.to, 1)
		if draft == chargeFlow {
			t.Fatalf("the case %q changes nothing", c.want)
		}
		p.flow(1, c.want, draft)
	}
	one := chargeFlow[:strings.Index(chargeFlow, "  - unit: MOD-0001\n    does: retries")] + "---\n"
	p.flow(1, "a flow has 2 steps or more, and this one has 1", one)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "flows", "charge.md")); err == nil {
		t.Error("a refused draft must store nothing")
	}
}

// A human's flow stays until its code changes: a reader may not change a
// current one, and a reader who replaces a stale step takes over only that
// step.
func TestAReaderReplacesAHumansFlowOnlyOnceItIsStale(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored flow charge, 3 steps, USER_STATED", chargeFlow, "--source", "conversation 2026-11-03")
	p.flow(1, "only a human changes it", strings.Replace(chargeFlow, "reads every charge", "reads each charge", 1))
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	p.flow(0, "replaced the human's steps 2 (conversation 2026-11-03)", strings.Replace(strings.Replace(chargeFlow, "const tries = 3", "const tries = 5", 1), "gives up after three tries", "gives up after five tries", 1))
	if stored := p.read("flows/charge.md"); strings.Count(stored, "writer: reader") != 1 || strings.Count(stored, "writer: human") != 2 {
		t.Errorf("the reader's step must replace the human's stale one alone:\n%s", stored)
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

// One flow file abbd cannot read is a named warning: sync, status and every
// abbd flow go on, and the well-formed flows are still checked.
func TestAMalformedFlowFileWarnsAndStopsNothing(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	if err := os.WriteFile(filepath.Join(p.root, ".abbd", "flows", "broken.md"), []byte("---\nsteps: [unclosed\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.resync()
	out := p.want(0, "warning: .abbd/flows/broken.md has frontmatter abbd cannot read", "status", "--check")
	if !strings.Contains(out, "fix its frontmatter, or move it out of .abbd/flows, to clear this") || !strings.Contains(out, "flows: 1 flow(s), 3 step(s), 0 STALE") {
		t.Errorf("status must name the file, say how to clear it, and still check the good flow:\n%s", out)
	}
	p.want(0, "question: How does a charge reach the ledger?", "flow", "charge")
	p.flow(0, "stored", strings.Replace(chargeFlow, "reads every charge", "reads each charge", 1))
}

// movedRetry keeps step 2's quote but changes the line under it, so step 2
// goes STALE while its draft still holds.
func movedRetry() string {
	return strings.Replace(retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"), "// line 4\n", "var backoff = 2\n", 1)
}

// A stale step that comes back as it was is refused even when another step
// changed, so rewording step 1 cannot restamp step 2.
func TestAStaleStepStoredAgainUnreadIsRefused(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	p.code("billing/retry.go", movedRetry())
	p.resync()
	p.want(1, "flow charge step 2", "status", "--check")
	p.flow(1, "step 2 says what a STALE step said", strings.Replace(chargeFlow, "Charge() inserts the charge row", "Charge() writes the charge row", 1))
	p.want(1, "flow charge step 2", "status", "--check")
	p.flow(0, "stored", strings.Replace(chargeFlow, "retries a failed insert", "retries a failed insert after a backoff", 1))
	p.want(0, "flows: 1 flow(s), 3 step(s), 0 STALE", "status", "--check")
}

// A reader replaces only the stale steps of a human's flow: the human's
// current steps must come back as written and keep the human's word, and
// the page marks them so.
func TestAReaderReplacesOnlyTheStaleStepsOfAHumansFlow(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "USER_STATED", chargeFlow, "--source", "conversation 2026-11-03")
	p.code("billing/retry.go", movedRetry())
	p.resync()
	reread := strings.Replace(chargeFlow, "retries a failed insert", "retries a failed insert after a backoff", 1)
	p.flow(1, "step 1 is a human's word (conversation 2026-11-03) and current", strings.Replace(reread, "Charge() inserts the charge row", "Charge() writes the charge row", 1))
	out := p.flow(0, "stored flow charge, 3 steps, INFERRED", reread)
	if !strings.Contains(out, "kept the human's steps 1, 3 (conversation 2026-11-03)") {
		t.Errorf("the reader's store must say which human steps it kept:\n%s", out)
	}
	stored := p.read("flows/charge.md")
	if strings.Count(stored, "writer: human") != 2 || strings.Count(stored, "source: conversation 2026-11-03") != 2 || strings.Count(stored, "writer: reader") != 1 {
		t.Errorf("steps 1 and 3 keep the human's word and step 2 is the reader's:\n%s", stored)
	}
	p.want(0, "flows: 1 flow(s), 3 step(s), 0 STALE", "status", "--check")
	page := p.blueprint()
	if n := strings.Count(page, `<span class="pill stated step-origin"`); n != 2 {
		t.Errorf("the page marks the two human steps of the mixed flow, found %d", n)
	}
}

// The question and caption of a human's flow are the human's word too: a
// reader who takes over a STALE step keeps them as they are.
func TestAReaderKeepsTheQuestionAndCaptionOfAHumansFlow(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "USER_STATED", chargeFlow, "--source", "conversation 2026-11-03")
	p.code("billing/retry.go", movedRetry())
	p.resync()
	reread := strings.Replace(chargeFlow, "retries a failed insert", "retries a failed insert after a backoff", 1)
	p.flow(1, "the question of flow charge is a human's word", strings.Replace(reread, "How does a charge reach the ledger?", "Why is the ledger empty?", 1))
	p.flow(1, "the caption of flow charge is a human's word", strings.Replace(reread, "From the insert to the book.", "Only the insert.", 1))
	p.flow(0, "stored flow charge, 3 steps, INFERRED", reread)
}

// The guard holds across stores: dropping a STALE step and storing it again,
// or storing it with one more quote, does not bring it back CURRENT unread.
func TestAStaleStepCannotComeBackUnreadAcrossStores(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	p.code("billing/retry.go", movedRetry())
	p.resync()
	p.want(1, "flow charge step 2", "status", "--check")
	step2 := chargeFlow[strings.Index(chargeFlow, "  - unit: MOD-0001\n    does: retries"):strings.Index(chargeFlow, "  - unit: MOD-0002")]
	p.flow(0, "stored flow charge, 2 steps", strings.Replace(chargeFlow, step2, "", 1))
	p.flow(1, "step 2 says what a STALE step said", chargeFlow)
	p.flow(1, "step 2 says what a STALE step said", strings.Replace(chargeFlow, "retries a failed insert", "Retries a failed  insert", 1))
	onLedger := strings.Replace(strings.Replace(chargeFlow, "  - unit: MOD-0001\n    does: retries", "  - unit: MOD-0002\n    does: retries", 1),
		`{path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}`, `{path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}`, 1)
	p.flow(1, "step 2 says what a STALE step said", onLedger)
	p.flow(1, "step 2 says what a STALE step said", strings.Replace(chargeFlow, "symbol: Charge, quote: \"func Charge() {}\"}", "symbol: Charge, quote: \"func Charge() {}\"}\n      - {path: billing/charge.go, lines: \"1\", quote: \"package billing\"}", 1))
	p.want(0, "flows: 1 flow(s), 2 step(s), 0 STALE", "status", "--check")
	p.flow(0, "stored", strings.Replace(chargeFlow, "retries a failed insert", "retries a failed insert after a backoff", 1))
	p.want(0, "flows: 1 flow(s), 3 step(s), 0 STALE", "status", "--check")
	p.code("billing/retry.go", retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	p.flow(0, "stored", chargeFlow)
	stored := p.read("flows/charge.md")
	if _, retired, ok := strings.Cut(stored, "retired:"); !ok || strings.Contains(retired, "does: retries a failed insert\n") || !strings.Contains(retired, "after a backoff") {
		t.Errorf("a retired step whose code is back as it was written is forgotten, and the step written on the moved code is retired:\n%s", stored)
	}
}

// A step rests on code its own unit holds: a quote in its unit's file but
// outside the anchored symbol does not count, and a quote that names a symbol
// must lie inside it.
func TestAFlowStepCitesCodeItsOwnUnitHolds(t *testing.T) {
	p := notesProject(t)
	post := "\n      - {path: ledger/book.go, lines: \"6\", symbol: Post, quote: \"func Post() {}\"}"
	p.flow(1, "none of the step's quotes is in the code of MOD-0002", strings.Replace(chargeFlow, post, "", 1))
	p.flow(0, "stored flow charge, 3 steps", chargeFlow)
}

// A quote's symbol is the reader's claim, used only to say which unit's code
// the quote lies in. abbd does not read declarations, so it stores a claim
// it cannot check.
func TestAQuotesSymbolIsTheReadersClaim(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored flow charge, 3 steps", strings.Replace(chargeFlow, `{path: ledger/book.go, lines: "3-4", quote:`, `{path: ledger/book.go, lines: "3-4", symbol: Post, quote:`, 1))
}

// stepTwoRetired stores the charge flow, moves the code under step 2, and
// stores the flow without step 2, so step 2 is kept among the retired steps.
func stepTwoRetired(t *testing.T) *project {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	p.code("billing/retry.go", movedRetry())
	p.resync()
	step2 := chargeFlow[strings.Index(chargeFlow, "  - unit: MOD-0001\n    does: retries"):strings.Index(chargeFlow, "  - unit: MOD-0002")]
	p.flow(0, "stored flow charge, 2 steps", strings.Replace(chargeFlow, step2, "", 1))
	if !strings.Contains(p.read("flows/charge.md"), "retired:") {
		t.Fatal("step 2 went STALE and left the flow, so it is retired")
	}
	return p
}

// A human is the authority on a flow: a retired step does not hold a human's
// store back, and the human's word clears the retired steps it says again.
func TestAHumanStoreClearsTheRetiredStepsItSays(t *testing.T) {
	p := stepTwoRetired(t)
	p.flow(0, "stored flow charge, 3 steps, USER_STATED", chargeFlow, "--source", "conversation 2026-11-03")
	if stored := p.read("flows/charge.md"); strings.Contains(stored, "retired:") {
		t.Errorf("the human said step 2 again, so it is no longer retired:\n%s", stored)
	}
}

// A retired step is forgotten once its unit leaves the state, so the list
// does not grow without bound.
func TestARetiredStepOnAUnitThatLeftIsForgotten(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n// cached\nfunc Post() {}\n")
	p.resync()
	p.want(1, "flow charge step 3", "status", "--check")
	two := chargeFlow[:strings.Index(chargeFlow, "  - unit: MOD-0002")] + "---\n\nFrom the insert to the book.\n"
	p.flow(0, "stored flow charge, 2 steps", two)
	if !strings.Contains(p.read("flows/charge.md"), "retired:") {
		t.Fatal("step 3 went STALE and left the flow, so it is retired")
	}
	p.extract(`{"units":[{"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}]}`)
	p.resync()
	p.flow(0, "stored flow charge, 2 steps", two)
	if stored := p.read("flows/charge.md"); strings.Contains(stored, "retired:") {
		t.Errorf("MOD-0002 left the state, so its retired step is forgotten:\n%s", stored)
	}
}

// A retired step keeps only its own unit, so it outlives a unit it merely
// called: once that unit leaves the state, the step's old words still cannot
// come back on the code that changed under them.
func TestARetiredStepOutlivesAUnitItCalled(t *testing.T) {
	p := notesProject(t)
	p.flow(0, "stored", chargeFlow)
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n// cached\nfunc Post() {}\n")
	p.resync()
	uncalled := strings.ReplaceAll(chargeFlow, "    calls: [DATA-0001]\n", "")
	two := uncalled[:strings.Index(uncalled, "  - unit: MOD-0002")] + "---\n\nFrom the insert to the book.\n"
	p.flow(0, "stored flow charge, 2 steps", two)
	if _, retired, ok := strings.Cut(p.read("flows/charge.md"), "retired:"); !ok || strings.Contains(retired, "calls:") {
		t.Errorf("step 3 went STALE and left the flow, so it is retired without the units it called:\n%s", p.read("flows/charge.md"))
	}
	p.want(0, "retracted DATA-0001", "retract", "DATA-0001", "--reason", "not a table", "--source", "conversation 2026-11-03")
	p.flow(1, "step 3 says what a STALE step said", uncalled)
}

// A store warns on a step whose does runs past 80 characters, and not on a
// long note or failure, which hold a rule or a cause in full.
func TestALongDoesWarnsAndALongNoteDoesNot(t *testing.T) {
	p := notesProject(t)
	long := strings.Repeat("and then some more ", 5)
	out := p.flow(0, "stored", strings.Replace(strings.Replace(chargeFlow, "reads every charge, no cache", "reads every charge, no cache, "+long, 1), "gives up after three tries", "gives up after three tries, "+long, 1))
	if strings.Contains(out, "long:") {
		t.Errorf("a long note or failure is no warning:\n%s", out)
	}
	if out := p.flow(0, "stored", strings.Replace(chargeFlow, "retries a failed insert", "retries a failed insert "+long, 1)); !strings.Contains(out, "long: flow charge step 2 does holds") {
		t.Errorf("a long does warns:\n%s", out)
	}
}
