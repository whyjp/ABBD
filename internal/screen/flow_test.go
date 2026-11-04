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

// A cycle is drawn in the view of the unit that really holds it: the
// drawing makes no box that the state lacks, and the line that closes the
// cycle climbs, dashed.
func TestACycleStaysInItsContainersView(t *testing.T) {
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
	if len(m.Insides) != 1 || m.Insides[0].ID != "MOD-0001" {
		t.Fatalf("only app holds units, so only app has a view: %+v", m.Insides)
	}
	app := m.Insides[0]
	b := boxes(app.Drawing)
	for _, u := range units[1:] {
		if _, ok := b[string(u.ID)]; !ok {
			t.Errorf("app's view should draw %s", u.ID)
		}
	}
	if len(b) != len(units)-1 {
		t.Errorf("app's view should draw its six units and nothing else: %+v", app.Drawing.Boxes)
	}
	shown := map[string]bool{}
	for _, w := range app.Drawing.Wires {
		for _, r := range w.Rels {
			shown[r] = true
		}
	}
	for _, u := range units {
		for _, r := range u.Relations {
			if k := key(u.ID, r.Kind, r.To); !shown[k] {
				t.Errorf("%s is not drawn in app's view", k)
			}
		}
	}
	s, err := readSVG(svg(t, app.Drawing))
	if err != nil {
		t.Fatal(err)
	}
	if mt := measure(s); mt.Back != 1 || mt.BackPlain != 0 {
		t.Errorf("the line that closes the cycle should climb dashed, and only it: %s", mt)
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
// arrival, though hover gives each its own classes; drawn as one bundle
// they do not.
func TestTheCheckerCountsLinesThatArriveApart(t *testing.T) {
	sheet := func(net int) Metrics {
		d := Drawing{Width: 400, Height: 300,
			Boxes: []Box{{ID: "a", X: 20, Y: 20, W: 100, H: 40}, {ID: "b", X: 260, Y: 20, W: 100, H: 40}, {ID: "c", X: 140, Y: 200, W: 100, H: 40}},
			Wires: []Wire{
				{D: polyline([]pt{{70, 60}, {70, 120}, {170, 120}, {170, 200}}), Kind: "depends_on", Net: net, Lit: "e1"},
				{D: polyline([]pt{{310, 60}, {310, 120}, {210, 120}, {210, 200}}), Kind: "depends_on", Net: net, Lit: "e2"},
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

// Two lines of one kind that leave one edge of a box apart count as a
// split, though hover gives each its own classes; a line that drops
// straight into the box below needs no bundle and does not.
func TestTheCheckerCountsLinesThatLeaveApart(t *testing.T) {
	sheet := func(second []pt) Metrics {
		d := Drawing{Width: 400, Height: 300,
			Boxes: []Box{{ID: "a", X: 140, Y: 20, W: 100, H: 40}, {ID: "b", X: 20, Y: 200, W: 100, H: 40}, {ID: "c", X: 160, Y: 200, W: 100, H: 40}},
			Wires: []Wire{
				{D: polyline([]pt{{170, 60}, {170, 120}, {70, 120}, {70, 200}}), Kind: "depends_on", Lit: "e1"},
				{D: polyline(second), Kind: "depends_on", Lit: "e2"},
			},
		}
		return measured(t, d)
	}
	if m := sheet([]pt{{200, 60}, {200, 140}, {220, 140}, {220, 200}}); m.Split != 1 || !slices.ContainsFunc(m.violations(400), func(v string) bool { return strings.HasPrefix(v, "4: ") }) {
		t.Errorf("two lines out of a drawn apart are a split, against rule 4: %s", m)
	}
	if m := sheet([]pt{{200, 60}, {200, 200}}); m.Split != 0 {
		t.Errorf("a line that drops straight from a into c is not: %s", m)
	}
}

// Every box that links somewhere opens a view the page holds, so a
// feature with no parts is drawn without a link, and one with parts opens
// its view.
func TestEveryLinkOpensAView(t *testing.T) {
	step := inside("FEAT-0003", "FEAT-0002")
	step.Kind = model.Feature
	units := []model.Unit{called("MOD-0001", "cli"), unit("FEAT-0001"), unit("FEAT-0002"), step}
	units[1].Kind, units[2].Kind = model.Feature, model.Feature
	m := build(units)
	have := map[string]bool{"#overview": true}
	for _, ins := range m.Insides {
		have["#m-"+ins.ID] = true
	}
	drawings := []Drawing{m.Overview}
	for _, ins := range m.Insides {
		drawings = append(drawings, ins.Drawing)
	}
	for _, d := range drawings {
		for _, b := range d.Boxes {
			if b.Href != "" && !have[b.Href] {
				t.Errorf("box %s links to %s, which no view holds", b.ID, b.Href)
			}
		}
	}
	if b := boxes(m.Overview); b["FEAT-0001"].Href != "" || b["FEAT-0002"].Href != "#m-FEAT-0002" {
		t.Errorf("a feature links to its view only when it holds parts: %+v", m.Overview.Boxes)
	}
}

// Where a line of one kind joins its trunk, lines of another kind that only
// cross there get no dot.
func TestAJoinMarksOnlyItsOwnKind(t *testing.T) {
	ps := []piece{{pt{0, 10}, pt{20, 10}, "bypass"}, {pt{10, 0}, pt{10, 20}, "bypass"}}
	if got := junctions(ps, []join{{pt{10, 10}, "contract"}}); len(got) != 0 {
		t.Fatalf("two crossing bypass lines got dots %v from a contract join", got)
	}
	if got := junctions(ps, []join{{pt{10, 10}, "bypass"}}); len(got) != 1 {
		t.Fatalf("a bypass join on crossing bypass lines got dots %v; want one", got)
	}
}
