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
		name string
		d    Drawing
		at   where
		want string
	}{
		{"coarse overview", withBoxes(3, "design"), atOverview, "fewer than 4"},
		{"loose contracts are not parts", func() Drawing {
			d := withBoxes(3, "design")
			d.Boxes = append(d.Boxes, Box{Class: "loose design"})
			return d
		}(), atOverview, "fewer than 4"},
		{"fine overview", withBoxes(13, "design"), atOverview, "more than 12"},
		{"empty nested page", withBoxes(0, "inner"), atNested, "fewer than 2"},
		{"one box under a top-level module", withBoxes(1, "inner"), atTop, "repeats the module"},
		{"dense page", func() Drawing {
			d := withBoxes(4, "inner")
			for i := range 9 {
				d.Relations = append(d.Relations, [2]string{fmt.Sprint(i), "x"})
			}
			return d
		}(), atNested, "9 lines between 4 boxes,"},
	}
	for _, c := range cases {
		got := strings.Join(level(c.name, c.d, c.at).Advice, "\n")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: advice %q, want %q", c.name, got, c.want)
		}
	}
	d := withBoxes(6, "design")
	d.Relations = [][2]string{{"a", "b"}, {"b", "c"}}
	if a := level("overview", d, atOverview).Advice; len(a) > 0 {
		t.Errorf("a readable overview asks for %q", a)
	}
}

// The lines into a box most of the view uses, such as shared types, are a
// shared layer and do not make the view dense.
func TestLevelsLeaveASharedLayerOutOfTheDensity(t *testing.T) {
	d := Drawing{}
	for _, id := range []string{"a", "b", "c", "d", "e", "types"} {
		d.Boxes = append(d.Boxes, Box{ID: id, Name: id, Class: "design"})
	}
	for _, from := range []string{"a", "b", "c", "d", "e"} {
		d.Relations = append(d.Relations, [2]string{from, "types"})
	}
	for _, r := range [][2]string{{"a", "b"}, {"a", "c"}, {"b", "c"}, {"b", "d"}, {"c", "d"}, {"c", "e"}, {"d", "e"}, {"e", "a"}} {
		d.Relations = append(d.Relations, r)
	}
	l := level("overview", d, atOverview)
	if len(l.Advice) > 0 || strings.Join(l.Hubs, ",") != "types" {
		t.Errorf("hubs %v, advice %q; want types as the hub and no advice", l.Hubs, l.Advice)
	}
	d.Relations = d.Relations[3:]
	if l := level("overview", d, atOverview); len(l.Hubs) > 0 {
		t.Errorf("hubs %v; a box two of five boxes use is no shared layer", l.Hubs)
	}
}

// A top-level module with nothing inside is a leaf of the overview, and its
// page holds its surroundings and its text.
func TestLevelsLeaveALeafModuleAlone(t *testing.T) {
	if a := level("MOD-0001 leaf", withBoxes(0, "inner"), atTop).Advice; len(a) > 0 {
		t.Errorf("a leaf module asks for %q", a)
	}
}
