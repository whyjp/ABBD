package screen

import "testing"

// On the self fixture's overview a contract line climbs back beside lines
// that run down the other way. They are drawn as a two-lane road (#232):
// the climbing lane keeps to its own right, 6px off, and no line runs
// both ways beside another at any other gap.
func TestOppositeLinesRideATwoLaneRoad(t *testing.T) {
	t.Parallel()
	s, err := sheetOf(fixture(t, "self").Overview)
	if err != nil {
		t.Fatal(err)
	}
	road := 0.0
	besidePairs(s, func(r, q axisRun, upright bool, along float64) {
		if twoLane(r, q, upright) {
			road += along
		}
	})
	if road == 0 {
		t.Error("no two lines run opposite ways as a two-lane road")
	}
	if off := twoLaneOff(s); off != 0 {
		t.Errorf("lines run opposite ways side by side off a two-lane road for %dpx", off)
	}
}
