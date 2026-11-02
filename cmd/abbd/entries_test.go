package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// entriesBase is an http api whose routes the script reads one by one, and
// a page that calls one of them.
const entriesBase = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"page","kind":"FEAT","name":"charges page","anchors":[{"path":"web/charges.ts","symbol":"Charges"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"/api","digest":"a1","anchors":[{"path":"api/http.go","symbol":"Serve"}],
  "entries":[{"name":"GET /charges","shape":"-> []Charge"},{"name":"GET /charges/{id}","shape":"-> Charge"},{"name":"POST /refunds","shape":"Refund -> 201"}]}
],"relations":[
 {"from":"billing","to":"api","kind":"provides"},
 {"from":"page","to":"api","kind":"consumes","entries":["GET /charges"]}
]}`

func entriesProject(t *testing.T) (*project, string) {
	t.Helper()
	p, exe := bare(t)
	p.code("api/http.go", "package api\n\nfunc Serve() {\n\thandle(\"GET /charges/{id}\", charge)\n}\n")
	p.extract(entriesBase)
	p.setup(0, "synced: 3 unit(s)", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	git(t, p.root, "branch", "-M", "main")
	return p, rev(t, p.root)
}

// Each entry is stored on its own, with the key a note names it by, and
// explain lists it with the units that use it.
func TestAContractKeepsEachEntryWithItsUsers(t *testing.T) {
	p, _ := entriesProject(t)
	file := p.read("architecture/IF-0001.md")
	for _, want := range []string{"key: get-charges-id", "name: GET /charges/{id}", "shape: -> Charge"} {
		if !strings.Contains(file, want) {
			t.Errorf("IF-0001.md misses %q:\n%s", want, file)
		}
	}
	if feat := p.read("features/FEAT-0001.md"); !strings.Contains(feat, "entries:\n        - get-charges") {
		t.Errorf("the page's relation names the entry it uses:\n%s", feat)
	}
	out := p.want(0, "entries:", "explain", "IF-0001")
	for _, want := range []string{
		"get-charges: GET /charges -> []Charge; consumes by FEAT-0001 charges page",
		"get-charges-id: GET /charges/{id} -> Charge",
		"post-refunds: POST /refunds Refund -> 201",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("explain misses %q:\n%s", want, out)
		}
	}
}

// A change to one route is that route's own line, and it reaches the
// boundary; a page calling one more route of an api it already calls is a
// line too, but stays inside the boundary.
func TestDiffNamesEachEntryThatChanged(t *testing.T) {
	p, base := entriesProject(t)
	now := strings.NewReplacer(
		`{"name":"GET /charges/{id}","shape":"-> Charge"}`, `{"name":"GET /charges/{id}","shape":"-> ChargeView"}`,
		`,{"name":"POST /refunds","shape":"Refund -> 201"}`, `,{"name":"DELETE /charges/{id}"}`,
	).Replace(entriesBase)
	p.extract(now)
	p.want(0, "", "sync")
	out := p.want(1, "reaches the boundary", "diff", "--base", base, "--check")
	for _, want := range []string{
		`~ entry IF-0001 http api GET /charges/{id}: "-> Charge" -> "-> ChargeView"`,
		"- entry IF-0001 http api POST /refunds Refund -> 201",
		"+ entry IF-0001 http api DELETE /charges/{id}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "~ contract") || strings.Contains(out, "GET /charges:") {
		t.Errorf("only the entries that changed are lines:\n%s", out)
	}
	var d struct {
		Entries []struct{ ID, Entry, Change, Old, New string } `json:"contract_entries_changed"`
	}
	if err := json.Unmarshal([]byte(p.want(0, "", "diff", "--base", base, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != 3 {
		t.Fatalf("--json carries each entry: %+v", d)
	}

	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "routes")
	p.extract(strings.Replace(now, `"entries":["GET /charges"]`, `"entries":["GET /charges","GET /charges/{id}"]`, 1))
	p.want(0, "", "sync")
	out = p.want(0, "+ entry use FEAT-0001 consumes IF-0001 GET /charges/{id} (charges page consumes http api)", "diff", "--base", "HEAD", "--check")
	if strings.Contains(out, "+ relation") {
		t.Errorf("the page already consumed the api:\n%s", out)
	}
}

// The revision that first lists a contract's entries says so once, not once
// per entry, and an older store with no entries still loads.
func TestDiffSaysOnceThatAContractNowListsItsEntries(t *testing.T) {
	p, exe := bare(t)
	p.extract(strings.Replace(diffBase, `"signature":"GET /charges"`, `"signature":"GET /charges,GET /charges/{id}"`, 1))
	p.setup(0, "synced", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	now := strings.Replace(diffBase, `"signature":"GET /charges"`, `"signature":"/","entries":[{"name":"GET /charges"},{"name":"GET /charges/{id}"}]`, 1)
	p.extract(strings.Replace(now, `{"from":"billing","to":"api","kind":"provides"}`, `{"from":"billing","to":"api","kind":"provides","entries":["GET /charges"]}`, 1))
	p.want(0, "", "sync")
	out := p.want(1, "~ contract IF-0001 http api now lists its 2 entries", "diff", "--base", "HEAD", "--check")
	if strings.Contains(out, "+ entry") {
		t.Errorf("adopting entries is one line:\n%s", out)
	}
}

// A reader's note on an entry is that entry's description: it shows on the
// entry's row, goes stale on its own, and a note on an entry the contract
// no longer lists is orphaned.
func TestANoteDescribesOneEntry(t *testing.T) {
	p, _ := entriesProject(t)
	p.wantNote(1, "IF-0001 lists no entry get-refund", "IF-0001.entries.get-refund", "Refunds one charge.", `api/http.go:4:handle("GET /charges/{id}", charge)`)
	p.wantNote(0, "understood IF-0001.entries.get-charges-id http api", "IF-0001.entries.get-charges-id", "One charge, with its refunds.", `api/http.go:4:handle("GET /charges/{id}", charge)`)
	p.want(0, "notes: 1 slot note(s), 0 STALE, 0 orphaned", "status", "--check")
	page := p.read("blueprint/index.html")
	for _, want := range []string{`<tr id="e-IF-0001.get-charges-id">`, "<code>GET /charges/{id}</code>", "<p>One charge, with its refunds.</p>", ">charges page<"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page misses %q", want)
		}
	}
	if strings.Contains(page, `id="n-IF-0001.entries.get-charges-id"`) {
		t.Errorf("an entry's note shows on its row, not among the contract's notes")
	}

	p.extract(strings.Replace(entriesBase, `{"name":"GET /charges/{id}","shape":"-> Charge"},`, "", 1))
	p.want(0, "", "sync")
	p.want(0, "orphaned: IF-0001 lists no entry get-charges-id now", "status", "--check")
}

// Entries are what the code spells, so the extraction gives each its own
// key, names only entries its target lists, and a supplement lists none.
func TestTheExtractionGuardsEntries(t *testing.T) {
	p, exe := bare(t)
	p.extract(strings.Replace(entriesBase, `{"name":"POST /refunds","shape":"Refund -> 201"}`, `{"name":"GET /charges/{ID}"}`, 1))
	p.setup(1, `share the key "get-charges-id"`, "--", exe)
	p.extract(strings.Replace(entriesBase, `"entries":["GET /charges"]`, `"entries":["GET /nope"]`, 1))
	p.setup(1, `its target has no entry "GET /nope"`, "--", exe)
	p.extract(strings.Replace(entriesBase, `{"key":"billing","kind":"MOD","name":"billing",`, `{"key":"billing","kind":"MOD","name":"billing","entries":[{"name":"x"}],`, 1))
	p.setup(1, "only contracts carry entries", "--", exe)
}

// One line of an edge that names no entries uses the whole contract, so a
// second, narrowed line of the same edge leaves the claim whole.
func TestAWholeUseOutweighsANarrowedLineOfTheSameEdge(t *testing.T) {
	p, exe := bare(t)
	p.extract(strings.Replace(entriesBase, `{"from":"page","to":"api","kind":"consumes","entries":["GET /charges"]}`,
		`{"from":"page","to":"api","kind":"consumes"},{"from":"page","to":"api","kind":"consumes","entries":["GET /charges"]}`, 1))
	p.setup(0, "synced: 3 unit(s)", "--", exe)
	if feat := p.read("features/FEAT-0001.md"); strings.Contains(feat, "entries:") {
		t.Errorf("the page consumes the whole api:\n%s", feat)
	}
}

// A use that comes to take the whole contract is one line saying so, not a
// line per entry it seems to drop.
func TestDiffSaysAUseWidenedToTheWholeContract(t *testing.T) {
	p, _ := entriesProject(t)
	p.extract(strings.Replace(entriesBase, `,"entries":["GET /charges"]`, "", 1))
	p.want(0, "", "sync")
	out := p.want(0, "~ entry use FEAT-0001 consumes IF-0001 now uses the whole contract (charges page consumes http api)", "diff", "--base", "HEAD")
	if strings.Contains(out, "- entry use") {
		t.Errorf("widening a use drops no entry:\n%s", out)
	}
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "whole")
	p.extract(entriesBase)
	p.want(0, "", "sync")
	out = p.want(0, "~ entry use FEAT-0001 consumes IF-0001 now uses only some of its entries (charges page consumes http api)", "diff", "--base", "HEAD")
	if strings.Contains(out, "+ entry use") {
		t.Errorf("the page used every entry before:\n%s", out)
	}
}

// A name with no letters or digits a key takes, such as Hangul, needs an
// explicit key, and the error says so.
func TestAnEntryWithNoKeyableNameAsksForAKey(t *testing.T) {
	p, exe := bare(t)
	p.extract(strings.Replace(entriesBase, `{"name":"POST /refunds","shape":"Refund -> 201"}`, `{"name":"금액"}`, 1))
	p.setup(1, `give it a "key"`, "--", exe)
	p.extract(strings.Replace(entriesBase, `{"name":"POST /refunds","shape":"Refund -> 201"}`, `{"name":"금액","key":"amount"}`, 1))
	p.setup(0, "synced: 3 unit(s)", "--", exe)
}

// Two contracts may each list an entry of the same key, and each row keeps
// an id of its own on the page.
func TestEntryRowsOfTwoContractsKeepTheirOwnIDs(t *testing.T) {
	p, exe := bare(t)
	p.extract(strings.Replace(entriesBase, `],"relations":[`, `,
 {"key":"admin","kind":"IF","name":"admin api","signature":"/admin","anchors":[{"path":"api/admin.go","symbol":"Admin"}],"entries":[{"name":"GET /charges"}]}
],"relations":[{"from":"billing","to":"admin","kind":"provides"},`, 1))
	p.setup(0, "synced: 4 unit(s)", "--", exe)
	page := p.read("blueprint/index.html")
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(` id="(e-[^"]+)"`).FindAllStringSubmatch(page, -1) {
		if seen[m[1]] {
			t.Errorf("the page repeats the id %q", m[1])
		}
		seen[m[1]] = true
	}
	for _, want := range []string{"e-IF-0001.get-charges", "e-IF-0002.get-charges"} {
		if !seen[want] {
			t.Errorf("the page has no row %q", want)
		}
	}
}

// A long route or type with no space in it wraps inside its cell, so a
// table of entries stays as wide as the page; a browser measured this, and
// the page keeps the rules that make it so.
func TestALongEntryWrapsInsideItsCell(t *testing.T) {
	p, _ := entriesProject(t)
	css := p.read("blueprint/index.html")
	css = css[:strings.Index(css, "</style>")]
	if !regexp.MustCompile(`\.entries td > code \{[^}]*max-width: 40ch;[^}]*overflow-wrap: anywhere;`).MatchString(css) {
		t.Errorf("a cell's name or shape is capped and wraps anywhere")
	}
	if regexp.MustCompile(`\.entries td[.\w]* \{[^}]*white-space: nowrap`).MatchString(css) {
		t.Errorf("no entry cell refuses to wrap")
	}
}
