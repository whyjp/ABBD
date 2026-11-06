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

// drawnAt is the cost of a drawing seen as seen and turned as turned, with
// moved chips or boxes off what they join.
func drawnAt(seen, turned, moved, crossed int) cost {
	return cost{seen: seen, turned: turned + stepCrossings*moved, moved: moved, crossed: crossed}
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
		t.Errorf("lanes turning more: picked %d, want the plain one", got)
	}
}

// A gathered drawing that crosses no more than every drawing with neither
// lanes nor gather is kept. A laned drawing that crosses less does not
// bound it.
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

// A gathered drawing that crosses more than some unkept drawing is measured
// against the drawing it replaces: it is kept where it crosses no more and
// turns no more lines that could drop straight than that one.
func TestAGatheredDrawingKeepsTheStraightDropsOfTheOneItReplaces(t *testing.T) {
	ts := []tactic{{}, {median: true}, {gather: true}}
	unkept, kept := drawnAt(50, 40, 0, 5), drawnAt(40, 40, 0, 10)
	kept.clear = 1
	gathered := drawnAt(30, 30, 0, 8)
	gathered.clear = 1
	if got := pickOf(ts, []cost{unkept, kept, gathered}); got != 2 {
		t.Errorf("a gather crossing less than the drawing it replaces, as many lines bent in a clear column: picked %d, want the gathered one", got)
	}
	gathered.clear = 2
	if got := pickOf(ts, []cost{unkept, kept, gathered}); got != 1 {
		t.Errorf("a gather bending one more line in a clear column: picked %d, want the median one", got)
	}
	gathered.clear, gathered.crossed = 1, 11
	if got := pickOf(ts, []cost{unkept, kept, gathered}); got != 1 {
		t.Errorf("a gather crossing more than the drawing it replaces: picked %d, want the median one", got)
	}
}

// Every laned drawing is measured against the drawing choose would keep
// with no lanes, not against another laned one: in a chain, each could move
// one chip more than the last.
func TestALanedDrawingIsNotMeasuredAgainstAnotherLanedOne(t *testing.T) {
	ts := []tactic{{}, {laned: true}, {laned: true, median: true}}
	plain, one, two := drawnAt(40, 32, 2, 0), drawnAt(30, 31, 3, 0), drawnAt(20, 30, 4, 0)
	if got := pickOf(ts, []cost{plain, one, two}); got != 1 {
		t.Errorf("lanes moving two chips more than the plain drawing, one more than other lanes: picked %d, want the laned one moving one more", got)
	}
}
