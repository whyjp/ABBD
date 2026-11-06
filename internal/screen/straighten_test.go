package screen

import (
	"strings"
	"testing"
)

// A line that passes a row down its chain leaves its box over the place it
// reaches where the row has room, so it drops straight. In self's
// internal/screen, overview plan stands over the chip → model, but its
// chain ran down the row below at the box's middle, so the line left there
// and turned twice into the chip.
func TestAChainStandsOverWhereItsLineDrops(t *testing.T) {
	for _, v := range views(fixture(t, "self")) {
		if v.name != "MOD-0034 internal/screen" {
			continue
		}
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		l := newLook(v, s)
		for _, w := range l.trace.Ways {
			if strings.Contains(l.shapeName(w.from), "overview plan") && strings.Contains(l.shapeName(w.to), "→ model") {
				if len(w.pts) != 2 || w.pts[0].x != w.pts[1].x {
					t.Errorf("overview plan reaches → model along %v, want one straight drop", w.pts)
				}
				return
			}
		}
		t.Fatal("internal/screen traces no line from overview plan to → model")
	}
	t.Fatal("self has no view of internal/screen")
}

// A chain moved straight leaves its track in the gap empty, so the tracks
// below it stay where they stood. In self's FEAT-0001, /abbd read the code
// stands over → pstack, but its line ran down a chain beside the box's left
// and turned twice into the chip. Moved, it freed a track over abbd diff's
// and record relation's: drawn again those rose a lane, so the steps of
// after work's and evidence's lines into abbd diff's track shrank to 16px
// jogs, the drawing turned no less, and the line stayed bent.
func TestAStraightenedLineLeavesTheTracksBelowInPlace(t *testing.T) {
	for _, v := range views(fixture(t, "self")) {
		if v.name != "FEAT-0001 /abbd" {
			continue
		}
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		l := newLook(v, s)
		found := 0
		for _, w := range l.trace.Ways {
			from, to := l.shapeName(w.from), l.shapeName(w.to)
			switch {
			case strings.Contains(from, "read the code through pstack") && strings.Contains(to, "→ pstack"):
				found++
				if len(w.pts) != 2 || w.pts[0].x != w.pts[1].x {
					t.Errorf("/abbd read the code reaches → pstack along %v, want one straight drop", w.pts)
				}
			case strings.Contains(to, "abbd diff"):
				found++
				for i := 2; i < len(w.pts)-1; i++ {
					if a, b := w.pts[i-1], w.pts[i]; a.x == b.x && abs(int(a.y-b.y)) < jogUnder {
						t.Errorf("%s steps %v to %v into abbd diff, under %dpx", from, a, b, jogUnder)
					}
				}
			}
		}
		if found != 3 {
			t.Fatalf("FEAT-0001 traces %d of the 3 lines to → pstack and abbd diff", found)
		}
		return
	}
	t.Fatal("self has no view of FEAT-0001")
}
