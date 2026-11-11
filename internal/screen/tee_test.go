package screen

import (
	"strings"
	"testing"
)

// On orca's src/ some lines drop straight across a flat stretch of another
// line from their own place, where no dot joins them. A reader takes each
// such T for a branch, so lint counts it as a crossing.
func TestATeeCountsAsACrossing(t *testing.T) {
	t.Parallel()
	got, err := Lint(fixture(t, "orca"), "MOD-0098 src/")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views(fixture(t, "orca")) {
		if v.name != "MOD-0098 src/" {
			continue
		}
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		r1 := measure(s).Crossings
		tt := got[0].Totals
		if tt.Tees == 0 {
			t.Fatal("want the tees of src/ counted")
		}
		if tt.Crossings != r1+tt.Tees {
			t.Errorf("crossings %d, want R1's %d plus %d tees", tt.Crossings, r1, tt.Tees)
		}
		return
	}
	t.Fatal("no view MOD-0098 src/")
}

// On pstack's setup and help, /poteto-help's line to 10-recipes stood
// straight under it and dropped across its box's own bus to 01-setup. It
// takes that bus instead, so no line drops across a line from its place,
// and the page pays only that line's two turns (weighed 20), not a
// redrawing of the page.
func TestAStraightDropDoesNotCrossItsOwnBus(t *testing.T) {
	t.Parallel()
	for _, v := range views(fixture(t, "pstack")) {
		if v.name != "MOD-0001 setup and help" {
			continue
		}
		got, err := lintView(v)
		if err != nil {
			t.Fatal(err)
		}
		if w := got.Totals.Weighed; w > 20 {
			t.Errorf("setup and help weighs %d, want at most 20", w)
		}
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		l := newLook(v, s)
		if at := tees(l); len(at) > 0 {
			t.Errorf("tees at %v", at)
		}
		for _, w := range l.trace.Ways {
			if strings.Contains(l.shapeName(w.from), "/poteto-help") && strings.Contains(l.shapeName(w.to), "10-recipes") {
				if n := len(w.pts) - 2; n > 2 {
					t.Errorf("/poteto-help to 10-recipes turns %d times, want its bus's 2", n)
				}
				return
			}
		}
		t.Fatal("no line from /poteto-help to 10-recipes")
	}
	t.Fatal("no view MOD-0001 setup and help")
}
