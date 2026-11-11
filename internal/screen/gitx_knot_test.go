package screen

import "testing"

// In self's overview abbd CLI's line to internal/gitx rides its own
// departure trunk down its chain, then the arrival trunk it leads. That
// trunk used to be refused the chain right under where the departure trunk
// ended, so it passed the next row in the middle of the open space and came
// back: the line ran left, right, left and right again, and screen's line
// on the same trunk with it (user-05's knot). The uncut overview, still the
// default for a repo that does not turn the cut on, draws all six. The cut
// overview counts most of them on a pill, so internal/gitx's focus view
// draws all six there, and any the cut overview still draws keeps the bound.
func TestALineIntoGitxTurnsBackAtMostOnce(t *testing.T) {
	t.Parallel()
	const focus = "overview focus MOD-0022 internal/gitx"
	want := map[string]bool{"uncut overview": true, focus: true}
	for _, v := range views(drawAll(t, "self")) {
		if v.name == "overview" {
			v.name = "uncut overview"
			wantSixIntoGitx(t, v)
			delete(want, v.name)
		}
	}
	for _, v := range views(fixture(t, "self")) {
		switch v.name {
		case focus:
			wantSixIntoGitx(t, v)
			delete(want, v.name)
		case "overview":
			linesIntoGitx(t, v)
		}
	}
	for name := range want {
		t.Errorf("self has no %s", name)
	}
}

func wantSixIntoGitx(t *testing.T, v view) {
	t.Helper()
	if seen := linesIntoGitx(t, v); seen != 6 {
		t.Errorf("%s: want 6 lines into internal/gitx, got %d", v.name, seen)
	}
}

// linesIntoGitx fails each line into internal/gitx that turns back more
// than once and returns how many lines it read.
func linesIntoGitx(t *testing.T, v view) int {
	t.Helper()
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
	return seen
}
