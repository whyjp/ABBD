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
	t.Parallel()
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

// grep reaches grep-matcher on a trunk down grep's column, beside
// grep-searcher, the box above grep-matcher. The line passes
// grep-searcher's row straight down that column and turns under it; it
// does not cross the wide space beside it at its middle, far right of both
// its ends, and come back. ripgrep's line to grep-matcher is a contract
// line: traced through the ink it once read as a second bypass line, which
// its route never was.
func TestALineKeepsToItsColumnPastAWideSpace(t *testing.T) {
	t.Parallel()
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
		if seen != 1 {
			t.Fatalf("want grep's line to grep-matcher, got %d", seen)
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
	t.Parallel()
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
	t.Parallel()
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
			// ripgrep stands right over the middle of the drawing, so its
			// line goes round the left: it leaves the middle above
			// grep-printer's row, comes down left of grep-printer and turns
			// right only under grep-searcher, onto the bus. It never comes
			// down the middle and swings round grep-searcher and back.
			if l.shapeName(w.from) == `box "ripgrep" (MOD-0001)` {
				var printer frect
				for id, i := range l.at {
					if strings.HasPrefix(l.shapeName(id), `box "grep-printer" `) {
						printer = l.s.shapes[i].r
					}
				}
				if printer.w == 0 {
					t.Fatal("no box grep-printer")
				}
				for i := 1; i < len(w.pts); i++ {
					a, b := w.pts[i-1], w.pts[i]
					if a.x == b.x && min(a.y, b.y) < printer.y+printer.h && max(a.y, b.y) > printer.y && a.x >= printer.x {
						t.Errorf("ripgrep to grep-matcher comes down at x=%g, not left of grep-printer (%g): %v", a.x, printer.x, w.pts)
					}
				}
				continue
			}
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
		return
	}
	t.Fatal("no view overview")
}

// A line down its own chain's lane keeps straight on it past a wide space
// only where the one line it meets in the row next to it is its own: a
// line that meets another's pass there would read as that line. Kept
// straight, it takes no lane of the space.
func TestALineKeepsItsChainOnlyPastItsOwnPass(t *testing.T) {
	space := func(passer int) (layout, int) {
		l := layout{
			pos:    map[string]rect{"a": {X: 0, W: 100}, "b": {X: 500, W: 100}},
			rows:   [][]string{{"a", "b"}, {"a", "b"}},
			width:  600,
			lanes:  map[[2]int]int{},
			passes: map[int][]int{0: {200}},
			beside: map[int][]int{},
			stubs:  map[int][]int{},
		}
		held := map[[2]int]int{{0, 200}: passer}
		mine := func(q, y int) bool { return held[[2]int{q, y}] == 7 }
		return l, l.gutterTo(1, 200, 200, 200, mine)
	}
	if l, x := space(7); x != 200 || l.lanes[[2]int{1, 1}] != 0 {
		t.Errorf("past its own pass the line runs at %d holding %d lanes; want 200 and none", x, l.lanes[[2]int{1, 1}])
	}
	if _, x := space(3); x == 200 {
		t.Errorf("past another line's pass the line still runs down its chain at 200")
	}
}
