package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// fieldsBase is a charges table whose columns the script reads with their
// types and whether each is required, billing writing the row and ledger
// reading two of its columns.
const fieldsBase = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"charges","kind":"DATA","name":"charges table","signature":"table charges","digest":"c1","anchors":[{"path":"billing/schema.sql","symbol":"charges"}],
  "entries":[{"name":"id","shape":"int64","required":true},{"name":"amount","shape":"numeric(12,2)","required":true},{"name":"note","shape":"text","required":false}]}
],"relations":[
 {"from":"billing","to":"charges","kind":"provides","entries":["id","amount","note"]},
 {"from":"ledger","to":"charges","kind":"consumes","entries":["id","amount"]}
]}`

// A field's table row says its type, whether it is required, who writes
// it and who reads it; a change to one field's type or need is that field's
// own line, and it reaches the boundary.
func TestADataContractShowsAndDiffsEachField(t *testing.T) {
	p, exe := bare(t)
	p.extract(fieldsBase)
	p.setup(0, "synced: 3 unit(s)", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")

	page := p.read("blueprint/index.html")
	for _, want := range []string{"<th>타입</th><th>필수</th><th>쓰는 단위</th><th>읽는 단위</th>", "<code>numeric(12,2)</code>", "<td>선택</td>"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page misses %q", want)
		}
	}
	row := page[strings.Index(page, `<tr id="e-amount">`):]
	row = row[:strings.Index(row, "</tr>")]
	if !strings.Contains(row, ">billing<") || !strings.Contains(row, ">ledger<") || strings.Index(row, ">billing<") > strings.Index(row, ">ledger<") {
		t.Errorf("amount's row names billing as its writer, then ledger as its reader:\n%s", row)
	}
	if out := p.want(0, "", "explain", "DATA-0001"); !strings.Contains(out, "amount: amount numeric(12,2) required; provides by MOD-0001 billing; consumes by MOD-0002 ledger") {
		t.Errorf("explain lists the field with its writer and reader:\n%s", out)
	}

	p.extract(strings.NewReplacer(
		`{"name":"amount","shape":"numeric(12,2)","required":true}`, `{"name":"amount","shape":"numeric(14,2)","required":true}`,
		`{"name":"note","shape":"text","required":false}`, `{"name":"note","shape":"text","required":true}`,
	).Replace(fieldsBase))
	p.want(0, "", "sync")
	out := p.want(1, "reaches the boundary", "diff", "--base", "HEAD", "--check")
	for _, want := range []string{
		`~ entry DATA-0001 charges table amount type: "numeric(12,2)" -> "numeric(14,2)"`,
		`~ entry DATA-0001 charges table note required: "optional" -> "required"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff misses %q:\n%s", want, out)
		}
	}
	var d struct {
		Entries []struct{ Entry, Field, Old, New string } `json:"contract_entries_changed"`
	}
	if err := json.Unmarshal([]byte(p.want(0, "", "diff", "--base", "HEAD", "--json")), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != 2 || d.Entries[0].Field != "shape" || d.Entries[1].Field != "required" {
		t.Fatalf("--json names the field of each entry that changed: %+v", d.Entries)
	}
}
