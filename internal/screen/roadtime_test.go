package screen

import (
	"runtime"
	"testing"
	"time"
)

// A drawing of 100 boxes on roads, each of 50 using two of the other 50,
// keeps every route and builds in well under a minute: weighing a move
// recounts only the lines, points and groups the move changes, where
// recounting the whole view for every move once kept it routing for over
// half an hour. The work is pinned by allocations, which are the same run
// to run and on a busy machine where time is not: the build makes about
// 123M in about 30s, where a whole recount for every move made 131M for
// half as many boxes. The time bound is generous, for a loaded machine,
// and off under the race detector.
// Mallocs counts the whole process, so this test must never call
// t.Parallel.
func TestAHundredBoxRoadDrawingBuildsQuickly(t *testing.T) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	m := pairs(50)
	took := time.Since(start)
	runtime.ReadMemStats(&after)
	d := m.Overview
	if !d.roads || len(d.Routes) != 100 {
		t.Fatalf("100 boxes drew %d routes on roads %v, want 100 on roads", len(d.Routes), d.roads)
	}
	n := after.Mallocs - before.Mallocs
	if n > 160_000_000 {
		t.Errorf("100 boxes on roads took %d allocations to build, past 160M", n)
	}
	if !raceOn && took > 3*time.Minute {
		t.Errorf("100 boxes on roads took %v to build, past 3m", took)
	}
}
