package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// chargeOverview is the charge path as a person would sketch it beside its
// primary flow: billing and the ledger in one zone, with the ledger's read
// of the charges table as the one chosen line.
const chargeOverview = `---
zones:
  - name: 돈
    units: [MOD-0001, MOD-0002]
    evidence:
      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}
lines:
  - from: MOD-0002
    to: MOD-0001
    word: 청구 금액
    evidence:
      - {path: ledger/book.go, lines: "3", quote: "const q = ` + "`" + `SELECT amount"}
---

장부는 청구를 읽는다.
`

func (p *project) overview(code int, contains, draft string) string {
	p.t.Helper()
	return p.want(code, contains, "overview", p.file(p.t, "overview.md", draft))
}

// An overview is stored with each part stamped, status counts it, explain
// says where a unit sits in it, its stage read from the stack marked
// primary, and graph.json gives the zones and the chosen lines with the
// relations under each.
func TestAnOverviewIsStoredAndGivenInGraphJSON(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.code(".abbd/project.yaml", p.read("project.yaml")+"language: ko\n")
	p.stack(0, "marked stack charge the primary flow", strings.Replace(chargeStack, "layers:\n", "primary: true\nlayers:\n", 1))
	p.overview(0, "stored the overview: 1 zones, 1 lines", chargeOverview)
	stored := p.read("overview.md")
	for _, want := range []string{"zones:", "format: 1", "code:", "장부는 청구를 읽는다."} {
		if !strings.Contains(stored, want) {
			t.Errorf("the stored overview must hold %q:\n%s", want, stored)
		}
	}
	p.want(0, "overview: 1 zones, 1 lines, 0 STALE", "status", "--check")
	out := p.want(0, "주 흐름 charge 2/2단계 billing", "explain", "DATA-0001")
	if !strings.Contains(out, "구역 돈") {
		t.Errorf("explain names the zone that holds the unit through its parent:\n%s", out)
	}
	if out := p.want(0, "name: 돈", "overview"); strings.Contains(out, "code:") || strings.Contains(out, "writer:") {
		t.Errorf("abbd overview prints the stored overview as a draft, without what abbd sets:\n%s", out)
	}
	p.want(0, "live overview line 1 (MOD-0002 MOD-0001): ledger/book.go", "evidence")

	var g struct {
		Primary struct{ Key string }
		Zones   []struct{ Name string }
		Lines   []struct {
			From, To, Word string
			Kinds, Vias    []string
			Relations      int
		}
		Units []struct{ ID, Zone string }
	}
	if err := json.Unmarshal([]byte(p.read("blueprint/graph.json")), &g); err != nil {
		t.Fatal(err)
	}
	if g.Primary.Key != "charge" {
		t.Errorf("graph.json gives the primary flow from the stack marked primary: %+v", g.Primary)
	}
	if len(g.Zones) != 1 || g.Zones[0].Name != "돈" {
		t.Errorf("graph.json gives the zones: %+v", g.Zones)
	}
	if len(g.Lines) != 1 || g.Lines[0].Word != "청구 금액" || g.Lines[0].Relations != 1 || strings.Join(g.Lines[0].Kinds, ",") != "consumes" || strings.Join(g.Lines[0].Vias, ",") != "data" {
		t.Errorf("graph.json gives each chosen line with the relations under it: %+v", g.Lines)
	}
	for _, u := range g.Units {
		if u.ID == "DATA-0001" && u.Zone != "돈" {
			t.Errorf("graph.json gives a unit the zone that holds it through its parent: %+v", u)
		}
	}
}

// A draft enters only as an overview true of the state: zones that name
// live overview boxes once between them, each under its own short name, and
// lines that draw a stored relation in its direction, each with a short
// word. A flow in the draft is refused, since the primary flow is the stack
// marked primary.
func TestAnOverviewDraftIsCheckedBeforeItIsStored(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	second := "  - name: 장부\n    units: [MOD-0002]\n    evidence:\n      - {path: ledger/book.go, lines: \"6\", symbol: Post, quote: \"func Post() {}\"}\nlines:"
	for _, c := range []struct{ from, to, want string }{
		{"    units: [MOD-0001, MOD-0002]", "    units: [MOD-0009]", "MOD-0009 is no longer in the state"},
		{"    units: [MOD-0001, MOD-0002]", "    units: []", "zone 돈 names no unit"},
		{"    units: [MOD-0001, MOD-0002]", "    units: [DATA-0001, MOD-0002]", "DATA-0001 sits inside MOD-0001, which the overview draws as one box; name MOD-0001"},
		{"\nlines:", "\n" + second, "MOD-0002 is named by zone 1 and by zone 2"},
		{"\nlines:", "\n" + strings.Replace(second, "장부", "돈", 1), "zones 1 and 2 are both named 돈; give each zone a name of its own"},
		{"  - name: 돈", "  - name: 청구가 기록되고 장부가 그것을 매일 밤 읽는다", "zone 청구가 기록되고 장부가 그것을 매일 밤 읽는다: phrase"},
		{"  - name: 돈", "  - name: ' 돈'", "one line, with no space at either end"},
		{"  - from: MOD-0002\n    to: MOD-0001", "  - from: MOD-0001\n    to: DATA-0001", "DATA-0001 sits inside MOD-0001, so the line runs inside one box; join units the overview draws in two boxes"},
		{"  - from: MOD-0002\n    to: MOD-0001", "  - from: MOD-0001\n    to: MOD-0002", "no live relation runs from MOD-0001, or a unit inside it, to MOD-0002, or a unit inside it; a line draws a stored relation, in its direction, and the relations run from MOD-0002 to MOD-0001"},
		{"  - from: MOD-0002\n    to: MOD-0001", "  - from: DATA-0001\n    to: MOD-0002", "no live relation runs from DATA-0001"},
		{"    word: 청구 금액", "    word: 장부가 매일 밤 청구 금액을 모두 읽는다", "at most 4 words"},
		{"    word: 청구 금액", "    word: ''", "says no word"},
		{"    units: [MOD-0001, MOD-0002]", "    unit: [MOD-0001, MOD-0002]", "field unit not found"},
		{"zones:", "flow:\n  name: 청구\nzones:", "unknown field flow; the primary flow is the stack marked primary: true, stored with abbd stack"},
		{"SELECT amount", "SELECT price", "the code does not hold"},
	} {
		draft := strings.Replace(chargeOverview, c.from, c.to, 1)
		if draft == chargeOverview {
			t.Fatalf("the case %q changes nothing", c.from)
		}
		p.overview(1, c.want, draft)
	}
}

