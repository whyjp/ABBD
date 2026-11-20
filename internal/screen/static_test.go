package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

func found(to model.ID, via model.Via) model.Relation {
	return model.Relation{To: to, Kind: model.DependsOn, Origin: model.Extracted, Via: via,
		Evidence: []model.Evidence{{Path: "src/a.rs", Lines: "1", Quote: "use b;"}}}
}

func drawsRel(d Drawing, rel string) bool {
	return slices.ContainsFunc(d.Wires, func(w Wire) bool { return slices.Contains(w.Rels, rel) })
}

func listsStatic(d Drawing, owner, other string) bool {
	return slices.ContainsFunc(d.Undrawn, func(u LeftOut) bool { return u.Name == owner && slices.Contains(u.Static, other) })
}

// An import an extractor found alone is left out of the overview at rest,
// listed under the picture as 정적 의존 under the box that uses the other,
// and drawn in that box's focus view; static_lines draws it again.
func TestAStaticLineRestsInTheList(t *testing.T) {
	units := ripgrep()
	units[0].Relations = append(units[0].Relations, found("MOD-0005", "import"))
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
	on := true
	shown := Build(Input{Units: units, Project: store.Project{StaticLines: &on}})
	if !drawsRel(shown.Overview, rel) {
		t.Error("static_lines: true does not draw the static line")
	}
	if len(shown.Overview.Undrawn) > 0 {
		t.Errorf("static_lines: true still lists %+v", shown.Overview.Undrawn)
	}
}

// A static line a reader stated, and one that reaches a contract, stay drawn.
func TestAStatedOrContractStaticLineStaysDrawn(t *testing.T) {
	units := ripgrep()
	units[0].Relations = append(units[0].Relations, guess("MOD-0005", model.DependsOn, "import"))
	if m := build(units); !drawsRel(m.Overview, "MOD-0001.depends_on.MOD-0005") {
		t.Error("the overview leaves out a static line a reader stated")
	}
	units = ripgrep()
	units[4].Relations = append(units[4].Relations, model.Relation{To: "IF-0001", Kind: model.Consumes, Origin: model.Extracted, Via: "import"})
	if m := build(units); len(m.Overview.Undrawn) > 0 {
		t.Errorf("the overview lists %+v, want a static use of a contract drawn", m.Overview.Undrawn)
	}
}

// Inside a module, a static line between two units is left out at rest,
// listed under the unit that uses the other, and drawn in its focus view.
func TestAnInsideLeavesItsStaticLineToTheList(t *testing.T) {
	units := append(ripgrep(),
		inside("MOD-0010", "MOD-0002", found("MOD-0011", "import"), rel("MOD-0012", model.DependsOn)),
		inside("MOD-0011", "MOD-0002"),
		inside("MOD-0012", "MOD-0002", rel("MOD-0011", model.DependsOn)),
	)
	for i := range units {
		if units[i].ID == "MOD-0010" || units[i].ID == "MOD-0011" || units[i].ID == "MOD-0012" {
			units[i].Name.Value = string(units[i].ID)
		}
	}
	m := build(units)
	i := slices.IndexFunc(m.Insides, func(in Inside) bool { return in.ID == "MOD-0002" })
	if i < 0 {
		t.Fatal("MOD-0002 has no inside view")
	}
	ins := m.Insides[i]
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
}
