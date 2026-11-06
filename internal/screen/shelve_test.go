package screen

import (
	"slices"
	"testing"
)

// Three lines cross one gap. The first two overlap and cross whichever
// lies above, so they take two tracks. The third overlaps only the second
// and crosses nothing in either order, so it takes the first track, clear
// of the first line, and the gap stays two tracks deep.
func TestAGapHoldsNoMoreTracksThanItsCrossingsNeed(t *testing.T) {
	spans := [][2]int{{0, 100}, {50, 200}, {150, 300}}
	meet := func(a, b int) bool { return spans[a][0] <= spans[b][1] && spans[b][0] <= spans[a][1] }
	held := func(a, b int) bool { return min(a, b) == 0 && max(a, b) == 1 }
	got := shelve(len(spans), meet, held, func(int) int { return 0 })
	if want := []int{0, 1, 0}; !slices.Equal(got, want) {
		t.Fatalf("tracks %v, want %v", got, want)
	}
	// Lines that overlap never share a track, held or not.
	loose := shelve(len(spans), meet, func(a, b int) bool { return false }, func(int) int { return 0 })
	for k := range spans {
		for q := range k {
			if meet(q, k) && loose[q] == loose[k] {
				t.Fatalf("lines %d and %d overlap on track %d", q, k, loose[k])
			}
		}
	}
}
