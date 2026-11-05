package screen

import (
	"fmt"
	"strings"
	"testing"
)

func withBoxes(n int, class string) Drawing {
	var d Drawing
	for range n {
		d.Boxes = append(d.Boxes, Box{Class: class})
	}
	return d
}

// A level asks for other units when the overview is too coarse or too
// fine, when a module's page draws no parts, and when a view draws more
// lines than its parts can carry. A level that reads well asks for nothing.
func TestLevelsAskForOtherUnits(t *testing.T) {
	cases := []struct {
		name     string
		d        Drawing
		overview bool
		want     string
	}{
		{"coarse overview", withBoxes(3, "design"), true, "fewer than 4"},
		{"loose contracts are not parts", func() Drawing {
			d := withBoxes(3, "design")
			d.Boxes = append(d.Boxes, Box{Class: "loose design"})
			return d
		}(), true, "fewer than 4"},
		{"fine overview", withBoxes(13, "design"), true, "more than 12"},
		{"empty page", withBoxes(0, "inner"), false, "fewer than 2"},
		{"dense page", func() Drawing {
			d := withBoxes(4, "inner")
			for i := range 9 {
				d.Relations = append(d.Relations, [2]string{fmt.Sprint(i), "x"})
			}
			return d
		}(), false, "9 lines between 4 boxes"},
	}
	for _, c := range cases {
		got := strings.Join(level(c.name, c.d, c.overview).Advice, "\n")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: advice %q, want %q", c.name, got, c.want)
		}
	}
	d := withBoxes(6, "design")
	d.Relations = [][2]string{{"a", "b"}, {"b", "c"}}
	if a := level("overview", d, true).Advice; len(a) > 0 {
		t.Errorf("a readable overview asks for %q", a)
	}
}
