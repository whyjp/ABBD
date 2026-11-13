package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// routeBetween is the one route of d whose ends are exactly a and b.
func routeBetween(t *testing.T, d Drawing, a, b string) []Route {
	t.Helper()
	var out []Route
	for _, r := range d.Routes {
		if len(r.ends) == 2 && slices.Contains(r.ends, a) && slices.Contains(r.ends, b) {
			out = append(out, r)
		}
	}
	return out
}

// Neighbouring stages on the spine join by a short straight hop from one
// box's side into the facing side of the next, as a pipeline is drawn: no
// line between them dips under the row and comes back up. Lines between the
// same neighbours run in lanes of their own that never meet.
func TestNeighbouringStagesJoinByAStraightHop(t *testing.T) {
	m := Build(Input{Units: pipeline(), Figures: pipelineStages()})
	d := m.Overview
	bs := boxes(d)
	for _, p := range [][2]string{{"MOD-0001", "MOD-0002"}, {"MOD-0004", "MOD-0005"}} {
		left, right := bs[p[0]], bs[p[1]]
		rs := routeBetween(t, d, p[0], p[1])
		if len(rs) == 0 {
			t.Fatalf("no line joins %s and %s", p[0], p[1])
		}
		var ys []int
		for _, r := range rs {
			runs := runs(r.D)
			if len(runs) != 1 {
				t.Fatalf("the line between %s and %s runs %s, want one straight hop", p[0], p[1], r.D)
			}
			a, b := runs[0][0], runs[0][1]
			if a.y != b.y || min(a.x, b.x) != left.X+left.W || max(a.x, b.x) != right.X {
				t.Errorf("the line between %s %+v and %s %+v runs %s, want straight across from one's side to the other's", p[0], left, p[1], right, r.D)
			}
			if a.y <= max(left.Y, right.Y) || a.y >= min(left.Y+left.H, right.Y+right.H) {
				t.Errorf("the hop %s runs off the height the two boxes share", r.D)
			}
			ys = append(ys, a.y)
		}
		slices.Sort(ys)
		if len(slices.Compact(ys)) != len(rs) {
			t.Errorf("the lines between %s and %s share one lane: %v", p[0], p[1], ys)
		}
	}
	if rs := routeBetween(t, d, "MOD-0004", "MOD-0005"); len(rs) != 1 || !rs[0].Both {
		t.Errorf("transforms and sinks use each other: on the overview one line reads both ways, got %+v", rs)
	}
	// config and sources stand apart, topology between them: theirs is no hop.
	for _, r := range routeBetween(t, d, "MOD-0001", "MOD-0003") {
		if len(runs(r.D)) == 1 {
			t.Errorf("a line between stages that are not neighbours runs straight across topology: %s", r.D)
		}
	}
}

// A spine keeps rule 4: no line leaves or reaches a box beside a bundle of
// its kind, on the overview and on an inside the flow crosses.
func TestASpineKeepsOneBundlePerPlaceAndDirection(t *testing.T) {
	m := Build(Input{Units: pipeline(), Figures: pipelineStages()})
	if mt := measured(t, m.Overview); mt.Split != 0 || mt.SplitIn != 0 {
		t.Errorf("the overview splits %d bundles leaving and %d arriving, want none", mt.Split, mt.SplitIn)
	}
	units := []model.Unit{
		called("MOD-0001", "core"),
		inside("MOD-0002", "MOD-0001", rel("MOD-0003", model.DependsOn)),
		inside("MOD-0003", "MOD-0001", rel("MOD-0004", model.DependsOn), rel("MOD-0002", model.DependsOn)),
		inside("MOD-0004", "MOD-0001", rel("MOD-0002", model.DependsOn), rel("MOD-0006", model.DependsOn)),
		inside("MOD-0006", "MOD-0001"),
		called("MOD-0005", "cli", rel("MOD-0001", model.DependsOn)),
	}
	m = Build(Input{Units: units, Figures: primaryStack(band("bridge", "MOD-0002"), band("manager", "MOD-0003"), band("ipc", "MOD-0004"))})
	for _, ins := range m.Insides {
		if ins.ID != "MOD-0001" {
			continue
		}
		if mt := measured(t, ins.Drawing); mt.Split != 0 || mt.SplitIn != 0 {
			t.Errorf("the inside splits %d bundles leaving and %d arriving, want none", mt.Split, mt.SplitIn)
		}
		if len(routeBetween(t, ins.Drawing, "MOD-0002", "MOD-0003")) != 2 {
			t.Errorf("bridge and manager use each other: want two lanes between them")
		}
	}
}

// The checker reads a hop between two boxes' facing sides as a bundle of
// its own, as it does a straight drop, and two lines of one kind that leave
// a box's edge at one point as one stub, whatever ink each runs on past it.
func TestTheCheckerReadsSideHopsAndSharedStubs(t *testing.T) {
	split := func(body string) (int, int) {
		s, err := readSVG(`<svg width="400" height="300">
<g class="box" data-id="MOD-0001"><rect x="20" y="20" width="100" height="40"/></g>
<g class="box" data-id="MOD-0002"><rect x="160" y="20" width="100" height="40"/></g>
<g class="box" data-id="MOD-0003"><rect x="20" y="200" width="100" height="40"/></g>
<g class="box" data-id="MOD-0004"><rect x="160" y="200" width="100" height="40"/></g>
` + body + `</svg>`)
		if err != nil {
			t.Fatal(err)
		}
		m := measure(s)
		return m.Split, m.SplitIn
	}
	hop := `<path class="w depends_on" d="M120 36 L160 36"><title>a</title></path>
<path class="w depends_on back" d="M160 46 L120 46"><title>b</title></path>
<path class="w depends_on" d="M70 60 L70 100 L60 100 L60 200"><title>c</title></path>
<path class="w depends_on" d="M200 60 L200 120 L80 120 L80 200"><title>d</title></path>
`
	if out, in := split(hop); out != 0 || in != 1 {
		t.Errorf("hops beside drops: %d split leaving, %d arriving; want the hops read as bundles of their own, and the two lines into MOD-0003's top as one split", out, in)
	}
	stub := `<path class="w depends_on" d="M70 60 L70 120 L200 120 L200 200"><title>a</title></path>
<path class="w depends_on back" d="M70 60 L70 140 L210 140 L210 60"><title>b</title></path>
`
	if out, _ := split(stub); out != 0 {
		t.Errorf("two lines from one point of one edge: %d split, want one stub", out)
	}
	apart := `<path class="w depends_on" d="M60 60 L60 120 L200 120 L200 200"><title>a</title></path>
<path class="w depends_on back" d="M80 60 L80 140 L210 140 L210 60"><title>b</title></path>
`
	if out, _ := split(apart); out != 1 {
		t.Errorf("two lines from two points of one edge: %d split, want 1", out)
	}
}
