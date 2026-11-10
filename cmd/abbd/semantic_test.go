package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The ledger reads the charges table billing writes. No import shows it, so
// only a reader finds it, and only the quotes keep it alive.
const chargesTable = `{"units":[
 {"key":"t","kind":"DATA","name":"charges table","parent":"b",
  "evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
],"relations":[
 {"from":"b","to":"t","kind":"provides","via":"data","evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},
 {"from":"l","to":"t","kind":"consumes","via":"data","evidence":[{"path":"ledger/book.go","lines":"3-4","quote":"SELECT amount FROM charges"}]}
]}`

const ledgerTopo = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
]}`

func semanticProject(t *testing.T) *project {
	t.Helper()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	p.setup(0, "synced: 3 unit(s)", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	return p
}

func (p *project) resync() string { p.t.Helper(); return p.want(0, "", "sync") }

// laterRevision commits the tree and changes it, so the next sync runs at a
// later revision, where an item still drifted dies.
func (p *project) laterRevision() {
	p.t.Helper()
	git(p.t, p.root, "add", "-A")
	git(p.t, p.root, "commit", "-qm", "next")
	p.code(fmt.Sprintf("next/%d.txt", time.Now().UnixNano()), "x\n")
}

func TestASupplementUnitOutsideTheScriptNeedsEvidence(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.extract(ledgerTopo)
	noEvidence := strings.Replace(chargesTable, `
  "evidence":[{"path":"billing/charge.go","lines":"3","quote":"INSERT INTO charges"}]},`, `"anchors":[{"path":"db/schema.sql"}]},`, 1)
	p.setup(1, "cite the code each one rests on", "--supplement", p.file(t, "bare.json", noEvidence), "--", exe)

	p = semanticProject(t)
	md := p.read("architecture/DATA-0001.md")
	for _, want := range []string{"value: charges table\n    writer: supplement", "- to: MOD-0001\n      writer: supplement", "quote: INSERT INTO charges", "path: billing/charge.go", "supplement_ref: t"} {
		if !strings.Contains(md, want) {
			t.Fatalf("a unit only the reader found should be stored with its evidence and parent; missing %q:\n%s", want, md)
		}
	}
	p.resync()
	if md := p.read("architecture/DATA-0001.md"); !strings.Contains(md, "charges table") || strings.Contains(md, "absent") {
		t.Fatalf("re-extraction by the script must keep a semantic unit whose evidence is live:\n%s", md)
	}
}

// Deleting a quote first drifts the coupling: it still counts, flagged, in
// the PR that removed it. A sync at a later revision that still finds it
// drifted kills it, and the dead coupling stays on record with its reason.
func TestDeletingTheEvidenceKillsTheCoupling(t *testing.T) {
	p := semanticProject(t)
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	contains(t, p.resync(), "evidence: 1 drifted, 0 dead")
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "status: drifted") || strings.Contains(md, "status: dead") {
		t.Fatalf("a quote gone from its file drifts first:\n%s", md)
	}
	p.laterRevision()
	p.resync()
	if md := p.read("architecture/MOD-0002.md"); !strings.Contains(md, "status: dead\n") || !strings.Contains(md, "and not refreshed") {
		t.Fatalf("the dead coupling stays on record, marked dead with its reason:\n%s", md)
	}
	if strings.Contains(p.read("architecture/MOD-0001.md"), "status: dead") {
		t.Fatalf("only the coupling whose evidence went away dies:\n%s", p.read("architecture/MOD-0001.md"))
	}
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.resync()
	p.laterRevision()
	p.resync()
	if !strings.Contains(p.read("architecture/DATA-0001.md"), "absent: true\nabsent_reason: evidence-dead\n") {
		t.Fatalf("a semantic unit whose evidence is gone is absent:\n%s", p.read("architecture/DATA-0001.md"))
	}
}

// Line drift within the slack is written back, so small edits above a quote
// never add up to a false removal.
func TestEvidenceFollowsAQuoteThatDriftsAcrossSyncs(t *testing.T) {
	p := semanticProject(t)
	p.code("ledger/book.go", "package ledger\n\n// a\n// b\n// c\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.resync()
	p.code("ledger/book.go", "package ledger\n\n// a\n// b\n// c\n// d\n// e\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.resync()
	if strings.Contains(p.read("architecture/MOD-0002.md"), "dead: true") {
		t.Errorf("the quote is still in ledger/book.go, 2 lines from where the last sync found it:\n%s", p.read("architecture/MOD-0002.md"))
	}
}

func TestRelationKindsMustFitTheUnitsTheyJoin(t *testing.T) {
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.extract(ledgerTopo)
	modProvidesMod := `{"units":[
 {"key":"b","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
],"relations":[{"from":"b","to":"l","kind":"provides"}]}`
	p.setup(1, "MOD provides MOD is not a relation ABBD knows", "--supplement", p.file(t, "sup.json", modProvidesMod), "--", exe)
	badVia := strings.Replace(chargesTable, `"via":"data"`, `"via":"telepathy"`, 1)
	p.setup(1, `unknown via "telepathy"`, "--supplement", p.file(t, "sup.json", badVia), "--", exe)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "architecture")); err == nil {
		t.Fatalf("a refused supplement must write nothing")
	}
}

type graphFile struct {
	Units []struct {
		ID, Parent string
		Evidence   []string
	} `json:"units"`
	Edges []struct {
		From, To, Origin, Via, Quote string
	} `json:"edges"`
	Modules []struct {
		ID       string
		Inferred []struct {
			From, To, Via string
			Quotes        []string
		}
	} `json:"modules"`
}

func (p *project) graph() graphFile {
	p.t.Helper()
	var g graphFile
	if err := json.Unmarshal([]byte(p.read("blueprint/graph.json")), &g); err != nil {
		p.t.Fatal(err)
	}
	return g
}

// A human judges a guessed coupling by how it arises and the code it rests
// on. The overview draws none; the inside of each module it touches lists it.
func TestTheBlueprintListsAGuessInsideAndDrawsNoneOnTheOverview(t *testing.T) {
	p := semanticProject(t)
	html := p.blueprint()
	for _, want := range []string{
		`ledger → charges table <span class="muted">(consumes, via data)</span>`,
		`<q>ledger/book.go &#34;SELECT amount FROM charges&#34;</q>`,
		`근거 <q>billing/charge.go &#34;INSERT INTO charges&#34;</q>`,
		`<a href="MOD-0001.html" title="MOD-0001">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("the blueprint should list the coupling with its via and quote; missing %s", want)
		}
	}
	g := p.graph()
	if raw := p.read("blueprint/graph.json"); strings.Contains(raw, `"overview"`) || strings.Contains(raw, `"Wires"`) {
		t.Fatal("graph.json holds the data and leaves every drawing to the pages")
	}
	listed := map[string]int{}
	for _, m := range g.Modules {
		for _, c := range m.Inferred {
			if c.Via == "data" && len(c.Quotes) == 1 {
				listed[m.ID]++
			}
		}
	}
	if listed["MOD-0001"] != 2 || listed["MOD-0002"] != 1 {
		t.Fatalf("billing's inside lists both couplings of its table, ledger's its own: %v", listed)
	}
	inferred := 0
	for _, e := range g.Edges {
		if e.Origin == "INFERRED" && e.Via == "data" && strings.Contains(e.Quote, "charges") {
			inferred++
		}
	}
	if inferred != 2 {
		t.Fatalf("graph.json should carry both couplings with origin, via and quote: %+v", g.Edges)
	}

	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.resync()
	p.laterRevision()
	p.resync()
	if strings.Contains(p.blueprint(), "ledger → charges table") {
		t.Fatalf("a coupling whose evidence is gone leaves the blueprint")
	}
}

// A module's text comes from a reader. The blueprint shows it beside the
// module's inside and marks it STALE once the module's code moves on.
func TestTheBlueprintShowsHowsTextAndMarksItStale(t *testing.T) {
	p := semanticProject(t)
	text := p.file(t, "how.md", "### Overview\nbilling writes each `charge` to the charges table.\n\n### Key Concepts\n- `Charge`: records one payment.\n")
	p.want(1, "is not a module", "understand", "DATA-0001", text)
	p.want(0, "understood MOD-0001 billing, INFERRED", "understand", "MOD-0001", text)
	html := p.blueprint()
	for _, want := range []string{`id="m-MOD-0001"`, `href="#m-MOD-0001"`, `billing writes each <code>charge</code> to the charges table.`, `리더가 쓴 설명`} {
		if !strings.Contains(html, want) {
			t.Fatalf("the module's inside should carry how's text; missing %s", want)
		}
	}
	if strings.Contains(html, `title="이 설명을 쓴 뒤`) {
		t.Fatalf("a text written against the code as it is must not be STALE")
	}
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge(currency string) {}\n")
	p.resync()
	if !strings.Contains(p.blueprint(), `title="이 설명을 쓴 뒤`) {
		t.Fatalf("the text should turn STALE once the module's code changes")
	}
	p.want(1, "code changed since this text was written", "understand", "MOD-0001", text)
}

