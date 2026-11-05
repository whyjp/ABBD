package screen

import (
	"regexp"
	"slices"
	"strconv"
	"testing"
)

// A box whose lines of one kind all run into the row right below it sends
// them down one stem that forks into a drop over each box, the way a person
// draws it, even where one of those boxes stands right under it: that line
// takes the stem and drops straight, so no line runs out of the box beside
// the stem and through its track. Orca's renderer/lib uses five boxes in
// the row below it, one of them straight under it.
func TestOneStemForksIntoARowBelow(t *testing.T) {
	m := fixture(t, "orca")
	var f *Focus
	for i := range m.Insides {
		if m.Insides[i].ID != "MOD-0072" {
			continue
		}
		for j := range m.Insides[i].Focus {
			if m.Insides[i].Focus[j].Unit == "MOD-0076" {
				f = &m.Insides[i].Focus[j]
			}
		}
	}
	if f == nil {
		t.Fatal("orca's desktop renderer UI has no focus view of renderer/lib")
	}
	i := slices.IndexFunc(f.Drawing.Boxes, func(b Box) bool { return b.ID == "MOD-0076" })
	if i < 0 {
		t.Fatal("the focus view draws no renderer/lib box")
	}
	b := f.Drawing.Boxes[i]
	move := regexp.MustCompile(`M(\d+) (\d+)`)
	stems := map[int]bool{}
	for _, w := range f.Drawing.Wires {
		if w.Kind != "depends_on" {
			continue
		}
		for _, mv := range move.FindAllStringSubmatch(w.D, -1) {
			x, _ := strconv.Atoi(mv[1])
			y, _ := strconv.Atoi(mv[2])
			if y == b.Y+b.H && x >= b.X && x <= b.X+b.W {
				stems[x] = true
			}
		}
	}
	if len(stems) != 1 {
		t.Errorf("renderer/lib sends its lines into the row below by %d stems, want 1: %v", len(stems), stems)
	}
}
