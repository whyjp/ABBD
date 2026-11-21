package screen

import "testing"

// A view's twins are the boxes whose ID ends in the twin mark, and lint sums
// them over the views.
func TestLintCountsTwinBoxes(t *testing.T) {
	d := Drawing{Boxes: []Box{{ID: "A"}, {ID: "B"}, {ID: "B" + twinMark}, {ID: "C" + twinMark}}}
	vl, err := lintView(view{name: "x", d: d})
	if err != nil {
		t.Fatal(err)
	}
	if vl.Totals.Twins != 2 {
		t.Fatalf("twins = %d, want 2", vl.Totals.Twins)
	}
	if got := Sum([]ViewLint{vl, vl}).Twins; got != 4 {
		t.Fatalf("summed twins = %d, want 4", got)
	}
}
