package screen

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// Lines from different modules into one module meet on one trunk that
// carries their count, and arrive as one bundle, rule 4.
func TestLinesIntoOneBoxShareATrunk(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "a", rel("MOD-0004", model.DependsOn)),
		called("MOD-0002", "b", rel("MOD-0004", model.DependsOn)),
		called("MOD-0003", "c", rel("MOD-0004", model.DependsOn)),
		called("MOD-0004", "d"),
	}
	d := build(units).Overview
	var counted []Wire
	for _, w := range d.Wires {
		if w.Count > 0 {
			counted = append(counted, w)
		}
	}
	var into *Wire
	for i, w := range counted {
		if w.Count == 3 && len(w.Rels) == 3 && !w.Joins {
			into = &counted[i]
		}
	}
	if into == nil {
		t.Fatalf("three lines into d should arrive as one bundle counted ×3, got %+v", counted)
	}
	if got := measured(t, d); got.SplitIn != 0 {
		t.Errorf("lines into d arrive apart: %s", got)
	}
}

// A cycle that is only part of a view folds into one box, and that box's
// own view draws the cycle. Nothing is left out.
func TestATangledCycleFoldsIntoABoxWithItsOwnView(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "app"),
		inside("MOD-0002", "MOD-0001", rel("MOD-0003", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		inside("MOD-0003", "MOD-0001", rel("MOD-0004", model.DependsOn)),
		inside("MOD-0004", "MOD-0001", rel("MOD-0002", model.DependsOn)),
		inside("MOD-0005", "MOD-0001", rel("MOD-0006", model.DependsOn)),
		inside("MOD-0006", "MOD-0001", rel("MOD-0007", model.DependsOn)),
		inside("MOD-0007", "MOD-0001"),
	}
	m := build(units)
	views := map[string]Inside{}
	for _, ins := range m.Insides {
		views[ins.ID] = ins
	}
	app, cycle := views["MOD-0001"], views["MOD-0001~1"]
	b := boxes(app.Drawing)
	if len(b) != 4 || b["MOD-0001~1"].Href != "#m-MOD-0001~1" {
		t.Fatalf("app's view should hold the folded cycle as one box that opens its view, and the chain beside it: %+v", app.Drawing.Boxes)
	}
	cb := boxes(cycle.Drawing)
	for _, id := range []string{"MOD-0002", "MOD-0003", "MOD-0004"} {
		if _, ok := cb[id]; !ok {
			t.Errorf("the fold's view should draw %s", id)
		}
	}
	shown := map[string]bool{}
	for _, ins := range m.Insides {
		for _, w := range ins.Drawing.Wires {
			for _, r := range w.Rels {
				shown[r] = true
			}
		}
	}
	for _, u := range units {
		for _, r := range u.Relations {
			if k := key(u.ID, r.Kind, r.To); !shown[k] {
				t.Errorf("%s is in no view after folding", k)
			}
		}
	}
	if len(boxes(m.Overview)) != 1 {
		t.Errorf("the overview is never folded: %+v", m.Overview.Boxes)
	}
}

// A line that climbs is counted as running back, and must be drawn dashed;
// a line that provides a contract reads from the contract down to its unit.
func TestTheCheckerCountsLinesThatClimb(t *testing.T) {
	sheet := func(class string, y1, y2 int) Metrics {
		d := Drawing{Width: 400, Height: 300,
			Boxes: []Box{{ID: "a", X: 100, Y: 20, W: 100, H: 40}, {ID: "b", X: 100, Y: 200, W: 100, H: 40}},
			Wires: []Wire{{D: polyline([]pt{{150, y1}, {150, y2}}), Kind: class}},
		}
		return measured(t, d)
	}
	if m := sheet("depends_on", 60, 200); m.Forward != 1 || m.Back != 0 {
		t.Errorf("a line down is forward: %s", m)
	}
	if m := sheet("depends_on", 200, 60); m.Back != 1 || m.BackPlain != 1 {
		t.Errorf("a line up drawn solid is a plain back line: %s", m)
	}
	if m := sheet("depends_on back", 200, 60); m.Back != 1 || m.BackPlain != 0 {
		t.Errorf("a line up drawn dashed is a marked back line: %s", m)
	}
	if m := sheet("provides", 200, 60); m.Forward != 1 || m.Back != 0 {
		t.Errorf("a unit providing the contract above it reads forward: %s", m)
	}
}

// E17 (evaluation round 5): a view's frame shows only what lies outside
// the unit, never one of its own units and never a unit that holds it.
func TestAFrameShowsOnlyWhatIsOutside(t *testing.T) {
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(units)
		for _, ins := range Build(Input{Units: units}).Insides {
			for _, p := range ins.Drawing.Ports {
				if p.Contract == "" {
					continue
				}
				if slices.Contains(tr.chain(model.ID(p.Contract)), model.ID(ins.ID)) || slices.Contains(tr.chain(model.ID(ins.ID)), model.ID(p.Contract)) {
					t.Errorf("%s: the frame of %s shows %s, which it holds or which holds it", name, ins.ID, p.Contract)
				}
			}
		}
	}
}

// Two lines of one kind that reach one edge of a box apart count as a split
// arrival; drawn as one bundle they do not.
func TestTheCheckerCountsLinesThatArriveApart(t *testing.T) {
	sheet := func(net int) Metrics {
		d := Drawing{Width: 400, Height: 300,
			Boxes: []Box{{ID: "a", X: 20, Y: 20, W: 100, H: 40}, {ID: "b", X: 260, Y: 20, W: 100, H: 40}, {ID: "c", X: 140, Y: 200, W: 100, H: 40}},
			Wires: []Wire{
				{D: polyline([]pt{{70, 60}, {70, 120}, {170, 120}, {170, 200}}), Kind: "depends_on", Net: net},
				{D: polyline([]pt{{310, 60}, {310, 120}, {210, 120}, {210, 200}}), Kind: "depends_on", Net: net},
			},
		}
		return measured(t, d)
	}
	if m := sheet(0); m.SplitIn != 1 || !slices.ContainsFunc(m.violations(400), func(v string) bool { return strings.HasPrefix(v, "4: ") }) {
		t.Errorf("two lines into c drawn apart are a split arrival, against rule 4: %s", m)
	}
	if m := sheet(1); m.SplitIn != 0 {
		t.Errorf("two lines into c drawn as one bundle are not: %s", m)
	}
}
