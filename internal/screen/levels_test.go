package screen

import (
	"fmt"
	"strings"
	"testing"
)

func withBoxes(n int, class string) Drawing {
	var d Drawing
	for i := range n {
		d.Boxes = append(d.Boxes, Box{ID: fmt.Sprint("b", i), Name: fmt.Sprint("b", i), Class: class})
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
			d := withBoxes(6, "inner")
			for i := range 6 {
				for j := i + 1; j < 6; j++ {
					d.Relations = append(d.Relations, [2]string{fmt.Sprint("b", i), fmt.Sprint("b", j)})
				}
			}
			return d
		}(), atNested, "15 lines between 6 boxes,"},
	}
	for _, c := range cases {
		got := strings.Join(level(c.name, c.d, c.at).Advice, "\n")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: advice %q, want %q", c.name, got, c.want)
		}
	}
	d := withBoxes(6, "design")
	d.Relations = [][2]string{{"b0", "b1"}, {"b1", "b2"}}
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

// Only lines between the view's parts count. A frame's chips, the modules
// outside, and the overview's loose contracts are no parts, so lines to them
// neither make a view dense nor make a box a shared layer. Two parts that use
// each other draw one line.
func TestLevelsCountOnlyLinesBetweenParts(t *testing.T) {
	d := withBoxes(6, "inner")
	for i := range 6 {
		for _, chip := range []string{"outside-a", "outside-b", "outside-c", "outside-d"} {
			d.Relations = append(d.Relations, [2]string{fmt.Sprint("b", i), chip}, [2]string{chip, fmt.Sprint("b", i)})
		}
	}
	d.Relations = append(d.Relations, [2]string{"b0", "b1"}, [2]string{"b1", "b0"}, [2]string{"b2", "b3"})
	l := level("MOD-0001 m", d, atNested)
	if l.Lines != 2 || len(l.Hubs) > 0 || len(l.Advice) > 0 {
		t.Errorf("lines %d, hubs %v, advice %q; want the 2 lines between parts, no hub and no advice", l.Lines, l.Hubs, l.Advice)
	}

	o := withBoxes(6, "design")
	o.Boxes = append(o.Boxes, Box{ID: "loose", Name: "loose", Class: "loose design"})
	for i := range 6 {
		o.Relations = append(o.Relations, [2]string{fmt.Sprint("b", i), "loose"})
	}
	if l := level("overview", o, atOverview); l.Boxes != 6 || l.Lines != 0 || len(l.Hubs) > 0 {
		t.Errorf("boxes %d, lines %d, hubs %v; a loose contract is no part and no shared layer", l.Boxes, l.Lines, l.Hubs)
	}
}

// When as many parts are used by most of the view as are not, none of them
// is the layer the rest stand on, and the view's density counts every line.
// Otherwise at most the two most used are.
func TestLevelsCapTheSharedLayers(t *testing.T) {
	d := withBoxes(8, "inner")
	for i := range 8 {
		for j := range 4 {
			if i != j {
				d.Relations = append(d.Relations, [2]string{fmt.Sprint("b", i), fmt.Sprint("b", j)})
			}
		}
	}
	if l := level("MOD-0001 m", d, atNested); len(l.Hubs) > 0 || !strings.Contains(strings.Join(l.Advice, ""), "22 lines between 8 boxes,") {
		t.Errorf("hubs %v, advice %q; want no hub when four of eight qualify, and the density counted in full", l.Hubs, l.Advice)
	}

	d = withBoxes(9, "inner")
	for i := range 9 {
		for _, hub := range []int{0, 1, 2} {
			if i != hub && (hub < 2 || i < 6) {
				d.Relations = append(d.Relations, [2]string{fmt.Sprint("b", i), fmt.Sprint("b", hub)})
			}
		}
	}
	if l := level("MOD-0001 m", d, atNested); strings.Join(l.Hubs, ",") != "b0,b1" {
		t.Errorf("hubs %v, want the two most used, b0 and b1", l.Hubs)
	}
}
