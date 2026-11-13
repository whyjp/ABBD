package screen

import (
	"math/bits"
	"slices"
	"sort"
)

// A move changes one route, or the few on one piece of road, while the
// cost reads every route. Each count of the cost is read on a line (runs
// beside or across each other), at a point (a far split), or among the
// routes of one group (a fork, a tee, a comb, a dotted route's own
// stretch), so roadIndex keeps what each line, point and group adds, and
// a change weighs a move by taking out what the routes it moves added there
// and putting in what they add after it. A view weighs thousands of moves,
// and recounting the whole view for each kept a drawing of 100 boxes
// routing for over half an hour.

// roadIndex is a view's routes as roadCost counts them, by line, point
// and group, and their cost.

// track is the line a straight run lies on: its x when it is upright, its
// y when it is not.
type track int

func onTrack(vertical bool, at int) track {
	if vertical {
		return track(at<<1 | 1)
	}
	return track(at << 1)
}

func (t track) vertical() bool { return t&1 == 1 }
func (t track) at() int        { return int(t >> 1) }

// leg is run k of a route, from its point k to k+1, and the span it covers
// on its track. flat says it runs along its track, not across it.
type leg struct {
	route, k, lo, hi int
	flat             bool
}

// roadSpan is a piece of road on a track: the runs of one style that touch
// or overlap there.
type roadSpan struct {
	style  string
	lo, hi int
}

// pass is a route passing a point: the way it comes in and goes out.
type pass struct {
	in, out  pt
	route, k int
}

// stubAt is a route's stub and where it stands.
type stubAt struct {
	stub
	route, x int
}

type roadIndex struct {
	v    roadView
	ways [][]pt
	cost roadCost
	// lines are the runs on each track, and spans the road they make there.
	lines map[track][]leg
	spans map[track][]roadSpan
	// ups and flats are where lines that hold runs lie, in order, and
	// upSpans and flatSpans the road on each.
	ups, flats         []int
	upSpans, flatSpans [][]roadSpan
	// verts are the routes turning at each point, cols and rows those
	// points by their x and y, and far the far splits at each.
	verts      map[pt][]pass
	cols, rows map[int][]pt
	far        map[pt]int
	turns      []int
	forks      []int
	tees       map[pt]int
	teesOf     [][]pt
	stubs      map[stub][]stubAt
	combs      map[stub]int
	unseen     []bool
	scratch    *change
}

// legsOf calls f with each run of route i that is not a point.
func legsOf(i int, w []pt, f func(track, leg)) {
	for k := 0; k+1 < len(w); k++ {
		a, b := w[k], w[k+1]
		if a == b {
			continue
		}
		v, at, lo, hi := runOf(a, b)
		f(onTrack(v, at), leg{i, k, lo, hi, a.x == b.x || a.y == b.y})
	}
}

// spansOf merges the runs on a track into the pieces of road they make.
func spansOf(legs []leg, style []string) []roadSpan {
	ss := make([]roadSpan, len(legs))
	for i, s := range legs {
		ss[i] = roadSpan{style[s.route], s.lo, s.hi}
	}
	slices.SortFunc(ss, func(a, b roadSpan) int {
		if a.style != b.style {
			if a.style < b.style {
				return -1
			}
			return 1
		}
		return a.lo - b.lo
	})
	var out []roadSpan
	for _, s := range ss {
		if n := len(out); n > 0 && out[n-1].style == s.style && s.lo <= out[n-1].hi {
			out[n-1].hi = max(out[n-1].hi, s.hi)
			continue
		}
		out = append(out, s)
	}
	return out
}

// besideCost adds what two pieces of road d apart on parallel lines cost.
func besideCost(c *roadCost, a, b roadSpan, d, sign int) {
	o := min(a.hi, b.hi) - max(a.lo, b.lo)
	switch {
	case d == 0 && a.style != b.style && o > 0:
		c.clash += sign * o
	case d < 5 && a.style != b.style && o >= 24:
		// Rule 6: lines of two styles never run under 5px apart.
		c.near += sign * o
	case d > 0 && d <= besideGap && o > 0:
		c.beside += sign * o
	}
}

