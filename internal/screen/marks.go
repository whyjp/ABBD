package screen

import (
	"fmt"
	"hash/fnv"
	"slices"
)

// Marks are what the page adds to routed ink so a reader can read it: a gap
// where one line passes over another (rule 24).

const (
	// gapHalf is how far a gap reaches each side of the line that passes
	// over it, the over line's stroke included, and gapEnd how far a
	// crossing must stand from the ends of both pieces to be gapped: closer
	// in, it reads as a line that ends on the other.
	gapHalf, gapEnd = 4, 6
)

// Gap is where a horizontal stretch of a line is cut because a vertical
// line crosses over it there.
type Gap struct{ X, Y int }

// seg is one straight stretch of a wire's ink.
type seg struct {
	a, b pt
	wire int
}

func (s seg) flat() bool { return s.a.y == s.b.y }

// lo and hi are the stretch's extent along its own direction.
func (s seg) lo() int {
	if s.flat() {
		return min(s.a.x, s.b.x)
	}
	return min(s.a.y, s.b.y)
}

func (s seg) hi() int {
	if s.flat() {
		return max(s.a.x, s.b.x)
	}
	return max(s.a.y, s.b.y)
}

func (s seg) bounds() box {
	return box{min(s.a.x, s.b.x), min(s.a.y, s.b.y), abs(s.a.x - s.b.x), abs(s.a.y - s.b.y)}
}

func segsOf(paths []drawn) []seg {
	var out []seg
	for i, p := range paths {
		for _, part := range p.parts {
			part = straight(part)
			for k := 1; k < len(part); k++ {
				if part[k-1] != part[k] {
					out = append(out, seg{part[k-1], part[k], i})
				}
			}
		}
	}
	return out
}

// mark adds the marks to d, whose wires were built from paths in order.
func (d *Drawing) mark(paths []drawn) {
	d.gaps(segsOf(paths))
}

// gaps cuts every horizontal piece where a vertical piece of another line
// crosses it away from both pieces' ends and no dot joins them. The
// horizontal line is always the one broken, so the choice never depends on
// the order the lines are drawn in.
func (d *Drawing) gaps(segs []seg) {
	dot := map[pt]bool{}
	for _, j := range d.Dots {
		dot[pt{j.X, j.Y}] = true
	}
	var hs, vs []seg
	for _, s := range segs {
		if s.flat() {
			hs = append(hs, s)
		} else {
			vs = append(vs, s)
		}
	}
	inside := func(v, lo, hi, room int) bool { return lo+room <= v && v <= hi-room }
	for _, h := range hs {
		for _, v := range vs {
			x, y := v.a.x, h.a.y
			if v.wire == h.wire || dot[pt{x, y}] || !inside(x, h.lo(), h.hi(), gapEnd) || !inside(y, v.lo(), v.hi(), gapEnd) {
				continue
			}
			// The hole would cut the horizontal line's own vertical ink.
			own := slices.ContainsFunc(vs, func(o seg) bool {
				return o.wire == h.wire && abs(o.a.x-x) <= gapHalf && inside(y, o.lo(), o.hi(), -gapHalf)
			})
			w := &d.Wires[h.wire]
			if g := (Gap{x, y}); !own && !slices.Contains(w.Gaps, g) {
				w.Gaps = append(w.Gaps, g)
			}
		}
	}
	// Mask ids are unique to the drawing's ink, so two drawings on one page
	// never share one unless they draw the same lines.
	f := fnv.New32a()
	fmt.Fprint(f, d.Width, d.Height)
	for _, w := range d.Wires {
		fmt.Fprint(f, w.D)
	}
	for i := range d.Wires {
		w := &d.Wires[i]
		if len(w.Gaps) == 0 {
			continue
		}
		slices.SortFunc(w.Gaps, func(a, b Gap) int {
			if a.Y != b.Y {
				return a.Y - b.Y
			}
			return a.X - b.X
		})
		w.Mask = fmt.Sprintf("x%x-%d", f.Sum32(), i)
	}
}
