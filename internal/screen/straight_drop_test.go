package screen

import (
	"strings"
	"testing"
)

// Outside a focus view too, a line between boxes in rows next to each
// other whose spans meet drops straight off its box's bus where that reads
// better, as a person draws it. In self's abbd CLI, review commands stands
// over blueprint publish; its line into it used to leave on the bus of its
// lines and turn twice.
func TestAFullViewLineDropsStraightIntoTheRowBelow(t *testing.T) {
	t.Parallel()
	for _, v := range views(fixture(t, "self")) {
		if v.name != "MOD-0001 abbd CLI" {
			continue
		}
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		l := newLook(v, s)
		for _, w := range l.trace.Ways {
			if strings.Contains(l.shapeName(w.from), "review commands") && strings.Contains(l.shapeName(w.to), "blueprint publish") {
				if len(w.pts) != 2 || w.pts[0].x != w.pts[1].x {
					t.Errorf("review commands reaches blueprint publish along %v, want one straight drop", w.pts)
				}
				return
			}
		}
		t.Fatal("abbd CLI traces no line from review commands to blueprint publish")
	}
	t.Fatal("self has no view of abbd CLI")
}