// trackView is the road on a set of lines A as some routes leave it: spans
// reads a line's road, A holds the lines it may read differently from the
// index, and fresh are those of A the index holds no run on.
type trackView struct {
	A     map[track]bool
	spans func(track) []roadSpan
	fresh []track
}

// besidesCost adds what pieces of road on A's lines cost beside any other.
func (x *roadIndex) besidesCost(c *roadCost, w trackView, sign int) {
	for a := range w.A {
		sa := w.spans(a)
		for i, p := range sa {
			for _, q := range sa[i+1:] {
				besideCost(c, p, q, 0, sign)
			}
		}
		if len(sa) == 0 {
			continue
		}
		beside := func(b track) {
			if w.A[b] && b.at() < a.at() {
				return
			}
			for _, p := range sa {
				for _, q := range w.spans(b) {
					besideCost(c, p, q, abs(b.at()-a.at()), sign)
				}
			}
		}
		ats := x.flats
		if a.vertical() {
			ats = x.ups
		}
		from, _ := slices.BinarySearch(ats, a.at()-besideGap)
		for _, at := range ats[from:] {
			if at > a.at()+besideGap {
				break
			}
			if at != a.at() {
				beside(onTrack(a.vertical(), at))
			}
		}
		for _, b := range w.fresh {
			if b.vertical() == a.vertical() && b != a && abs(b.at()-a.at()) <= besideGap {
				beside(b)
			}
		}
	}
}

// crossings counts the points where pieces of road cross.
func (x *roadIndex) crossings() int {
	at := map[pt]bool{}
	for k, u := range x.ups {
		for _, s := range x.upSpans[k] {
			from, _ := slices.BinarySearch(x.flats, s.lo+1)
			for m := from; m < len(x.flats) && x.flats[m] < s.hi; m++ {
				for _, t := range x.flatSpans[m] {
					if u > t.lo && u < t.hi {
						at[pt{u, x.flats[m]}] = true
					}
				}
			}
		}
	}
	return len(at)
}

// farAt counts the far splits at point p: where routes of one style that
// arrive as one strand leave it toward two places or more, in a gap next
// to none of their rows, since a road splits only near the ends it splits
// toward (J1). ps are the routes turning at p, and through those passing
// it straight.
func (v roadView) farAt(p pt, ps, through []pass) int {
	if len(ps)+len(through) < 2 {
		// A strand splits only where two passes leave it two ways.
		return 0
	}
	type dest struct {
		to string
		at int
	}
	type strand struct {
		style string
		in    pt
		// outs are the ways it leaves, a bit for each, and dests the
		// places it leaves for, with the row position of each.
		outs  uint16
		dests []dest
	}
	var buf [8]strand
	strands := buf[:0]
	for _, ps := range [][]pass{ps, through} {
		for _, q := range ps {
			s := slices.IndexFunc(strands, func(s strand) bool { return s.style == v.style[q.route] && s.in == q.in })
			if s < 0 {
				s = len(strands)
				strands = append(strands, strand{style: v.style[q.route], in: q.in})
			}
			k := &strands[s]
			k.outs |= 1 << ((q.out.x+1)*3 + q.out.y + 1)
			to := v.to[q.route]
			if d := slices.IndexFunc(k.dests, func(d dest) bool { return d.to == to }); d >= 0 {
				k.dests[d].at = v.dest[q.route]
			} else {
				k.dests = append(k.dests, dest{to, v.dest[q.route]})
			}
		}
	}
	n := 0
	for _, k := range strands {
		// Lines that arrive from two sides or more merge there, and a merge
		// may split at once anywhere, as J1 holds it.
		sides := 0
		for _, o := range strands {
			if o.style == k.style {
				sides++
			}
		}
		if bits.OnesCount16(k.outs) < 2 || len(k.dests) < 2 || sides > 1 {
			continue
		}
		far := 1 << 30
		for _, d := range k.dests {
			far = min(far, abs(v.position(p.y)-d.at))
		}
		if far > 1 {
			n++
		}
	}
	return n
}

