package main

import (
	"encoding/json"
	"strings"
	"testing"
)

type graphFile struct {
	Units []struct {
		ID, Parent string
		Evidence   []string
		Couplings  []struct{ Key, Via string }
	} `json:"units"`
	Edges []struct {
		Class, Title, Via, Quote string
	} `json:"edges"`
}

func (p *project) graph() graphFile {
	p.t.Helper()
	var g graphFile
	if err := json.Unmarshal([]byte(p.read("generated/graph.json")), &g); err != nil {
		p.t.Fatal(err)
	}
	return g
}

// A human judges a guessed coupling by how it arises and the code it rests
// on, so the screen shows both beside the dashed edge.
func TestTheScreenShowsASemanticCouplingWithItsEvidence(t *testing.T) {
	p := activeSemanticProject(t)
	html := p.read("generated/explorer.html")
	for _, want := range []string{
		`추정 결합 <span class="key">MOD-0002.consumes.DATA-0001</span> via data`,
		`<q>ledger/book.go:3-4 &#34;SELECT amount FROM charges&#34;</q>`,
		`근거 <q>billing/charge.go:3 &#34;INSERT INTO charges&#34;</q>`,
		`MOD-0001 안`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("the unit card should show the coupling, its via and its quote; missing %s", want)
		}
	}
	g := p.graph()
	dashed := 0
	for _, e := range g.Edges {
		if e.Class == "guess" && e.Via == "data" && strings.Contains(e.Quote, "charges") && strings.Contains(e.Title, "via data") {
			dashed++
		}
	}
	parent := false
	for _, u := range g.Units {
		parent = parent || (u.ID == "DATA-0001" && u.Parent == "MOD-0001")
	}
	if dashed != 2 || !parent {
		t.Fatalf("graph.json should carry both dashed couplings with via and quote, and the unit's parent: %+v", g)
	}

	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.want(0, "active", "mode", "on", "--session", "s1")
	if strings.Contains(p.read("generated/explorer.html"), "MOD-0002.consumes.DATA-0001") {
		t.Fatalf("a coupling whose evidence is gone leaves the screen")
	}
}

// A module's text comes from pstack's how. The screen shows it beside the
// module's inside and marks it STALE once the module's code moves on.
func TestTheScreenShowsHowsTextAndMarksItStale(t *testing.T) {
	p := activeSemanticProject(t)
	text := p.file(t, "how.md", "### Overview\nbilling writes each `charge` to the charges table.\n\n### Key Concepts\n- `Charge`: records one payment.\n")
	p.want(1, "is not a top-level module", "understand", "DATA-0001", text)
	p.want(0, "understood MOD-0001 billing, INFERRED", "understand", "MOD-0001", text)
	html := p.screen().view(t)
	for _, want := range []string{`id="m-MOD-0001"`, `href="#m-MOD-0001"`, `billing writes each <code>charge</code> to the charges table.`, `pstack how가 쓴 설명`} {
		if !strings.Contains(html, want) {
			t.Fatalf("the module's inside should carry how's text; missing %s", want)
		}
	}
	if strings.Contains(html, `title="이 설명을 쓴 뒤`) {
		t.Fatalf("a text written against the code as it is must not be STALE")
	}
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge(currency string) {}\n")
	if !strings.Contains(p.screen().view(t), `title="이 설명을 쓴 뒤`) {
		t.Fatalf("the text should turn STALE once the module's code changes")
	}
}
