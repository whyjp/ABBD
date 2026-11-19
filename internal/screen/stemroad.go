package screen

import "sort"

// The passes before move whole pieces of road, and a piece holds the runs
// that go either way along its track. Where lines leave one place by one
// stem toward boxes of any row, or come into one place from one row, the
// branches that go the same way across may turn off or onto the stem at
// different heights on tracks that also carry lines the other way: moving
// the whole track would set opposite lines on one road, so those passes
// leave them.
// gatherStems moves only the runs going one way, onto a sibling's height, and
// keeps a move only where it costs nothing else.

// stemKey is the group of routes whose branches gather: those that leave
// one place by its stem the same way across, into any row, or that reach
// one place from one row the same way in.
type stemKey struct {
	style, place string
	last         bool
	row, side    int
}

// stemTurn is route i's group and the height at which it turns off or
// onto its stem, the stem being its first run, or its last when last is
// set, where that run stands upright.
func (v roadView) stemTurn(ways [][]pt, i int, last bool) (stemKey, int, bool) {
	w := ways[i]
	y, side, ok := turnOff(w, last)
	if !ok || !goesOn(w, last) {
		return stemKey{}, 0, false
	}
	if last {
		return stemKey{v.style[i], v.to[i], true, v.src[i], side}, y, true
	}
	return stemKey{v.style[i], v.from[i], false, 0, sign(w[2].x - w[1].x)}, y, true
}

// goesOn says route w, past the flat run off its stem, goes on the way
// the stem goes, or, when last is set, came that way before the flat run
// onto it. A branch that turns back past its flat run has its box behind
// the stem's turn: gathered onto a sibling's track further out, it would
// run out and back again.
func goesOn(w []pt, last bool) bool {
	n := len(w)
	if n < 4 {
		return true
	}
	if last {
		return sign(w[n-3].y-w[n-4].y) == sign(w[n-1].y-w[n-2].y)
	}
	return sign(w[3].y-w[2].y) == sign(w[1].y-w[0].y)
}

// ownLegShrinks says that route w, turning off or onto its stem at height
// from, runs less of its way alone when it turns at to instead: its leg
// between the turn and its own box, the last run past the turn, or, when
// last is set, the run before it. A branch gathered onto a sibling's turn
// further from its own box would run its own leg longer, down through rows
// of boxes it passed on the stem before.
func ownLegShrinks(w []pt, last bool, from, to int) bool {
	n := len(w)
	if n < 4 {
		return false
	}
	if last {
		return sign(to-from) == sign(w[n-4].y-w[n-3].y)
	}
	return sign(to-from) == sign(w[3].y-w[2].y)
}

// spread counts, over every stem group, the heights past the first at
// which its routes turn off or onto the stem.
func (v roadView) spread(ways [][]pt) int {
	ys := map[stemKey]map[int]bool{}
	for _, last := range []bool{false, true} {
		for i := range ways {
			k, y, ok := v.stemTurn(ways, i, last)
			if !ok {
				continue
			}
			if ys[k] == nil {
				ys[k] = map[int]bool{}
			}
			ys[k][y] = true
		}
	}
	n := 0
	for _, s := range ys {
		n += len(s) - 1
	}
	return n
}

// opposed is how far runs share each track going opposite ways.
func opposed(ways [][]pt) map[track]int {
	type run struct{ lo, hi, way int }
	by := map[track][]run{}
	for _, w := range ways {
		for k := 0; k+1 < len(w); k++ {
			if w[k] == w[k+1] {
				continue
			}
			v, at, lo, hi := runOf(w[k], w[k+1])
			t := onTrack(v, at)
			by[t] = append(by[t], run{lo, hi, heading(w[k], w[k+1])})
		}
	}
	out := map[track]int{}
	for t, rs := range by {
		sort.Slice(rs, func(a, b int) bool { return rs[a].lo < rs[b].lo })
		for a, p := range rs {
			for _, q := range rs[a+1:] {
				if q.lo >= p.hi {
					break
				}
				if p.way != q.way {
					out[t] += min(p.hi, q.hi) - q.lo
				}
			}
		}
	}
	return out
}

// opposesMore says some track carries more of opposite lines side by side
// in now than in was: lines that leave one opposite partner must not find
// another.
func opposesMore(was, now map[track]int) bool {
	for t, n := range now {
		if n > was[t] {
			return true
		}
	}
	return false
}

// gatherStems sets one way's branches of a stem group on one track, by a move
// of the runs going that way alone that sets a branch on a sibling's turn
// nearer its own box (ownLegShrinks), lowers spread and costs nothing
// else: no roadCost count, no fork level as the lint reads it and no
// track's stretch of opposite lines side by side grows.
func (r *routing) gatherStems(ways [][]pt, v roadView, cost roadCost) ([][]pt, bool) {
	rs := roadRunsBy(ways, v, true)
	at := runsAt(rs)
	type turn struct{ route, y int }
	groups := map[stemKey][]turn{}
	var keys []stemKey
	for _, last := range []bool{false, true} {
		for i := range ways {
			k, y, ok := v.stemTurn(ways, i, last)
			if !ok {
				continue
			}
			if groups[k] == nil {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], turn{i, y})
		}
	}
	seen := map[[2]int]bool{}
	var moves []roadMove
	for _, k := range keys {
		g := groups[k]
		flat := func(i int) int {
			if k.last {
				return at[[2]int{i, len(ways[i]) - 3}]
			}
			return at[[2]int{i, 1}]
		}
		for x, p := range g {
			for _, q := range g[x+1:] {
				if p.y == q.y {
					continue
				}
				a, b := flat(p.route), flat(q.route)
				if !v.joins(ways, rs[a], rs[b]) {
					continue
				}
				for _, o := range []struct {
					m    [2]int
					by   turn
					onto int
				}{{[2]int{a, b}, p, q.y}, {[2]int{b, a}, q, p.y}} {
					m := o.m
					if !seen[m] && rs[m[0]].moves(rs[m[1]].at) && ownLegShrinks(ways[o.by.route], k.last, o.by.y, o.onto) {
						seen[m] = true
						moves = append(moves, roadMove{rs[m[0]], rs[m[1]].at, 0})
					}
				}
			}
		}
	}
	if len(moves) == 0 {
		return nil, false
	}
	spread, levels, opp := v.spread(ways), forkLevelsOf(ways, v), opposed(ways)
	return r.firstKept(ways, v, cost, moves, false, func(next [][]pt, nc roadCost) bool {
		return v.spread(next) < spread && cost.withers(nc) && nc.meets <= cost.meets && !opposesMore(opp, opposed(next)) &&
			forkLevelsOf(next, v) <= levels
	})
}
