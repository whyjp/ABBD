package screen

import (
	"strings"
	"sync"
	"testing"
)

// A road never bends a route to board it: every route of every view of
// every fixture turns no more once on the roads than as it was routed, so
// a line that could drop straight still does.
func TestARoadAddsNoTurnToARoute(t *testing.T) {
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
	buildEach(t, fresh)
	if views == 0 {
		t.Fatal("no view was drawn on roads")
	}
	t.Logf("%d views on roads, %d routes gained turns", views, gained)
}

// On pstack's overview MOD-0001 stands over MOD-0003 with room to drop
// straight: their mutual line drops straight, as on main, rather than
// stepping over to ride the road beside it.
func TestALineThatCanDropStraightDoes(t *testing.T) {
	t.Parallel()
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

// A road move is judged route by route: one that bends a route it touches
// is refused even where it straightens another as much, and in the long-run
// pass one that gives a route a dogleg is refused too.
func TestAMoveBendsNoRouteItTouches(t *testing.T) {
	t.Parallel()
	bent := []pt{{0, 0}, {100, 0}, {100, 100}, {200, 100}}
	flat := []pt{{0, 0}, {200, 0}}
	legged := []pt{{0, 0}, {100, 0}, {100, 10}, {200, 10}, {200, 100}}
	wider := []pt{{0, 0}, {100, 0}, {100, 50}, {200, 50}, {200, 100}}
	for _, c := range []struct {
		name      string
		was, now  [][]pt
		long, out bool
	}{
		{"one route bends as another straightens", [][]pt{flat, bent}, [][]pt{bent, flat}, false, true},
		{"routes straighten", [][]pt{bent, bent}, [][]pt{flat, bent}, false, false},
		{"a dogleg in the long-run pass", [][]pt{wider}, [][]pt{legged}, true, true},
		{"a dogleg in the near passes", [][]pt{wider}, [][]pt{legged}, false, false},
	} {
		touched := map[int]bool{}
		for i := range c.was {
			touched[i] = true
		}
		if got := bendsMore(c.was, c.now, touched, c.long); got != c.out {
			t.Errorf("%s: bendsMore = %v, want %v", c.name, got, c.out)
		}
	}
}
