package screen

import (
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
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
