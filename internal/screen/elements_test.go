package screen

import (
	"encoding/json"
	"testing"
)

// small is a view of three boxes and a frame chip with four lines between
// them: one carries a word, two ride a bundle that says ×2, a lead of two
// hops numbers the flow over two of the lines and names one step, and the
// overview counts one more line on a box as +1. Its ink reads as 6 straight
// runs, and its 4 traced lines may take 12 runs drawn apart.
func small() (Drawing, inkMeasure) {
	return Drawing{
		Boxes: []Box{{ID: "a"}, {ID: "b", More: 1}, {ID: "c"}},
		Ports: []Port{{X: 10, Y: 0, Label: "api"}, {X: 40, Y: 0, Label: "+2", Folded: true}, {X: 40, Y: 0, Label: "+2", Folded: true}},
		Wires: []Wire{{Label: &Label{S: "data"}}, {Count: 2}, {}},
		ways:  make([]Route, 4),
		Counted: [][2]string{
			{"a", "c"},
		},
		Lead: &Lead{Hops: []Hop{{N: 1, way: 0, Word: "reads"}, {N: 2, way: 1}}},
	}, inkMeasure{runs: 6, bends: 2, room: 12, traced: 4}
}

func TestElementsCountWhatAReaderSeesAtRest(t *testing.T) {
	t.Parallel()
	e := elementsOf(small())
	// 3 boxes and 2 chips, the folded ones drawn as one; 6 runs of ink;
	// the line's word and the step's; ×2, +1 and the two hop numbers.
	if e.Shapes != 5 || e.Ink != 6 || e.Bends != 2 || e.Words != 2 || e.Badges != 4 || e.Count != 17 {
		t.Fatalf("got %+v, want 5 shapes, 6 runs with 2 bends, 2 words, 4 badges, 17 in all", e)
	}
	// Skeleton: 5 shapes, the 2 lines the lead rides as one run each, and
	// its 2 numbers. Every line apart: 5 shapes, 12 runs for the 4 traced
	// lines and 3 for the counted one, a word on each of the 5, the 2
	// numbers and the step's word.
	if e.Flow != 2 || e.Possible != 5 || e.Apart != 15 || e.Min != 9 || e.Max != 28 {
		t.Fatalf("got %+v, want flow 2, 5 possible lines taking 15 runs apart, range 9..28", e)
	}
	if e.Percent == nil || *e.Percent != 43 || *e.WarnAt != 25 || e.Over {
		t.Fatalf("got %+v, want 43%% of the range and a warning past 25, not reached", e)
	}
}

func TestElementsWarnAtAShareOfEachViewsOwnRange(t *testing.T) {
	t.Parallel()
	tiny := Drawing{Boxes: make([]Box, 2), ways: make([]Route, 1)}
	tk := inkMeasure{runs: 1, room: 1, traced: 1}
	big := Drawing{Boxes: make([]Box, 30), ways: make([]Route, 40), Counted: make([][2]string, 20)}
	te, be := elementsOf(tiny, tk), elementsOf(big, inkMeasure{runs: 40, room: 120, traced: 40})
	// tiny: 2..4, its one straight line; big: 30..270, its 40 lines drawn
	// straight where 60 could take 180 runs apart.
	if te.Min != 2 || te.Max != 4 || be.Min != 30 || be.Max != 270 {
		t.Fatalf("ranges %d..%d and %d..%d, want 2..4 and 30..270", te.Min, te.Max, be.Min, be.Max)
	}
	if *te.WarnAt != 3 || *be.WarnAt != 234 {
		t.Fatalf("warning points %d and %d, want 3 and 234: each at %d%% of its own range", *te.WarnAt, *be.WarnAt, WarnShare)
	}
	if te.Over || be.Over || be.Count != 70 {
		t.Fatalf("tiny %+v big %+v: neither passes its own point, though big holds 70 elements", te, be)
	}
	// One word on tiny's line takes it one element past its point.
	tiny.Wires = []Wire{{Label: &Label{S: "data"}}}
	if te = elementsOf(tiny, tk); te.Count != 4 || !te.Over || *te.Percent != 100 {
		t.Fatalf("worded tiny %+v, want 4 elements at 100%%, past its point", te)
	}
}

