package screen

import (
	"slices"
	"testing"
)

// pickOf is the index of the tactic choose keeps of ts, drawing each with
// the cost given for it.
func pickOf(ts []tactic, costs []cost) int {
	return choose(ts, func(t tactic, read *cost) (int, bool) {
		i := slices.Index(ts, t)
		*read = costs[i]
		return i, true
	}, func(int, *cost) {})
}

// drawnAt is the cost of a drawing that lint weighs at weighed, with moved
// chips or boxes off what they join, seen as seen.
func drawnAt(seen, weighed, moved, crossed int) cost {
	return cost{seen: seen, linted: weighed + stepCrossings*moved, moved: moved, crossed: crossed}
}

// A laned drawing is measured against the drawing choose would keep
// without it. Another drawing that loses to that one, however few chips it
// moves or little it weighs, does not pass the laned one over.
func TestALanedDrawingIsMeasuredAgainstTheOneItReplaces(t *testing.T) {
	ts := []tactic{{}, {median: true}, {laned: true}}
	plain := drawnAt(40, 32, 2, 0)
	unkept := drawnAt(60, 30, 0, 0)
	if got := pickOf(ts, []cost{plain, unkept, drawnAt(20, 30, 3, 0)}); got != 2 {
		t.Errorf("lanes moving one chip more and weighing less than the kept drawing: picked %d, want the laned one", got)
	}
	if got := pickOf(ts, []cost{plain, unkept, drawnAt(20, 30, 4, 0)}); got != 0 {
		t.Errorf("lanes moving two chips more: picked %d, want the plain one", got)
	}
	if got := pickOf(ts, []cost{plain, unkept, drawnAt(20, 33, 3, 0)}); got != 0 {
		t.Errorf("lanes weighing more: picked %d, want the plain one", got)
	}
}

// A gathered drawing must cross no more than every drawing with neither
// lanes nor gather. A laned drawing that crosses less does not bound it.
func TestAGatheredDrawingIsBoundByTheUnlanedOnes(t *testing.T) {
	ts := []tactic{{}, {laned: true}, {gather: true}}
	plain, laned := drawnAt(50, 40, 0, 10), drawnAt(45, 60, 3, 5)
	if got := pickOf(ts, []cost{plain, laned, drawnAt(30, 30, 0, 8)}); got != 2 {
		t.Errorf("a gather crossing less than the plain drawing: picked %d, want the gathered one", got)
	}
	if got := pickOf(ts, []cost{plain, laned, drawnAt(30, 30, 0, 11)}); got != 0 {
		t.Errorf("a gather crossing more than the plain drawing: picked %d, want the plain one", got)
	}
}
