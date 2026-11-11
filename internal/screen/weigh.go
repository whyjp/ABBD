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
	// stepCrossings is how many crossings one staircase step is worth.
	stepCrossings = 4
	// teeCrossings is what turned weighs a tee at: more than the two
	// excess turns a line pays to take its bus instead, since a T reads as
	// a branch, a relation that is not there.
	teeCrossings = 2*stepCrossings + 1
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
// of rules 13 and 15, then what abbd lint reads from the drawing (hard
// breaches, then its weighed sum, then the crossings with each excess turn
// and tee priced high), then the staircase steps along each routed line's
// way with its crossings, then lines that turn more than twice, braid, and the
// dashed ink of the lines that climb (rule 13). breaks counts the lines
// that run more than detourLimit out of their way and the back lines past
// rule 13's bound. steps counts, once per pair of places, each turn past
// the second and each stretch between two turns shorter than jogUnder (a
// jog), so a line that turns many times reads as a staircase however long
// its steps.
type cost struct {
	touches, breaks, steps, crossings, over2, braid, climb int
	// hard, seen, turned and crossed are what abbd lint reads from the
	// drawn view (judge), crossed its crossings.
	hard, seen, turned, crossed int
	// clear counts the lines that turn where their ends stand in one
	// column with a clear straight drop between (B1 allowing none).
	clear int
	// moved counts the placement breaches and folded chips.
	moved int
	// over is how far, in pixels, the drawing runs past one screen (S),
	// read for the overview only. Whether it runs past at all is weighed
	// right after the hard breaches: a reader who must scroll cannot read
	// it at a glance. How far is weighed only after turned, so a drawing a
	// few pixels shorter that still scrolls wins nothing by crossing more.
	// Weighed so for a module's page too, it took 162 more over the
	// fixtures for one view fewer past the screen.
	over int
	// roomy says the counted tactic would give some ×N room, so drawing
	// with it would draw something else.
	roomy bool
}

// past is 1 for a drawing that runs past one screen, else 0.
func (c cost) past() int { return min(c.over, 1) }

// judge reads the drawn view the way abbd lint does: its hard breaches,
// then seen, lint's weighed sum, then turned, the crossings with each tee
// as teeCrossings and each excess turn as stepCrossings. Both count each
// placement breach and each chip the frame folds into a "+N" chip as
// stepCrossings more, so a drawing that saves turns by folding chips or by
// standing boxes and chips away from what they join reads no better. The
// steps come before the excess turns because a line that turns twice in a
// clear column has 2 excess turns and no step, and weighing those as 4
// crossings each traded crossings for corners a reader does not see as a
// staircase. Weighing them as none let one crossing buy 12 of them.
func (c *cost) judge(d Drawing) {
	vl, err := lintView(view{d: d, budget: insideWidth, room: screenH})
	if err != nil {
		c.hard = 1 << 20
		return
	}
	t := vl.Totals
	moved := t.Placement
	for _, p := range d.Ports {
		if p.Folded {
			moved++
		}
	}
	c.hard, c.moved, c.crossed = t.Hard, moved, t.Crossings
	for _, b := range vl.Breaches {
		if b.Criterion == "B1" && b.Allowed == 0 {
			c.clear++
		}
	}
	c.seen = t.Weighed + stepCrossings*moved
	c.turned = t.Crossings + (teeCrossings-1)*t.Tees + stepCrossings*(t.ExcessTurns+moved)
}

