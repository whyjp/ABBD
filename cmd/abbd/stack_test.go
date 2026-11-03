package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chargeStack is the charge path in two layers: the books on top, which read
// the charges billing writes below them. Each layer cites the code of the
// unit it holds.
const chargeStack = `---
question: What are the layers of the charge path?
layers:
  - name: books
    role: posts each charge into the ledger
    units: [MOD-0002]
    evidence:
      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}
  - name: billing
    role: writes the charge rows and retries a failed insert
    units: [MOD-0001]
    evidence:
      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}
      - {path: billing/retry.go, lines: "3", quote: "const tries = 3"}
---

The books sit on billing.
`

// upsideDown puts billing on top, so the ledger's read of the charges table
// runs from the lower layer up to the higher one.
func upsideDown() string {
	books := chargeStack[strings.Index(chargeStack, "  - name: books"):strings.Index(chargeStack, "  - name: billing")]
	billing := chargeStack[strings.Index(chargeStack, "  - name: billing"):strings.Index(chargeStack, "---\n\nThe books")]
	return strings.Replace(strings.Replace(chargeStack, books, "", 1), billing, billing+books, 1)
}

func (p *project) stack(code int, contains, draft string, extra ...string) string {
	p.t.Helper()
	return p.want(code, contains, append([]string{"stack", "charge", p.file(p.t, "stack.md", draft)}, extra...)...)
}

// A stack is stored with each layer stamped on the code it cites, and the
// page draws it as bands from the top down, each with its role and the units
// it holds, linked to the inside that shows each unit.
func TestAStackIsStoredAndDrawnInBands(t *testing.T) {
	p := notesProject(t)
	out := p.stack(0, "stored stack charge, 2 layers, INFERRED", chargeStack)
	if !strings.Contains(out, "breaches: 0 relation(s) run up the stack") {
		t.Errorf("a store says how many relations run up the stack:\n%s", out)
	}
	stored := p.read("stacks/charge.md")
	for _, want := range []string{"question: What are the layers of the charge path?", "layers:", "writer: reader", "code:", "The books sit on billing."} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored stack must hold %q:\n%s", want, stored)
		}
	}
	status := p.want(0, "stacks: 1 stack(s), 2 layer(s), 0 STALE", "status", "--check")
	if !strings.Contains(status, "breaches: 0 relation(s) run up a stack") {
		t.Errorf("status reports the stack's breaches:\n%s", status)
	}
	page := p.blueprint()
	for _, want := range []string{`id="stack-charge"`, `id="stack-charge.2"`, "posts each charge into the ledger", `href="MOD-0002.html" title="MOD-0002"`, `<a href="#stack-charge">What are the layers of the charge path?</a>`, "이 모듈이 놓인 층", `index.html#stack-charge`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page must draw %q", want)
		}
	}
	if out := p.want(0, "question: What are the layers of the charge path?", "stack", "charge"); !strings.Contains(out, "const tries = 3") || strings.Contains(out, "code:") {
		t.Errorf("abbd stack KEY prints the stored stack as a draft, without what abbd sets:\n%s", out)
	}
}

// A draft enters only as layers true of the state: two layers or more, each
// unit held by one layer only, counting the units inside the ones a layer
// names, and each layer's quotes in the code of its own units.
func TestAStackDraftIsCheckedBeforeItIsStored(t *testing.T) {
	p := notesProject(t)
	for _, c := range []struct{ from, to, want string }{
		{"    units: [MOD-0002]", "    units: [MOD-0002, DATA-0001]", "DATA-0001 is held by layer 1 and, inside MOD-0001, by layer 2; a unit sits in one layer"},
		{"    units: [MOD-0002]", "    units: [MOD-0002, MOD-0001]", "MOD-0001 is named by layer 1 and by layer 2"},
		{`      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}`, `      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}`, "layer 1: none of the layer's quotes is in the code of its units"},
		{"    units: [MOD-0002]", "    units: [MOD-0009]", "MOD-0009 is no longer in the state"},
		{"    role: posts each charge into the ledger\n", "    role: ''\n", "says its role"},
		{"const tries = 3", "const tries = 4", "the code does not hold"},
	} {
		draft := strings.Replace(chargeStack, c.from, c.to, 1)
		if draft == chargeStack {
			t.Fatalf("the case %q changes nothing", c.want)
		}
		p.stack(1, c.want, draft)
	}
	one := chargeStack[:strings.Index(chargeStack, "  - name: billing")] + "---\n"
	p.stack(1, "a stack has 2 layers or more, and this one has 1", one)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "stacks", "charge.md")); err == nil {
		t.Error("a refused draft must store nothing")
	}
}

// An edit near one layer's quote stales that layer alone, and the check
// fails until the stack is read again; the same layer on the moved code is
// refused.
func TestAStackLayerGoesStaleOnItsOwn(t *testing.T) {
	p := notesProject(t)
	p.stack(0, "stored", chargeStack)
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	out := p.want(1, "1 stack layer(s) STALE", "status", "--check")
	if !strings.Contains(out, "stale: stack charge layer 2 (billing): billing/retry.go (the quote is gone") || strings.Contains(out, "layer 1 (") {
		t.Errorf("only layer 2 cites the changed code:\n%s", out)
	}
	p.stack(1, "the code does not hold", chargeStack)
	requoted := strings.Replace(chargeStack, "const tries = 3", "const tries = 5", 1)
	p.stack(1, "layer 2 says what a STALE layer said", requoted)
	p.stack(0, "stored", strings.Replace(requoted, "retries a failed insert", "retries a failed insert five times", 1))
	p.want(0, "stacks: 1 stack(s), 2 layer(s), 0 STALE", "status", "--check")
}

