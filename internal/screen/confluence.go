package screen

import (
	"fmt"
	"slices"
	"sort"
)

// Lines are bus routes on a highway (rule 4). The ink is the road: lines of
// one style that run side by side are drawn as one road, which they join
// and leave where their ways part, with a dot at each split and merge
// (rule 17). Each line keeps its own route, its whole way from the shape
// it leaves to the shape it reaches, unseen at rest and lit whole when a
// reader points at it, so the road may join places no one line joins
// (rule 18) while hover and the model never do.
//
// confluent moves straight runs of the routes onto their neighbours: a run
// within roadRoom of a parallel run of its style, beside it for roadMeet or
// more, moves onto it; a fork's branch that turns off its stem at another
// height than its siblings moves onto theirs; and a run before a stair
// moves onto the run past it, as TALA's ShortcutEdgeRoutes takes out a
// bend the grid forced (re-implemented, no code copied). A route's first
// and last run move only along the edge span its line may meet. A move is
// kept only where no route then passes through a box, turns back, folds
// onto itself or gains a jog, no route's first or last run comes to lie on
// a line of another kind or dotting wherever it lands (a local road), and
// the view gains no crossing, turn, fork height or split far from the
// places it splits toward (J1), and loses one, or runs less ink beside
// other ink. Then the road is drawn from the routes.

const (
	roadRoom = 12
	roadMeet = 16
	// roadEnd is the least a route's first and last run keep, so an
	// arrowhead and an origin mark still have a straight stretch.
	roadEnd = 20
	// roadMoves bounds the moves one view takes, and roadClear is how far a
	// moved run keeps off a box.
	roadMoves = 400
	roadClear = 2
)

// straightRun is a straight piece of road: the collinear runs of routes of
// one style that touch or overlap. at is its x when it is vertical and its
// y when it is not, and lo, hi the span it covers.
type straightRun struct {
	vertical   bool
	at, lo, hi int
	style      string
	// segs are the route runs it holds, by route and run index.
	segs [][2]int
	// slide is how far it may move: a route's first or last run meets a
	// box's top or bottom, so it moves only along the span of the edge its
	// line may meet, and one that meets a box's side not at all.
	slide [2]int
}

// moves says the run may move to at.
func (r straightRun) moves(at int) bool { return r.slide[0] <= at && at <= r.slide[1] }

func runOf(a, b pt) (vertical bool, at, lo, hi int) {
	if a.x == b.x {
		return true, a.x, min(a.y, b.y), max(a.y, b.y)
	}
	return false, a.y, min(a.x, b.x), max(a.x, b.x)
}

// roadRuns groups the routes' runs into straight pieces of road.
func roadRuns(ways [][]pt, rv roadView) []straightRun {
	style := rv.style
	type key struct {
		style    string
		vertical bool
		at       int
	}
	type seg struct {
		lo, hi int
		at     [2]int
		slide  [2]int
	}
	by := map[key][]seg{}
	var keys []key
	for i, w := range ways {
		for j := 1; j < len(w); j++ {
			if w[j-1] == w[j] {
				continue
			}
			v, at, lo, hi := runOf(w[j-1], w[j])
			k := key{style[i], v, at}
			if _, ok := by[k]; !ok {
				keys = append(keys, k)
			}
			slide := [2]int{-1 << 30, 1 << 30}
			for side, meets := range []bool{j == 1, j == len(w)-1} {
				switch e := rv.ends[i][side]; {
				case !meets:
				case !v:
					slide = [2]int{1, 0}
				default:
					slide = [2]int{max(slide[0], e.lo), min(slide[1], e.hi)}
				}
			}
			by[k] = append(by[k], seg{lo, hi, [2]int{i, j - 1}, slide})
		}
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].style != keys[b].style {
			return keys[a].style < keys[b].style
		}
		if keys[a].vertical != keys[b].vertical {
			return keys[a].vertical
		}
		return keys[a].at < keys[b].at
	})
	var out []straightRun
	for _, k := range keys {
		ss := by[k]
		sort.SliceStable(ss, func(a, b int) bool { return ss[a].lo < ss[b].lo })
		for _, s := range ss {
			if n := len(out); n > 0 && out[n-1].style == k.style && out[n-1].vertical == k.vertical && out[n-1].at == k.at && s.lo <= out[n-1].hi {
				r := &out[n-1]
				r.hi = max(r.hi, s.hi)
				r.segs = append(r.segs, s.at)
				r.slide = [2]int{max(r.slide[0], s.slide[0]), min(r.slide[1], s.slide[1])}
				continue
			}
			out = append(out, straightRun{k.vertical, k.at, s.lo, s.hi, k.style, [][2]int{s.at}, s.slide})
		}
	}
	return out
}