// Whoever read the module is named with its text, so a reading done without
// pstack is told apart from one done with it.
func TestTheReaderOfAModuleTextIsKeptAndExplained(t *testing.T) {
	p := semanticProject(t)
	text := p.file(t, "how.md", "## Overview\nbilling writes each `charge` to the charges table.\n\n## Key Concepts\n- **Charge** (`billing/charge.go`): records one payment.\n")
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", text, "--reader", "abbd reader")
	stored, err := os.ReadFile(filepath.Join(p.root, ".abbd", "understanding", "MOD-0001.md"))
	if err != nil || !strings.Contains(string(stored), "reader: abbd reader\n") {
		t.Fatalf("the stored text should name its reader: %v\n%s", err, stored)
	}
	p.want(0, "how (INFERRED by abbd reader, CURRENT):", "explain", "MOD-0001")
	p.want(1, "pass --reader only with a new text", "understand", "MOD-0001", "--restamp", "--reader", "abbd reader")
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", text)
	p.want(0, "how (INFERRED, CURRENT):", "explain", "MOD-0001")
}

// Saving the text again with other line endings or trailing blanks does not
// make it a new reading, so it cannot clear STALE either.
func TestRestampingTheSameTextWithOtherLineEndingsIsRefused(t *testing.T) {
	p := semanticProject(t)
	words := "### Overview\nbilling writes each `charge` to the charges table.\n\n### Key Concepts\n- `Charge`: records one payment.\n"
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", p.file(t, "how.md", words))
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge(currency string) {}\n")
	p.resync()
	crlf := strings.ReplaceAll(strings.ReplaceAll(words, "\n", "  \r\n"), "Overview", "Overview\t")
	p.want(1, "code changed since this text was written", "understand", "MOD-0001", p.file(t, "crlf.md", crlf))
	if !strings.Contains(p.blueprint(), `title="이 설명을 쓴 뒤`) {
		t.Fatalf("the text must stay STALE")
	}
}

