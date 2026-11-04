package screen

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

type flatSeg struct {
	a, b fpt
	wire int
}

func flatSegs(t *testing.T, d Drawing) (hs, vs []flatSeg) {
	t.Helper()
	for i, w := range d.Wires {
		parts, err := flatten(w.D)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range parts {
			for k := 1; k < len(p); k++ {
				a, b := p[k-1], p[k]
				switch {
				case a == b:
				case a.y == b.y:
					hs = append(hs, flatSeg{a, b, i})
				case a.x == b.x:
					vs = append(vs, flatSeg{a, b, i})
				}
			}
		}
	}
	return hs, vs
}

func between(v, a, b, room float64) bool { return min(a, b)+room <= v && v <= max(a, b)-room }

// Where a vertical line passes over a horizontal one away from both ends,
// and no dot says they join, the horizontal line is cut on each side of
// it, and nowhere else (rule 24).
func TestEveryCrossingIsAGap(t *testing.T) {
	total, crossings := 0, 0
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			d := v.d
			hs, vs := flatSegs(t, d)
			dots := map[fpt]bool{}
			for _, j := range d.Dots {
				dots[fpt{float64(j.X), float64(j.Y)}] = true
			}
			want := map[string]bool{}
			for _, h := range hs {
				for _, o := range vs {
					p := fpt{o.a.x, h.a.y}
					if o.wire == h.wire || dots[p] || !between(p.x, h.a.x, h.b.x, gapEnd) || !between(p.y, o.a.y, o.b.y, gapEnd) {
						continue
					}
					own := false
					for _, s := range vs {
						if s.wire == h.wire && abs(int(s.a.x-p.x)) <= gapHalf && between(p.y, s.a.y, s.b.y, -gapHalf) {
							own = true
						}
					}
					if !own {
						want[fmt.Sprint(h.wire, p.x, p.y)] = true
					}
				}
			}
			got := map[string]bool{}
			for i, w := range d.Wires {
				for _, g := range w.Gaps {
					got[fmt.Sprint(i, float64(g.X), float64(g.Y))] = true
				}
			}
			for k := range want {
				if !got[k] {
					t.Errorf("%s %s: crossing %s drawn without a gap", name, v.name, k)
				}
			}
			for k := range got {
				if !want[k] {
					t.Errorf("%s %s: gap %s at no crossing", name, v.name, k)
				}
			}
			pts := map[string]bool{}
			for _, w := range d.Wires {
				for _, g := range w.Gaps {
					pts[fmt.Sprint(g)] = true
				}
			}
			total += len(pts)
			crossings += measured(t, d).Crossings
			var b bytes.Buffer
			if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
				t.Fatal(err)
			}
			for _, w := range d.Wires {
				if w.Mask != "" && (!strings.Contains(b.String(), `<mask id="`+w.Mask+`"`) || !strings.Contains(b.String(), `mask="url(#`+w.Mask+`)"`)) {
					t.Errorf("%s %s: mask %s not drawn", name, v.name, w.Mask)
				}
			}
		}
	}
	t.Logf("%d gapped points across the fixtures, which count %d crossings", total, crossings)
}
