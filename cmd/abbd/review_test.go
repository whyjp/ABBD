package main

import (
	"strings"
	"testing"
)

// review names both users of a new route and the feature that holds one,
// and the whole-contract user of a route the change removed,
// lists what the change reaches, puts a coupling only a reader found under
// what is worth a look, and exits 0 whatever it finds.
func TestReviewNamesTheUsersOfANewEntry(t *testing.T) {
	p, base := entriesProject(t)
	p.code("web/refund.ts", "export function Refund() {}\n")
	p.extract(strings.NewReplacer(
		`,{"name":"POST /refunds","shape":"Refund -> 201"}]}`, `,{"name":"PUT /charges/{id}"}]},
 {"key":"cli","kind":"MOD","name":"cli","anchors":[{"path":"web/refund.ts","symbol":"Cli"}]},
 {"key":"button","kind":"FEAT","name":"refund button","parent":"page","anchors":[{"path":"web/refund.ts","symbol":"Refund"}]}`,
		`"entries":["GET /charges"]}`, `"entries":["GET /charges","PUT /charges/{id}"]},
 {"from":"button","to":"api","kind":"consumes","entries":["PUT /charges/{id}"]},
 {"from":"cli","to":"api","kind":"consumes"}`,
	).Replace(entriesBase))
	p.want(0, "", "sync")
	p.want(0, "INFERRED", "record", "relation", "MOD-0001", "depends_on", "FEAT-0001", "--via", "call", "--evidence", `api/http.go:4:handle("GET /charges/{id}", charge)`)
	out := p.want(0, "review since "+base[:12], "review", "--base", base)
	contains(t, out,
		"+ entry IF-0001 http api PUT /charges/{id}",
		"IF-0001 http api PUT /charges/{id}: FEAT-0001 charges page consumes (EXTRACTED); FEAT-0002 refund button consumes (EXTRACTED, in FEAT-0001 charges page); MOD-0002 cli consumes the whole contract (EXTRACTED)",
		"IF-0001 http api POST /refunds: MOD-0002 cli consumes the whole contract (EXTRACTED)",
		"what the change reaches directly:",
		"worth a look in what the change touched:\n  a reader's coupling no human stated: MOD-0001 depends_on FEAT-0001",
	)
}

// review seeds what the change reaches from the units a new relation really
// joins, and lists a STALE text only in what the change touched.
func TestReviewKeepsToWhatTheChangeTouched(t *testing.T) {
	p, base := diffProject(t)
	p.code("billing/charger.go", "package billing\n\nfunc Run() {}\n")
	p.code("api/http.go", "package api\n\nfunc Serve() {}\n")
	text := p.file(t, "t.md", "### Overview\nIt runs.\n")
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", text)
	p.want(0, "understood MOD-0004", "understand", "MOD-0004", text)
	p.code("billing/charger.go", "package billing\n\nfunc Run() { send() }\n")
	p.code("notify/mail.go", "package notify\n\nfunc Send() { api.Serve(); log() }\n")
	p.want(0, "", "sync")
	out := p.want(0, "", "review", "--base", base)
	contains(t, out, "MOD-0003 charger: seed", "the text of MOD-0001 billing is STALE", "the text of MOD-0004 notify is STALE")
}

// A STALE text on a unit the change did not touch stays out of review.
func TestReviewLeavesAStaleTextElsewhereOut(t *testing.T) {
	p, _ := entriesProject(t)
	p.code("web/charges.ts", "export function Charges() {}\n")
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.want(0, "", "sync")
	text := p.file(t, "t.md", "### Overview\nIt runs.\n")
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", text)
	p.want(0, "understood FEAT-0001", "understand", "FEAT-0001", text)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() { audit() }\n")
	p.want(0, "", "sync")
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "texts")
	base := rev(t, p.root)
	p.code("web/charges.ts", "export function Charges() { refund() }\n")
	p.code("web/refund.ts", "export function Refund() {}\n")
	p.extract(strings.Replace(entriesBase, `
 {"key":"page"`, `
 {"key":"button","kind":"FEAT","name":"refund button","parent":"page","anchors":[{"path":"web/refund.ts","symbol":"Refund"}]},
 {"key":"page"`, 1))
	p.want(0, "", "sync")
	out := p.want(0, "", "review", "--base", base)
	contains(t, out, "the text of FEAT-0001 charges page is STALE")
	if strings.Contains(out, "MOD-0001 billing is STALE") {
		t.Errorf("a STALE text the change did not touch is listed:\n%s", out)
	}
}

// A change that edits a module's code and leaves the topology alone still
// seeds review from the units anchored at the changed file.
func TestReviewSeedsFromChangedCode(t *testing.T) {
	p, _ := entriesProject(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.want(0, "", "sync")
	text := p.file(t, "t.md", "### Overview\nIt charges.\n")
	p.want(0, "understood MOD-0001", "understand", "MOD-0001", text)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "text")
	base := rev(t, p.root)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() { retry() }\n")
	p.want(0, "", "sync")
	out := p.want(0, "", "review", "--base", base)
	contains(t, out, "none in the topology", "code changed under MOD-0001", "MOD-0001 billing: seed", "the text of MOD-0001 billing is STALE")
}

// A changed file whose name has a space still finds the unit anchored at it.
func TestReviewSeedsFromAChangedFileWithASpace(t *testing.T) {
	p, exe := bare(t)
	p.code("pay ments/charge.go", "package pay")
	p.extract(`{"units":[{"key":"pay","kind":"MOD","name":"payments","anchors":[{"path":"pay ments/charge.go","symbol":"Charge"}]}]}`)
	p.setup(0, "synced", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	base := rev(t, p.root)
	p.code("pay ments/charge.go", "package pay\n\nfunc Charge() {}\n")
	p.want(0, "", "sync")
	contains(t, p.want(0, "", "review", "--base", base), "code changed under MOD-0001", "MOD-0001 payments: seed")
}