func dir(a, b pt) pt { return pt{sign(b.x - a.x), sign(b.y - a.y)} }

// turnsAt calls f with each point route i turns at, or stops at short of
// its ends, and the way it passes there.
func turnsAt(i int, w []pt, f func(pt, pass)) {
	for k := 1; k < len(w)-1; k++ {
		f(w[k], pass{dir(w[k-1], w[k]), dir(w[k], w[k+1]), i, k})
	}
}

// throughAt is the runs on legs that pass p without turning there.
func throughAt(p pt, ways [][]pt, legs ...[]leg) []pass {
	var out []pass
	for _, ss := range legs {
		for _, s := range ss {
			a, b := ways[s.route][s.k], ways[s.route][s.k+1]
			if s.flat && p != a && p != b && on(p, a, b) {
				d := dir(a, b)
				out = append(out, pass{d, d, s.route, s.k})
			}
		}
	}
	slices.SortFunc(out, func(a, b pass) int {
		if a.route != b.route {
			return a.route - b.route
		}
		return a.k - b.k
	})
	return out
}

// forkCount counts the routes of one fork group that turn off their stem
// at another height than the group's first.
func forkCount(group []int, ways [][]pt) int {
	first, n := -1, 0
	for _, i := range group {
		if w := ways[i]; len(w) >= 3 && w[0].x == w[1].x {
			if first < 0 {
				first = i
			} else if ways[first][1].y != w[1].y {
				n++
			}
		}
	}
	return n
}

// teesIn are the tees among the routes from one place: where a route that
// drops straight crosses a flat run of another, a T a reader takes for a
// branch (lint's tees), once for each such run.
func teesIn(group []int, ways [][]pt) []pt {
	var out []pt
	for _, i := range group {
		// A route drops straight where it straightens to one upright run;
		// straight can leave a fold that a second pass removes. straight
		// keeps the ends, so ends at two x never drop straight.
		w := ways[i]
		if w[0].x != w[len(w)-1].x {
			continue
		}
		if w = straight(w); len(w) != 2 {
			continue
		}
		x, lo, hi := w[0].x, min(w[0].y, w[1].y), max(w[0].y, w[1].y)
		for _, j := range group {
			if j == i {
				continue
			}
			o := ways[j]
			for k := 1; k < len(o); k++ {
				if a, b := o[k-1], o[k]; a.y == b.y && a.y > lo && a.y < hi && x > min(a.x, b.x) && x < max(a.x, b.x) {
					out = append(out, pt{x, a.y})
				}
			}
		}
	}
	return out
}

// stubsOf are route i's stubs.
func (v roadView) stubsOf(ways [][]pt, i int) []stubAt {
	var out []stubAt
	w := ways[i]
	for _, k := range []int{0, len(w) - 2} {
		if k < 0 {
			continue
		}
		if s, ok := v.stubOf(ways, i, k); ok {
			out = append(out, stubAt{s, i, w[k].x})
		}
	}
	return out
}

// combCount counts the teeth of the comb that one place's stubs of one
// kind, dotting and way draw: the stubs that stand within combGap of the
// next (lint's combs).
func combCount(at []stubAt) int {
	xs := make([]int, len(at))
	for i, s := range at {
		xs[i] = s.x
	}
	slices.Sort(xs)
	xs = slices.Compact(xs)
	n := 0
	for i := 1; i < len(xs); i++ {
		if xs[i]-xs[i-1] <= combGap {
			n++
		}
	}
	return n
}