// A stored overview whose quotes moved, or whose line lost every relation
// under it, reads STALE: status fails its check and names each part, and
// the evidence check lists the quote that drifted.
func TestAnOverviewGoesStaleWithItsCodeAndItsRelations(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.overview(0, "stored the overview", chargeOverview)
	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT total\n    FROM charges`\n\nfunc Post() {}\n")
	p.resync()
	out := p.want(1, "1 overview part(s) STALE", "status", "--check")
	if !strings.Contains(out, "stale: overview line 1 (MOD-0002 MOD-0001) is STALE: ledger/book.go") {
		t.Errorf("status names the STALE line and the code that moved:\n%s", out)
	}
	p.want(0, "drifted overview line 1 (MOD-0002 MOD-0001)", "evidence")

	p.code("ledger/book.go", "package ledger\n\nconst q = `SELECT amount\n    FROM charges`\n\nfunc Post() {}\n")
	p.extract(ledgerTopo)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p.setup(0, "", "--", exe)
	out = p.want(1, "STALE", "status", "--check")
	if !strings.Contains(out, "no live relation runs from MOD-0002") {
		t.Errorf("a line whose relations all vanished is reported, not drawn as if it held:\n%s", out)
	}
}

// A stored overview whose units moved inside another, or left the state,
// reads STALE: the zone names the box to use instead, the line whose ends
// came to nest runs inside one box, and a zone whose unit is gone says so.
func TestAnOverviewGoesStaleWhenItsUnitsMoveOrLeave(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.overview(0, "stored the overview", chargeOverview)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p.extract(strings.Replace(ledgerTopo, `"name":"ledger",`, `"name":"ledger","parent":"billing",`, 1))
	p.setup(0, "", "--supplement", p.file(t, "sup.json", chargesTable), "--", exe)
	out := p.want(1, "STALE", "status", "--check")
	for _, want := range []string{
		"stale: overview zone 1 (돈) is STALE: MOD-0002 sits inside MOD-0001, which the overview draws as one box; name MOD-0001; draw it again from the state",
		"MOD-0002 sits inside MOD-0001, so the line runs inside one box",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status names %q:\n%s", want, out)
		}
	}

	p.extract(`{"units":[{"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]}]}`)
	noLedger := strings.Replace(chargesTable, `,
 {"key":"l","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}`, "", 1)
	noLedger = strings.Replace(noLedger, `,
 {"from":"l","to":"t","kind":"consumes","via":"data","evidence":[{"path":"ledger/book.go","lines":"3-4","quote":"SELECT amount FROM charges"}]}`, "", 1)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", noLedger), "--", exe)
	out = p.want(1, "STALE", "status", "--check")
	if !strings.Contains(out, "stale: overview zone 1 (돈) is STALE: MOD-0002 is no longer in the state") {
		t.Errorf("a zone whose unit left the state is STALE and says so:\n%s", out)
	}
}

// abbd overview with no FILE prints a draft a writer chooses from: the
// boxes with the stage each stands at, the primary flow's stages, and each
// pair of boxes a live relation joins, ready to keep once it says a word.
// A candidate kept as printed stores, and the draft then marks it chosen.
func TestTheOverviewDraftOffersWhatAWriterChoosesFrom(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.stack(0, "marked stack charge the primary flow", strings.Replace(chargeStack, "layers:\n", "primary: true\nlayers:\n", 1))
	draft := p.want(0, "", "overview")
	for _, want := range []string{
		"# The boxes the overview draws (2)",
		"MOD-0001   billing",
		"stage 2 billing",
		"# The primary flow is stack charge (What are the layers of the charge path?), its layers the stages:",
		"#   1 books: MOD-0002",
		"# Line candidates (1)",
		"#     ledger -> billing: runs 1 (consumes data 1)",
		"zones: []\nlines: []\n---",
	} {
		if !strings.Contains(draft, want) {
			t.Errorf("the draft holds %q:\n%s", want, draft)
		}
	}
	var cand string
	for _, l := range strings.Split(draft, "\n") {
		if strings.HasPrefix(l, "#   - {from: MOD-0002, to: MOD-0001") {
			cand = strings.TrimPrefix(l, "#   ")
		}
	}
	if !strings.Contains(cand, `quote: "SELECT amount`) {
		t.Fatalf("the candidate cites the quote of the relation under it: %q\n%s", cand, draft)
	}
	kept := strings.Replace(draft, "lines: []", "lines:\n  "+strings.Replace(cand, `word: ""`, "word: 청구 금액", 1), 1)
	p.overview(0, "stored the overview: 0 zones, 1 lines", kept)
	again := p.want(0, "word: 청구 금액", "overview")
	if !strings.Contains(again, "#   - {from: MOD-0002, to: MOD-0001, word: \"\", evidence: [{path: \"ledger/book.go\"") || !strings.Contains(again, "# chosen below") {
		t.Errorf("the draft marks a candidate already chosen:\n%s", again)
	}
}

// A box no stage names stands at the first stage that names a unit inside
// it, as the overview draws it, and explain says so and names that unit.
func TestExplainGivesTheStageABoxStandsAtThroughAUnitInsideIt(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.code(".abbd/project.yaml", p.read("project.yaml")+"language: ko\n")
	inner := strings.Replace(strings.Replace(chargeStack, "layers:\n", "primary: true\nlayers:\n", 1), "    units: [MOD-0001]", "    units: [DATA-0001]", 1)
	inner = strings.Replace(inner, `      - {path: billing/charge.go, lines: "4", symbol: Charge, quote: "func Charge() {}"}`, `      - {path: billing/charge.go, lines: "3", quote: "INSERT INTO charges"}`, 1)
	p.stack(0, "marked stack charge the primary flow", inner)
	p.want(0, "주 흐름 charge 2/2단계 billing, 안의 DATA-0001로", "explain", "MOD-0001")
}

// abbd status's help names the overview's parts among what fails --check.
func TestStatusHelpNamesTheOverviewAmongWhatFailsTheCheck(t *testing.T) {
	t.Parallel()
	if !strings.Contains(usage, "a table's row\n                                     or a zone or line of the overview is STALE") {
		t.Error("status --check fails on a STALE zone or line of the overview, and its help says so")
	}
}

// A candidate whose relations quote no code, as a build model's edges do,
// gets the line of its from box that calls what it runs to, said to be found
// by name; one with no such line says so and where to look. The draft is a
// whole file, so it stores as printed, and a candidate kept with its found
// quote stores too.
func TestTheOverviewDraftFindsAQuoteNoRelationGives(t *testing.T) {
	t.Parallel()
	p, exe := bare(t)
	p.code("app/main.go", "package app\n\nfunc Main() {\n\tbilling.Charge()\n}\n")
	p.code("billing/charge.go", "package billing\n\nfunc Charge() {}\n")
	p.code("ledger/book.go", "package ledger\n\nfunc Post() {}\n")
	p.extract(`{"units":[
 {"key":"app","kind":"MOD","name":"app","anchors":[{"path":"app"},{"path":"app/main.go"}]},
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"ledger","kind":"MOD","name":"ledger","anchors":[{"path":"ledger/book.go","symbol":"Post"}]}
],"relations":[
 {"from":"app","to":"billing","kind":"depends_on","via":"call"},
 {"from":"app","to":"ledger","kind":"depends_on","via":"import"}
]}`)
	p.setup(0, "synced: 3 unit(s)", "--", exe)
	draft := p.want(0, "", "overview")
	contains(t, draft,
		"# This whole output is FILE",
		"# A zone names boxes from this list.",
		`evidence: [{path: "app/main.go", lines: "4", quote: "billing.Charge()"}]}`,
		"the quote is a line of app that names Charge",
		"no line of app names what it runs to; quote the line where it crosses",
	)
	p.overview(0, "stored the overview: 0 zones, 0 lines", draft)
	var cand string
	for _, l := range strings.Split(draft, "\n") {
		if strings.Contains(l, `quote: "billing.Charge()"`) {
			cand = strings.TrimPrefix(l, "#   ")
		}
	}
	kept := strings.Replace(draft, "lines: []", "lines:\n  "+strings.Replace(cand, `word: ""`, "word: 청구", 1), 1)
	p.overview(0, "stored the overview: 0 zones, 1 lines", kept)
}

// A file with no frontmatter is refused with what an overview draft is.
func TestAnOverviewDraftWithoutFrontmatterSaysWhatADraftIs(t *testing.T) {
	t.Parallel()
	p := notesProject(t)
	p.overview(1, "missing frontmatter: an overview draft is the whole file abbd overview prints", "zones: []\nlines: []\n")
}
