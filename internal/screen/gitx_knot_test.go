package screen

import "testing"

// In self's overview abbd CLI's line to internal/gitx rides its own
// departure trunk down its chain, then the arrival trunk it leads. That
// trunk used to be refused the chain right under where the departure trunk
// ended, so it passed the next row in the middle of the open space and came
// back: the line ran left, right, left and right again, and screen's line
// on the same trunk with it (user-05's knot).
func TestALineIntoGitxTurnsBackAtMostOnce(t *testing.T) {
	for _, v := range views(fixture(t, "self")) {
		if v.name != "overview" {
			continue
		}
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
				t.Errorf("%s reaches internal/gitx along %v, turning back %d times", l.shapeName(w.from), w.pts, flips)
			}
		}
		if seen != 6 {
			t.Fatalf("want 6 lines into internal/gitx, got %d", seen)
		}
		return
	}
	t.Fatal("self has no overview")
}
