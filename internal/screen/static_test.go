package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

func found(to model.ID, kind model.RelationKind, via model.Via) model.Relation {
	return model.Relation{To: to, Kind: kind, Origin: model.Extracted, Via: via,
		Evidence: []model.Evidence{{Path: "src/a.rs", Lines: "1", Quote: "use b;"}}}
}

func drawsRel(d Drawing, rel string) bool {
	return slices.ContainsFunc(d.Wires, func(w Wire) bool { return slices.Contains(w.Rels, rel) })
}

func listsStatic(d Drawing, owner, other string) bool {
	return slices.ContainsFunc(d.Undrawn, func(u LeftOut) bool { return u.Name == owner && slices.Contains(u.Static, other) })
}

func showStatic(units []model.Unit) Model {
	on := true
	return Build(Input{Units: units, Project: store.Project{StaticLines: &on}})
}

// An import an extractor found alone is left out of the overview at rest,
// listed under the picture as 정적 의존 under the box that uses the other,
// and drawn in that box's focus view; static_lines draws it again.
func TestAStaticLineRestsInTheList(t *testing.T) {
	units := ripgrep()
	units[0].Relations = append(units[0].Relations, found("MOD-0005", model.DependsOn, "import"))
	const rel = "MOD-0001.depends_on.MOD-0005"
	m := build(units)
	if drawsRel(m.Overview, rel) {
		t.Error("the overview draws a static line at rest")
	}
	if !listsStatic(m.Overview, "ripgrep", "grep-printer") {
		t.Errorf("the overview lists %+v, want ripgrep's static line to grep-printer", m.Overview.Undrawn)
	}
	i := slices.IndexFunc(m.Focus, func(f Focus) bool { return f.Unit == "MOD-0001" })
	if i < 0 || !drawsRel(m.Focus[i].Drawing, rel) {
		t.Error("ripgrep's focus view does not draw its static line")
	}
	shown := showStatic(units)
	if !drawsRel(shown.Overview, rel) {
		t.Error("static_lines: true does not draw the static line")
	}
	if len(shown.Overview.Undrawn) > 0 {
		t.Errorf("static_lines: true still lists %+v", shown.Overview.Undrawn)
	}
}

// A static line a reader stated stays drawn.
func TestAStatedStaticLineStaysDrawn(t *testing.T) {
	units := ripgrep()
	units[0].Relations = append(units[0].Relations, guess("MOD-0005", model.DependsOn, "import"))
	if m := build(units); !drawsRel(m.Overview, "MOD-0001.depends_on.MOD-0005") {
		t.Error("the overview leaves out a static line a reader stated")
	}
}

// A use of a contract that rests on an import only still draws the line to
// the contract's provider.
func TestAStaticUseOfAContractStaysDrawn(t *testing.T) {
	units := append(ripgrep(),
		called("IF-0003", "trait Printer (grep-printer)"),
		called("MOD-0006", "grep-cli", found("IF-0003", model.Consumes, "import"), rel("MOD-0004", model.DependsOn)))
	units[4].Relations = append(units[4].Relations, found("IF-0003", model.Provides, "trait"))
	m := build(units)
	if !drawsRel(m.Overview, "MOD-0006.consumes.IF-0003") {
		t.Errorf("the overview leaves out a static use of a contract; it lists %+v", m.Overview.Undrawn)
	}
}

// inferring draws units with the project's inferred lead on, and with
// static_lines set as shown says.
func inferring(units []model.Unit, shown bool) Model {
	on := true
	p := store.Project{InferredLead: &on}
	if shown {
		p.StaticLines = &on
	}
	return Build(Input{Units: units, Project: p})
}

// Where the project infers its lead, hiding a static line leaves the
// overview's lead: a box something uses keeps a line into it, so no second
// box nothing uses appears. With no inferred lead, the line is left out.
func TestHidingAStaticLineKeepsTheLead(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "a", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "b", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "c", rel("MOD-0004", model.DependsOn)),
		called("MOD-0004", "d", rel("MOD-0005", model.DependsOn), found("MOD-0006", model.DependsOn, "import")),
		called("MOD-0005", "e"),
		called("MOD-0006", "f", rel("MOD-0003", model.DependsOn)),
	}
	if inferring(units, true).Overview.Lead == nil {
		t.Fatal("the overview with every line draws no lead")
	}
	if inferring(units, false).Overview.Lead == nil {
		t.Error("hiding the static line d → f takes the overview's lead away")
	}
	if m := build(units); drawsRel(m.Overview, "MOD-0004.depends_on.MOD-0006") {
		t.Error("with no inferred lead, the static line d → f is still drawn")
	}
}

