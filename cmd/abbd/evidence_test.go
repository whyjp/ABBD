package main

import (
	"strings"
	"testing"
)

// A quote that moved to another file is reported where it is now.
func TestEvidenceNamesTheFileAMovedQuoteIsIn(t *testing.T) {
	p, exe := bare(t)
	const call = "billing.Charge(ctx, order.Amount, order.Currency)"
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {\n\t"+call+"\n}\n")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger"}]}
]}`)
	p.setup(0, "synced", "--", exe)
	p.want(0, "INFERRED", "record", "relation", "MOD-0002", "depends_on", "MOD-0001", "--via", "call", "--evidence", "ledger/book.go:4:"+call)
	p.code("ledger/book.go", "package ledger\n")
	p.code("ledger/post.go", "package ledger\n\n// Post books an order.\nfunc Post() {\n\t"+call+"\n}\n")
	out := p.want(0, "1 live, 0 dead", "evidence")
	if !strings.Contains(out, "found at ledger/post.go:5") {
		t.Fatalf("a moved quote should be reported in its new file:\n%s", out)
	}
}
