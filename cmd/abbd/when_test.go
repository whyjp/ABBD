package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	p.want(0, line, "diff", "--base", base, "--page")
	page, err := os.ReadFile(filepath.Join(p.root, ".abbd", "compare", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	contains(t, string(page), "조건 변경 1")
	if strings.Contains(string(page), "토폴로지에는 바뀐 것이 없습니다.") {
		t.Errorf("the compare page says nothing changed when a condition did")
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

// A CMake script reads every link but not the if() around it, so the
// supplement gives that condition on the edge the script prints. It shows
// after setup and after each sync, and one that differs from the script's is
// a conflict.
func TestTheSupplementGivesTheConditionOfAnEdgeTheScriptReads(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.code("CMakeLists.txt", "if(WIN32)\n  target_link_libraries(ledger PRIVATE billing)\nendif()\n")
	topo := func(when string) string {
		return strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"import"`+when+`}]}`, 1)
	}
	sup := p.file(t, "sup.json", `{"units":[],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"import","when":"WIN32",
 "evidence":[{"path":"CMakeLists.txt","lines":"1","quote":"if(WIN32)"}]}]}`)
	p.extract(topo(""))
	out := p.setup(0, "synced: 2 unit(s)", "--supplement", sup, "--", exe)
	if strings.Contains(out, "conflict:") {
		t.Errorf("a condition the script does not read is reported as a conflict:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0001 billing (extracted, via import; when WIN32, inferred)")
	contains(t, p.blueprint(), "조건: WIN32 (판독)")
	contains(t, p.read("blueprint/graph.json"), `"When": "WIN32"`, `"WhenOrigin": "INFERRED"`)
	p.resync()
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0001 billing (extracted, via import; when WIN32, inferred)")

	p.extract(topo(`,"when":"cfg(unix)"`))
	out = p.setup(0, "synced: 2 unit(s)", "--supplement", sup, "--", exe)
	contains(t, out, `conflict: supplement relation ledger -depends_on-> billing gives condition "WIN32", the script reads "cfg(unix)"; the script's shows`)
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0001 billing (extracted, via import, when cfg(unix))")

	// A link scope is not a condition, so the supplement's condition shows
	// over it.
	p.extract(topo(`,"when":"PRIVATE"`))
	if out := p.setup(0, "synced: 2 unit(s)", "--supplement", sup, "--", exe); strings.Contains(out, "conflict:") {
		t.Errorf("a condition over a link scope is reported as a conflict:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0001 billing (extracted, via import; when WIN32, inferred)")
}

// A sync that runs after the script starts reading another condition says
// so, as setup does, and so does a link scope the supplement gives over
// another the script reads: one link has one scope.
func TestSyncReportsAConditionTheScriptNowReadsOtherwise(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.code("CMakeLists.txt", "if(WIN32)\n  target_link_libraries(ledger PUBLIC billing)\nendif()\n")
	topo := func(when string) string {
		return strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"import"`+when+`}]}`, 1)
	}
	sup := func(when string) string {
		return p.file(t, "sup.json", `{"units":[],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"import","when":"`+when+`",
 "evidence":[{"path":"CMakeLists.txt","lines":"1","quote":"if(WIN32)"}]}]}`)
	}
	p.extract(topo(""))
	p.setup(0, "synced: 2 unit(s)", "--supplement", sup("WIN32"), "--", exe)
	p.extract(topo(`,"when":"cfg(unix)"`))
	contains(t, p.resync(), `conflict: MOD-0002.depends_on.MOD-0001: the supplement gives condition "WIN32", the script's "cfg(unix)" shows`)
	contains(t, p.want(0, "", "status"), `conflict: MOD-0002.depends_on.MOD-0001: the supplement gives condition "WIN32"`)

	p.extract(topo(`,"when":"PRIVATE"`))
	out := p.setup(0, "synced: 2 unit(s)", "--supplement", sup("PUBLIC"), "--", exe)
	contains(t, out, `conflict: supplement relation ledger -depends_on-> billing gives scope "PUBLIC", the script reads "PRIVATE"; the script's shows`)
	if n := strings.Count(out, "conflict:"); n != 1 {
		t.Errorf("setup reports the conflict %d times:\n%s", n, out)
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0001 billing (extracted, via import, scope PRIVATE)")
}

// The evidence of a condition the supplement gives on an edge the script
// reads is listed and counted like any claim's: drifted, then dead, until a
// reader retracts it. The condition never leaves the edge without a word.
func TestAConditionsEvidenceIsListedAndCountedUntilRetracted(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.code("CMakeLists.txt", "if(WIN32)\n  target_link_libraries(ledger PRIVATE billing)\nendif()\n")
	p.extract(strings.Replace(ledgerTopo, "\n]}", `],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"link"}]}`, 1))
	sup := p.file(t, "sup.json", `{"units":[],"relations":[{"from":"ledger","to":"billing","kind":"depends_on","via":"link","when":"WIN32",
 "evidence":[{"path":"CMakeLists.txt","lines":"1","quote":"if(WIN32)"}]}]}`)
	p.setup(0, "synced: 2 unit(s)", "--supplement", sup, "--", exe)
	contains(t, p.want(0, "", "evidence"), `live MOD-0002.depends_on.MOD-0001.when "WIN32": CMakeLists.txt`, "1 live, 0 drifted, 0 dead")

	p.laterRevision()
	p.code("CMakeLists.txt", "if(MSVC)\n  target_link_libraries(ledger PRIVATE billing)\nendif()\n")
	contains(t, p.resync(), "evidence: 1 drifted, 0 dead; abbd evidence lists them")
	contains(t, p.want(0, "", "evidence"), `MOD-0002.depends_on.MOD-0001.when "WIN32": CMakeLists.txt`, "0 live, 1 drifted, 0 dead")
	contains(t, p.want(0, "", "explain", "MOD-0002"), "when WIN32, inferred")

	p.laterRevision()
	contains(t, p.resync(), "evidence: 0 drifted, 1 dead; abbd evidence lists them")
	contains(t, p.want(0, "", "evidence"), `dead MOD-0002.depends_on.MOD-0001.when "WIN32"`, "0 live, 0 drifted, 1 dead")
	contains(t, p.want(0, "", "status"), "evidence: 0 drifted, 1 dead")
	if out := p.want(0, "", "explain", "MOD-0002"); strings.Contains(out, "WIN32") {
		t.Errorf("a dead condition still shows:\n%s", out)
	}

	p.want(0, "retracted MOD-0002.depends_on.MOD-0001.when", "retract", "MOD-0002.depends_on.MOD-0001.when", "--reason", "the if() is gone")
	if out := p.resync(); strings.Contains(out, "evidence:") {
		t.Errorf("a retracted condition is still counted:\n%s", out)
	}
	contains(t, p.want(0, "", "explain", "MOD-0002"), "MOD-0001 billing (extracted, via link)")
}
