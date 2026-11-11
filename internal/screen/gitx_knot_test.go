package screen

import "testing"

// In self's overview abbd CLI's line to internal/gitx rides its own
// departure trunk down its chain, then the arrival trunk it leads. That
// trunk used to be refused the chain right under where the departure trunk
// ended, so it passed the next row in the middle of the open space and came
// back: the line ran left, right, left and right again, and screen's line
// on the same trunk with it (user-05's knot). The cut overview counts most
// of those lines on a pill, so all six are read where they are drawn,
// internal/gitx's focus view, and any the overview still draws keeps the
// same bound.
func TestALineIntoGitxTurnsBackAtMostOnce(t *testing.T) {
	t.Parallel()
	focused := false
	for _, v := range views(fixture(t, "self")) {
		whole := v.name == "overview focus MOD-0022 internal/gitx"
		if v.name != "overview" && !whole {
			continue
		}
		focused = focused || whole
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		l := newLook(v, s)
		seen := 0
		for _, w := range l.trace.Ways {
			if l.shapeName(w.to) != `box "internal/gitx" (MOD-0022)` {
				continue
			}
			seen++
			flips, dir := 0, 0
			for i := 1; i < len(w.pts); i++ {
				d := w.pts[i].x - w.pts[i-1].x
				if d == 0 {
					continue
				}
				h := 1
				if d < 0 {
					h = -1
				}
				if dir != 0 && h != dir {
					flips++
				}
				dir = h
			}
			if flips > 1 {
				t.Errorf("%s: %s reaches internal/gitx along %v, turning back %d times", v.name, l.shapeName(w.from), w.pts, flips)
			}
		}
		if whole && seen != 6 {
			t.Fatalf("%s: want 6 lines into internal/gitx, got %d", v.name, seen)
		}
	}
	if !focused {
		t.Fatal("self has no focus view of internal/gitx")
	}
}
