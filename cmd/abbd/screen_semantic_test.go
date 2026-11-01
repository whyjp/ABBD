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