// unseenOf says dotted route i has no straight stretch of planMin that no
// solid route of its style runs on, where legsOn reads the runs on a track:
// dress draws a path dotted only where all its lines arise in design and
// it runs planMin or more, so such a line shows nowhere as what it is.
func (v roadView) unseenOf(i int, w []pt, legsOn func(track) []leg) bool {
	own := 0
	for k := 1; k < len(w); k++ {
		vert, at, lo, hi := runOf(w[k-1], w[k])
		var cover [][2]int
		for _, s := range legsOn(onTrack(vert, at)) {
			if v.plan[s.route] || v.style[s.route] != v.style[i] {
				continue
			}
			if l, h := max(lo, s.lo), min(hi, s.hi); l < h {
				cover = append(cover, [2]int{l, h})
			}
		}
		sort.Slice(cover, func(x, y int) bool { return cover[x][0] < cover[y][0] })
		at2 := lo
		for _, c := range cover {
			own = max(own, c[0]-at2)
			at2 = max(at2, c[1])
		}
		own = max(own, hi-at2)
	}
	return own < planMin
}

func newRoadIndex(v roadView, ways [][]pt) *roadIndex {
	x := &roadIndex{v: v, ways: ways, lines: map[track][]leg{}, spans: map[track][]roadSpan{},
		verts: map[pt][]pass{}, cols: map[int][]pt{}, rows: map[int][]pt{}, far: map[pt]int{},
		turns: make([]int, len(ways)), tees: map[pt]int{}, stubs: map[stub][]stubAt{}, combs: map[stub]int{}, unseen: make([]bool, len(ways))}
	c := &x.cost
	for i, w := range ways {
		legsOf(i, w, func(l track, s leg) { x.lines[l] = append(x.lines[l], s) })
		turnsAt(i, w, func(p pt, q pass) {
			if len(x.verts[p]) == 0 {
				x.cols[p.x] = append(x.cols[p.x], p)
				x.rows[p.y] = append(x.rows[p.y], p)
			}
			x.verts[p] = append(x.verts[p], q)
		})
		x.turns[i] = max(0, len(straight(w))-2)
		c.turns += x.turns[i]
		for _, s := range v.stubsOf(ways, i) {
			x.stubs[s.stub] = append(x.stubs[s.stub], s)
		}
	}
	all := trackView{A: map[track]bool{}, spans: func(l track) []roadSpan { return x.spans[l] }}
	for l, legs := range x.lines {
		x.spans[l] = spansOf(legs, v.style)
		all.A[l] = true
		if l.vertical() {
			x.ups = append(x.ups, l.at())
		} else {
			x.flats = append(x.flats, l.at())
		}
	}
	slices.Sort(x.ups)
	slices.Sort(x.flats)
	for _, at := range x.ups {
		x.upSpans = append(x.upSpans, x.spans[onTrack(true, at)])
	}
	for _, at := range x.flats {
		x.flatSpans = append(x.flatSpans, x.spans[onTrack(false, at)])
	}
	for _, ps := range x.cols {
		slices.SortFunc(ps, func(a, b pt) int { return a.y - b.y })
	}
	for _, ps := range x.rows {
		slices.SortFunc(ps, func(a, b pt) int { return a.x - b.x })
	}
	x.besidesCost(c, all, 1)
	c.crossings = x.crossings()
	for p, ps := range x.verts {
		x.far[p] = v.farAt(p, ps, throughAt(p, ways, x.lines[onTrack(true, p.x)], x.lines[onTrack(false, p.y)]))
		c.far += x.far[p]
	}
	for _, g := range v.forkGroups {
		n := forkCount(g, ways)
		x.forks = append(x.forks, n)
		c.forks += n
	}
	for _, g := range v.fromGroups {
		ps := teesIn(g, ways)
		x.teesOf = append(x.teesOf, ps)
		for _, p := range ps {
			x.tees[p]++
		}
	}
	c.tees = len(x.tees)
	for s, at := range x.stubs {
		x.combs[s] = combCount(at)
		c.combs += x.combs[s]
	}
	legsOn := func(l track) []leg { return x.lines[l] }
	for i, w := range ways {
		if v.dotted(i) && v.unseenOf(i, w, legsOn) {
			x.unseen[i] = true
			c.unseen++
		}
	}
	return x
}

