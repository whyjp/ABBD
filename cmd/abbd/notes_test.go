package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// retryCode is billing's retry rule, with the line the failure note cites
// near the top and the line the states note cites near the bottom, far
// enough apart that an edit near one leaves the other's lines alone.
func retryCode(top, bottom string) string {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		switch i {
		case 3:
			b.WriteString(top + "\n")
		case 25:
			b.WriteString(bottom + "\n")
		default:
			fmt.Fprintf(&b, "// line %d\n", i)
		}
	}
	return b.String()
}

func notesProject(t *testing.T) *project {
	t.Helper()
	p := semanticProject(t)
	p.code("billing/retry.go", retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	return p
}

func (p *project) note(ref, text string, evidence ...string) (int, string) {
	p.t.Helper()
	args := []string{"understand", ref, p.file(p.t, "note.md", text)}
	for _, e := range evidence {
		args = append(args, "--evidence", e)
	}
	return p.cmd("", args...)
}

func (p *project) wantNote(code int, contains, ref, text string, evidence ...string) string {
	p.t.Helper()
	got, out := p.note(ref, text, evidence...)
	if got != code || !strings.Contains(out, contains) {
		p.t.Fatalf("abbd understand %s: want exit %d containing %q, got exit %d:\n%s", ref, code, contains, got, out)
	}
	return out
}

const (
	failureQuote = "billing/retry.go:3:const tries = 3"
	statesQuote  = `billing/retry.go:25:var states = []string{"pending", "charged"}`
)

// A note describes only the code it cites. An edit near one note's quote
// stales that note and leaves the module's other notes, and the module's
// own text, current; the check fails until a reader stores it again.
func TestANoteGoesStaleOnlyWhenTheCodeItCitesChanges(t *testing.T) {
	p := notesProject(t)
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", p.file(t, "how.md", "## Overview\n\nbilling inserts charges.\n"))
	p.wantNote(0, "understood MOD-0001.failure billing, INFERRED", "MOD-0001.failure", "A charge is tried three times.", failureQuote)
	p.wantNote(0, "understood MOD-0001.states", "MOD-0001.states", "| state | next |\n|---|---|\n| pending | charged |", statesQuote)
	p.wantNote(0, "understood DATA-0001.semantics charges table", "DATA-0001.semantics", "One row per charge.", "billing/charge.go:3:INSERT INTO charges")
	p.want(0, "notes: 3 slot note(s), 0 STALE, 0 orphaned", "status", "--check")

	p.code("billing/retry.go", retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}")+"// a new last line\n")
	p.resync()
	p.want(0, "notes: 3 slot note(s), 0 STALE", "status", "--check")

	p.code("billing/retry.go", strings.Replace(retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"), "// line 24\n", "var refunded = true\n", 1))
	p.resync()
	out := p.want(1, "1 note(s) STALE", "status", "--check")
	for _, want := range []string{
		"notes: 3 slot note(s), 1 STALE, 0 orphaned",
		"stale: MOD-0001.states, the code it cites changed: billing/retry.go (the lines around it changed)",
		"understanding: 1 module text(s), 0 STALE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status must say %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "stale: MOD-0001.failure") || strings.Contains(out, "stale: DATA-0001") {
		t.Errorf("only the note whose lines changed may go STALE:\n%s", out)
	}
	if out := p.want(0, "states (INFERRED, STALE, the code it cites changed", "explain", "MOD-0001"); !strings.Contains(out, "failure (INFERRED, CURRENT)") {
		t.Errorf("explain must mark each note on its own:\n%s", out)
	}

	p.wantNote(1, "read it again", "MOD-0001.states", "| state | next |\n|---|---|\n| pending | charged |", statesQuote)
	p.wantNote(0, "understood", "MOD-0001.states", "| state | next |\n|---|---|\n| pending | charged |\n| charged | refunded |", statesQuote)
	p.want(0, "notes: 3 slot note(s), 0 STALE", "status", "--check")

	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	p.want(1, "stale: MOD-0001.failure, the code it cites changed: billing/retry.go (the quote is gone, or no longer once in the file)", "status", "--check")
}

// A slot is one abbd knows, on a module or a contract, and a note enters
// only on quotes the code holds now.
func TestANoteNeedsLiveEvidenceAndASlotAbbdKnows(t *testing.T) {
	p := notesProject(t)
	for _, c := range []struct {
		ref, want string
		ev        []string
	}{
		{"MOD-0001.failure", "cite at least one --evidence", nil},
		{"MOD-0001.failure", "the code does not hold", []string{"billing/retry.go:3:const tries = 4"}},
		{"MOD-0001.gotchas", `"gotchas" is no slot; the slots are semantics, failure, states, decisions, wire_sample`, []string{failureQuote}},
		{"MOD-0001.decisions", "name it MOD-0001.decisions.KEY", []string{failureQuote}},
		{"MOD-0001.semantics.main", "takes no key", []string{failureQuote}},
		{"MOD-0001.decisions.Tries", "lower-case letters, digits and hyphens", []string{failureQuote}},
		{"MOD-0099.semantics", "MOD-0099 is not a module or a contract of the state", []string{failureQuote}},
	} {
		p.wantNote(1, c.want, c.ref, "A charge is tried three times.", c.ev...)
	}
	p.want(1, "--evidence is for a slot", "understand", "MOD-0001", p.file(t, "how.md", "billing.\n"), "--evidence", failureQuote)
	p.want(1, "moving units never stales it", "understand", "MOD-0001.failure", "--restamp")
	if got := snapshot(t, p.root); strings.Contains(fmt.Sprint(keys(got)), "understanding/") {
		t.Fatalf("a refused note must store nothing: %v", keys(got))
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A reader writes what the code decided; the human states why, with where.
// Storing the decision again keeps the human's reason.
func TestAHumanStatesWhyADecisionWasTaken(t *testing.T) {
	p := notesProject(t)
	p.want(1, "holds no decision; store it first", "record", "fact", "MOD-0001.decisions.three-tries", "--value", "card networks", "--source", "conversation 2026-11-03")
	p.wantNote(0, "understood MOD-0001.decisions.three-tries", "MOD-0001.decisions.three-tries", "A charge is tried three times before it fails.", failureQuote)
	p.want(0, "recorded MOD-0001.decisions.three-tries's why, USER_STATED from https://example.com/pr/7", "record", "fact", "MOD-0001.decisions.three-tries", "--value", "the card network bans a fourth try", "--source", "https://example.com/pr/7")
	p.want(1, "--source", "record", "fact", "MOD-0001.decisions.three-tries", "--value", "a guess")
	p.wantNote(0, "understood", "MOD-0001.failure", "tried three times", failureQuote)
	p.want(1, "only a decision takes a human's why", "record", "fact", "MOD-0001.failure", "--value", "x", "--source", "y")

	p.wantNote(0, "understood", "MOD-0001.decisions.three-tries", "A charge is tried at most three times.", failureQuote)
	out := p.want(0, "why (USER_STATED, https://example.com/pr/7): the card network bans a fourth try", "explain", "MOD-0001")
	if !strings.Contains(out, "decisions.three-tries (INFERRED, CURRENT)") || !strings.Contains(out, "tried at most three times") {
		t.Errorf("explain must show the decision with its new text and the kept why:\n%s", out)
	}
	if page := p.read("blueprint/index.html"); !strings.Contains(page, "the card network bans a fourth try") || !strings.Contains(page, "https://example.com/pr/7") {
		t.Error("the blueprint must show the stated why and its source")
	}
}

// The blueprint shows a module's notes beside its inside and a contract's in
// the inside of the module that provides it, drawing a states matrix as a
// table and a wire sample as code.
func TestTheBlueprintShowsEachNoteBesideItsUnit(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.states", "| state | next |\n|---|---|\n| pending | charged |", statesQuote)
	p.wantNote(0, "understood", "DATA-0001.wire_sample", "```sql\nINSERT INTO charges (amount) VALUES (?)\n```", "billing/charge.go:3:INSERT INTO charges")
	page := p.read("blueprint/index.html")
	billing := page[strings.Index(page, `id="m-MOD-0001"`):]
	billing = billing[:strings.Index(billing, "</section>")]
	for _, want := range []string{
		`id="n-MOD-0001.states"`, "<th>state</th>", "<td>pending</td>",
		`id="c-DATA-0001"`, `id="n-DATA-0001.wire_sample"`, "<pre>INSERT INTO charges (amount) VALUES (?)</pre>",
	} {
		if !strings.Contains(billing, want) {
			t.Errorf("billing's inside must hold %q", want)
		}
	}
	if strings.Contains(billing, "|---|") {
		t.Error("a table's rule line must not render as a row")
	}
	p.want(0, "blueprint: CURRENT", "status", "--check")

	p.code("billing/retry.go", strings.Replace(retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"), "// line 24\n", "var refunded = true\n", 1))
	p.resync()
	page = p.read("blueprint/index.html")
	states := page[strings.Index(page, `id="n-MOD-0001.states"`):]
	if !strings.Contains(states[:strings.Index(states, "</h3>")], "pill STALE") {
		t.Error("the stale note's heading must carry STALE")
	}
	wire := page[strings.Index(page, `id="n-DATA-0001.wire_sample"`):]
	if strings.Contains(wire[:strings.Index(wire, "</h3>")], "pill STALE") {
		t.Error("a note whose code did not change must not read STALE")
	}
}

// A note is read like a module's text: the agent who stores it, and status,
// hear what reads as history, and nothing is refused for it.
func TestANoteIsReadForHistoryAndLength(t *testing.T) {
	p := notesProject(t)
	out := p.wantNote(0, "understood", "MOD-0001.failure", "## Retries\n\nA charge is tried three times since PR #12.", failureQuote)
	if !strings.Contains(out, `history: MOD-0001.failure "Retries" says "PR #12"`) {
		t.Errorf("understand must report the history phrase:\n%s", out)
	}
	p.want(0, "1 phrase(s) that read as history", "status", "--check")
}

// A note whose unit left the state, or whose slot abbd no longer knows, shows
// nowhere and no command can store it again. The check names it and passes.
func TestStatusCheckReportsAnOrphanNoteAndPasses(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.failure", "A charge is tried three times.", failureQuote)
	text := strings.Replace(p.read("understanding/MOD-0001/failure.md"), "unit: MOD-0001", "unit: MOD-0099", 1)
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0099", "failure.md"), text)
	text = strings.Replace(p.read("understanding/MOD-0001/failure.md"), "slot: failure", "slot: gotchas", 1)
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0001", "gotchas.md"), text)
	p.resync()
	out := p.want(0, "notes: 3 slot note(s), 0 STALE, 2 orphaned", "status", "--check")
	for _, want := range []string{"understanding/MOD-0099/failure.md", "understanding/MOD-0001/gotchas.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("status must say how to drop %s:\n%s", want, out)
		}
	}
}

// The skill names every slot, so an agent knows what each one is for.
func TestTheSkillNamesEverySlot(t *testing.T) {
	text := readPlugin(t, "skills", "abbd", "SKILL.md")
	for _, s := range model.Slots {
		if !strings.Contains(text, "`"+s.Name+"`") {
			t.Errorf("the abbd skill must name the slot %s", s.Name)
		}
	}
}

// A contract's notes sit with the module the code says provides it, even
// when a reader also found a module of a lower ID providing it.
func TestAContractsNotesSitWithTheProviderTheCodeShows(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// calls ledger.Post\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"post","kind":"IF","name":"Post","anchors":[{"path":"ledger/book.go","symbol":"Post"}],"signature":"func Post()"}
],"relations":[{"from":"ledger","to":"post","kind":"provides"}]}`)
	p.setup(0, "synced", "--", exe)
	p.want(0, "INFERRED", "record", "relation", "MOD-0001", "provides", "IF-0001", "--via", "call", "--evidence", "billing/charge.go:3:calls ledger.Post")
	p.wantNote(0, "understood IF-0001.semantics", "IF-0001.semantics", "Post books one entry.", "ledger/book.go:3:func Post() {}")
	page := p.read("blueprint/index.html")
	ledger := page[strings.Index(page, `id="m-MOD-0002"`):]
	if !strings.Contains(ledger[:strings.Index(ledger, "</section>")], `id="c-IF-0001"`) {
		t.Fatal("the contract's notes belong in ledger's inside, the provider the code shows")
	}
}

// Storing the same words on the same quotes in another order is still not
// reading the code again.
func TestReorderingTheQuotesDoesNotFreshenAStaleNote(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.states", "states text", statesQuote, failureQuote)
	p.code("billing/retry.go", strings.Replace(retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"), "// line 24\n", "var refunded = true\n", 1))
	p.resync()
	p.wantNote(1, "read it again", "MOD-0001.states", "states text", failureQuote, statesQuote)
	p.want(1, "1 note(s) STALE", "status", "--check")
}

// A note follows its quote when code above it moves, so it cites only a
// quote its file holds once; a quote that is in the file twice is refused.
func TestANoteFollowsItsQuoteWhenTheCodeAboveMoves(t *testing.T) {
	p := notesProject(t)
	body := strings.Repeat("// filler\n", 12) + "func a() error { return nil }\n\nfunc b() error { return nil }\n"
	p.code("billing/nil.go", "package billing\n"+body)
	p.resync()
	p.wantNote(1, `holds "return nil" more than once`, "MOD-0001.failure", "fails nil", "billing/nil.go:16:return nil")
	p.wantNote(0, "understood", "MOD-0001.failure", "fails nil", "billing/nil.go:16:func b() error { return nil }")
	p.code("billing/nil.go", "package billing\n"+strings.Repeat("// pad\n", 10)+body)
	p.resync()
	p.want(0, "notes: 1 slot note(s), 0 STALE", "status", "--check")
}

// Respacing the words, or citing a quote twice, is not reading the code
// again either.
func TestRespacingOrRepeatingDoesNotFreshenAStaleNote(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.states", "states.\n\nmore.", statesQuote)
	p.code("billing/retry.go", strings.Replace(retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"), "// line 24\n", "var refunded = true\n", 1))
	p.resync()
	p.wantNote(1, "read it again", "MOD-0001.states", "states.\n\n\nmore.", statesQuote)
	p.wantNote(1, "read it again", "MOD-0001.states", "states.\n\nmore.", statesQuote, statesQuote)
}

// When the file comes to hold a note's quote twice, the note cannot tell
// which copy it was written on, so it reads STALE rather than following
// whichever copy now sits near its old lines.
func TestANoteWhoseQuoteIsNoLongerUniqueReadsStale(t *testing.T) {
	p := notesProject(t)
	block := "func b() error {\n\treturn retry(charge)\n}\n"
	p.code("billing/b.go", "package billing\n"+strings.Repeat("// filler\n", 12)+block)
	p.resync()
	p.wantNote(0, "understood", "MOD-0001.failure", "b retries.", "billing/b.go:15:return retry(charge)")
	p.code("billing/b.go", "package billing\n"+strings.Repeat("// filler\n", 12)+block+strings.Repeat("// more\n", 12)+block)
	p.resync()
	p.want(1, "stale: MOD-0001.failure, the code it cites changed: billing/b.go (the quote is gone, or no longer once in the file)", "status", "--check")
}

// A provider whose provides relation comes from a unit inside it, read by
// the script, is still the home over a reader's provider.
func TestAContractsHomeCountsWhatItsUnitsProvide(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// calls ledger.Post\nfunc Charge() {}\n")
	p.code("ledger/core/book.go", "package core\n\nfunc Post() {}\n")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger"}]},
 {"key":"core","kind":"MOD","name":"core","parent":"ledger","anchors":[{"path":"ledger/core/book.go","symbol":"Post"}]},
 {"key":"post","kind":"IF","name":"Post","anchors":[{"path":"ledger/core/book.go","symbol":"Post"}],"signature":"func Post()"}
],"relations":[{"from":"core","to":"post","kind":"provides"}]}`)
	p.setup(0, "synced", "--", exe)
	p.want(0, "INFERRED", "record", "relation", "MOD-0001", "provides", "IF-0001", "--via", "call", "--evidence", "billing/charge.go:3:calls ledger.Post")
	p.wantNote(0, "understood IF-0001.semantics", "IF-0001.semantics", "Post books one entry.", "ledger/core/book.go:3:func Post() {}")
	page := p.read("blueprint/index.html")
	ledger := page[strings.Index(page, `id="m-MOD-0002"`):]
	if !strings.Contains(ledger[:strings.Index(ledger, "</section>")], `id="c-IF-0001"`) {
		t.Fatal("the contract's notes belong in ledger's inside, whose unit the code shows providing it")
	}
}

// A human's why is about the decision they commented on. A decision stored
// again on other code is another decision, so the why does not carry over to
// it, and the reader who stored it hears that it was dropped.
func TestAWhyDoesNotSurviveADecisionOnOtherCode(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.decisions.k", "A charge is tried three times.", failureQuote)
	p.want(0, "recorded", "record", "fact", "MOD-0001.decisions.k", "--value", "the card network bans a fourth try", "--source", "https://example.com/pr/7")
	out := p.wantNote(0, "understood", "MOD-0001.decisions.k", "A charge moves from pending to charged.", statesQuote)
	if !strings.Contains(out, "dropped the why") || !strings.Contains(out, "abbd record fact MOD-0001.decisions.k") {
		t.Errorf("understand must say it dropped the why and how to state one again:\n%s", out)
	}
	if out := p.want(0, "decisions.k (INFERRED, CURRENT)", "explain", "MOD-0001"); strings.Contains(out, "fourth try") {
		t.Errorf("the old why must not show under a decision on other code:\n%s", out)
	}
	if strings.Contains(p.read("blueprint/index.html"), "fourth try") {
		t.Error("the blueprint must not show the old why under the new decision")
	}
}

// A note file that is not at the path its frontmatter names is no note of
// its slot: status names the file as it sits, the check skips it, and the
// note at the right path goes STALE and current on its own.
func TestANoteFileAtTheWrongPathIsOrphaned(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.states", "states text", statesQuote)
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0001", "a.md"), p.read("understanding/MOD-0001/states.md"))
	p.code("billing/retry.go", strings.Replace(retryCode("const tries = 3", "var states = []string{\"pending\", \"charged\"}"), "// line 24\n", "var refunded = true\n", 1))
	p.resync()
	out := p.want(1, "1 note(s) STALE", "status", "--check")
	for _, want := range []string{
		"orphaned: .abbd/understanding/MOD-0001/a.md says it is MOD-0001.states, whose note belongs at .abbd/understanding/MOD-0001/states.md",
		"delete .abbd/understanding/MOD-0001/a.md",
		"stale: MOD-0001.states, the code it cites changed",
		"abbd understand MOD-0001.states FILE --evidence",
		"or delete .abbd/understanding/MOD-0001/states.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status must say %q:\n%s", want, out)
		}
	}
	p.wantNote(0, "understood", "MOD-0001.states", "states text, with refunds", statesQuote)
	p.want(0, "notes: 2 slot note(s), 0 STALE, 1 orphaned", "status", "--check")
	if out := p.want(0, "with refunds", "explain", "MOD-0001"); strings.Count(out, "states (INFERRED") != 1 {
		t.Errorf("explain must show the slot's one note:\n%s", out)
	}
}

// A markdown file without frontmatter beside the notes, such as a README,
// is no note: every command reads past it, and status names it. A file that
// opens frontmatter is a note, and a malformed one still fails.
func TestAStrayMarkdownFileBesideTheNotesIsAWarning(t *testing.T) {
	p := notesProject(t)
	p.wantNote(0, "understood", "MOD-0001.failure", "A charge is tried three times.", failureQuote)
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0001", "README.md"), "# what these are\n")
	p.resync()
	p.want(0, "warning: .abbd/understanding/MOD-0001/README.md has no frontmatter", "status", "--check")
	p.want(0, "failure (INFERRED, CURRENT)", "explain", "MOD-0001")
	write(t, filepath.Join(p.root, ".abbd", "understanding", "MOD-0001", "semantics.md"), "---\nunit: MOD-0001\n")
	p.want(1, "unterminated frontmatter", "status")
}
