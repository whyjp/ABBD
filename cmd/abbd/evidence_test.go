package main

import (
	"strings"
	"testing"
)

// A quote that moved to another file drifts: it never follows by itself, so
// a call that merely looks the same elsewhere cannot keep it alive. A reader
// refreshes it where it is now, and the refresh is idempotent.
func TestAMovedQuoteDriftsUntilAReaderRefreshesIt(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	const call = "billing.Charge(ctx, order.Amount, order.Currency)"
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {\n\t"+call+"\n}\n")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger"}]}
]}`)
	p.setup(0, "synced", "--", exe)
	edge := "MOD-0002.depends_on.MOD-0001"
	p.want(0, "INFERRED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:4:"+call)
	p.code("ledger/book.go", "package ledger\n")
	p.code("ledger/post.go", "package ledger\n\n// Post books an order.\nfunc Post() {\n\t"+call+"\n}\n")
	out := p.want(0, "0 live, 1 drifted, 0 dead", "evidence")
	contains(t, out, "drifted "+edge+" via call")
	contains(t, p.want(0, "", "sync"), "evidence: 1 drifted, 0 dead")
	contains(t, p.want(0, "", "status"), "evidence: 1 drifted, 0 dead")

	p.want(1, "the code does not hold", "record", "evidence", edge, "--evidence", "ledger/book.go:4:"+call)
	p.want(0, "refreshed "+edge, "record", "evidence", edge, "--evidence", "ledger/post.go:5:"+call)
	before := units(t, p.root)
	p.want(0, "refreshed "+edge, "record", "evidence", edge, "--evidence", "ledger/post.go:5:"+call)
	if !equal(before, units(t, p.root)) {
		t.Fatalf("a second identical refresh rewrote the state")
	}
	contains(t, p.want(0, "1 live, 0 drifted, 0 dead", "evidence"), "found at 5")
	contains(t, p.want(0, "", "status"), "evidence: 0 drifted, 0 dead")
}

// Refreshing a supplement claim puts a reader's claim above it. The
// supplement's stays stored, so a re-run of setup keeps the reader's in front.
func TestRefreshingASupplementCouplingPutsTheReadersAboveIt(t *testing.T) {
	t.Parallel()
	p := semanticProject(t)
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount, currency\n    FROM charges`\n\nfunc Post() {}\n")
	p.resync()
	edge := "MOD-0002.consumes.DATA-0001"
	p.want(0, "refreshed "+edge, "record", "evidence", edge, "--evidence", "ledger/book.go:3-4:SELECT amount, currency FROM charges")
	md := p.read("architecture/MOD-0002.md")
	if !strings.Contains(md, "writer: reader") || !strings.Contains(md, "hidden:") {
		t.Fatalf("the reader's claim shows and the supplement's stays stored beneath it:\n%s", md)
	}
	p.setup(0, "synced", "--supplement", p.file(t, "again.json", chargesTable))
	contains(t, p.want(0, "", "explain", "MOD-0002"), "consumes DATA-0001 charges table via data\n    ledger/book.go:3-4 \"SELECT amount, currency FROM charges\"")
}