// change is the index's routes with the touched ones ts as next has them.
type change struct {
	x       *roadIndex
	next    [][]pt
	touched []bool
	ts      []int
	// was and legs are the touched routes' runs by track, before and
	// after, A every track either lies on, and on and spans the runs and
	// road on those tracks after.
	was, legs map[track][]leg
	A         map[track]bool
	on        map[track][]leg
	spans     map[track][]roadSpan
	room      []pt
}

// change weighs one move at a time, so each reuses the last one's room.
func (x *roadIndex) change(next [][]pt, ts []int) *change {
	ch := x.scratch
	if ch == nil {
		ch = &change{x: x, touched: make([]bool, len(next)), was: map[track][]leg{}, legs: map[track][]leg{}, A: map[track]bool{}, on: map[track][]leg{}, spans: map[track][]roadSpan{}}
		x.scratch = ch
	}
	for _, i := range ch.ts {
		ch.touched[i] = false
	}
	for _, m := range []map[track][]leg{ch.was, ch.legs, ch.on} {
		clear(m)
	}
	clear(ch.A)
	clear(ch.spans)
	ch.next, ch.ts = next, ts
	for _, i := range ts {
		ch.touched[i] = true
		legsOf(i, x.ways[i], func(l track, s leg) { ch.was[l] = append(ch.was[l], s); ch.A[l] = true })
		legsOf(i, next[i], func(l track, s leg) { ch.legs[l] = append(ch.legs[l], s); ch.A[l] = true })
	}
	return ch
}

// legsOn is the runs on track l after the change.
func (ch *change) legsOn(l track) []leg {
	if !ch.A[l] {
		return ch.x.lines[l]
	}
	if out, ok := ch.on[l]; ok {
		return out
	}
	var out []leg
	for _, s := range ch.x.lines[l] {
		if !ch.touched[s.route] {
			out = append(out, s)
		}
	}
	out = append(out, ch.legs[l]...)
	ch.on[l] = out
	return out
}

func (ch *change) spansOn(l track) []roadSpan {
	if !ch.A[l] {
		return ch.x.spans[l]
	}
	s, ok := ch.spans[l]
	if !ok {
		s = spansOf(ch.legsOn(l), ch.x.v.style)
		ch.spans[l] = s
	}
	return s
}

// costWithin is the cost of the routes after the change. It stops, false,
// at the first count past limit's that roadCost.better and withers both
// forbid to grow, so a move that cannot be kept is not weighed whole.
func (ch *change) costWithin(limit roadCost) (roadCost, bool) {
	x, next, ts := ch.x, ch.next, ch.ts
	v, c := x.v, x.cost
	for _, i := range ts {
		ch.room = straightInto(ch.room, next[i])
		c.turns += max(0, len(ch.room)-2) - x.turns[i]
	}
	if c.turns > limit.turns {
		return c, false
	}
	forks, froms := map[int]bool{}, map[int]bool{}
	for _, i := range ts {
		forks[v.forkOf[i]], froms[v.fromOf[i]] = true, true
	}
	for g := range forks {
		c.forks += forkCount(v.forkGroups[g], next) - x.forks[g]
	}
	if c.forks > limit.forks {
		return c, false
	}
	tees := map[pt]int{}
	for g := range froms {
		for _, p := range x.teesOf[g] {
			tees[p]--
		}
		for _, p := range teesIn(v.fromGroups[g], next) {
			tees[p]++
		}
	}
	c.tees += pointsGained(x.tees, tees)
	if c.tees > limit.tees {
		return c, false
	}
	if c.combs += ch.combs(); c.combs > limit.combs {
		return c, false
	}
	if c.unseen += ch.unseen(); c.unseen > limit.unseen {
		return c, false
	}
	was := trackView{A: ch.A, spans: func(l track) []roadSpan { return x.spans[l] }}
	now := trackView{A: ch.A, spans: ch.spansOn}
	for l := range ch.A {
		if _, ok := x.lines[l]; !ok {
			now.fresh = append(now.fresh, l)
		}
	}
	if c.crossings += ch.crossed(now.fresh); c.crossings > limit.crossings {
		return c, false
	}
	x.besidesCost(&c, was, -1)
	x.besidesCost(&c, now, 1)
	if c.clash > limit.clash || c.near > limit.near {
		return c, false
	}
	c.far += ch.far()
	return c, c.far <= limit.far
}