// The same holds inside a module: where the lead is inferred, the static
// line that is the only one into a box something uses stays drawn.
func TestAnInsideKeepsTheStaticLineItsLeadNeeds(t *testing.T) {
	units := named(append(ripgrep(),
		inside("MOD-0010", "MOD-0002", rel("MOD-0011", model.DependsOn)),
		inside("MOD-0011", "MOD-0002", rel("MOD-0012", model.DependsOn)),
		inside("MOD-0012", "MOD-0002", found("MOD-0013", model.DependsOn, "import")),
		inside("MOD-0013", "MOD-0002", rel("MOD-0011", model.DependsOn)),
	))
	const rel = "MOD-0012.depends_on.MOD-0013"
	if ins := insideOf(t, inferring(units, false)); !drawsRel(ins.Drawing, rel) {
		t.Errorf("the inside leaves out the only line into MOD-0013 while its lead is inferred; it lists %+v", ins.Drawing.Undrawn)
	}
	if ins := insideOf(t, build(units)); drawsRel(ins.Drawing, rel) {
		t.Error("with no inferred lead, the inside still draws the static line")
	}
}

// insideOf returns the inside of MOD-0002.
func insideOf(t *testing.T, m Model) Inside {
	t.Helper()
	i := slices.IndexFunc(m.Insides, func(in Inside) bool { return in.ID == "MOD-0002" })
	if i < 0 {
		t.Fatal("MOD-0002 has no inside view")
	}
	return m.Insides[i]
}

func named(units []model.Unit) []model.Unit {
	for i := range units {
		if units[i].Name.Value == "" {
			units[i].Name.Value = string(units[i].ID)
		}
	}
	return units
}

// Inside a module, a static line between two units is left out at rest,
// listed under the unit that uses the other, and drawn in its focus view;
// static_lines draws it again.
func TestAnInsideLeavesItsStaticLineToTheList(t *testing.T) {
	units := named(append(ripgrep(),
		inside("MOD-0010", "MOD-0002", found("MOD-0011", model.DependsOn, "import"), rel("MOD-0012", model.DependsOn)),
		inside("MOD-0011", "MOD-0002"),
		inside("MOD-0012", "MOD-0002", rel("MOD-0011", model.DependsOn)),
	))
	ins := insideOf(t, build(units))
	const rel = "MOD-0010.depends_on.MOD-0011"
	if drawsRel(ins.Drawing, rel) {
		t.Error("the inside draws a static line at rest")
	}
	if !listsStatic(ins.Drawing, "MOD-0010", "MOD-0011") {
		t.Errorf("the inside lists %+v, want MOD-0010's static line to MOD-0011", ins.Drawing.Undrawn)
	}
	if !drawsRel(ins.Drawing, "MOD-0010.depends_on.MOD-0012") {
		t.Error("the inside leaves out a run-time line")
	}
	f := slices.IndexFunc(ins.Focus, func(f Focus) bool { return f.Unit == "MOD-0010" })
	if f < 0 || !drawsRel(ins.Focus[f].Drawing, rel) {
		t.Error("MOD-0010's focus view does not draw its static line")
	}
	if href := ins.Drawing.Undrawn[0].Href; href != "#"+focusID("MOD-0002", "MOD-0010") {
		t.Errorf("the list links %s, want MOD-0010's focus view", href)
	}
	shown := insideOf(t, showStatic(units))
	if !drawsRel(shown.Drawing, rel) || len(shown.Drawing.Undrawn) > 0 {
		t.Errorf("static_lines: true does not draw the inside's static line, or lists %+v", shown.Drawing.Undrawn)
	}
}

