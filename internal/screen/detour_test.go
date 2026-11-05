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

// On orca's src/ the WSL guest agent sends one line down its trunk to
// shared types and one to @orca/process-host, whose arrival trunk every
// place on it also reaches shared types from. The line to process-host
// runs its track onto that trunk: it turns twice, with no small step
// beside the trunk, since following its track from the trunk leads a
// reader only where the trunk's lines already go.
func TestAHarmlessJoinTakesNoStep(t *testing.T) {
	for _, v := range views(fixture(t, "orca")) {
		if v.name != "MOD-0098 src/" {
			continue
		}
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		l := newLook(v, s)
		seen := 0
		for _, w := range l.trace.Ways {
			if l.shapeName(w.from) != `box "WSL guest agent" (MOD-0094)` || l.shapeName(w.to) != `box "@orca/process-host" (MOD-0070)` {
				continue
			}
			seen++
			if n := len(w.pts) - 2; n != 2 {
				t.Errorf("WSL guest agent to process-host turns %d times, want 2: %v", n, w.pts)
			}
		}
		if seen != 1 {
			t.Fatalf("want one line from WSL guest agent to process-host, got %d", seen)
		}
		return
	}
	t.Fatal("no view MOD-0098 src/")
}

// grep-pcre2, grep-printer and grep-regex reach grep-matcher's contracts
// on one arrival bus past grep-searcher, which stands right over
// grep-matcher, so the bus passes that row beside it. grep-printer's stub
// stands over the trunk and runs straight on into it, and grep-regex, on
// the far side of grep-searcher, passes the row on its own side and joins
// the bus's track under it: no contract line runs over grep-matcher's
// whole width to the trunk and turns back to its drop.
func TestAContractLineJoinsItsBusOnItsOwnSide(t *testing.T) {
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
			if l.shapeName(w.to) != `box "grep-matcher" (MOD-0008)` || w.kind != "contract" {
				continue
			}
			seen++
			for i := 2; i+1 < len(w.pts); i++ {
				a, b, c, d := w.pts[i-2], w.pts[i-1], w.pts[i], w.pts[i+1]
				if a.y == b.y && c.y == d.y && (b.x-a.x)*(d.x-c.x) < 0 {
					t.Errorf("%s to grep-matcher turns back: %v", l.shapeName(w.from), w.pts)
				}
			}
		}
		if seen < 3 {
			t.Fatalf("want the contract lines into grep-matcher, got %d", seen)
		}
	}
}
