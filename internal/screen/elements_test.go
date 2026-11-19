package screen

import (
	"encoding/json"
	"testing"
)

// small is a view of three boxes and a frame chip with four lines between
// them: one carries a word, two ride a bundle that says ×2, a lead of two
// hops numbers the flow over two of the lines and names one step, and the
// overview counts one more line on a box as +1.
func small() Drawing {
	return Drawing{
		Boxes: []Box{{ID: "a"}, {ID: "b", More: 1}, {ID: "c"}},
		Ports: []Port{{X: 10, Y: 0, Label: "api"}, {X: 40, Y: 0, Label: "+2", Folded: true}, {X: 40, Y: 0, Label: "+2", Folded: true}},
		Wires: []Wire{{Label: &Label{S: "data"}}, {Count: 2}, {}},
		ways:  make([]Route, 4),
		Counted: [][2]string{
			{"a", "c"},
		},
		Lead: &Lead{Hops: []Hop{{N: 1, way: 0, Word: "reads"}, {N: 2, way: 1}}},
	}
}

func TestElementsCountWhatAReaderSeesAtRest(t *testing.T) {
	t.Parallel()
	e := elementsOf(small())
	// 3 boxes and 2 chips, the folded ones drawn as one; 4 lines; the
	// line's word and the step's; ×2, +1 and the two hop numbers.
	if e.Shapes != 5 || e.Lines != 4 || e.Words != 2 || e.Badges != 4 || e.Count != 15 {
		t.Fatalf("got %+v, want 5 shapes, 4 lines, 2 words, 4 badges, 15 in all", e)
	}
	// Skeleton: 5 shapes, the 2 lines the lead rides and its 2 numbers.
	// Every line apart: 5 shapes, the 4 drawn and 1 counted lines with a
	// word each, the 2 numbers and the step's word.
	if e.Flow != 2 || e.Possible != 5 || e.Min != 9 || e.Max != 18 {
		t.Fatalf("got %+v, want flow 2, 5 possible lines, range 9..18", e)
	}
	if e.Percent == nil || *e.Percent != 67 || *e.WarnAt != 15 || e.Over {
		t.Fatalf("got %+v, want 67%% of the range and a warning past 15, not reached", e)
	}
}

func TestElementsWarnAtAShareOfEachViewsOwnRange(t *testing.T) {
	t.Parallel()
	tiny := Drawing{Boxes: make([]Box, 2), ways: make([]Route, 1)}
	big := Drawing{Boxes: make([]Box, 30), ways: make([]Route, 40), Counted: make([][2]string, 20)}
	te, be := elementsOf(tiny), elementsOf(big)
	// tiny: 2..4, its one line drawn; big: 30..150, its 40 lines drawn.
	if te.Min != 2 || te.Max != 4 || be.Min != 30 || be.Max != 150 {
		t.Fatalf("ranges %d..%d and %d..%d, want 2..4 and 30..150", te.Min, te.Max, be.Min, be.Max)
	}
	if *te.WarnAt != 3 || *be.WarnAt != 120 {
		t.Fatalf("warning points %d and %d, want 3 and 120: each at %d%% of its own range", *te.WarnAt, *be.WarnAt, WarnShare)
	}
	if te.Over || be.Over || be.Count != 70 {
		t.Fatalf("tiny %+v big %+v: neither passes its own point, though big holds 70 elements", te, be)
	}
	// One word on tiny's line takes it one element past its point.
	tiny.Wires = []Wire{{Label: &Label{S: "data"}}}
	if te = elementsOf(tiny); te.Count != 4 || !te.Over || *te.Percent != 100 {
		t.Fatalf("worded tiny %+v, want 4 elements at 100%%, past its point", te)
	}
}

// Merging three worded lines into one bundle keeps the three lines, drops
// two of the three words and adds one ×3: one element fewer.
func TestElementsOfAMergedBundle(t *testing.T) {
	t.Parallel()
	apart := Drawing{Boxes: make([]Box, 3), ways: make([]Route, 3), Wires: []Wire{{Label: &Label{S: "a"}}, {Label: &Label{S: "b"}}, {Label: &Label{S: "c"}}}}
	merged := Drawing{Boxes: make([]Box, 3), ways: make([]Route, 3), Wires: []Wire{{Label: &Label{S: "a"}, Count: 3}}}
	a, m := elementsOf(apart), elementsOf(merged)
	if a.Lines != m.Lines || a.Count != 9 || m.Count != 8 {
		t.Fatalf("apart %+v merged %+v, want the same 3 lines and 9 then 8 elements", a, m)
	}
}

// A view whose skeleton is all it could draw has no range to stand in, so
// it has no share and no warning point, and its JSON still encodes.
func TestElementsOfAViewWithNoRange(t *testing.T) {
	t.Parallel()
	e := elementsOf(Drawing{Boxes: make([]Box, 3)})
	if e.Min != 3 || e.Max != 3 || e.Percent != nil || e.WarnAt != nil || e.Over {
		t.Fatalf("got %+v, want 3..3 with no share and no warning", e)
	}
	if _, err := json.Marshal(e); err != nil {
		t.Fatal(err)
	}
}

// Every view of the fixtures counts at least its skeleton, and passes the
// view with every line drawn apart and worded only by its bundle badges.
func TestElementsOfTheFixturesStandInTheirRange(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"self", "orca", "pstack", "ripgrep"} {
		views, err := Lint(fixture(t, name), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range views {
			e := v.Elements
			if e.Count < e.Min || e.Count > e.Max+e.Badges {
				t.Errorf("%s %s: %d outside %d..%d", name, v.View, e.Count, e.Min, e.Max)
			}
		}
		if sum := SumElements(views); sum.Count == 0 || sum.Min > sum.Count {
			t.Errorf("%s: totals %+v", name, sum)
		}
	}
}