// moved is the routes with run r moved to at.
func moved(ways [][]pt, r straightRun, at int) [][]pt {
	out := slices.Clone(ways)
	touched := map[int]bool{}
	for _, s := range r.segs {
		i, j := s[0], s[1]
		if !touched[i] {
			out[i] = slices.Clone(ways[i])
			touched[i] = true
		}
		for _, k := range []int{j, j + 1} {
			if r.vertical {
				out[i][k].x = at
			} else {
				out[i][k].y = at
			}
		}
	}
	return out
}

// folds says the route runs back over itself somewhere.
func folds(w []pt) bool {
	for k := 2; k < len(w); k++ {
		a, b, c := w[k-2], w[k-1], w[k]
		if a == b || b == c {
			continue
		}
		if (a.x == b.x && b.x == c.x && (b.y-a.y)*(c.y-b.y) < 0) || (a.y == b.y && b.y == c.y && (b.x-a.x)*(c.x-b.x) < 0) {
			return true
		}
	}
	return false
}

// backTurns counts the runs that head against the run two before them.
func backTurns(w []pt) int {
	n := 0
	for k := 3; k < len(w); k++ {
		d1 := pt{sign(w[k-2].x - w[k-3].x), sign(w[k-2].y - w[k-3].y)}
		d2 := pt{sign(w[k].x - w[k-1].x), sign(w[k].y - w[k-1].y)}
		if d1.x == -d2.x && d1.y == -d2.y && d1 != (pt{}) {
			n++
		}
	}
	return n
}

func jogsOf(w []pt) int {
	n := 0
	for k := 2; k < len(w)-1; k++ {
		if abs(w[k].x-w[k-1].x)+abs(w[k].y-w[k-1].y) < jogLen {
			n++
		}
	}
	return n
}

// crosses says the run a-c passes through the inside of box b, its two
// ends excepted, where it meets the box it leaves or reaches.
func crosses(a, c pt, b box) bool {
	lo, hi := pt{min(a.x, c.x), min(a.y, c.y)}, pt{max(a.x, c.x), max(a.y, c.y)}
	return lo.x < b.x+b.w-1 && hi.x > b.x+1 && lo.y < b.y+b.h-1 && hi.y > b.y+1
}

// keepsShape says route w, moved from was, still reads as a line: its ends
// stay on their edges with room for a head, it passes and runs along no
// box, never folds and gains no turn-back or jog.
func keepsShape(was, w []pt, boxes []box) bool {
	if folds(w) {
		return false
	}
	w = straight(w)
	if len(w) < 2 || w[0].y != was[0].y || w[len(w)-1].y != was[len(was)-1].y {
		return false
	}
	end := func(a, b pt) int { return abs(a.x-b.x) + abs(a.y-b.y) }
	if end(w[0], w[1]) < min(roadEnd, end(was[0], was[1])) || end(w[len(w)-1], w[len(w)-2]) < min(roadEnd, end(was[len(was)-1], was[len(was)-2])) {
		return false
	}
	if backTurns(w) > backTurns(was) || jogsOf(w) > jogsOf(was) {
		return false
	}
	return boxed(w, boxes) <= boxed(was, boxes)
}

// boxed counts the runs of route w that pass through a box or within
// roadClear of one, so they read as running along it, but where its first
// or last run meets the shape at its end.
func boxed(w []pt, boxes []box) int {
	toward := func(p, q pt, d int) pt { return pt{p.x + d*sign(q.x-p.x), p.y + d*sign(q.y-p.y)} }
	n := 0
	for k := 1; k < len(w); k++ {
		a, c := w[k-1], w[k]
		if k == 1 {
			a = toward(a, c, roadClear+1)
		}
		if k == len(w)-1 {
			c = toward(c, a, roadClear+1)
		}
		for _, b := range boxes {
			if crosses(a, c, box{b.x - roadClear, b.y - roadClear, b.w + 2*roadClear, b.h + 2*roadClear}) {
				n++
			}
		}
	}
	return n
}

