package screen

import (
	"slices"
	"testing"
)

// A road many lines ride lights, under the pointer, exactly the lines that
// ride it: the relations of the routes of its kind along its ink are the
// relations it carries, no more and no fewer, however many places and
// kinds merge onto it.
func TestEveryRoadLightsExactlyItsRiders(t *testing.T) {
	shared := 0
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			d := v.d
			if d.Routes == nil {
				continue
			}
			for _, w := range d.Wires {
				var lit []string
				ink := runs(w.D)
				for _, r := range d.Routes {
					if (d.roads || r.Kind == w.Kind) && alongside(ink, r.runs) {
						lit = append(lit, r.rels...)
					}
				}
				slices.Sort(lit)
				want := slices.Clone(w.Rels)
				slices.Sort(want)
				if !slices.Equal(slices.Compact(lit), slices.Compact(want)) {
					t.Errorf("%s %s: the road %s lights %v, carries %v", name, v.name, w.D, slices.Compact(lit), want)
				}
				if len(w.lines) > 1 {
					shared++
				}
			}
		}
	}
	if shared == 0 {
		t.Error("no road carries more than one line, so nothing was checked")
	}
}