// crossed is how many crossings the change adds. Where the routes' runs
// on a line change only between lo and hi, a piece of road there covers
// what it did outside lo..hi, so only the lines across it from lo to hi
// may cross it anew or no more. fresh are the lines the change lays road
// on that held none.
func (ch *change) crossed(fresh []track) int {
	x := ch.x
	type hull struct {
		t      track
		lo, hi int
	}
	var hulls []hull
	for _, legs := range []map[track][]leg{ch.was, ch.legs} {
		for l, ss := range legs {
			for _, s := range ss {
				k := slices.IndexFunc(hulls, func(h hull) bool { return h.t == l })
				if k < 0 {
					k = len(hulls)
					hulls = append(hulls, hull{l, s.lo, s.hi})
				}
				hulls[k].lo, hulls[k].hi = min(hulls[k].lo, s.lo), max(hulls[k].hi, s.hi)
			}
		}
	}
	covers := func(ss []roadSpan, at int) bool {
		for _, s := range ss {
			if s.lo < at && at < s.hi {
				return true
			}
		}
		return false
	}
	n := 0
	for _, h := range hulls {
		a := h.t
		was, now := x.spans[a], ch.spansOn(a)
		// across weighs the point where line b, whose road the index holds
		// as base, crosses a. A point both lines' hulls hold is weighed
		// from the upright one.
		across := func(b track, base []roadSpan) {
			bNow := base
			if k := slices.IndexFunc(hulls, func(h hull) bool { return h.t == b }); k >= 0 {
				if !a.vertical() && hulls[k].lo <= a.at() && a.at() <= hulls[k].hi {
					return
				}
				bNow = ch.spansOn(b)
			}
			if covers(now, b.at()) && covers(bNow, a.at()) {
				n++
			}
			if covers(was, b.at()) && covers(base, a.at()) {
				n--
			}
		}
		ats, spans := x.ups, x.upSpans
		if a.vertical() {
			ats, spans = x.flats, x.flatSpans
		}
		from, _ := slices.BinarySearch(ats, h.lo)
		for k := from; k < len(ats) && ats[k] <= h.hi; k++ {
			across(onTrack(!a.vertical(), ats[k]), spans[k])
		}
		for _, f := range fresh {
			if f.vertical() != a.vertical() && h.lo <= f.at() && f.at() <= h.hi {
				across(f, nil)
			}
		}
	}
	return n
}

// pointsGained is how many points the counts d add to or take from those of
// was that stand above none.
func pointsGained(was, d map[pt]int) int {
	n := 0
	for p, by := range d {
		switch k := was[p]; {
		case k == 0 && k+by > 0:
			n++
		case k > 0 && k+by == 0:
			n--
		}
	}
	return n
}

