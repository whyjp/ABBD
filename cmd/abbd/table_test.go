package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chargeTable asks what each part of the charge path does to the charges
// table, with a plain column beside it. Both cells under the table's column
// ride a stored relation: billing provides the table and the ledger
// consumes it.
const chargeTable = `---
question: Which part of the charge path touches the charges table?
columns:
  - {name: charges, unit: DATA-0001}
  - {name: retries}
rows:
  - unit: MOD-0001
    cells: {charges: inserts one row per charge, retries: tries a failed insert three times}
    evidence:
      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}
      - {path: billing/retry.go, lines: "3", quote: "const tries = 3"}
  - unit: MOD-0002
    cells: {charges: reads the amount of each charge}
    evidence:
      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}
---

The ledger reads what billing writes.
`

func (p *project) table(code int, contains, draft string, extra ...string) string {
	p.t.Helper()
	return p.want(code, contains, append([]string{"table", "charge", p.file(p.t, "table.md", draft)}, extra...)...)
}

// A table is stored with each row stamped on the code it cites, and the page
// draws it as a grid whose row heads open the inside that shows each unit.
func TestATableIsStoredAndDrawnAsAGrid(t *testing.T) {
	p := notesProject(t)
	out := p.table(0, "stored table charge, 2 rows, INFERRED", chargeTable)
	if !strings.Contains(out, "cells: 0 cell(s) say a use no stored relation carries, 0 empty cell(s) sit over a stored relation") {
		t.Errorf("a store says how many cells the relations do not bear out:\n%s", out)
	}
	stored := p.read("tables/charge.md")
	for _, want := range []string{"question: Which part of the charge path touches the charges table?", "columns:", "unit: DATA-0001", "rows:", "writer: reader", "code:", "The ledger reads what billing writes."} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored table must hold %q:\n%s", want, stored)
		}
	}
	status := p.want(0, "tables: 1 table(s), 2 row(s), 0 STALE", "status", "--check")
	if !strings.Contains(status, "cells: 0 cell(s) say a use no stored relation carries") {
		t.Errorf("status reports the table's cells:\n%s", status)
	}
	page := p.blueprint()
	for _, want := range []string{`id="table-charge"`, `id="table-charge.2"`, `<table class="dg-matrix">`, `data-col="charges"`, "inserts one row per charge", `title="저장된 관계 사용"`, `href="MOD-0002.html" title="MOD-0002"`, `<a href="table-charge.html">Which part of the charge path touches the charges table?</a>`, "2행 × 2열", "이 모듈이 행으로 놓인 표", `href="table-charge.html"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page must draw %q", want)
		}
	}
	if graph := p.read("blueprint/graph.json"); !strings.Contains(graph, `"tables": [`) || !strings.Contains(graph, `"Says": "reads the amount of each charge"`) {
		t.Error("graph.json must hold the table")
	}
	if out := p.want(0, "question: Which part of the charge path touches the charges table?", "table", "charge"); !strings.Contains(out, "unit: DATA-0001") || !strings.Contains(out, "const tries = 3") || strings.Contains(out, "code:") {
		t.Errorf("abbd table KEY prints the stored table as a draft, without what abbd sets:\n%s", out)
	}
}

// A draft enters only as a grid true of the state: two columns or more, each
// a short label named once, each unit heading one row, each cell under a
// named column, each unit a column or a row names present, and each row's
// quotes in its own unit's code.
func TestATableDraftIsCheckedBeforeItIsStored(t *testing.T) {
	p := notesProject(t)
	for _, c := range []struct{ from, to, want string }{
		{"cells: {charges: reads the amount of each charge}", "cells: {charge: reads the amount of each charge}", `row 2 fills a cell under "charge", which no column names`},
		{"  - unit: MOD-0002\n    cells: {charges: reads the amount of each charge}\n    evidence:\n      - {path: ledger/book.go, lines: \"6\", symbol: Post, quote: \"func Post() {}\"}",
			"  - unit: MOD-0001\n    cells: {charges: reads the amount of each charge}\n    evidence:\n      - {path: billing/charge.go, lines: \"4\", symbol: Charge, quote: \"func Charge() {}\"}",
			"MOD-0001 heads row 1 and row 2; a unit heads one row"},
		{"unit: DATA-0001}", "unit: DATA-0009}", `DATA-0009, which column "charges" names, is no longer in the state`},
		{"  - {name: retries}", "  - {name: charges}", `column "charges" is named twice`},
		{"  - {name: retries}", "  - {name: ' retries'}", "has no spaces around it"},
		{"  - {name: retries}", "  - {name: how many times a failed insert is tried again}", "is a short label of 32 characters at most"},
		{"  - {name: retries}\n", "", "a table names 2 columns or more"},
		{"cells: {charges: reads the amount of each charge}", "cells: {charges: ''}", `the cell under "charges" is empty`},
		{`      - {path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}`, `      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}`, "row 2: none of the row's quotes is in the code of MOD-0002"},
		{"const tries = 3", "const tries = 4", "the code does not hold"},
	} {
		draft := strings.Replace(chargeTable, c.from, c.to, 1)
		if draft == chargeTable {
			t.Fatalf("the case %q changes nothing", c.want)
		}
		p.table(1, c.want, draft)
	}
	one := chargeTable[:strings.Index(chargeTable, "  - unit: MOD-0002")] + "---\n"
	p.table(1, "a table has 2 rows or more, and this one has 1", one)
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "tables", "charge.md")); err == nil {
		t.Error("a refused draft must store nothing")
	}
	p.want(1, "a flow has no columns", "flow", "charge", p.file(t, "flow.md", strings.Replace(chargeFlow, "steps:", "columns:\n  - {name: a}\n  - {name: b}\nsteps:", 1)))
}

// An edit near one row's quote stales that row alone, and the check fails
// until the table is read again; the same row on the moved code is refused.
func TestATableRowGoesStaleOnItsOwn(t *testing.T) {
	p := notesProject(t)
	p.table(0, "stored", chargeTable)
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	out := p.want(1, "1 table row(s) STALE", "status", "--check")
	if !strings.Contains(out, "stale: table charge row 1 (MOD-0001): billing/retry.go (the quote is gone") || strings.Contains(out, "row 2 (") {
		t.Errorf("only row 1 cites the changed code:\n%s", out)
	}
	if page := p.blueprint(); !strings.Contains(page, `<tr class="row stale" id="table-charge.1">`) {
		t.Error("the page marks the STALE row")
	}
	requoted := strings.Replace(chargeTable, "const tries = 3", "const tries = 5", 1)
	p.table(1, "row 1 says what a STALE row said", requoted)
	p.table(0, "stored", strings.Replace(requoted, "tries a failed insert three times", "tries a failed insert five times", 1))
	p.want(0, "tables: 1 table(s), 2 row(s), 0 STALE", "status", "--check")
}

// A cell that says a use no stored relation carries, and an empty cell over
// a stored relation, are reported by the store and by status and marked on
// the page, and neither fails the check: only a reader of the code can say
// whether the relation or the cell is wrong.
func TestACellTheRelationsDoNotBearOutIsReportedNotChecked(t *testing.T) {
	p := notesProject(t)
	draft := strings.NewReplacer(
		"  - {name: retries}", "  - {name: retries}\n  - {name: books, unit: MOD-0002}",
		"retries: tries a failed insert three times}", "retries: tries a failed insert three times, books: hands each charge to the books}",
		"cells: {charges: reads the amount of each charge}", "cells: {retries: never retries a read}",
	).Replace(chargeTable)
	p.table(0, "cells: 1 cell(s) say a use no stored relation carries, 1 empty cell(s) sit over a stored relation", draft)
	out := p.want(0, "cells: 1 cell(s) say a use no stored relation carries, 1 empty cell(s) sit over a stored relation; reported, not checked", "status", "--check")
	for _, want := range []string{
		`unbacked: table charge row 1: MOD-0001 billing says "hands each charge to the books" under MOD-0002 ledger, and no stored relation joins them`,
		`unsaid: table charge row 2: MOD-0002 ledger consumes DATA-0001 charges table (INFERRED), and the cell under "charges" is empty`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status must report %q:\n%s", want, out)
		}
	}
	page := p.blueprint()
	for _, want := range []string{`class="unbacked" title="저장된 관계 없음"`, `class="unsaid" title="저장된 관계 사용"`, "◦ 사용", "관계 없는 칸 1", "빈 칸 1"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page must mark the cell %q", want)
		}
	}
}

// auditTable asks the charge table's question of billing and audit, with a
// column on the ledger, so removing the ledger leaves a column whose unit
// is gone and no row that fills it.
func auditTable() string {
	return strings.NewReplacer(
		"  - {name: charges, unit: DATA-0001}", "  - {name: books, unit: MOD-0002}",
		"cells: {charges: inserts one row per charge,", "cells: {",
		"  - unit: MOD-0002\n    cells: {charges: reads the amount of each charge}", "  - unit: MOD-0003\n    cells: {retries: logs each attempt}",
		`{path: ledger/book.go, lines: "6", symbol: Post, quote: "func Post() {}"}`, `{path: audit/log.go, lines: "3", symbol: Log, quote: "func Log() {}"}`,
	).Replace(chargeTable)
}

func (p *project) dropLedger() {
	p.t.Helper()
	p.extract(strings.Replace(auditTopo, " {\"key\":\"ledger\",\"kind\":\"MOD\",\"name\":\"ledger\",\"anchors\":[{\"path\":\"ledger/book.go\",\"symbol\":\"Post\"}]},\n", "", 1))
	p.resync()
}

// A table whose column's unit left the state fails the check until it is
// stored again without that column's unit.
func TestATableOnAColumnThatLeftFailsTheCheck(t *testing.T) {
	p := notesProject(t)
	p.code("audit/log.go", "package audit\n\nfunc Log() {}\n")
	p.extract(auditTopo)
	p.resync()
	p.table(0, "stored", auditTable())
	p.dropLedger()
	p.want(1, `stale: table charge: MOD-0002, which column "books" names, is no longer in the state`, "status", "--check")
	p.table(1, `MOD-0002, which column "books" names, is no longer in the state`, auditTable())
	p.table(0, "stored", strings.Replace(auditTable(), "  - {name: books, unit: MOD-0002}", "  - {name: books}", 1))
	p.want(0, "tables: 1 table(s), 2 row(s), 0 STALE", "status", "--check")
}

// A human's column whose unit left the state is open to a reader, so CI
// never waits on the human: the reader may drop the column's unit or point
// it elsewhere, keeps the human's other columns and rows as written, and is
// told it changed the human's head.
func TestAReaderMendsAHumansColumnWhoseUnitLeft(t *testing.T) {
	p := notesProject(t)
	p.code("audit/log.go", "package audit\n\nfunc Log() {}\n")
	p.extract(auditTopo)
	p.resync()
	p.table(0, "USER_STATED", auditTable(), "--source", "conversation 2026-11-04")
	p.dropLedger()
	p.want(1, "is no longer in the state", "status", "--check")
	dropped := strings.Replace(auditTable(), "  - {name: books, unit: MOD-0002}", "  - {name: books}", 1)
	p.table(1, "the column head of table charge is a human's word", strings.ReplaceAll(dropped, "retries", "tries"))
	out := p.table(0, "stored table charge, 2 rows, USER_STATED", dropped)
	if !strings.Contains(out, "kept the human's rows 1, 2 (conversation 2026-11-04)") || !strings.Contains(out, "changed the human's columns books, since their units left the state") {
		t.Errorf("the reader keeps the human's rows and says it changed the head:\n%s", out)
	}
	p.want(0, "tables: 1 table(s), 2 row(s), 0 STALE", "status", "--check")
}

// Renaming a column does not make a STALE row's cells new words: the same
// cells on the same changed code are refused under any column.
func TestARenamedColumnDoesNotBringAStaleRowBack(t *testing.T) {
	p := notesProject(t)
	p.table(0, "stored", chargeTable)
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	requoted := strings.Replace(chargeTable, "const tries = 3", "const tries = 5", 1)
	p.table(1, "row 1 says what a STALE row said", strings.ReplaceAll(requoted, "retries", "retry"))
	p.table(1, "row 1 says what a STALE row said", strings.ReplaceAll(strings.ReplaceAll(requoted, "retries", "retry"), "charges", "rows"))
}

// A human's table stays until its code changes. A reader may not change a
// current one, and once a row is STALE the reader replaces that row alone
// and keeps the human's head.
func TestAReaderReplacesAHumansTableOnlyOnceItIsStale(t *testing.T) {
	p := notesProject(t)
	p.table(0, "stored table charge, 2 rows, USER_STATED", chargeTable, "--source", "conversation 2026-11-04")
	p.table(1, "only a human changes it", strings.Replace(chargeTable, "reads the amount of each charge", "reads each charge", 1))
	p.code("billing/retry.go", retryCode("const tries = 5", "var states = []string{\"pending\", \"charged\"}"))
	p.resync()
	reread := strings.Replace(strings.Replace(chargeTable, "const tries = 3", "const tries = 5", 1), "three times", "five times", 1)
	p.table(1, "the column head of table charge is a human's word", strings.ReplaceAll(reread, "retries", "tries"))
	p.table(0, "replaced the human's rows 1 (conversation 2026-11-04)", reread)
	if stored := p.read("tables/charge.md"); strings.Count(stored, "writer: reader") != 1 || strings.Count(stored, "writer: human") != 1 {
		t.Errorf("the reader's row must replace the human's stale one alone:\n%s", stored)
	}
}

// A store warns on a cell past 80 characters.
func TestALongCellWarns(t *testing.T) {
	p := notesProject(t)
	long := strings.Repeat(" and then some more", 5)
	if out := p.table(0, "stored", strings.Replace(chargeTable, "reads the amount of each charge", "reads the amount of each charge"+long, 1)); !strings.Contains(out, "long: table charge row 2 charges holds") {
		t.Errorf("a long cell warns:\n%s", out)
	}
}
