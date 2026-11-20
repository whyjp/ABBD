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

// The overview's names are read too: a name that says only "shared" or
// "base" is asked to say a role, and names in two languages are asked to
// pick one. A level with clear names asks for nothing.
func TestLevelsAskForNamesThatSayWhatABoxDoes(t *testing.T) {
	box := func(names ...string) Drawing {
		var d Drawing
		for i, n := range names {
			d.Boxes = append(d.Boxes, Box{ID: fmt.Sprint("b", i), Name: n, Class: "design"})
		}
		return d
	}
	cases := []struct {
		name  string
		d     Drawing
		want  string
		quiet bool
	}{
		{"empty korean", box("라우팅", "엔진", "공통 모듈", "응답"), "공통 모듈", false},
		{"empty english", box("router", "engine", "shared utils", "reply"), "shared utils", false},
		{"a word of its own", box("라우팅", "엔진", "기본 자료구조와 유틸", "응답"), "", true},
		{"mixed languages", box("라우팅", "엔진", "binding", "render"), "binding, render", false},
		{"one bare name is a proper noun", box("라우팅", "엔진", "응답", "ginS"), "", true},
		{"all english", box("router", "engine", "binding", "render"), "", true},
		{"core can be a part's own name", box("라우팅", "엔진", "core", "응답"), "", true},
		{"acronyms are spoken in any language", box("라우팅", "엔진", "API", "CLI"), "", true},
		{"accented latin names alone", box("éditeur", "façade", "routeur", "moteur"), "", true},
		{"a list of three joined items", box("라우팅", "엔진", "파서·프로세서·억제 서비스", "응답"), "파서·프로세서·억제 서비스", false},
		{"two joined items are one pair", box("라우팅", "엔진", "저장·복원", "응답"), "", true},
		{"accented latin beside korean is bare", box("라우팅", "엔진", "éditeur", "façade"), "éditeur, façade", false},
	}
	for _, c := range cases {
		got := strings.Join(level("overview", c.d, atOverview).Advice, "\n")
		if c.quiet && got != "" || !c.quiet && !strings.Contains(got, c.want) {
			t.Errorf("%s: advice %q, want %q (quiet %v)", c.name, got, c.want, c.quiet)
		}
	}
}

// Both name lines can stand together, a box that is no design part is not
// read, and only the overview's names are.
func TestLevelsReadNamesOnlyOfOverviewParts(t *testing.T) {
	both := withBoxes(0, "design")
	for i, n := range []string{"라우팅", "엔진", "binding", "render", "공통 모듈"} {
		both.Boxes = append(both.Boxes, Box{ID: fmt.Sprint("b", i), Name: n, Class: "design"})
	}
	got := level("overview", both, atOverview).Advice
	if len(got) != 2 || !strings.Contains(got[0], "공통 모듈") || !strings.Contains(got[1], "binding, render") {
		t.Errorf("both lines: %q", got)
	}
	loose := Drawing{Boxes: []Box{{ID: "x", Name: "공통 모듈", Class: "loose design"}, {ID: "y", Name: "shared", Class: "inner"}}}
	if a := level("overview", loose, atOverview).Advice; strings.Contains(strings.Join(a, " "), "the overview names") || strings.Contains(strings.Join(a, " "), "mixes names") {
		t.Errorf("a box that is no part was read: %q", a)
	}
	inside := Drawing{Boxes: []Box{{ID: "a", Name: "공통 모듈", Class: "inner"}, {ID: "b", Name: "common", Class: "inner"}, {ID: "c", Name: "binding", Class: "inner"}}}
	if a := level("MOD-1 x", inside, atNested).Advice; strings.Contains(strings.Join(a, " "), "the overview names") || strings.Contains(strings.Join(a, " "), "mixes names") {
		t.Errorf("a module's page was read for names: %q", a)
	}
}