// beats says c reads better than o: the first measure in which they differ
// is lower in c. An excess turn or a staircase step costs a reader
// stepCrossings crossings, so a drawing with fewer wins unless it buys each
// one it saves with more crossings than that.
func (c cost) beats(o cost) bool {
	a := [...]int{c.touches, c.breaks, c.hard, c.past(), c.seen, c.turned, c.over, stepCrossings*c.steps + c.crossings, c.steps, c.over2, c.braid, c.climb}
	b := [...]int{o.touches, o.breaks, o.hard, o.past(), o.seen, o.turned, o.over, stepCrossings*o.steps + o.crossings, o.steps, o.over2, o.braid, o.climb}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func costOf(r routing, reqs []wireReq) cost {
	w := weigh(routing{drawn: r.drawn}, reqs)
	c := cost{touches: w.touches, crossings: w.crossings, braid: w.braid, roomy: r.roomy}
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
			c.steps += len(pts) - 4
		}
		for i := 2; i < len(pts)-1; i++ {
			if span(pts[i-1], pts[i]) < jogUnder {
				c.steps++
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

// laneMoves is how many more placement breaches and folded chips a laned
// drawing may have than the drawing it takes the place of: its lanes save
// steps by standing chips away from their boxes, and seen weighs each chip
// moved as only one step.
const laneMoves = 1

// choose draws a view with every tactic and keeps the first, the one the
// plain tactic orders on the whole width, unless a later one beats the best
// so far. draw leaves what each routing costs in its read, and says false
// for a tactic that has nothing to draw differently in this view. judge
// adds what lint reads from the drawn view. It runs only on a drawing that
// does not already lose on the touches and breaks its routing costs to an
// unlaned drawing, since beats compares those first and lint is most of
// the work.
//
// A laned drawing is kept only where it stands at most laneMoves more chips
// or boxes off what they join, and turns no more, than the drawing choose
// would keep with no lanes, however few its steps: turned weighs an excess
// turn as stepCrossings crossings and seen as one. It is measured against that
// drawing alone, not the least of every other drawing, since the lanes only
// move chips, which moved counts, and a drawing choose would not keep
// anyway costs a reader nothing; measured against the least, a gain in a
// drawing that still loses moved the bound and flipped the view.
//
// A gathered drawing is kept where it crosses no more than every drawing
// with neither lanes nor gather: gathering changes what a chip stands for,
// which no measure counts, so it should only spare crossings, never add
// them to save turns. Where it crosses more than one of them, it is kept
// only where it crosses no more than the drawing choose would keep without
// it, and turns no more lines that could drop straight (B1 in a clear
// column): it gives each box a chip of its own so that the box takes one
// straight line from the frame, so a gathered drawing that saves turns
// elsewhere by bending lines that had dropped straight reads worse than
// its count says. Measured against the least alone, a gain in a drawing
// choose would not keep flipped a view to a gathered one or away from it.
//
// A drawing ranked by fold or entry that stands more
// boxes off what they join than the least of the plainly ranked ones is
// passed over as well: a new ranking should take rows away, not move boxes.
// It is passed over too unless it has fewer hard breaches than every
// plainly ranked drawing, or fits the screen where none of them does: a new
// ranking moves every box's centre and so rewrites every focus page seated
// by them, so it must buy more than a few saved turns, or a small change to
// the units would flip the overview between rankings. A drawing these
// checks pass over sets no bound of the others.
//
// A laned or gathered drawing sets no bound either: not the least
// crossings, not the ranking bounds, and not the touches and breaks a
// later drawing must match, since fits measures it against the drawing
// it would replace, which draws other chips. The ranking bounds never meet
// a gathered drawing anyway: only an overview ranks anew, by fold or
// entry, and only a module's inside gathers.
func choose[T any](ts []tactic, draw func(t tactic, read *cost) (T, bool), judge func(v T, read *cost)) T {
	var vs []T
	var cs []cost
	var kept []tactic
	var bc cost
	b := newGates()
	for i, t := range ts {
		var c cost
		v, ok := draw(t, &c)
		if !ok {
			continue
		}
		if i > 0 && (c.touches > bc.touches || c.touches == bc.touches && c.breaks > bc.breaks) {
			continue
		}
		judge(v, &c)
		if !t.laned && !t.gather {
			if !b.admit(t, c) {
				continue
			}
			if i == 0 || c.beats(bc) {
				bc = c
			}
		}
		vs, cs, kept = append(vs, v), append(cs, c), append(kept, t)
	}
	k, without := pick(kept, cs, make([]bool, len(cs)), b.crossed)
	if k < 0 {
		var none T
		return none
	}
	// The kept drawing is drawn again with room for its counts, where some
	// count would get it, and that is kept if the checks above pass it and
	// it reads no worse. Only the kept drawing's roomy is read, and it was
	// drawn without counted.
	best, bt := vs[k], kept[k]
	if cs[k].roomy {
		t := bt
		t.counted = true
		var c cost
		if v, ok := draw(t, &c); ok {
			judge(v, &c)
			pass := without < 0 || fits(t, c, cs[without], b.crossed)
			if !t.laned && !t.gather {
				pass = b.admit(t, c)
			}
			if pass && !cs[k].beats(c) {
				best, bt = v, t
			}
		}
	}
	// The kept drawing is drawn once more with its lines on roads.
	bt.roads = true
	var c cost
	if v, ok := draw(bt, &c); ok {
		best = v
	}
	return best
}

// pick is which of the drawings of costs cs, those out left out, choose
// keeps, and which it would keep without that one where it is laned or
// gathered, or -1. For a laned drawing that is the one it would keep with
// no lanes: measured against another laned drawing, each laned one in a
// chain could move laneMoves more chips than the last. least is the fewest
// crossings of a drawing with neither lanes nor gather. Ties go to the
// earlier drawing.
func pick(ts []tactic, cs []cost, out []bool, least int) (int, int) {
	w := -1
	for i, c := range cs {
		if !out[i] && (w < 0 || c.beats(cs[w])) {
			w = i
		}
	}
	if w < 0 || !ts[w].laned && !ts[w].gather {
		return w, -1
	}
	out[w] = true
	r, rr := pick(ts, cs, out, least)
	out[w] = false
	o := r
	if ts[w].laned && r >= 0 && ts[r].laned {
		o = rr
	}
	if o < 0 || fits(ts[w], cs[w], cs[o], least) {
		return w, o
	}
	return r, rr
}

// fits says a drawing of cost c drawn with t, laned or gathered, may take
// the place of one of cost o, where least is the fewest crossings of a
// drawing with neither lanes nor gather.
func fits(t tactic, c, o cost, least int) bool {
	if t.laned && (c.moved > o.moved+laneMoves || c.turned-stepCrossings*c.moved > o.turned-stepCrossings*o.moved) {
		return false
	}
	return !t.gather || c.crossed <= least || c.crossed <= o.crossed && c.clear <= o.clear
}

// gates are the least costs, over the drawings choose kept a place for with
// a tactic that neither lanes, gathers nor ranks anew, that a drawing with
// one of those tactics is measured against.
type gates struct {
	crossed, ranked, hard, past int
}

func newGates() gates {
	return gates{crossed: 1 << 20, ranked: 1 << 20, hard: 1 << 20, past: 1}
}

// admit says whether choose may keep a drawing of cost c drawn with t, and,
// where it may, lowers b by c where t is the plain kind of tactic the bound
// is over. Laned and gathered drawings are measured by fits instead.
func (b *gates) admit(t tactic, c cost) bool {
	anew := t.fold || t.entry || t.plain
	if anew && (c.moved > b.ranked || c.hard >= b.hard && c.past() >= b.past) {
		return false
	}
	if !anew {
		b.ranked, b.hard, b.past = min(b.ranked, c.moved), min(b.hard, c.hard), min(b.past, c.past())
	}
	b.crossed = min(b.crossed, c.crossed)
	return true
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