// roadCost is what a reader of the drawn road meets: where pieces of road
// cross, how often the routes turn, how far road runs beside road, how far
// pieces of different styles run on one another, the splits far from
// their ends, and the forks whose branches leave at different heights.
type roadCost struct {
	crossings, turns, beside, clash, near, far, forks, tees, unseen int
}

// roadView is what confluent weighs routes against: each route's style and
// ends, the places it leaves and reaches, the row position (look.position)
// of the place it reaches, and the position of a height on the page.
type roadView struct {
	style []string
	// plan says each route's relations all arise only in design, so it is
	// drawn dotted where it rides alone, and kinds is each route's kind.
	plan  []bool
	kinds []string
	// ends are each route's two ends, whose spans its first and last runs
	// may slide along.
	ends     [][2]end
	from, to []string
	dest     []int
	position func(y int) int
}

func (v roadView) cost(ways [][]pt) roadCost {
	c := roadCost{far: v.farSplits(ways), forks: len(v.forks(ways)), tees: v.tees(ways), unseen: v.unseen(ways)}
	for _, w := range ways {
		c.turns += max(0, len(straight(w))-2)
	}
	rs := roadRuns(ways, v)
	seen := map[pt]bool{}
	for i, a := range rs {
		for _, b := range rs[i+1:] {
			if a.vertical == b.vertical {
				o := min(a.hi, b.hi) - max(a.lo, b.lo)
				switch d := abs(a.at - b.at); {
				case d == 0 && a.style != b.style && o > 0:
					c.clash += o
				case d < 5 && a.style != b.style && o >= 24:
					// Rule 6: lines of two styles never run under 5px apart.
					c.near += o
				case d > 0 && d <= besideGap && o > 0:
					c.beside += o
				}
				continue
			}
			v, h := a, b
			if !v.vertical {
				v, h = b, a
			}
			if v.at > h.lo && v.at < h.hi && h.at > v.lo && h.at < v.hi {
				seen[pt{v.at, h.at}] = true
			}
		}
	}
	c.crossings = len(seen)
	return c
}

func (c roadCost) better(o roadCost) bool {
	if c.crossings > o.crossings || c.turns > o.turns || c.clash > o.clash || c.near > o.near || c.far > o.far || c.forks > o.forks || c.tees > o.tees || c.unseen > o.unseen {
		return false
	}
	return c.crossings < o.crossings || c.turns < o.turns || c.forks < o.forks || c.beside < o.beside
}

// forks are pairs of routes that leave one place for boxes in one row and
// turn off their stem onto flat runs at different heights, as forkLevels
// reads them, by route.
func (v roadView) forks(ways [][]pt) [][2]int {
	type fork struct {
		style, from string
		dest        int
	}
	first := map[fork]int{}
	var out [][2]int
	for i, w := range ways {
		if len(w) < 3 || w[0].x != w[1].x {
			continue
		}
		f := fork{v.style[i], v.from[i], v.dest[i]}
		j, ok := first[f]
		switch {
		case !ok:
			first[f] = i
		case ways[j][1].y != w[1].y:
			out = append(out, [2]int{j, i})
		}
	}
	return out
}

