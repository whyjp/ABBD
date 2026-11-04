package screen

import (
	"fmt"
	"math"
	"sort"
)

// Ink is what the routed paths look like to a reader: straight pieces of
// one kind, which meet where one ends on another. Rule 17 marks every
// point where ink splits or merges with a dot, and rule 22 sets each ×N
// beside the one stretch of ink it counts.

// Dot marks a point where lines of one kind split or merge.
type Dot struct {
	X, Y int
	Kind string
	// Back says only climbing lines meet there, so the dot is drawn as
	// faint as they are.
	Back bool `json:",omitempty"`
	// Lit are the classes that light the dot with its lines (hover.go),
	// and Hue the palette key it is drawn in.
	Lit string `json:"-"`
	Hue string `json:"-"`
}

// faint marks the dots that only climbing lines meet at.
func faint(dots []Dot, out []drawn) []Dot {
	for i, d := range dots {
		p, any, all := pt{d.X, d.Y}, false, true
		for _, w := range out {
			if hue(w.kind) != d.Kind {
				continue
			}
			for _, part := range w.parts {
				for j := 1; j < len(part); j++ {
					if on(p, part[j-1], part[j]) {
						any, all = true, all && w.back
					}
				}
			}
		}
		dots[i].Back = any && all
	}
	return dots
}

// join is where a line of one kind crosses onto a trunk of its kind.
type join struct {
	at   pt
	kind string
}

type piece struct {
	a, b pt
	kind string
}

// hue is the ink a kind joins: two modules that use each other are joined
// as a direct dependency.
func hue(kind string) string {
	if kind == "mutual" {
		return "bypass"
	}
	return kind
}

func pieces(out []drawn) []piece {
	var ps []piece
	for _, d := range out {
		k := hue(d.kind)
		for _, part := range d.parts {
			for i := 1; i < len(part); i++ {
				if part[i] != part[i-1] {
					ps = append(ps, piece{part[i-1], part[i], k})
				}
			}
		}
	}
	return ps
}

// on says p lies on the axis-parallel piece a-b.
func on(p, a, b pt) bool {
	return min(a.x, b.x) <= p.x && p.x <= max(a.x, b.x) && min(a.y, b.y) <= p.y && p.y <= max(a.y, b.y) && (a.x == b.x && p.x == a.x || a.y == b.y && p.y == a.y)
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// junctions are the points where ink of one colour leaves in three or more
// directions, and the points where a line crosses onto a trunk that runs on
// past it: there two pieces cross, and only the dot says they join.
func junctions(ps []piece, joins []join) []Dot {
	arms := map[string]map[pt]bool{}
	where := map[string]Dot{}
	arm := func(k string, p, to pt) {
		key := fmt.Sprint(k, p)
		if arms[key] == nil {
			arms[key] = map[pt]bool{}
			where[key] = Dot{X: p.x, Y: p.y, Kind: k}
		}
		arms[key][pt{sign(to.x - p.x), sign(to.y - p.y)}] = true
	}
	for _, s := range ps {
		for _, j := range joins {
			if p := j.at; j.kind == s.kind && p != s.a && p != s.b && on(p, s.a, s.b) {
				arm(s.kind, p, s.a)
				arm(s.kind, p, s.b)
			}
		}
	}
	for i, s := range ps {
		arm(s.kind, s.a, s.b)
		arm(s.kind, s.b, s.a)
		for j, o := range ps {
			if i == j || o.kind != s.kind {
				continue
			}
			for _, p := range []pt{o.a, o.b} {
				if p != s.a && p != s.b && on(p, s.a, s.b) {
					arm(s.kind, p, s.a)
					arm(s.kind, p, s.b)
				}
			}
		}
	}
	var out []Dot
	for key, a := range arms {
		if len(a) >= 3 {
			out = append(out, where[key])
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

type box struct{ x, y, w, h int }

func gapTo(b box, a, c pt) float64 {
	// Distance from rectangle b to the axis-parallel piece a-c.
	lo, hi := pt{min(a.x, c.x), min(a.y, c.y)}, pt{max(a.x, c.x), max(a.y, c.y)}
	dx := float64(max(0, b.x-hi.x, lo.x-(b.x+b.w)))
	dy := float64(max(0, b.y-hi.y, lo.y-(b.y+b.h)))
	return math.Hypot(dx, dy)
}

// anchor moves each ×N to the side of its own stretch of ink where the
// nearest other ink is farthest, so the count reads as that stretch's.
// Labels stay off the boxes.
func anchor(out []drawn, ps []piece, boxes []box) {
	var taken []box
	for k := range out {
		d := &out[k]
		if d.count < 2 || d.own[0] == d.own[1] {
			continue
		}
		w := textWidth(fmt.Sprintf("×%d", d.count), 11) + 2
		a, b := d.own[0], d.own[1]
		lo, hi := min(a.y, b.y), max(a.y, b.y)
		best, bestGap := d.label, -1.0
		for y := lo + 6; y <= hi+6; y += 2 {
			for _, x := range []int{a.x + 5, a.x - 5 - w} {
				lb := box{x, y - 9, w, 11}
				clear := true
				for _, bx := range append(append([]box(nil), boxes...), taken...) {
					if lb.x < bx.x+bx.w+2 && bx.x-2 < lb.x+lb.w && lb.y < bx.y+bx.h+2 && bx.y-2 < lb.y+lb.h {
						clear = false
					}
				}
				if !clear {
					continue
				}
				near := math.Inf(1)
				for _, p := range ps {
					if mine(p, a, b) {
						continue
					}
					near = math.Min(near, gapTo(lb, p.a, p.b))
				}
				if near > bestGap {
					best, bestGap = pt{x, y}, near
				}
			}
		}
		d.label = best
		taken = append(taken, box{best.x, best.y - 9, w, 11})
	}
}

// mine says piece p runs along the stretch a-b, as the duplicate pieces of
// a bus's members do.
func mine(p piece, a, b pt) bool {
	return on(p.a, a, b) && on(p.b, a, b) || on(a, p.a, p.b) && on(b, p.a, p.b)
}

func joinPoints(out []drawn) []join {
	var ps []join
	for _, d := range out {
		ps = append(ps, d.joinsAt...)
	}
	return ps
}
