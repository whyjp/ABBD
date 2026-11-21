package screen

import (
	"slices"
	"testing"
)

// The colours a focus view's candidates are judged in come from the first
// view of each box, drawn before any tactic. They must be the colours the
// page is painted in, or the check reads a drawing the page does not show.
func TestFocusColoursDoNotDependOnTheTactic(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		m := fixture(t, name)
		if len(m.hues) == 0 {
			t.Fatalf("%s: no colours chosen", name)
		}
		for id, h := range m.hues {
			if got, ok := m.focusHues[id]; !ok || got != h {
				t.Errorf("%s: %s is colour %d on the page and %d where the focus views were judged", name, id, h, got)
			}
		}
		if len(m.focusHues) != len(m.hues) {
			t.Errorf("%s: %d units coloured where the focus views were judged, %d on the page", name, len(m.focusHues), len(m.hues))
		}
	}
}

// A focus view of an inside is kept on the totals its drawing reads as the
// page shows it: what the check read is what lint reads.
func TestFocusIsJudgedAsLintReadsIt(t *testing.T) {
	t.Parallel()
	seen := 0
	for _, name := range fixtures {
		for _, ins := range fixture(t, name).Insides {
			for _, f := range ins.Focus {
				seen++
				for _, b := range f.Drawing.Boxes {
					if coloured(b) && b.Hue == "" {
						t.Errorf("%s %s focus %s: box %s is not painted", name, ins.ID, f.Unit, b.ID)
					}
				}
				vl, err := lintView(view{d: f.Drawing, budget: overviewWidth, room: screenH - focusNote})
				if err != nil {
					t.Fatal(err)
				}
				if got := gateTotals(vl.Totals); !slices.Equal(got, f.judged) {
					t.Errorf("%s %s focus %s: judged %v, lint reads %v", name, ins.ID, f.Unit, f.judged, got)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no fixture has an inside focus view")
	}
}
