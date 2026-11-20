package screen

import "slices"

// hops says which requests run straight across between two boxes that
// stand next to each other in one row, both ends on the boxes themselves,
// not on a chip: two stages of the primary flow, on the spine or in a
// stage's own row down the flow, or a box a beside tactic stood next to the
// box its only lines join (arrange). Such a line runs straight across from
// one box's facing side into the other's, as a pipeline is drawn: it joins
// no bus, so it never leaves its box beside a bundle of its kind (rule 4)
// nor dips under the row and comes back up (B3). It is nil when no line
// does.
func (l *layout) hops(reqs []wireReq) []bool {
	f := l.axis.flow
	bare := func(e end) bool { return e.node != "" && (e.key == e.node || e.key == "plain|"+e.node) }
	joins := func(a, b string) bool {
		return f != nil && f.stage[a] > 0 && f.stage[b] > 0 || l.partner[a] == b || l.partner[b] == a
	}
	var out []bool
	for i, r := range reqs {
		a, b := r.from.node, r.to.node
		if !bare(r.from) || !bare(r.to) || a == b || !joins(a, b) || l.row[a] != l.row[b] || !l.besides(a, b) {
			continue
		}
		if out == nil {
			out = make([]bool, len(reqs))
		}
		out[i] = true
	}
	return out
}

// besides says boxes a and b stand next to each other in their row, no box
// between them, and their heights overlap by room for a hop.
func (l *layout) besides(a, b string) bool {
	pa, pb := l.pos[a], l.pos[b]
	if pb.X < pa.X {
		pa, pb = pb, pa
	}
	if min(pa.Y+pa.H, pb.Y+pb.H)-max(pa.Y, pb.Y) < 2*slotGap {
		return false
	}
	return !slices.ContainsFunc(l.rows[l.row[a]], func(id string) bool {
		p := l.pos[id]
		return id != a && id != b && p.X+p.W > pa.X+pa.W && p.X < pb.X
	})
}

// withHops routes the requests hop leaves, then draws the hops in, every
// request index back on reqs. Hops between one pair of boxes stand slotGap
// apart about the middle of the height the two boxes share, so two lines
// between the same neighbours, one each way, run as two lanes and never
// merge; but a true reverse pair between a box a beside tactic placed and
// its partner, in a view that folds its pairs, is one line with a head at
// each end (fold.go), as it was placed to be.
func (l *layout) withHops(reqs []wireReq, hop []bool) routing {
	var at []int
	var rest []wireReq
	for i, q := range reqs {
		if !hop[i] {
			at, rest = append(at, i), append(rest, q)
		}
	}
	r := l.routeLines(rest)
	re := func(is []int) []int {
		out := make([]int, len(is))
		for k, i := range is {
			out[k] = at[i]
		}
		return out
	}
	for k := range r.drawn {
		r.drawn[k].reqs, r.drawn[k].counted = re(r.drawn[k].reqs), re(r.drawn[k].counted)
	}
	r.bent = re(r.bent)
	ways, back := make([][]pt, len(reqs)), make([]bool, len(reqs))
	for k, i := range at {
		ways[i], back[i] = r.ways[k], r.back[k]
	}
	if r.rider != nil {
		rider := make([]int, len(reqs))
		for i := range rider {
			rider[i] = -1
		}
		for k, i := range at {
			if j := r.rider[k]; j >= 0 {
				rider[i] = at[j]
			}
		}
		r.rider = rider
	}
	pair := map[[2]string][]int{}
	var pairs [][2]string
	for i, q := range reqs {
		if !hop[i] {
			continue
		}
		k := pairKey(q.from.node, q.to.node)
		if _, ok := pair[k]; !ok {
			pairs = append(pairs, k)
		}
		pair[k] = append(pair[k], i)
	}
	for _, k := range pairs {
		is := pair[k]
		if l.foldsHops(reqs, k, is) {
			// The kept line runs the way the eye reads, right within a row
			// (readFirst), and the line it folds rides it back.
			i, j := is[0], is[1]
			if l.pos[reqs[i].from.node].X > l.pos[reqs[i].to.node].X {
				i, j = j, i
			}
			left, right := l.pos[reqs[i].from.node], l.pos[reqs[i].to.node]
			y := (max(left.Y, right.Y) + min(left.Y+left.H, right.Y+right.H)) / 2
			a, b := pt{left.X + left.W, y}, pt{right.X, y}
			if r.rider == nil {
				r.rider = make([]int, len(reqs))
				for n := range r.rider {
					r.rider[n] = -1
				}
			}
			r.rider[j] = i
			ways[i], ways[j] = []pt{a, b}, []pt{b, a}
			back[i], back[j] = false, false
			line := []pt{a, b}
			r.drawn = append(r.drawn, drawn{D: polyline(line), parts: [][]pt{line}, reqs: []int{i, j}, starts: []pt{a, noStart}, kind: reqs[i].kind, both: true})
			continue
		}
		left, right := l.pos[k[0]], l.pos[k[1]]
		if right.X < left.X {
			left, right = right, left
		}
		mid := (max(left.Y, right.Y) + min(left.Y+left.H, right.Y+right.H)) / 2
		for n, i := range is {
			q := reqs[i]
			y := mid + (2*n-len(is)+1)*slotGap/2
			a, b := pt{left.X + left.W, y}, pt{right.X, y}
			if l.pos[q.from.node].X > l.pos[q.to.node].X {
				a, b = b, a
			}
			back[i] = (b.x < a.x) != (q.kind == kindProvides)
			ways[i] = []pt{a, b}
			line := []pt{a, b}
			r.drawn = append(r.drawn, drawn{D: polyline(line), parts: [][]pt{line}, reqs: []int{i}, starts: []pt{a}, back: back[i], kind: q.kind})
		}
	}
	r.ways, r.back = ways, back
	return r
}

// staged says r joins two stages of a flow laid across.
func (l *layout) staged(r wireReq) bool {
	f := l.axis.flow
	return f != nil && f.across && f.stage[r.from.node] > 0 && f.stage[r.to.node] > 0
}

// over says r runs between two stages of a flow laid across that no hop
// joins: it runs in the gap above the spine, clear of every line the
// stages send down to the boxes below them.
func (l *layout) over(r wireReq) bool {
	return l.staged(r) && l.row[r.from.node] == l.row[r.to.node]
}

// foldsHops says the hops is, between the boxes of k, are a true reverse pair
// a view that folds its pairs draws as one line: one line each way, of one
// kind, between a box a beside tactic placed and its partner.
func (l *layout) foldsHops(reqs []wireReq, k [2]string, is []int) bool {
	if l.pairs == foldNone || len(is) != 2 || l.partner[k[0]] != k[1] && l.partner[k[1]] != k[0] {
		return false
	}
	p, q := reqs[is[0]], reqs[is[1]]
	return p.kind == q.kind && p.kind != kindMutual && p.kind != kindBypass && p.from.node == q.to.node && p.to.node == q.from.node && !p.lead && !q.lead && p.plan == q.plan
}