// An inside decides by pair, as the overview does: a pair with a run-time
// line is drawn whole and not listed, and a static use of a contract or a
// static line between two stages of the primary flow stays drawn.
func TestAnInsideKeepsWhatAStaticLineMustNotHide(t *testing.T) {
	mixed := named(append(ripgrep(),
		inside("MOD-0010", "MOD-0002", found("MOD-0011", model.DependsOn, "import")),
		inside("MOD-0011", "MOD-0002", rel("MOD-0010", model.DependsOn)),
	))
	if ins := insideOf(t, build(mixed)); len(ins.Drawing.Undrawn) > 0 || !drawsRel(ins.Drawing, "MOD-0010.depends_on.MOD-0011") {
		t.Errorf("a pair with a run-time line lists %+v or leaves its static line out", ins.Drawing.Undrawn)
	}

	contract := named(append(ripgrep(),
		inside("MOD-0010", "MOD-0002", found("IF-0001", model.Consumes, "import"), rel("MOD-0011", model.DependsOn)),
		inside("MOD-0011", "MOD-0002"),
	))
	if ins := insideOf(t, build(contract)); !drawsRel(ins.Drawing, "MOD-0010.consumes.IF-0001") {
		t.Errorf("the inside leaves out a static use of a contract; it lists %+v", ins.Drawing.Undrawn)
	}

	staged := named(append(ripgrep(),
		inside("MOD-0010", "MOD-0002", found("MOD-0011", model.DependsOn, "import"), rel("MOD-0012", model.DependsOn)),
		inside("MOD-0011", "MOD-0002"),
		inside("MOD-0012", "MOD-0002", rel("MOD-0011", model.DependsOn)),
	))
	stack := store.Stack{Question: "q", Primary: true, Known: true, Key: "walk", Parts: []store.Part[model.Layer]{
		{Claim: model.Layer{Name: "first", Role: "r", Units: []model.ID{"MOD-0010"}}},
		{Claim: model.Layer{Name: "second", Role: "r", Units: []model.ID{"MOD-0012"}}},
		{Claim: model.Layer{Name: "third", Role: "r", Units: []model.ID{"MOD-0011"}}},
	}}
	m := Build(Input{Units: staged, Figures: store.Figures{Stacks: []store.Stack{stack}}})
	if ins := insideOf(t, m); !drawsRel(ins.Drawing, "MOD-0010.depends_on.MOD-0011") {
		t.Errorf("the inside leaves out a static line between two stages of the primary flow; it lists %+v", ins.Drawing.Undrawn)
	}
}

// An inside counts the pairs it left out of the picture, as the overview
// does, so its levels read the lines it has and not only the drawn ones.
func TestAnInsideCountsTheStaticPairsItLeftOut(t *testing.T) {
	units := named(append(ripgrep(),
		inside("MOD-0010", "MOD-0002", found("MOD-0011", model.DependsOn, "import")),
		inside("MOD-0011", "MOD-0002"),
	))
	ins := insideOf(t, build(units))
	if !slices.Contains(ins.Drawing.Counted, [2]string{"MOD-0010", "MOD-0011"}) {
		t.Errorf("the inside counts %v, want the pair it left out", ins.Drawing.Counted)
	}
	at := atTop
	got := level("in", ins.Drawing, at)
	want := level("in", insideOf(t, showStatic(units)).Drawing, at)
	if got.Lines != want.Lines || got.Lines != 1 {
		t.Errorf("levels count %d lines at rest and %d with static_lines, want 1 both", got.Lines, want.Lines)
	}
}

// Each full view lists, as text, what every box uses and is used by, the
// pairs left out of the picture included, so a capture of the page holds what
// a focus view shows.
func TestAViewListsItsBoxesNeighborsAsText(t *testing.T) {
	units := ripgrep()
	units[0].Relations = append(units[0].Relations, found("MOD-0005", model.DependsOn, "import"))
	m := build(units)
	names := func(rs []Near) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Name)
		}
		return out
	}
	by := map[string]Neighbor{}
	for _, n := range m.Overview.Neighbors {
		by[n.Name] = n
	}
	if got := names(by["ripgrep"].Uses); !slices.Contains(got, "grep-printer") || !slices.Contains(got, "grep") {
		t.Errorf("ripgrep uses %v, want grep and the grep-printer a static line left out of the picture", got)
	}
	if got := names(by["grep-printer"].UsedBy); !slices.Contains(got, "ripgrep") {
		t.Errorf("grep-printer is used by %v, want ripgrep", got)
	}
	pages, err := Pages(m)
	if err != nil {
		t.Fatal(err)
	}
	page := string(pages["index.html"])
	for _, want := range []string{"상자별 이웃", "이 상자가 쓰는 상자:", "이 상자를 쓰는 상자:", "<details class=\"neighbors\" open>"} {
		if !strings.Contains(page, want) {
			t.Errorf("the overview page lacks %q", want)
		}
	}
}
