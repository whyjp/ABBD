package screen

// A routing is weighed by what a reader pays to follow it, measured the way
// docs/blueprint-svg.md bounds a drawing: where lines of different bundles
// meet or run together away from their ends, and how often lines turn.
const (
	// meetZone is how close to a stroke's end two strokes may meet: lines
	// that share a place converge there by design.
	meetZone = 26
	// sideBy is how close two strokes of different bundles run before they
	// read as one braid, and jogUnder how short a run between two turns
	// reads as a jog.
	sideBy, jogUnder = 12, 24
)

// weight is what one routing costs a reader. touches counts pairs of strokes
// of one kind where one ends on the other, though not every place on one
// reaches every place on the other: a reader could follow ink from a place
// into a box it never reaches. braid is how far, in pixels, strokes of
// different bundles run side by side, and turns how often strokes bend plus
// how many short jogs they make.
type weight struct{ touches, crossings, braid, turns int }

// beats says w is no worse than o for a reader anywhere and turns less.
func (w weight) beats(o weight) bool {
	return w.touches <= o.touches && w.crossings <= o.crossings && w.braid <= o.braid && w.turns < o.turns
}

// cost is what a whole drawn view costs a reader: weigh's measures over its
// ink, with turns counted along each stroke, and climb, the length of the
// dashed ink of the lines that climb (rule 13).
type cost struct {
	weight
	climb int
}

// beats says c is no worse than o anywhere, climbing ink included, and
// crosses or turns less.
func (c cost) beats(o cost) bool {
	return c.touches <= o.touches && c.crossings <= o.crossings && c.braid <= o.braid && c.turns <= o.turns && c.climb <= o.climb &&
		(c.crossings < o.crossings || c.turns < o.turns)
}

func costOf(ds []drawn, reqs []wireReq) cost {
	c := cost{weight: weigh(routing{drawn: ds}, reqs)}
	for _, d := range ds {
		for _, p := range d.parts {
			pts := straight(p)
			c.turns += max(0, len(pts)-2)
			for i := 1; i < len(pts); i++ {
				if i >= 2 && i < len(pts)-1 && span(pts[i-1], pts[i]) < jogUnder {
					c.turns++
				}
				if d.back {
					c.climb += span(pts[i-1], pts[i])
				}
			}
		}
	}
	return c
}

// choose builds a view with every tactic and keeps the first, the one the
// plain tactic orders, unless a later one beats the best so far.
func choose[T any](build func(tactic) (T, cost)) T {
	best, bc := build(tactics[0])
	for _, t := range tactics[1:] {
		if v, c := build(t); c.beats(bc) {
			best, bc = v, c
		}
	}
	return best
}

type strand struct {
	pts   []pt
	group int
	kind  string
	reqs  []int
}

func weigh(r routing, reqs []wireReq) weight {
	var ss []strand
	for k, d := range r.drawn {
		g := d.net
		if g == 0 {
			g = -1 - k
		}
		for _, p := range d.parts {
			if pts := straight(p); len(pts) > 1 {
				ss = append(ss, strand{pts, g, d.kind, d.reqs})
			}
		}
	}
	type rel struct{ from, to, kind string }
	rels := map[rel]bool{}
	for _, r := range reqs {
		rels[rel{place(r.from), place(r.to), r.kind}] = true
	}
	// joined says every place on a reaches every place on b, either way.
	joined := func(a, b strand) bool {
		for _, i := range a.reqs {
			for _, j := range b.reqs {
				if i == j {
					return true
				}
			}
		}
		for _, i := range a.reqs {
			for _, j := range b.reqs {
				ri, rj := reqs[i], reqs[j]
				if !rels[rel{place(ri.from), place(rj.to), ri.kind}] || !rels[rel{place(rj.from), place(ri.to), ri.kind}] {
					return false
				}
			}
		}
		return true
	}
	var w weight
	for _, pts := range r.ways {
		w.turns += max(0, len(pts)-2)
		for i := 2; i < len(pts)-1; i++ {
			if span(pts[i-1], pts[i]) < jogUnder {
				w.turns++
			}
		}
	}
	for a := range ss {
		for b := a + 1; b < len(ss); b++ {
			sa, sb := ss[a], ss[b]
			if sa.group == sb.group {
				continue
			}
			w.crossings += crossings(sa.pts, sb.pts)
			w.braid += beside(sa.pts, sb.pts)
			if sa.kind == sb.kind && (endsOn(sa.pts, sb.pts) || endsOn(sb.pts, sa.pts)) && !joined(sa, sb) {
				w.touches++
			}
		}
	}
	return w
}

func span(a, b pt) int { return abs(a.x-b.x) + abs(a.y-b.y) }

func nearEnd(p pt, pts []pt) bool {
	return within(p, pts[0], meetZone) || within(p, pts[len(pts)-1], meetZone)
}

func within(p, q pt, r int) bool {
	dx, dy := p.x-q.x, p.y-q.y
	return dx*dx+dy*dy < r*r
}

// crossings counts where two strokes cross away from their ends, a crossing
// within a few pixels of another counted once.
func crossings(a, b []pt) int {
	var seen []pt
	for i := 1; i < len(a); i++ {
		for j := 1; j < len(b); j++ {
			p, ok := meet(a[i-1], a[i], b[j-1], b[j])
			if !ok || nearEnd(p, a) || nearEnd(p, b) {
				continue
			}
			dup := false
			for _, q := range seen {
				dup = dup || within(p, q, 6)
			}
			if !dup {
				seen = append(seen, p)
			}
		}
	}
	return len(seen)
}

// meet is where a run from a to b crosses a run from c to d at a right
// angle, each counted from its start up to but not at its end.
func meet(a, b, c, d pt) (pt, bool) {
	if a.y != b.y {
		a, b, c, d = c, d, a, b
	}
	if a.y != b.y || c.x != d.x || a.x == b.x || c.y == d.y {
		return pt{}, false
	}
	p := pt{c.x, a.y}
	return p, onward(p.x, a.x, b.x) && onward(p.y, c.y, d.y)
}

func onward(v, from, to int) bool {
	if from < to {
		return from <= v && v < to
	}
	return to < v && v <= from
}

// beside is how far, sampled every two pixels, stroke a runs parallel to
// stroke b within sideBy of it, away from both strokes' ends.
func beside(a, b []pt) int {
	total, along, last := 0, 0, -1
	for i := 1; i < len(a); i++ {
		p, q := a[i-1], a[i]
		l := span(p, q)
		dx, dy := sign(q.x-p.x), sign(q.y-p.y)
		for t := 0; t < l; t += 2 {
			at := along + t
			s := pt{p.x + dx*t, p.y + dy*t}
			if last >= 0 && !nearEnd(s, a) && !nearEnd(s, b) {
				for j := 1; j < len(b); j++ {
					c, d := b[j-1], b[j]
					if (dx != 0) == (c.y == d.y) && c != d && within(s, closest(s, c, d), sideBy) {
						total += at - last
						break
					}
				}
			}
			last = at
		}
		along += l
	}
	return total
}

// closest is the point of the run from c to d nearest to p.
func closest(p, c, d pt) pt {
	return pt{max(min(c.x, d.x), min(p.x, max(c.x, d.x))), max(min(c.y, d.y), min(p.y, max(c.y, d.y)))}
}

// endsOn says a point where stroke b starts, turns or ends lies on stroke a.
func endsOn(a, b []pt) bool {
	for _, p := range b {
		for i := 1; i < len(a); i++ {
			if on(p, a[i-1], a[i]) {
				return true
			}
		}
	}
	return false
}
