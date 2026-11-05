package screen

import (
	"regexp"
	"slices"
	"strconv"
	"testing"
)

// A box whose lines of one kind all run into the row right below it sends
// them down one stem that forks into a drop over each box, the way a person
// draws it, even where some of those boxes stand right under it: no line
// runs out of the box beside the stem and through its track. In orca's
// desktop renderer UI, renderer/app-shell uses five boxes in the row below
// it, two of them under it, and renderer/lib five, one under it.
func TestOneStemForksIntoARowBelow(t *testing.T) {
	m := fixture(t, "orca")
	i := slices.IndexFunc(m.Insides, func(in Inside) bool { return in.ID == "MOD-0072" })
	if i < 0 {
		t.Fatal("orca has no desktop renderer UI")
	}
	move := regexp.MustCompile(`M(\d+) (\d+)`)
	for _, unit := range []string{"MOD-0073", "MOD-0076"} {
		j := slices.IndexFunc(m.Insides[i].Focus, func(f Focus) bool { return f.Unit == unit })
		if j < 0 {
			t.Fatalf("desktop renderer UI has no focus view of %s", unit)
		}
		d := m.Insides[i].Focus[j].Drawing
		k := slices.IndexFunc(d.Boxes, func(b Box) bool { return b.ID == unit })
		if k < 0 {
			t.Fatalf("the focus view of %s draws no box for it", unit)
		}
		b := d.Boxes[k]
		stems := map[int]bool{}
		for _, w := range d.Wires {
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
			t.Errorf("%s sends its lines into the row below by %d stems, want 1: %v", unit, len(stems), stems)
		}
	}
}