// Editors on Windows prepend a byte order mark when they save UTF-8; it is
// not a word of the text, so it cannot make the same reading look new.
func TestRestampingTheSameTextWithAByteOrderMarkIsRefused(t *testing.T) {
	p := semanticProject(t)
	words := "### Overview\nbilling writes each `charge` to the charges table.\n"
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", p.file(t, "how.md", words))
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge(currency string) {}\n")
	p.resync()
	p.want(1, "code changed since this text was written", "understand", "MOD-0001", p.file(t, "bom.md", "\uFEFF"+words))
}

// status names the units that say nothing of what they are for.
func TestStatusReportsUndescribedUnits(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "described: 3 unit(s) without a purpose or responsibility: DATA-0001, MOD-0001, MOD-0002\n", "status")
	p.want(0, "", "record", "fact", "MOD-0001.responsibility", "--value", "writes each charge", "--source", "pr 1")
	p.want(0, "described: 2 unit(s) without a purpose or responsibility: DATA-0001, MOD-0002\n", "status")
}

// status says how many top-level modules carry a text, who read them, and
// which carry none.
func TestStatusSaysWhichModulesHaveAText(t *testing.T) {
	p := semanticProject(t)
	p.want(0, "coverage: 0 of 2 top-level module(s) and feature(s) have a text; none yet: MOD-0001, MOD-0002\n", "status")
	text := p.file(t, "how.md", "### Overview\nbilling charges.\n")
	p.want(0, "", "understand", "MOD-0001", text, "--reader", "abbd reader")
	p.want(0, "coverage: 1 of 2 top-level module(s) and feature(s) have a text, read by abbd reader 1; none yet: MOD-0002\n", "status")
}

