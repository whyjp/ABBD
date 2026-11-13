package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const whenTopo = `{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]},
 {"key":"notify","kind":"MOD","name":"notify","anchors":[{"path":"notify/mail.go","symbol":"Send"}]}
],"relations":[
 {"from":"billing","to":"ledger","kind":"depends_on","via":"import"WHEN},
 {"from":"notify","to":"ledger","kind":"depends_on","via":"import","when":"dev"}
]}`

// A dependency that comes to hold only under a feature is a change a
// reviewer must see: diff names the old and new condition, in text, in
// --json and under --unit of either end.
func TestDiffNamesAConditionThatChanged(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.extract(strings.Replace(whenTopo, "WHEN", "", 1))
	p.setup(0, "synced: 3 unit(s)", "--", exe)
	git(t, p.root, "add", "-A")
	git(t, p.root, "commit", "-qm", "base")
	base := rev(t, p.root)
	p.extract(strings.Replace(whenTopo, "WHEN", `,"when":"feature = \"audit\""`, 1))
	p.resync()

	line := `~ condition MOD-0001 depends_on MOD-0002: "" -> "feature = \"audit\"" (billing depends_on ledger)`
	out := p.want(0, line, "diff", "--base", base)
	if strings.Contains(out, "MOD-0003 depends_on") {
		t.Errorf("an unchanged condition is reported:\n%s", out)
	}
	var d struct {
		Conditions []struct {
			From, To string
			Old, New string
		} `json:"relation_conditions_changed"`
	}
	if err := json.Unmarshal([]byte(p.want(0, "", "diff", "--base", base, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Conditions) != 1 || d.Conditions[0].From != "MOD-0001" || d.Conditions[0].Old != "" || d.Conditions[0].New != `feature = "audit"` {
		t.Errorf("--json does not carry the condition change: %+v", d.Conditions)
	}
	p.want(0, line, "diff", "--base", base, "--unit", "MOD-0002")
	if out := p.want(0, "", "diff", "--base", base, "--unit", "MOD-0003"); strings.Contains(out, "~ condition") {
		t.Errorf("--unit keeps a condition change outside the unit:\n%s", out)
	}
}

// The script's conditions are rebuilt at every sync and the supplement's
// stay as setup gave them. A link scope prints as a scope, not a condition.
func TestSyncReplacesTheScriptsConditionAndKeepsTheSupplements(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\n// INSERT INTO charges (amount) VALUES (?)\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	topo := func(when string) string {
		return strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"import","when":"`+when+`"}]}`, 1)
	}
	sup := strings.Replace(chargesTable, `"kind":"consumes","via":"data",`, `"kind":"consumes","via":"data","when":"feature = \"books\"",`, 1)
	p.extract(topo("dev"))
	p.setup(0, "synced: 3 unit(s)", "--supplement", p.file(t, "sup.json", sup), "--", exe)
	contains(t, p.want(0, "", "explain", "MOD-0002"), "via import, when dev)", `when feature = "books"`)

	p.extract(topo("build"))
	p.resync()
	out := p.want(0, "", "explain", "MOD-0002")
	contains(t, out, "via import, when build)", `when feature = "books"`)
	if strings.Contains(out, "when dev") {
		t.Errorf("sync kept the script's old condition:\n%s", out)
	}

	p.extract(topo("PRIVATE"))
	p.resync()
	out = p.want(0, "", "explain", "MOD-0002")
	contains(t, out, "via import, scope PRIVATE)")
	if strings.Contains(out, "when PRIVATE") {
		t.Errorf("a link scope prints as a condition:\n%s", out)
	}
}