// A stack whose unit left the state fails the check, since storing the stack
// again brings it current.
func TestAStackOnAUnitThatLeftFailsTheCheck(t *testing.T) {
	p := notesProject(t)
	p.stack(0, "stored", chargeStack)
	p.extract(`{"units":[{"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}]}`)
	p.resync()
	p.want(1, "stale: stack charge layer 1 (books): MOD-0002 is no longer in the state", "status", "--check")
}

// A relation that runs up the stack is drawn in the lower band and reported
// by status, and does not fail the check: it is debt or a layer drawn wrong,
// which only a design call clears.
func TestABreachIsDrawnAndReportedAndDoesNotFailTheCheck(t *testing.T) {
	p := notesProject(t)
	p.stack(0, "breaches: 1 relation(s) run up the stack", upsideDown())
	out := p.want(0, "breaches: 1 relation(s) run up a stack", "status", "--check")
	if !strings.Contains(out, "breach: stack charge: MOD-0002 ledger (layer 2 books) consumes DATA-0001 charges table (layer 1 billing)") {
		t.Errorf("status names each breach by its ends and their layers:\n%s", out)
	}
	page := p.blueprint()
	for _, want := range []string{`class="chip breach"`, "↑ ledger → charges table", "거꾸로 1"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page must draw the breach %q", want)
		}
	}
	p.stack(0, "breaches: 0 relation(s) run up the stack", chargeStack)
}

// A human's stack stays until its code changes: a reader may not change a
// current one, and a reader who replaces a stale layer takes over only that
// layer.
func TestAReaderReplacesAHumansStackOnlyOnceItIsStale(t *testing.T) {
	p := notesProject(t)
	p.stack(0, "stored stack charge, 2 layers, USER_STATED", chargeStack, "--source", "conversation 2026-11-03")
	p.stack(1, "only a human changes it", strings.Replace(chargeStack, "posts each charge", "posts every charge", 1))
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	reread := strings.Replace(strings.Replace(chargeStack, "const tries = 3", "const tries = 5", 1), "retries a failed insert", "retries a failed insert five times", 1)
	p.stack(1, "layer 1 is a human's word (conversation 2026-11-03) and current", strings.Replace(reread, "posts each charge", "posts every charge", 1))
	p.stack(0, "replaced the human's layers 2 (conversation 2026-11-03)", reread)
	if stored := p.read("stacks/charge.md"); strings.Count(stored, "writer: reader") != 1 || strings.Count(stored, "writer: human") != 1 {
		t.Errorf("the reader's layer must replace the human's stale one alone:\n%s", stored)
	}
}

// A store warns on a layer whose role runs past 80 characters, and not on
// its name.
func TestALongRoleWarnsAndALongNameDoesNot(t *testing.T) {
	p := notesProject(t)
	long := strings.Repeat("and then some more ", 5)
	if out := p.stack(0, "stored", strings.Replace(chargeStack, "name: books", "name: books "+long, 1)); strings.Contains(out, "long:") {
		t.Errorf("a long name is no warning:\n%s", out)
	}
	if out := p.stack(0, "stored", strings.Replace(chargeStack, "posts each charge into the ledger", "posts each charge into the ledger "+long, 1)); !strings.Contains(out, "long: stack charge layer 1 role holds") {
		t.Errorf("a long role warns:\n%s", out)
	}
}

// auditTopo adds an audit module beside billing and ledger, so a stack can
// name a unit that neither of them holds.
const auditTopo = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"audit","kind":"MOD","name":"audit","anchors":[{"path":"audit/log.go","symbol":"Log"}]}
]}`

// A human's stack that a move in the state breaks as a whole is open to a
// reader, as a stale layer is: once billing moves under the ledger, two
// layers hold it, and a reader may replace the layers the break names, so
// CI never waits on the human. A layer that comes back as written keeps the
// human's word.
func TestAReaderMendsAHumansStackTheStateBrokeAsAWhole(t *testing.T) {
	p := notesProject(t)
	p.code("audit/log.go", "package audit\n\nfunc Log() {}\n")
	p.extract(auditTopo)
	p.resync()
	p.stack(0, "stored stack charge, 2 layers, USER_STATED", chargeStack, "--source", "conversation 2026-11-03")
	p.extract(strings.Replace(auditTopo, `"name":"billing",`, `"name":"billing","parent":"ledger",`, 1))
	p.resync()
	p.want(1, "MOD-0001 is held by layer 2 and, inside MOD-0002, by layer 1", "status", "--check")
	mended := strings.Replace(strings.Replace(strings.Replace(strings.Replace(chargeStack,
		"name: billing", "name: audit", 1),
		"writes the charge rows and retries a failed insert", "logs each posted charge", 1),
		"    units: [MOD-0001]", "    units: [MOD-0003]", 1),
		`      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}
      - {path: billing/retry.go, lines: "3", quote: "const tries = 3"}`, `      - {path: audit/log.go, lines: "3", symbol: Log, quote: "func Log() {}"}`, 1)
	out := p.stack(0, "stored stack charge, 2 layers, INFERRED", mended)
	if !strings.Contains(out, "kept the human's layers 1 (conversation 2026-11-03)") || !strings.Contains(out, "replaced the human's layers 2 (conversation 2026-11-03)") {
		t.Errorf("the reader keeps the human's layer that came back and replaces the one the break named:\n%s", out)
	}
	p.want(0, "stacks: 1 stack(s), 2 layer(s), 0 STALE", "status", "--check")
}
