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
	// backShare is rule 13's bound: back lines allowed are one, or this
	// share of the arriving lines. detourLimit is rule 15's: how much
	// longer than the shortest grid path between its ends one line may run.
	backShare   = 0.2
	detourLimit = 240
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

// cost is what a whole drawn view costs a reader, in the order a reader
// pays it: ink that joins places no relation joins (rule 18), then breaks
// of rules 13 and 15, then the staircases along each line's way, then
// crossings, lines that turn more than twice, braid, and the dashed ink of
// the lines that climb (rule 13). breaks counts the lines that run more
// than detourLimit out of their way and the back lines past rule 13's
// bound. jogs counts, once per pair of places, the stretches between two
// turns shorter than jogUnder, as the trace in trace_test.go counts them
// on the page.
type cost struct {
	touches, breaks, jogs, crossings, over2, braid, climb int
}

// beats says c reads better than o: the first measure in which they differ
// is lower in c. A staircase costs a reader more than a crossing, so a
// drawing with fewer jogs wins whatever it crosses.
func (c cost) beats(o cost) bool {
	a := [...]int{c.touches, c.breaks, c.jogs, c.crossings, c.over2, c.braid, c.climb}
	b := [...]int{o.touches, o.breaks, o.jogs, o.crossings, o.over2, o.braid, o.climb}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func costOf(r routing, reqs []wireReq) cost {
	w := weigh(routing{drawn: r.drawn}, reqs)
	c := cost{touches: w.touches, crossings: w.crossings, braid: w.braid}
	// A reader traces each pair of places once, along its fewest turns.
	type pair struct{ from, to string }
	best := map[pair][]pt{}
	var pairs []pair
	for i, pts := range r.ways {
		k := pair{place(reqs[i].from), place(reqs[i].to)}
		if old, ok := best[k]; !ok {
			pairs = append(pairs, k)
			best[k] = pts
		} else if len(pts) < len(old) {
			best[k] = pts
		}
	}
	for _, k := range pairs {
		pts := best[k]
		if len(pts) > 4 {
			c.over2++
		}
		for i := 2; i < len(pts)-1; i++ {
			if span(pts[i-1], pts[i]) < jogUnder {
				c.jogs++
			}
		}
	}
	arrive, back := 0, 0
	for _, d := range r.drawn {
		if !d.joins {
			arrive++
			if d.back {
				back++
			}
		}
		for _, p := range d.parts {
			pts := straight(p)
			run := 0
			for i := 1; i < len(pts); i++ {
				run += span(pts[i-1], pts[i])
			}
			if run-span(pts[0], pts[len(pts)-1]) > detourLimit {
				c.breaks++
			}
			if d.back {
				c.climb += run
			}
		}
	}
	c.breaks += max(0, back-max(1, int(backShare*float64(arrive))))
	return c
}

// weighed keeps what routed ink costs a reader in read. A focus view has no
// read, so it is never weighed.
func weighed(read *cost, r routing, reqs []wireReq) {
	if read != nil {
		*read = costOf(r, reqs)
	}
}

// choose draws a view with every tactic and keeps the first, the one the
// plain tactic orders on the whole width, unless a later one beats the best
// so far. draw leaves what each drawing costs in its read, and says false
// for a tactic that has nothing to draw differently in this view.
func choose[T any](draw func(t tactic, read *cost) (T, bool)) T {
	var best T
	var bc cost
	for i, t := range tactics {
		var c cost
		if v, ok := draw(t, &c); ok && (i == 0 || c.beats(bc)) {
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
	// lo and hi bound the strand, so two strands too far apart to meet
	// or run side by side are not compared point by point.
	lo, hi pt
}

func bound(pts []pt) (pt, pt) {
	lo, hi := pts[0], pts[0]
	for _, p := range pts {
		lo, hi = pt{min(lo.x, p.x), min(lo.y, p.y)}, pt{max(hi.x, p.x), max(hi.y, p.y)}
	}
	return lo, hi
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
				lo, hi := bound(pts)
				ss = append(ss, strand{pts, g, d.kind, d.reqs, lo, hi})
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
			if sa.group == sb.group || sa.lo.x > sb.hi.x+sideBy || sb.lo.x > sa.hi.x+sideBy || sa.lo.y > sb.hi.y+sideBy || sb.lo.y > sa.hi.y+sideBy {
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