// Ink is counted as a reader sees it: a line that turns twice reads as
// three runs, and lines that share one trunk read as one.
func TestElementsCountRunsOfInk(t *testing.T) {
	t.Parallel()
	line := []turnsOf{{allowed: 2}}
	straight := measureInk([]inkStretch{{0, fpt{0, 0}, fpt{0, 100}}}, line)
	bent := measureInk([]inkStretch{{0, fpt{0, 0}, fpt{0, 40}}, {0, fpt{0, 40}, fpt{50, 40}}, {0, fpt{50, 40}, fpt{50, 100}}}, line)
	if straight.runs != 1 || straight.bends != 0 || bent.runs != 3 || bent.bends != 2 || bent.room != 3 {
		t.Fatalf("straight %+v bent %+v, want 1 run against 3 runs and 2 bends, room 3", straight, bent)
	}
	three := []turnsOf{{allowed: 0}, {allowed: 0}, {allowed: 0}}
	trunk := measureInk([]inkStretch{{0, fpt{10, 0}, fpt{10, 50}}, {1, fpt{10, 20}, fpt{10, 80}}, {2, fpt{10, 80}, fpt{10, 100}}}, three)
	apart := measureInk([]inkStretch{{0, fpt{10, 0}, fpt{10, 100}}, {1, fpt{20, 0}, fpt{20, 100}}, {2, fpt{30, 0}, fpt{30, 100}}}, three)
	if trunk.runs != 1 || apart.runs != 3 {
		t.Fatalf("trunk %+v apart %+v, want 1 run against 3", trunk, apart)
	}
	// Two views alike but for the bends stand apart by them in their range.
	d := Drawing{Boxes: make([]Box, 2), ways: make([]Route, 1)}
	if s, b := elementsOf(d, straight), elementsOf(d, bent); *s.Percent >= *b.Percent {
		t.Fatalf("straight at %d%%, bent at %d%%: fewer bends must stand lower", *s.Percent, *b.Percent)
	}
}

// Merging three worded lines onto one trunk keeps the three lines, draws
// one run where three ran, drops two of the three words and adds one ×3.
func TestElementsOfAMergedBundle(t *testing.T) {
	t.Parallel()
	ways := make([]Route, 3)
	apart := Drawing{Boxes: make([]Box, 3), ways: ways, Wires: []Wire{{Label: &Label{S: "a"}}, {Label: &Label{S: "b"}}, {Label: &Label{S: "c"}}}}
	merged := Drawing{Boxes: make([]Box, 3), ways: ways, Wires: []Wire{{Label: &Label{S: "a"}, Count: 3}}}
	room := inkMeasure{room: 3, traced: 3}
	ka, km := room, room
	ka.runs, km.runs = 3, 1
	a, m := elementsOf(apart, ka), elementsOf(merged, km)
	if a.Lines != m.Lines || a.Count != 9 || m.Count != 6 {
		t.Fatalf("apart %+v merged %+v, want the same 3 lines and 9 then 6 elements", a, m)
	}
}

// A view whose skeleton is all it could draw has no range to stand in, so
// it has no share and no warning point, and its JSON still encodes.
func TestElementsOfAViewWithNoRange(t *testing.T) {
	t.Parallel()
	e := elementsOf(Drawing{Boxes: make([]Box, 3)}, inkMeasure{})
	if e.Min != 3 || e.Max != 3 || e.Percent != nil || e.WarnAt != nil || e.Over {
		t.Fatalf("got %+v, want 3..3 with no share and no warning", e)
	}
	if _, err := json.Marshal(e); err != nil {
		t.Fatal(err)
	}
}

// Every view of the fixtures counts at least its skeleton.
func TestElementsOfTheFixturesStandAboveTheirSkeleton(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"self", "orca", "pstack", "ripgrep"} {
		views, err := Lint(fixture(t, name), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range views {
			if e := v.Elements; e.Count < e.Min {
				t.Errorf("%s %s: %d under its skeleton %d", name, v.View, e.Count, e.Min)
			}
		}
		if sum := SumElements(views); sum.Count == 0 || sum.Min > sum.Count {
			t.Errorf("%s: totals %+v", name, sum)
		}
	}
}
