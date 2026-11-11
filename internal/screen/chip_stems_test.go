package screen

import (
	"math"
	"testing"
)

// On Orca's src/ page the mobile/ entry chip sends lines of two kinds down
// the chain to shared types and schemas (the user's marks on user-11,
// user-13 and user-15). The line into shared types drops straight where
// the trunk it rode ends, not a lane aside and back, and the chip's
// stems of the two kinds run apart, not 16px beside each other down the
// whole page as one doubled line.
func TestAChipsStemsLeaveItCleanly(t *testing.T) {
	t.Parallel()
	var src *view
	for _, v := range views(fixture(t, "orca")) {
		if v.name == "MOD-0098 src/" {
			src = &v
		}
	}
	if src == nil {
		t.Fatal("no view MOD-0098 src/")
	}
	s, err := sheetOf(src.d)
	if err != nil {
		t.Fatal(err)
	}
	l := newLook(*src, s)
	type run struct{ x, lo, hi float64 }
	var runs []run
	for _, w := range l.trace.Ways {
		if l.shapeName(w.from) != `chip "← mobile/"` {
			continue
		}
		if l.shapeName(w.to) == `box "shared types and schemas" (MOD-0081)` && w.kind == "consumes" && len(w.pts) != 2 {
			t.Errorf("the line into shared types turns: %v", w.pts)
		}
		for i := 1; i < len(w.pts); i++ {
			if a, b := w.pts[i-1], w.pts[i]; a.x == b.x {
				runs = append(runs, run{a.x, math.Min(a.y, b.y), math.Max(a.y, b.y)})
			}
		}
	}
	for i, a := range runs {
		for _, b := range runs[i+1:] {
			if d := math.Abs(a.x - b.x); d > 0 && d <= 16 && math.Min(a.hi, b.hi) > math.Max(a.lo, b.lo) {
				t.Errorf("stems at x=%g and x=%g run %gpx beside each other", a.x, b.x, math.Min(a.hi, b.hi)-math.Max(a.lo, b.lo))
			}
		}
	}
}