// far is how many far splits the change adds: at the points the touched
// routes turn at, before and after, and at the points other routes turn
// at on the runs the touched routes leave or take.
func (ch *change) far() int {
	x, v := ch.x, ch.x.v
	turns := map[pt][]pass{}
	ps := map[pt]bool{}
	for _, i := range ch.ts {
		turnsAt(i, x.ways[i], func(p pt, _ pass) { ps[p] = true })
		turnsAt(i, ch.next[i], func(p pt, q pass) { ps[p] = true; turns[p] = append(turns[p], q) })
	}
	for _, legs := range []map[track][]leg{ch.was, ch.legs} {
		for l, ss := range legs {
			on := x.rows[l.at()]
			if l.vertical() {
				on = x.cols[l.at()]
			}
			for _, s := range ss {
				along := func(p pt) int {
					if l.vertical() {
						return p.y
					}
					return p.x
				}
				from, _ := slices.BinarySearchFunc(on, s.lo+1, func(p pt, at int) int { return along(p) - at })
				for _, p := range on[from:] {
					if along(p) >= s.hi {
						break
					}
					ps[p] = true
				}
			}
		}
	}
	n := 0
	for p := range ps {
		var now []pass
		for _, q := range x.verts[p] {
			if !ch.touched[q.route] {
				now = append(now, q)
			}
		}
		now = append(now, turns[p]...)
		slices.SortFunc(now, func(a, b pass) int {
			if a.route != b.route {
				return a.route - b.route
			}
			return a.k - b.k
		})
		if len(now) > 0 {
			n += v.farAt(p, now, throughAt(p, ch.next, ch.legsOn(onTrack(true, p.x)), ch.legsOn(onTrack(false, p.y))))
		}
		n -= x.far[p]
	}
	return n
}

// combs is how many comb teeth the change adds.
func (ch *change) combs() int {
	x := ch.x
	after := map[stub][]stubAt{}
	for _, i := range ch.ts {
		for _, s := range x.v.stubsOf(x.ways, i) {
			if _, ok := after[s.stub]; !ok {
				after[s.stub] = nil
			}
		}
		for _, s := range x.v.stubsOf(ch.next, i) {
			after[s.stub] = append(after[s.stub], s)
		}
	}
	n := 0
	for s, added := range after {
		var at []stubAt
		for _, a := range x.stubs[s] {
			if !ch.touched[a.route] {
				at = append(at, a)
			}
		}
		n += combCount(append(at, added...)) - x.combs[s]
	}
	return n
}

// unseen is how many dotted routes the change hides or shows: the touched
// ones, and those on a track a touched solid route leaves or takes.
func (ch *change) unseen() int {
	x, v := ch.x, ch.x.v
	rs := map[int]bool{}
	for _, i := range ch.ts {
		if v.dotted(i) {
			rs[i] = true
			continue
		}
		for _, legs := range []map[track][]leg{ch.was, ch.legs} {
			for l, ss := range legs {
				if !slices.ContainsFunc(ss, func(s leg) bool { return s.route == i }) {
					continue
				}
				for _, s := range x.lines[l] {
					if v.dotted(s.route) && !ch.touched[s.route] {
						rs[s.route] = true
					}
				}
			}
		}
	}
	n := 0
	for i := range rs {
		now := v.unseenOf(i, ch.next[i], ch.legsOn)
		switch {
		case now && !x.unseen[i]:
			n++
		case !now && x.unseen[i]:
			n--
		}
	}
	return n
}

// endMixes counts the pairs of route runs side by side on one track, one
// of them a touched route's, that break the local road rule (mix), before
// the change and after. A move that adds one puts another kind, dotting or
// end shape on a route's own end, wherever on the track it lands, whatever
// run it set out to join.
func (ch *change) endMixes() (was, now int) {
	count := func(ways [][]pt, legs map[track][]leg, on func(track) []leg) int {
		n := 0
		for l := range legs {
			ss := on(l)
			for a, p := range ss {
				for _, q := range ss[a+1:] {
					if p.route == q.route || !ch.touched[p.route] && !ch.touched[q.route] || ch.x.v.style[p.route] != ch.x.v.style[q.route] {
						continue
					}
					if min(p.hi, q.hi) > max(p.lo, q.lo) && ch.x.v.mix(ways, [2]int{p.route, p.k}, [2]int{q.route, q.k}) {
						n++
					}
				}
			}
		}
		return n
	}
	return count(ch.x.ways, ch.was, func(l track) []leg { return ch.x.lines[l] }), count(ch.next, ch.legs, ch.legsOn)
}
