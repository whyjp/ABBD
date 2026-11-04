package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Pointing at a shape keeps lit exactly the shapes a relation of the view
// joins it to, and every line with an end there, in every view of every
// real state. A twin is lit with its unit, and a chip that stands for
// several ports with every one of them.
func TestPointingAtAShapeKeepsWhatItJoins(t *testing.T) {
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			d := v.d
			lit := map[string][]string{}
			for _, b := range d.Boxes {
				lit[string(base(model.ID(b.ID)))] = strings.Fields(b.Lit)
			}
			for _, p := range d.Ports {
				for _, k := range p.Keys {
					lit[k] = strings.Fields(p.Lit)
				}
			}
			own := map[string]string{}
			for id, cls := range lit {
				if len(cls) == 0 || cls[0][0] != 'p' {
					t.Fatalf("%s %s: %s takes no part in pointing", name, v.name, id)
				}
				own[id] = cls[0][1:]
			}
			want := map[string][]string{}
			for id := range lit {
				want[own[id]] = []string{"n" + own[id]}
			}
			for _, r := range d.Relations {
				a, b := string(base(model.ID(r[0]))), string(base(model.ID(r[1])))
				_, okA := own[a]
				_, okB := own[b]
				if okA && okB {
					want[own[a]] = append(want[own[a]], "n"+own[b])
					want[own[b]] = append(want[own[b]], "n"+own[a])
				}
			}
			for id, cls := range lit {
				got := slices.Clone(cls[1:])
				w := slices.Clone(want[own[id]])
				slices.Sort(got)
				slices.Sort(w)
				w = slices.Compact(w)
				if !slices.Equal(got, w) {
					t.Errorf("%s %s: pointing at %s keeps %v lit, its relations join %v", name, v.name, id, got, w)
				}
			}
			for _, w := range d.Wires {
				for _, e := range w.ends {
					if o, ok := own[string(base(model.ID(e)))]; ok && !slices.Contains(strings.Fields(w.Lit), "e"+o) {
						t.Errorf("%s %s: a line to %s does not light when it is pointed at", name, v.name, e)
					}
				}
			}
		}
	}
}
