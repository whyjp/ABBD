package screen

import (
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// The road index weighs a move by what it changes. These tests hold it to
// the whole-view recount it replaced: fullCost and its parts below are that
// recount, kept as it stood before the index.

// A route that straight leaves folded back over its first column still
// drops straight once straightened again, so a line from the same place
// crossing it makes a tee.
func TestATeeCountsOnARouteFoldedOverItsColumn(t *testing.T) {
	ways := [][]pt{
		straight([]pt{{12, 24}, {72, 24}, {72, 0}, {72, 0}, {72, 24}, {12, 24}, {12, -36}}),
		{{48, 12}, {-24, 12}},
	}
	v := roadView{style: []string{"0", "0"}, plan: []bool{false, false}, kinds: []string{"k", "k"},
		ends: make([][2]end, 2), from: []string{"a", "a"}, to: []string{"b", "c"}, dest: []int{1, 2},
		position: func(y int) int { return y / 16 }}.grouped()
	if got := newRoadIndex(v, ways).cost.tees; got != 1 {
		t.Errorf("route %v crossed at y=12 from the same place made %d tees, want 1", ways[0], got)
	}
}

// Random views of routes that share tracks, overlap, touch, fold and turn
// at shared points, changed one to three routes at a time, weigh every
// change as the whole-view recount does, and a move the index refuses
// early is one the recount refuses too.
func TestTheRoadIndexWeighsChangesAsAWholeRecount(t *testing.T) {
	r := rand.New(rand.NewSource(237))
	limitless := roadCost{1 << 30, 1 << 30, 1 << 30, 1 << 30, 1 << 30, 1 << 30, 1 << 30, 1 << 30, 1 << 30, 1 << 30}
	for trial := range 2000 {
		nr := 2 + r.Intn(10)
		grid := 3 + r.Intn(6)
		step := []int{2, 4, 8, 12, 24}[r.Intn(5)]
		styles := []string{"0", "1", "2"}[:1+r.Intn(3)]
		places := []string{"a", "b", "c", "d"}[:1+r.Intn(4)]
		v := roadView{position: func(y int) int { return y / 16 }}
		ways := make([][]pt, nr)
		for i := range ways {
			ways[i] = randomRoute(r, grid, step)
			v.style = append(v.style, styles[r.Intn(len(styles))])
			v.plan = append(v.plan, r.Intn(3) == 0)
			v.kinds = append(v.kinds, []string{"k", "j"}[r.Intn(2)])
			var e [2]end
			for s := range e {
				if r.Intn(2) == 0 {
					e[s].node = places[r.Intn(len(places))]
				}
			}
			v.ends = append(v.ends, e)
			v.from = append(v.from, places[r.Intn(len(places))])
			v.to = append(v.to, places[r.Intn(len(places))])
			v.dest = append(v.dest, r.Intn(5))
		}
		v = v.grouped()
		x := newRoadIndex(v, ways)
		if got, want := x.cost, v.fullCost(ways); got != want {
			t.Fatalf("trial %d: index %+v, recount %+v\nways %v", trial, got, want, ways)
		}
		for m := range 20 {
			next := slices.Clone(ways)
			var ts []int
			touched := map[int]bool{}
			for _, i := range r.Perm(nr)[:1+r.Intn(min(3, nr))] {
				ts = append(ts, i)
				touched[i] = true
				if r.Intn(3) == 0 {
					next[i] = randomRoute(r, grid, step)
					continue
				}
				w := slices.Clone(ways[i])
				if len(w) >= 2 {
					k, d := r.Intn(len(w)-1), (r.Intn(5)-2)*step/2
					if w[k].x == w[k+1].x {
						w[k].x += d
						w[k+1].x += d
					} else {
						w[k].y += d
						w[k+1].y += d
					}
				}
				next[i] = straight(w)
			}
			slices.Sort(ts)
			want := v.fullCost(next)
			if got, ok := x.change(next, ts).costWithin(limitless); !ok || got != want {
				t.Fatalf("trial %d move %d of %v: index %+v (kept %v), recount %+v\nways %v\nnext %v", trial, m, ts, got, ok, want, ways, next)
			}
			was, now := x.change(next, ts).endMixes()
			if w, n := v.fullEndMixes(ways, touched), v.fullEndMixes(next, touched); was != w || now != n {
				t.Fatalf("trial %d move %d: end mixes %d to %d, recount %d to %d", trial, m, was, now, w, n)
			}
			cur := x.cost
			if _, ok := x.change(next, ts).costWithin(cur); !ok && !(want.crossings > cur.crossings || want.turns > cur.turns || want.clash > cur.clash || want.near > cur.near || want.far > cur.far || want.forks > cur.forks || want.tees > cur.tees || want.unseen > cur.unseen || want.combs > cur.combs) {
				t.Fatalf("trial %d move %d: refused early a move the recount keeps, %+v against %+v", trial, m, want, cur)
			}
		}
	}
}

// randomRoute is an orthogonal route on a coarse grid, so that routes
// share tracks, overlap, cross and turn at shared points often.
func randomRoute(r *rand.Rand, grid, step int) []pt {
	p := pt{r.Intn(grid) * step, r.Intn(grid) * step}
	w := []pt{p}
	vert := r.Intn(2) == 0
	for range 1 + r.Intn(6) {
		d := (1 + r.Intn(grid)) * step
		if r.Intn(2) == 0 {
			d = -d
		}
		if vert {
			p.y += d
		} else {
			p.x += d
		}
		w = append(w, p)
		vert = !vert
	}
	return straight(w)
}

func (v roadView) fullCost(ways [][]pt) roadCost {
	c := roadCost{far: v.fullFarSplits(ways), forks: len(v.forks(ways, true)), tees: v.fullTees(ways), unseen: v.fullUnseen(ways), combs: v.fullCombs(ways)}
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
func (v roadView) fullCombs(ways [][]pt) int {
	at := map[stub][]int{}
	for i, w := range ways {
		for _, k := range []int{0, len(w) - 2} {
			if k < 0 {
				continue
			}
			if s, ok := v.stubOf(ways, i, k); ok {
				at[s] = append(at[s], w[k].x)
			}
		}
	}
	n := 0
	for _, xs := range at {
		slices.Sort(xs)
		xs = slices.Compact(xs)
		for i := 1; i < len(xs); i++ {
			if xs[i]-xs[i-1] <= combGap {
				n++
			}
		}
	}
	return n
}
func (v roadView) fullUnseen(ways [][]pt) int {
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
func (v roadView) fullEndMixes(ways [][]pt, touched map[int]bool) int {
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
func (v roadView) fullTees(ways [][]pt) int {
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
func (v roadView) fullFarSplits(ways [][]pt) int {
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
		// Lines that arrive from two sides or more merge there, and a merge
		// may split at once anywhere, as J1 holds it.
		sides := map[string]int{}
		for k := range outs {
			sides[k.style]++
		}
		for k, o := range outs {
			if len(o) < 2 || len(dests[k]) < 2 || sides[k.style] > 1 {
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
