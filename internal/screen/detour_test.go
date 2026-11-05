package screen

import (
	"strings"
	"testing"
)

// On ripgrep's overview grep sends lines down a trunk to the boxes below
// and onto the bus that ripgrep shares into the row under it. grep-regex
// stands almost under grep, so its line drops onto that bus and into it:
// it never rides on to another box's drop and back (B3), and no line of
// the view runs beside itself (B4).
func TestALineToTheBoxBelowDoesNotRidePastIt(t *testing.T) {
	got, err := Lint(fixture(t, "ripgrep"), "overview")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got[0].Breaches {
		if b.Criterion == "B4" || b.Criterion == "B3" && strings.Contains(b.Text, `"grep" (MOD-0006) to box "grep-regex"`) {
			t.Errorf("%s: %s", b.Criterion, b.Text)
		}
	}
}

// grep and ripgrep reach grep-matcher on a trunk down grep's column,
// beside grep-searcher, the box above grep-matcher. The lines pass
// grep-searcher's row straight down that column and turn under it; they
// do not cross the wide space beside it at its middle, far right of both
// their ends, and come back.
func TestALineKeepsToItsColumnPastAWideSpace(t *testing.T) {
	for _, v := range views(fixture(t, "ripgrep")) {
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
			if l.shapeName(w.to) != `box "grep-matcher" (MOD-0008)` || w.kind != "bypass" {
				continue
			}
			seen++
			a, b := w.pts[0].x, w.pts[len(w.pts)-1].x
			for _, p := range w.pts {
				if p.x > max(a, b) || p.x < min(a, b) {
					t.Errorf("%s to grep-matcher passes x=%g, outside its ends' %g to %g", l.shapeName(w.from), p.x, a, b)
				}
			}
		}
		if seen != 2 {
			t.Fatalf("want grep's and ripgrep's lines to grep-matcher, got %d", seen)
		}
	}
}