// unseen counts the dotted routes with no straight stretch of planMin that
// no solid route of their style runs on: dress draws a path dotted only
// where all its lines arise in design and it runs planMin or more, so such
// a line shows nowhere as what it is.
func (v roadView) unseen(ways [][]pt) int {
	n := 0
	for i, w := range ways {
		if !v.plan[i] {
			continue
		}
		own := 0
		for k := 1; k < len(w); k++ {
			a, b := w[k-1], w[k]
			vert, at, lo, hi := runOf(a, b)
			var cover [][2]int
			for j, o := range ways {
				if v.plan[j] || v.style[j] != v.style[i] {
					continue
				}
				for m := 1; m < len(o); m++ {
					if ov, oat, olo, ohi := runOf(o[m-1], o[m]); ov == vert && oat == at && o[m-1] != o[m] {
						if l, h := max(lo, olo), min(hi, ohi); l < h {
							cover = append(cover, [2]int{l, h})
						}
					}
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
		if own < planMin {
			n++
		}
	}
	return n
}

// joins says runs a and b may ride one road. In the middle any lines
// may, a highway whatever their kind or dotting; a run that holds a line's
// first or last run is a local road, which only lines of one kind and
// dotting whose ends there are the same sort of shape, a box or a chip,
// ride, so each line keeps its own look where it leaves and arrives.
func (v roadView) joins(ways [][]pt, a, b straightRun) bool {
	local, kind, asset := false, "", ""
	for _, r := range []straightRun{a, b} {
		for _, s := range r.segs {
			i, j := s[0], s[1]
			for side, end := range []bool{j == 0, j == len(ways[i])-2} {
				if !end {
					continue
				}
				local = true
				if k := fmt.Sprint(v.ends[i][side].node != ""); asset == "" {
					asset = k
				} else if asset != k {
					return false
				}
			}
		}
	}
	if !local {
		return true
	}
	for _, r := range []straightRun{a, b} {
		for _, s := range r.segs {
			if k := fmt.Sprint(v.kinds[s[0]], v.plan[s[0]]); kind == "" {
				kind = k
			} else if kind != k {
				return false
			}
		}
	}
	return true
}

// endMixes counts the pairs of route runs side by side on one track, one
// of them on a touched route, that break the local road rule (mix). A move
// that adds one puts another kind, dotting or end shape on a route's own end,
// wherever on the track it lands, whatever run it set out to join.
func (v roadView) endMixes(ways [][]pt, touched map[int]bool) int {
	n := 0
	for _, r := range roadRuns(ways, v) {
		for x, a := range r.segs {
			for _, b := range r.segs[x+1:] {
				if a[0] == b[0] || !touched[a[0]] && !touched[b[0]] {
					continue
				}
				_, _, alo, ahi := runOf(ways[a[0]][a[1]], ways[a[0]][a[1]+1])
				_, _, blo, bhi := runOf(ways[b[0]][b[1]], ways[b[0]][b[1]+1])
				if min(ahi, bhi) > max(alo, blo) && v.mix(ways, a, b) {
					n++
				}
			}
		}
	}
	return n
}

// mix says route runs a and b, side by side on one track, break the local
// road rule (joins): one of them is its route's first or last run, and the
// two differ in kind or dotting, or both are ends and meet no shape of one
// sort. A route of one run has both its ends there, so either sort will do.
func (v roadView) mix(ways [][]pt, a, b [2]int) bool {
	assets := func(s [2]int) map[bool]bool {
		out := map[bool]bool{}
		for side, end := range []bool{s[1] == 0, s[1] == len(ways[s[0]])-2} {
			if end {
				out[v.ends[s[0]][side].node != ""] = true
			}
		}
		return out
	}
	x, y := assets(a), assets(b)
	switch {
	case len(x) == 0 && len(y) == 0:
		return false
	case v.kinds[a[0]] != v.kinds[b[0]] || v.plan[a[0]] != v.plan[b[0]]:
		return true
	case len(x) == 0 || len(y) == 0:
		return false
	}
	for k := range x {
		if y[k] {
			return false
		}
	}
	return true
}

// tees counts the points where a route that drops straight crosses a flat
// run of another route from its place, of any kind: a T a reader takes for
// a branch (lint's tees).
func (v roadView) tees(ways [][]pt) int {
	seen := map[pt]bool{}
	for i, w := range ways {
		if w = straight(w); len(w) != 2 || w[0].x != w[1].x {
			continue
		}
		x, lo, hi := w[0].x, min(w[0].y, w[1].y), max(w[0].y, w[1].y)
		for j, o := range ways {
			if j == i || v.from[j] != v.from[i] {
				continue
			}
			for k := 1; k < len(o); k++ {
				if a, b := o[k-1], o[k]; a.y == b.y && a.y > lo && a.y < hi && x > min(a.x, b.x) && x < max(a.x, b.x) {
					seen[pt{x, a.y}] = true
				}
			}
		}
	}
	return len(seen)
}

// farSplits counts the points where routes of one style that arrive as one
// strand leave it toward two places or more, in a gap next to none of
// their rows: a road splits only near the ends it splits toward (J1).
func (v roadView) farSplits(ways [][]pt) int {
	type pass struct {
		in, out pt
		route   int
	}
	dir := func(a, b pt) pt { return pt{sign(b.x - a.x), sign(b.y - a.y)} }
	at := map[pt][]pass{}
	for i, w := range ways {
		for k := 1; k < len(w)-1; k++ {
			at[w[k]] = append(at[w[k]], pass{dir(w[k-1], w[k]), dir(w[k], w[k+1]), i})
		}
	}
	n := 0
	for p, ps := range at {
		for i, w := range ways {
			for k := 1; k < len(w); k++ {
				if a, b := w[k-1], w[k]; p != a && p != b && on(p, a, b) {
					d := dir(a, b)
					ps = append(ps, pass{d, d, i})
				}
			}
		}
		type strand struct {
			style string
			in    pt
		}
		outs, dests := map[strand]map[pt]bool{}, map[strand]map[string]int{}
		for _, q := range ps {
			k := strand{v.style[q.route], q.in}
			if outs[k] == nil {
				outs[k], dests[k] = map[pt]bool{}, map[string]int{}
			}
			outs[k][q.out], dests[k][v.to[q.route]] = true, v.dest[q.route]
		}
		for k, o := range outs {
			if len(o) < 2 || len(dests[k]) < 2 {
				continue
			}
			far := 1 << 30
			for _, d := range dests[k] {
				far = min(far, abs(v.position(p.y)-d))
			}
			if far > 1 {
				n++
			}
		}
	}
	return n
}

// confluent moves the routes onto shared roads and draws the road.
func (r *routing) confluent(reqs []wireReq, position func(y int) int, rows int) {
	ink, plan, kinds := make([]string, len(reqs)), make([]bool, len(reqs)), make([]string, len(reqs))
	dest, to, from := make([]int, len(reqs)), make([]string, len(reqs)), make([]string, len(reqs))
	for i, q := range reqs {
		to[i], from[i] = place(q.to), place(q.from)
		if q.from.node != "" {
			from[i] = "node|" + q.from.node
		}
		ink[i], plan[i], kinds[i] = fmt.Sprint(r.back[i]), q.plan, q.kind
		dest[i] = min(max(2*q.to.row+1, -1), 2*rows+1)
	}
	ends := make([][2]end, len(reqs))
	for i, q := range reqs {
		ends[i] = [2]end{q.from, q.to}
	}
	v := roadView{ink, plan, kinds, ends, from, to, dest, position}
	ways := make([][]pt, len(r.ways))
	for i, w := range r.ways {
		ways[i] = straight(w)
	}
	cost := v.cost(ways)
	for n := 0; n < roadMoves; n++ {
		next, ok := r.step(ways, v, cost)
		if !ok {
			break
		}
		ways = next
		for i := range ways {
			ways[i] = straight(ways[i])
		}
		cost = v.cost(ways)
	}
	r.ways = ways
	r.drawn = roads(ways, ink, reqs, r.back)
}

// step finds the first move that reads better than cost, in order: runs
// onto a neighbour beside them, longest shared stretch first, then forks
// set on one track and stairs straightened.
func (r *routing) step(ways [][]pt, v roadView, cost roadCost) ([][]pt, bool) {
	rs := roadRuns(ways, v)
	type move struct {
		run, at, gain int
	}
	var moves []move
	for i, a := range rs {
		for k, b := range rs {
			if k == i || !a.moves(b.at) || a.vertical != b.vertical || a.style != b.style || !v.joins(ways, a, b) {
				continue
			}
			if d := abs(a.at - b.at); d == 0 || d > roadRoom {
				continue
			}
			if o := min(a.hi, b.hi) - max(a.lo, b.lo); o >= roadMeet {
				moves = append(moves, move{i, b.at, o})
			}
		}
	}
	// at is the piece of road each route run lies on.
	at := map[[2]int]int{}
	for k, run := range rs {
		for _, s := range run.segs {
			at[s] = k
		}
	}
	// A fork: lines of one place into one row that turn off their stem at
	// different heights. Moving one's first turn onto another's puts the
	// branches on one track.
	for _, f := range v.forks(ways) {
		a, b := at[[2]int{f[0], 1}], at[[2]int{f[1], 1}]
		if !v.joins(ways, rs[a], rs[b]) {
			continue
		}
		if rs[a].moves(rs[b].at) {
			moves = append(moves, move{a, rs[b].at, 0})
		}
		if rs[b].moves(rs[a].at) {
			moves = append(moves, move{b, rs[a].at, 0})
		}
	}
	// A stair: a run, a short step across, and a run on past it the same
	// way. Moving either run onto the other takes out the step.
	for i, w := range ways {
		for j := 1; j+1 < len(w)-1; j++ {
			a, b := at[[2]int{i, j - 1}], at[[2]int{i, j + 1}]
			if rs[a].vertical != rs[b].vertical {
				continue
			}
			if rs[a].moves(rs[b].at) {
				moves = append(moves, move{a, rs[b].at, 0})
			}
			if rs[b].moves(rs[a].at) {
				moves = append(moves, move{b, rs[a].at, 0})
			}
		}
	}
	sort.SliceStable(moves, func(i, j int) bool { return moves[i].gain > moves[j].gain })
	for _, m := range moves {
		next := moved(ways, rs[m.run], m.at)
		ok := true
		for _, s := range rs[m.run].segs {
			if !keepsShape(ways[s[0]], next[s[0]], r.boxes) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		touched := map[int]bool{}
		for _, s := range rs[m.run].segs {
			next[s[0]] = straight(next[s[0]])
			touched[s[0]] = true
		}
		if v.endMixes(next, touched) <= v.endMixes(ways, touched) && v.cost(next).better(cost) {
			return next, true
		}
	}
	return nil, false
}

// roads draws the road the routes ride: one path for each stretch that the
// same routes ride, from a shape, a split or a merge to the next.
func roads(ways [][]pt, style []string, reqs []wireReq, back []bool) []drawn {
	type edge struct{ a, b pt }
	type node struct {
		style string
		p     pt
	}
	riders := map[string]map[edge][]int{}
	var styles []string
	cuts := map[string][]pt{}
	for i, w := range ways {
		if _, ok := riders[style[i]]; !ok {
			riders[style[i]] = map[edge][]int{}
			styles = append(styles, style[i])
		}
		cuts[style[i]] = append(cuts[style[i]], w...)
	}
	sort.Strings(styles)
	norm := func(a, b pt) edge {
		if b.x < a.x || b.x == a.x && b.y < a.y {
			a, b = b, a
		}
		return edge{a, b}
	}
	for i, w := range ways {
		for j := 1; j < len(w); j++ {
			a, b := w[j-1], w[j]
			var cut []pt
			for _, p := range cuts[style[i]] {
				if p != a && p != b && on(p, a, b) {
					cut = append(cut, p)
				}
			}
			sort.Slice(cut, func(x, y int) bool {
				return abs(cut[x].x-a.x)+abs(cut[x].y-a.y) < abs(cut[y].x-a.x)+abs(cut[y].y-a.y)
			})
			ps := append(append([]pt{a}, slices.Compact(cut)...), b)
			for k := 1; k < len(ps); k++ {
				e := norm(ps[k-1], ps[k])
				if !slices.Contains(riders[style[i]][e], i) {
					riders[style[i]][e] = append(riders[style[i]][e], i)
				}
			}
		}
	}
	ends := map[node]bool{}
	for i, w := range ways {
		ends[node{style[i], w[0]}], ends[node{style[i], w[len(w)-1]}] = true, true
	}
	var out []drawn
	for _, s := range styles {
		es := riders[s]
		at := map[pt][]edge{}
		var keys []edge
		for e, is := range es {
			slices.Sort(is)
			at[e.a], at[e.b] = append(at[e.a], e), append(at[e.b], e)
			keys = append(keys, e)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := keys[i], keys[j]
			if a.a != b.a {
				return a.a.y < b.a.y || a.a.y == b.a.y && a.a.x < b.a.x
			}
			return a.b.y < b.b.y || a.b.y == b.b.y && a.b.x < b.b.x
		})
		// A path runs on through a point that only it passes, by the same
		// routes, and that ends no route.
		onward := func(p pt, e edge) (edge, bool) {
			if ends[node{s, p}] || len(at[p]) != 2 {
				return edge{}, false
			}
			o := at[p][0]
			if o == e {
				o = at[p][1]
			}
			return o, slices.Equal(es[o], es[e])
		}
		done := map[edge]bool{}
		for _, e := range keys {
			if done[e] {
				continue
			}
			done[e] = true
			pts := []pt{e.a, e.b}
			for cur, p := e, e.b; ; {
				o, ok := onward(p, cur)
				if !ok || done[o] {
					break
				}
				done[o] = true
				p = map[bool]pt{true: o.b, false: o.a}[o.a == p]
				pts, cur = append(pts, p), o
			}
			for cur, p := e, e.a; ; {
				o, ok := onward(p, cur)
				if !ok || done[o] {
					break
				}
				done[o] = true
				p = map[bool]pt{true: o.b, false: o.a}[o.a == p]
				pts, cur = append([]pt{p}, pts...), o
			}
			out = append(out, road(pts, es[e], ways, reqs, back))
		}
	}
	// A bus is the paths of the lines that leave one place, or that reach
	// one, as rule 4 bundles them: one network to a reader, whose pieces
	// neither cross nor stand beside one another as two lines do. A path
	// that leaves a box and reaches one joins both buses into one.
	root := map[string]string{}
	find := func(k string) string {
		for root[k] != "" && root[k] != k {
			k = root[k]
		}
		return k
	}
	keys := make([][]string, len(out))
	for k, d := range out {
		var at [2]string
		for side := range 2 {
			placeOf := func(i int) string {
				e := [2]end{reqs[i].from, reqs[i].to}[side]
				if e.node != "" {
					return "node|" + e.node
				}
				return place(e)
			}
			p := placeOf(d.reqs[0])
			if !slices.ContainsFunc(d.reqs, func(i int) bool { return placeOf(i) != p }) {
				at[side] = fmt.Sprint(side, p, d.kind, d.back)
			}
		}
		leaves, arrives := at[0] != "" && d.starts[0] != noStart, at[1] != "" && !d.joins
		switch {
		case leaves && arrives:
			keys[k] = []string{at[0], at[1]}
		case leaves || at[0] != "" && !arrives:
			keys[k] = []string{at[0]}
		case at[1] != "":
			keys[k] = []string{at[1]}
		}
		for _, key := range keys[k][min(1, len(keys[k])):] {
			if a, b := find(keys[k][0]), find(key); a != b {
				root[b] = a
			}
		}
	}
	nets := map[string]int{}
	for k := range out {
		if len(keys[k]) == 0 {
			continue
		}
		r := find(keys[k][0])
		if _, ok := nets[r]; !ok {
			nets[r] = len(nets) + 1
		}
		out[k].net = nets[r]
	}
	return out
}

// road is one path of the road, ridden by the routes is, turned the way
// they run, with a count where it meets a shape and carries more than one.
func road(pts []pt, is []int, ways [][]pt, reqs []wireReq, back []bool) drawn {
	first := ways[is[0]]
	pos := func(p pt) int {
		for k := 1; k < len(first); k++ {
			if on(p, first[k-1], first[k]) {
				return k*100000 + abs(p.x-first[k-1].x) + abs(p.y-first[k-1].y)
			}
		}
		return -1
	}
	if pos(pts[0]) > pos(pts[len(pts)-1]) {
		slices.Reverse(pts)
	}
	for _, i := range is {
		if ways[i][0] == pts[len(pts)-1] {
			slices.Reverse(pts)
			break
		}
	}
	// A piece several kinds ride wears the kind most of them have, the
	// first by name among equals: only its ends, local roads of one kind,
	// carry heads.
	n := map[string]int{}
	for _, i := range is {
		n[reqs[i].kind]++
	}
	kind := reqs[is[0]].kind
	for k, c := range n {
		if c > n[kind] || c == n[kind] && k < kind {
			kind = k
		}
	}
	d := drawn{D: polyline(pts), parts: [][]pt{pts}, reqs: is, kind: kind, back: back[is[0]], road: true}
	last := pts[len(pts)-1]
	arrives := false
	for _, i := range is {
		st := noStart
		if ways[i][0] == pts[0] {
			st = pts[0]
		}
		d.starts = append(d.starts, st)
		arrives = arrives || ways[i][len(ways[i])-1] == last
	}
	d.joins = !arrives
	if len(is) > 1 {
		switch {
		case arrives:
			d.count, d.own = len(is), [2]pt{pts[len(pts)-2], last}
		case d.starts[0] != noStart:
			d.count, d.own = len(is), [2]pt{pts[0], pts[1]}
		}
		if d.count > 0 {
			d.counted = is
			d.label = pt{d.own[0].x + 5, (d.own[0].y+d.own[1].y)/2 + 4}
		}
	}
	return d
}
