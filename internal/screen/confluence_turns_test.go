package screen

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// A road never bends a route to board it: every route of every pstack
// view turns no more once on the roads than as it was routed, so a line
// that could drop straight still does.
func TestARoadAddsNoTurnToARoute(t *testing.T) {
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	views, gained := 0, 0
	roadsDrawn = func(routed, ways [][]pt) {
		mu.Lock()
		defer mu.Unlock()
		views++
		for i := range ways {
			if was, now := len(straight(routed[i]))-2, len(straight(ways[i]))-2; now > was {
				gained++
				t.Errorf("route %d turns %d times on the roads, %d as routed: %v -> %v", i, now, was, straight(routed[i]), ways[i])
			}
		}
	}
	defer func() { roadsDrawn = nil }()
	Build(Input{Units: units})
	if views == 0 {
		t.Fatal("no view was drawn on roads")
	}
	t.Logf("%d views on roads, %d routes gained turns", views, gained)
}

// On pstack's overview MOD-0001 stands over MOD-0003 with room to drop
// straight: their mutual line drops straight, as on main, rather than
// stepping over to ride the road beside it.
func TestALineThatCanDropStraightDoes(t *testing.T) {
	for _, v := range views(fixture(t, "pstack")) {
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
			if !strings.HasSuffix(l.shapeName(w.from), "(MOD-0001)") || !strings.HasSuffix(l.shapeName(w.to), "(MOD-0003)") {
				continue
			}
			seen++
			if n := len(turns(w.pts)) - 2; n > 0 {
				t.Errorf("MOD-0001 to MOD-0003 turns %d times: %v", n, w.pts)
			}
		}
		if seen == 0 {
			t.Fatal("no line from MOD-0001 to MOD-0003")
		}
		return
	}
	t.Fatal("no overview")
}
