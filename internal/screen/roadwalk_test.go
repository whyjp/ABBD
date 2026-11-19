package screen

import "testing"

// Three lines leave one stub down a road: one runs on straight, and two
// turn off together. Each turn rides the road, each line passes one
// junction, and the junction bends the road, since more lines turn there
// than run on. With one line turning off, it does not.
func TestRoadWalkCountsTurnsAndJunctions(t *testing.T) {
	t.Parallel()
	on := []fpt{{0, 0}, {0, 100}}
	off := []fpt{{0, 0}, {0, 50}, {50, 50}}
	got := walkRoads([][]fpt{on, off})
	if got.turns != 1 || got.junctions != 2 || got.bent != 0 {
		t.Errorf("one line off the road: %+v, want 1 turn, 2 junctions, none bent", got)
	}
	got = walkRoads([][]fpt{on, off, off})
	if got.turns != 2 || got.junctions != 3 || got.bent != 1 || len(got.bentAt) != 1 || got.bentAt[0] != (fpt{0, 50}) {
		t.Errorf("two lines off the road: %+v, want 2 turns, 3 junctions, 1 bent at 0,50", got)
	}
	// Lines that only meet end to end share no ink.
	if got := walkRoads([][]fpt{{{0, 0}, {0, 50}}, {{0, 50}, {0, 100}}}); got.turns+got.junctions+got.bent != 0 {
		t.Errorf("lines meeting end to end: %+v, want nothing", got)
	}
}
