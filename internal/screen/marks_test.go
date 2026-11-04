package screen

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
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

// Every label sits beside a straight stretch of its own line, off every
// box, chip and other line (rule 25).
func TestLabelsReadAlongTheirLine(t *testing.T) {
	for _, name := range fixtures {
		m := fixture(t, name)
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(units)
		rels, worded, shown, all, labelled := 0, 0, 0, 0, 0
		for _, v := range views(m) {
			d := v.d
			hs, vs := flatSegs(t, d)
			segs := append(hs, vs...)
			all++
			any := false
			drawn, word, said := map[string]bool{}, map[string]bool{}, map[string]bool{}
			for _, w := range d.Wires {
				for _, r := range w.Rels {
					drawn[r] = true
					if via(tr, w.Rels) != "" {
						word[r] = true
					}
					if w.Label != nil {
						said[r] = true
					}
				}
			}
			rels += len(drawn)
			worded += len(word)
			shown += len(said)
			for i, w := range d.Wires {
				l := w.Label
				if l == nil {
					continue
				}
				any = true
				tw := float64(textWidth(l.S, labelPx))
				lb := [4]float64{float64(l.X) - tw/2, float64(l.Y) - 8, tw, 9}
				if l.Up {
					lb = [4]float64{float64(l.X) - 8, float64(l.Y) - tw/2, 9, tw}
				}
				over := func(x, y, w, h float64) bool {
					return lb[0] < x+w && x < lb[0]+lb[2] && lb[1] < y+h && y < lb[1]+lb[3]
				}
				for _, b := range d.Boxes {
					if over(float64(b.X), float64(b.Y), float64(b.W), float64(b.H)) {
						t.Errorf("%s %s: label %q on box %s", name, v.name, l.S, b.ID)
					}
				}
				for _, p := range d.Ports {
					if over(float64(p.X), float64(p.RectY()), float64(p.W), float64(p.H)) {
						t.Errorf("%s %s: label %q on a chip", name, v.name, l.S)
					}
				}
				near := false
				for _, s := range segs {
					x0, y0 := min(s.a.x, s.b.x), min(s.a.y, s.b.y)
					if over(x0, y0, max(s.a.x, s.b.x)-x0, max(s.a.y, s.b.y)-y0) {
						t.Errorf("%s %s: label %q on a line", name, v.name, l.S)
					}
					if s.wire != i && (w.Net == 0 || d.Wires[s.wire].Net != w.Net) {
						continue
					}
					if l.Up && s.a.x == s.b.x && math.Abs(s.a.x-float64(l.X)) <= 12 && between(float64(l.Y), s.a.y, s.b.y, tw/2) ||
						!l.Up && s.a.y == s.b.y && math.Abs(s.a.y-float64(l.Y)) <= 12 && between(float64(l.X), s.a.x, s.b.x, tw/2) {
						near = true
					}
				}
				if !near {
					t.Errorf("%s %s: label %q beside no stretch of its line", name, v.name, l.S)
				}
			}
			if any {
				labelled++
			}
		}
		t.Logf("%s: summed over views, %d drawn relations, %d on a line with one word, %d on a labelled line; %d of %d views carry a label", name, rels, worded, shown, labelled, all)
	}
}
