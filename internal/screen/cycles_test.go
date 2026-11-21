package screen

import (
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// Boxes that use each other round a cycle are a finding about the code:
// Levels names each cycle of every full view by its boxes, the overview's
// read with every line drawn, and OverviewCycles gives the overview's
// without routing a view, the same groups.
func TestLevelsNameEachCycleOfBoxes(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "api", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "store", rel("MOD-0001", model.DependsOn)),
		called("MOD-0004", "cli", rel("MOD-0001", model.DependsOn)),
		inside("MOD-0005", "MOD-0004", rel("MOD-0006", model.DependsOn)),
		inside("MOD-0006", "MOD-0004", rel("MOD-0005", model.DependsOn)),
		inside("MOD-0007", "MOD-0004", rel("MOD-0005", model.DependsOn)),
	}
	in := Input{Units: units}
	m := Build(in)
	got := map[string][][]string{}
	for _, l := range Levels(m) {
		if len(l.Cycles) > 0 {
			got[l.View] = l.Cycles
		}
	}
	want := map[string][][]string{
		"overview":     {{"api", "core", "store"}},
		"MOD-0004 cli": {{"MOD-0005", "MOD-0006"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cycles by view = %v, want %v", got, want)
	}
	if got := OverviewCycles(in); !reflect.DeepEqual(got, want["overview"]) {
		t.Errorf("OverviewCycles = %v, want the overview's level's %v", got, want["overview"])
	}
}

// A view whose uses all run one way has no cycle to name.
func TestAViewWithoutACycleNamesNone(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "api", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "store"),
		called("MOD-0004", "cli", rel("MOD-0001", model.DependsOn)),
	}
	in := Input{Units: units}
	for _, l := range Levels(Build(in)) {
		if len(l.Cycles) > 0 {
			t.Errorf("%s names cycles %v", l.View, l.Cycles)
		}
	}
	if got := OverviewCycles(in); got != nil {
		t.Errorf("OverviewCycles = %v, want none", got)
	}
}

// The overview's cycles are the code's: a cut or a record that leaves a
// line of the cycle undrawn still leaves Levels and OverviewCycles naming
// the same group.
func TestTheOverviewsCyclesHoldWhateverItDraws(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "api", rel("MOD-0002", model.DependsOn), rel("MOD-0004", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn), rel("MOD-0004", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0003", "store", rel("MOD-0001", model.DependsOn), rel("MOD-0004", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0004", "cli", rel("MOD-0005", model.DependsOn)),
		called("MOD-0005", "log"),
	}
	on := true
	var rec store.Overview
	rec.Known = true
	rec.Lines = []store.Part[model.Line]{{Claim: model.Line{From: "MOD-0001", To: "MOD-0002", Word: "calls"}}}
	want := [][]string{{"api", "core", "store"}}
	for name, in := range map[string]Input{
		"cut":    {Units: units, Project: store.Project{OverviewCut: &on}},
		"record": {Units: units, Overview: rec},
	} {
		m := Build(in)
		if got := Levels(m)[0].Cycles; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the overview's level names %v, want %v", name, got, want)
		}
		if got := OverviewCycles(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: OverviewCycles = %v, want %v", name, got, want)
		}
		if len(m.Overview.Counted) == 0 {
			t.Errorf("%s: the overview draws every line, so the test checks nothing", name)
		}
	}
}

// Four boxes that all use one another are a tangle, named by their boxes; a
// ring of four is not.
func TestLevelsNameAMutualCliqueAsATangle(t *testing.T) {
	names := []string{"a", "b", "c", "d"}
	var units []model.Unit
	for i, n := range names {
		var rels []model.Relation
		for j := range names {
			if j != i {
				rels = append(rels, rel(model.ID("MOD-000"+string(rune('1'+j))), model.DependsOn))
			}
		}
		units = append(units, called(model.ID("MOD-000"+string(rune('1'+i))), n, rels...))
	}
	var got [][]string
	for _, l := range Levels(Build(Input{Units: units})) {
		if l.View == "overview" {
			got = l.Tangles
		}
	}
	if want := [][]string{{"a", "b", "c", "d"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("tangles = %v, want %v", got, want)
	}
	ring := []model.Unit{
		called("MOD-0001", "a", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "b", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "c", rel("MOD-0004", model.DependsOn)),
		called("MOD-0004", "d", rel("MOD-0001", model.DependsOn)),
	}
	for _, l := range Levels(Build(Input{Units: ring})) {
		if len(l.Tangles) > 0 {
			t.Errorf("%s names tangles %v in a ring", l.View, l.Tangles)
		}
	}
}
