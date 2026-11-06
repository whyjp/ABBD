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
