package screen

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// The drawings place nodes in layers by what uses what: a node sits above
// everything it uses, so a binary is on top and its leaf libraries at the
// bottom. A layer wider than the drawing's budget wraps into several rows.
// Lines run on a grid: down out of a box, across in the gap between two rows
// on a track of their own, down through a row in the space between boxes,
// and into the box they reach. docs/blueprint-svg.md states the rules this
// layout keeps; legible_test.go measures them. Everything is sorted before it
// is placed, so the same graph always gives the same picture.

type lnode struct {
	id   string
	w, h int
}

// ledge runs from a node to one it uses.
type ledge struct{ from, to string }

type rect struct{ X, Y, W, H int }

type layout struct {
	pos map[string]rect
	row map[string]int
	// rows are the wrapped layers, top to bottom.
	rows  [][]string
	width int
	// top is where gap 0 begins. Gap g lies above row g; gap len(rows) lies
	// below the last row.
	top            int
	gapTop, gapH   []int
	rowTop, rowH   []int
	height         int
	lanes          map[[2]int]int
	heights        map[string]int
	minGap, endGap int
}

const (
	gapX       = 40
	padX, padY = 24, 20
	// lane is the spacing between parallel lines; bundle is how many lines
	// share a channel before the next one starts a new channel, rule 2.
	lane   = 8
	bundle = 4
	// busAt is how many lines into one place are drawn as one bundle.
	busAt = 3
	// Width budgets, rule 5: what the page shows of a drawing at 1440px wide.
	overviewBudget = 1320
	insideBudget   = 920
)

func arrange(nodes []lnode, edges []ledge, budget int) layout {
	slices.SortFunc(nodes, func(a, b lnode) int { return strings.Compare(a.id, b.id) })
	byID := map[string]lnode{}
	for _, n := range nodes {
		byID[n.id] = n
	}
	out := map[string][]string{}
	touched := map[string]bool{}
	for _, e := range edges {
		if _, ok := byID[e.from]; !ok {
			continue
		}
		if _, ok := byID[e.to]; !ok || e.from == e.to || slices.Contains(out[e.from], e.to) {
			continue
		}
		out[e.from] = append(out[e.from], e.to)
		touched[e.from], touched[e.to] = true, true
	}
	for k := range out {
		slices.Sort(out[k])
	}
	// A cycle cannot be layered. The nodes are put in an order that leaves
	// as few edges as it can pointing backwards (Eades, Lin and Smyth: peel
	// sinks to the back and sources to the front, else the node with the most
	// more outgoing than incoming edges); those edges are left out of the
	// ranking only.
	keep := map[ledge]bool{}
	{
		in := map[string]map[string]bool{}
		outs := map[string]map[string]bool{}
		left := map[string]bool{}
		for _, n := range nodes {
			left[n.id] = true
			in[n.id], outs[n.id] = map[string]bool{}, map[string]bool{}
		}
		for v, ws := range out {
			for _, w := range ws {
				outs[v][w], in[w][v] = true, true
			}
		}
		var front, back []string
		remove := func(v string) {
			delete(left, v)
			for w := range outs[v] {
				delete(in[w], v)
			}
			for w := range in[v] {
				delete(outs[w], v)
			}
		}
		for len(left) > 0 {
			var ids []string
			for v := range left {
				ids = append(ids, v)
			}
			slices.Sort(ids)
			moved := false
			for _, v := range ids {
				if len(outs[v]) == 0 {
					back = append([]string{v}, back...)
					remove(v)
					moved = true
				}
			}
			for _, v := range ids {
				if left[v] && len(in[v]) == 0 {
					front = append(front, v)
					remove(v)
					moved = true
				}
			}
			if moved {
				continue
			}
			best := ids[0]
			for _, v := range ids {
				if len(outs[v])-len(in[v]) > len(outs[best])-len(in[best]) {
					best = v
				}
			}
			front = append(front, best)
			remove(best)
		}
		at := map[string]int{}
		for i, v := range append(front, back...) {
			at[v] = i
		}
		for v, ws := range out {
			for _, w := range ws {
				if at[v] < at[w] {
					keep[ledge{v, w}] = true
				}
			}
		}
	}
	indeg := map[string]int{}
	for e := range keep {
		indeg[e.to]++
	}
	rank := map[string]int{}
	var queue []string
	for _, n := range nodes {
		if indeg[n.id] == 0 {
			queue = append(queue, n.id)
		}
	}
	var order []string
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		order = append(order, v)
		for _, w := range out[v] {
			if !keep[ledge{v, w}] {
				continue
			}
			rank[w] = max(rank[w], rank[v]+1)
			if indeg[w]--; indeg[w] == 0 {
				queue = append(queue, w)
			}
		}
	}
	// A node nothing uses sits just above the highest thing it uses rather
	// than on the top layer, so the top layer does not collect every root.
	for i := len(order) - 1; i >= 0; i-- {
		v := order[i]
		low := -1
		for _, w := range out[v] {
			if keep[ledge{v, w}] && (low < 0 || rank[w] < low) {
				low = rank[w]
			}
		}
		used := false
		for _, n := range nodes {
			used = used || keep[ledge{n.id, v}]
		}
		if !used && low > 0 {
			rank[v] = low - 1
		}
	}
	n := 0
	for _, r := range rank {
		n = max(n, r+1)
	}
	if len(nodes) > 0 {
		n = max(n, 1)
	}
	// Nodes with no line at all gather on a layer of their own at the bottom.
	loose := false
	for _, nd := range nodes {
		loose = loose || !touched[nd.id]
	}
	if loose && len(touched) > 0 {
		for _, nd := range nodes {
			if !touched[nd.id] {
				rank[nd.id] = n
			}
		}
		n++
	}
	neighbours := map[string][]string{}
	for v, ws := range out {
		for _, w := range ws {
			neighbours[v] = append(neighbours[v], w)
			neighbours[w] = append(neighbours[w], v)
		}
	}
	for k := range neighbours {
		slices.Sort(neighbours[k])
	}
	ranks := make([][]string, n)
	for _, nd := range nodes {
		ranks[rank[nd.id]] = append(ranks[rank[nd.id]], nd.id)
	}
	order2(ranks, rank, neighbours)

	l := layout{pos: map[string]rect{}, row: map[string]int{}, lanes: map[[2]int]int{}, heights: map[string]int{}, minGap: 36, endGap: 12}
	room := budget - 2*padX
	for _, ids := range ranks {
		l.rows = append(l.rows, wrap(ids, byID, room)...)
	}
	for r, ids := range l.rows {
		l.width = max(l.width, rowWidth(ids, byID))
		for _, id := range ids {
			l.row[id] = r
			l.heights[id] = byID[id].h
		}
	}
	l.width += 2 * padX
	for _, ids := range l.rows {
		x := (l.width - rowWidth(ids, byID)) / 2
		for i, id := range ids {
			if i > 0 {
				x += gapX
			}
			l.pos[id] = rect{X: x, W: byID[id].w, H: byID[id].h}
			x += byID[id].w
		}
	}
	l.align(neighbours)
	return l
}

func rowWidth(ids []string, byID map[string]lnode) int {
	w := 0
	for i, id := range ids {
		if i > 0 {
			w += gapX
		}
		w += byID[id].w
	}
	return w
}

// align moves each box toward the mean centre of the boxes it is joined to,
// keeping each row's order and spacing, so lines run straighter. A few
// passes down and up settle it.
func (l *layout) align(neighbours map[string][]string) {
	for pass := 0; pass < 6; pass++ {
		order := make([]int, len(l.rows))
		for i := range order {
			order[i] = i
			if pass%2 == 1 {
				order[i] = len(l.rows) - 1 - i
			}
		}
		for _, r := range order {
			ids := l.rows[r]
			want := make([]int, len(ids))
			for i, id := range ids {
				p := l.pos[id]
				sum, k := 0, 0
				for _, o := range neighbours[id] {
					if q, ok := l.pos[o]; ok && l.row[o] != r {
						sum += q.X + q.W/2
						k++
					}
				}
				want[i] = p.X
				if k > 0 {
					want[i] = sum/k - p.W/2
				}
			}
			// Left to right keeps the spacing; right to left keeps the
			// right margin; the box ends between its two bounds.
			xs := make([]int, len(ids))
			edge := padX
			for i, id := range ids {
				if i > 0 {
					edge += gapX
				}
				xs[i] = max(want[i], edge)
				edge = xs[i] + l.pos[id].W
			}
			edge = l.width - padX
			for i := len(ids) - 1; i >= 0; i-- {
				w := l.pos[ids[i]].W
				xs[i] = min(xs[i], edge-w)
				if i > 0 {
					edge = xs[i] - gapX
				}
			}
			for i, id := range ids {
				p := l.pos[id]
				p.X = xs[i]
				l.pos[id] = p
			}
		}
	}
}

// order2 sorts each layer by the mean position of its neighbours, sweeping
// down and up a few times, and keeps the order with the fewest crossings
// between adjacent layers.
func order2(ranks [][]string, rank map[string]int, neighbours map[string][]string) {
	at := func() map[string]float64 {
		out := map[string]float64{}
		for _, r := range ranks {
			for i, id := range r {
				out[id] = (float64(i) + 0.5) / float64(len(r))
			}
		}
		return out
	}
	crossings := func() int {
		pos := at()
		total := 0
		for r := 0; r+1 < len(ranks); r++ {
			var es [][2]float64
			for _, v := range ranks[r] {
				for _, w := range neighbours[v] {
					if rank[w] == r+1 {
						es = append(es, [2]float64{pos[v], pos[w]})
					}
				}
			}
			for i := range es {
				for j := i + 1; j < len(es); j++ {
					if (es[i][0]-es[j][0])*(es[i][1]-es[j][1]) < 0 {
						total++
					}
				}
			}
		}
		return total
	}
	sweep := func(r int, above bool) {
		pos := at()
		key := map[string]float64{}
		for _, id := range ranks[r] {
			sum, k := 0.0, 0
			for _, o := range neighbours[id] {
				if (above && rank[o] < r) || (!above && rank[o] > r) {
					sum += pos[o]
					k++
				}
			}
			key[id] = pos[id]
			if k > 0 {
				key[id] = sum / float64(k)
			}
		}
		sort.SliceStable(ranks[r], func(i, j int) bool { return key[ranks[r][i]] < key[ranks[r][j]] })
	}
	snapshot := func() [][]string {
		out := make([][]string, len(ranks))
		for i, r := range ranks {
			out[i] = slices.Clone(r)
		}
		return out
	}
	best, bestN := snapshot(), crossings()
	for i := 0; i < 8; i++ {
		for r := 1; r < len(ranks); r++ {
			sweep(r, true)
		}
		for r := len(ranks) - 2; r >= 0; r-- {
			sweep(r, false)
		}
		if c := crossings(); c < bestN {
			best, bestN = snapshot(), c
		}
	}
	copy(ranks, best)
}

// wrap splits a layer wider than room into rows of about equal width.
func wrap(ids []string, byID map[string]lnode, room int) [][]string {
	total := rowWidth(ids, byID)
	if total <= room || len(ids) < 2 {
		return [][]string{ids}
	}
	rows := (total + room - 1) / room
	target := (total + rows - 1) / rows
	var out [][]string
	var cur []string
	w := 0
	for _, id := range ids {
		add := byID[id].w
		if len(cur) > 0 {
			add += gapX
		}
		if len(cur) > 0 && (w+add > room || w+add/2 > target && len(out) < rows-1) {
			out = append(out, cur)
			cur, w, add = nil, 0, byID[id].w
		}
		cur = append(cur, id)
		w += add
	}
	return append(out, cur)
}

// end is one end of a line: a node, or a port on the frame's top (row -1)
// or bottom (row len(rows)). The line attaches somewhere in [lo, hi]; ends
// that share a key share that range and spread over it.
type end struct {
	node   string
	row    int
	lo, hi int
	key    string
}

type wireReq struct {
	from, to end
	// kind keeps lines of different kinds out of one bundle.
	kind string
}

// drawn is one path on the page. It draws one request, or a bundle of
// several that end in the same place, with Count saying how many.
type drawn struct {
	D      string
	reqs   []int
	starts []pt
	label  pt
	// net groups the paths of one bundle drawn as several paths; count is
	// how many lines a bundle carries, on the path that shows the count.
	net, count int
	parts      [][]pt
}

type pt struct{ x, y int }

type plan struct {
	gaps []int
	xs   []int
	// up says the line climbs: it leaves a box's top and enters one's bottom.
	up, same bool
	fx, tx   int
	bus      int
	// fan says the bundle leaves one place: its lines share the leader's
	// route and branch off in their last gap.
	fan bool
}

// route draws every request. Lines that leave one place in one direction
// always share a trunk; whether lines into one place do too is decided by
// trying both and keeping the one whose lines cross least, the merged one on
// a tie. Each try is deterministic, so the choice is too.
func (l *layout) route(reqs []wireReq) []drawn {
	best, bestN := 0, -1
	for v, opt := range bundlings {
		if n := crossings(l.routeWith(reqs, opt)); bestN < 0 || n < bestN {
			best, bestN = v, n
		}
	}
	return l.routeWith(reqs, bundlings[best])
}

// bundling says which bundles a routing makes: fan-out from one place, and
// fan-in into one place.
type bundling struct{ out, in bool }

var bundlings = []bundling{{true, true}, {true, false}}

// crossings counts where two drawn paths of different bundles cross.
func crossings(ds []drawn) int {
	type seg struct {
		a, b pt
		net  int
	}
	var segs []seg
	for i, d := range ds {
		net := -1 - i
		if d.net > 0 {
			net = d.net
		}
		for _, part := range d.parts {
			for k := 1; k < len(part); k++ {
				segs = append(segs, seg{part[k-1], part[k], net})
			}
		}
	}
	n := 0
	for i := range segs {
		for j := i + 1; j < len(segs); j++ {
			a, b := segs[i], segs[j]
			if a.net == b.net {
				continue
			}
			// Paths run on a grid: one segment is horizontal, the other
			// vertical, and they cross strictly inside both.
			h, v := a, b
			if h.a.y != h.b.y {
				h, v = b, a
			}
			if h.a.y != h.b.y || v.a.x != v.b.x {
				continue
			}
			if v.a.x > min(h.a.x, h.b.x) && v.a.x < max(h.a.x, h.b.x) && h.a.y > min(v.a.y, v.b.y) && h.a.y < max(v.a.y, v.b.y) {
				n++
			}
		}
	}
	return n
}

func (l *layout) routeWith(reqs []wireReq, opt bundling) []drawn {
	l.lanes = map[[2]int]int{}
	n := len(l.rows)
	plans := make([]plan, len(reqs))
	side := func(e end, up, isFrom, same bool) int {
		// +1 is a box's bottom, -1 its top.
		switch {
		case same:
			return 1
		case isFrom == up:
			return -1
		}
		return 1
	}
	type slotKey struct {
		key  string
		side int
	}
	members := map[slotKey][]int{}
	ends := map[slotKey][]int{}
	fans := map[slotKey][]int{}
	for i, r := range reqs {
		p := &plans[i]
		a, b := r.from.row, r.to.row
		p.up, p.same = b < a, a == b
		switch {
		case p.same:
			p.gaps = []int{a + 1}
		case p.up:
			for g := a; g > b; g-- {
				p.gaps = append(p.gaps, g)
			}
		default:
			for g := a + 1; g <= b; g++ {
				p.gaps = append(p.gaps, g)
			}
		}
		fk := slotKey{r.from.key, side(r.from, p.up, true, p.same)}
		tk := slotKey{r.to.key, side(r.to, p.up, false, p.same)}
		members[fk] = append(members[fk], i)
		ends[tk] = append(ends[tk], i)
		// A box's lines bundle by the box, whichever of its chips they leave.
		if r.from.node != "" {
			fk.key = "node|" + r.from.node
		}
		fans[fk] = append(fans[fk], i)
	}
	// Bundles, rule 2. Lines of one kind that leave one place in one
	// direction travel as one trunk led by the one that goes furthest, and
	// branch off in their last gap. Of the lines left, those of one kind into
	// one place travel as one trunk led by the one that starts furthest away,
	// and join it in the first gap they cross.
	p0 := 0
	buses := map[int][]int{}
	leader := map[int]int{}
	bundleOf := func(is []int) {
		p0++
		buses[p0] = is
		lead := is[0]
		for _, i := range is {
			plans[i].bus = p0
			if len(plans[i].gaps) > len(plans[lead].gaps) {
				lead = i
			}
		}
		leader[p0] = lead
	}
	var fanKeys []slotKey
	for k := range fans {
		fanKeys = append(fanKeys, k)
	}
	sort.Slice(fanKeys, func(i, j int) bool {
		if fanKeys[i].key != fanKeys[j].key {
			return fanKeys[i].key < fanKeys[j].key
		}
		return fanKeys[i].side < fanKeys[j].side
	})
	for _, k := range fanKeys {
		byKind := map[string][]int{}
		var kinds []string
		for _, i := range fans[k] {
			if plans[i].same {
				continue
			}
			kd := fmt.Sprint(reqs[i].kind, plans[i].up)
			if len(byKind[kd]) == 0 {
				kinds = append(kinds, kd)
			}
			byKind[kd] = append(byKind[kd], i)
		}
		for _, kd := range kinds {
			if is := byKind[kd]; opt.out && len(is) >= 2 {
				bundleOf(is)
				for _, i := range is {
					plans[i].fan = true
				}
			}
		}
	}
	var busKeys []slotKey
	for k := range ends {
		busKeys = append(busKeys, k)
	}
	sort.Slice(busKeys, func(i, j int) bool {
		if busKeys[i].key != busKeys[j].key {
			return busKeys[i].key < busKeys[j].key
		}
		return busKeys[i].side < busKeys[j].side
	})
	for _, k := range busKeys {
		byKind := map[string][]int{}
		var kinds []string
		for _, i := range ends[k] {
			if plans[i].same || plans[i].bus > 0 {
				continue
			}
			if len(byKind[reqs[i].kind]) == 0 {
				kinds = append(kinds, reqs[i].kind)
			}
			byKind[reqs[i].kind] = append(byKind[reqs[i].kind], i)
		}
		for _, kind := range kinds {
			if is := byKind[kind]; opt.in && len(is) >= busAt {
				bundleOf(is)
			}
		}
	}
	center := func(e end) int { return (e.lo + e.hi) / 2 }
	place := func(groups map[slotKey][]int, other func(int) int, set func(int, int), e func(int) end) {
		var keys []slotKey
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].key != keys[j].key {
				return keys[i].key < keys[j].key
			}
			return keys[i].side < keys[j].side
		})
		for _, k := range keys {
			is := groups[k]
			// One slot per bundle, one per other line.
			var slots [][]int
			seen := map[int]int{}
			for _, i := range is {
				if b := plans[i].bus; b > 0 && set == nil {
					continue
				}
				if b := plans[i].bus; b > 0 {
					if j, ok := seen[b]; ok {
						slots[j] = append(slots[j], i)
						continue
					}
					seen[b] = len(slots)
				}
				slots = append(slots, []int{i})
			}
			sort.SliceStable(slots, func(a, b int) bool {
				oa, ob := 0, 0
				for _, i := range slots[a] {
					oa += other(i)
				}
				for _, i := range slots[b] {
					ob += other(i)
				}
				oa, ob = oa/len(slots[a]), ob/len(slots[b])
				if oa != ob {
					return oa < ob
				}
				return slots[a][0] < slots[b][0]
			})
			r := e(is[0])
			for s, group := range slots {
				x := r.lo + (r.hi-r.lo)*(2*s+1)/(2*len(slots))
				for _, i := range group {
					set(i, x)
				}
			}
		}
	}
	place(members, func(i int) int { return center(reqs[i].to) }, func(i, x int) { plans[i].fx = x }, func(i int) end { return reqs[i].from })
	place(ends, func(i int) int { return center(reqs[i].from) }, func(i, x int) { plans[i].tx = x }, func(i int) end { return reqs[i].to })

	// The x of each passage through a row between the ends.
	for i := range reqs {
		p := &plans[i]
		if p.bus > 0 && leader[p.bus] != i {
			continue
		}
		p.xs = []int{p.fx}
		k := len(p.gaps) - 1
		for j := 0; j < k; j++ {
			var r int
			if p.up {
				r = p.gaps[j] - 1
			} else {
				r = p.gaps[j]
			}
			want := p.fx + (p.tx-p.fx)*(j+1)/(k+1)
			if p.bus > 0 && !p.fan {
				// A trunk runs straight above the place it reaches.
				want = p.tx
			}
			p.xs = append(p.xs, l.gutter(r, want))
		}
		p.xs = append(p.xs, p.tx)
	}
	for i := range reqs {
		p := &plans[i]
		if p.bus == 0 || leader[p.bus] == i {
			continue
		}
		lead := plans[leader[p.bus]]
		if p.fan {
			k := len(p.gaps) - 1
			p.gaps = p.gaps[k:]
			p.xs = []int{lead.xs[k], p.tx}
			continue
		}
		k := len(lead.gaps) - len(p.gaps)
		p.gaps = p.gaps[:1]
		p.xs = []int{p.fx, lead.xs[k+1]}
	}
	// Tracks: each crossing of a gap gets a height of its own; a bundle is
	// one crossing however many lines it carries. A crossing joins the gap's
	// top edge at some x and its bottom edge at others; its track is ordered
	// against the others so few of those joins cut through another track.
	type span struct {
		lo, hi      int
		owner       int
		tops, downs []int
	}
	spans := make([]map[int]*span, n+1)
	var owners = make([][]int, n+1)
	for i := range reqs {
		p := &plans[i]
		for j, g := range p.gaps {
			a, b := p.xs[j], p.xs[j+1]
			if a == b && !p.same {
				continue
			}
			owner := i
			if p.bus > 0 {
				owner = -p.bus
			}
			if spans[g] == nil {
				spans[g] = map[int]*span{}
			}
			sp, ok := spans[g][owner]
			if !ok {
				sp = &span{lo: min(a, b), hi: max(a, b), owner: owner}
				spans[g][owner] = sp
				owners[g] = append(owners[g], owner)
			}
			sp.lo, sp.hi = min(sp.lo, a, b), max(sp.hi, a, b)
			switch {
			case p.same:
				sp.tops = append(sp.tops, a, b)
			case p.up:
				sp.downs, sp.tops = append(sp.downs, a), append(sp.tops, b)
			default:
				sp.tops, sp.downs = append(sp.tops, a), append(sp.downs, b)
			}
		}
	}
	inside := func(xs []int, s *span) int {
		c := 0
		for _, x := range xs {
			if x > s.lo && x < s.hi {
				c++
			}
		}
		return c
	}
	// cost is how many crossings putting a above b makes between them.
	cost := func(a, b *span) int { return inside(a.downs, b) + inside(b.tops, a) }
	track := map[[2]int]int{}
	tracks := make([]int, n+1)
	for g := range spans {
		left := slices.Clone(owners[g])
		var order []*span
		for len(left) > 0 {
			best, bestC := 0, 1<<30
			for k, o := range left {
				c := 0
				for _, q := range left {
					if q != o {
						c += cost(spans[g][o], spans[g][q])
					}
				}
				if c < bestC {
					best, bestC = k, c
				}
			}
			order = append(order, spans[g][left[best]])
			left = slices.Delete(left, best, best+1)
		}
		// Each crossing takes the highest track below every crossing ordered
		// before it that it overlaps.
		var placed []*span
		for _, s := range order {
			t := 0
			for _, q := range placed {
				if q.lo <= s.hi+lane && s.lo <= q.hi+lane {
					t = max(t, track[[2]int{g, q.owner}]+1)
				}
			}
			track[[2]int{g, s.owner}] = t
			tracks[g] = max(tracks[g], t+1)
			placed = append(placed, s)
		}
	}
	// Place the rows now that each gap's height is known.
	l.gapTop, l.gapH = make([]int, n+1), make([]int, n+1)
	l.rowTop, l.rowH = make([]int, n), make([]int, n)
	y := l.top
	for g := 0; g <= n; g++ {
		h := l.minGap
		if g == 0 || g == n {
			h = l.endGap
		}
		if tracks[g] > 0 {
			h = max(h, lane*(tracks[g]+1)+lane/2+(tracks[g]-1)/bundle*lane)
		}
		l.gapTop[g], l.gapH[g] = y, h
		y += h
		if g == n {
			break
		}
		rh := 0
		for _, id := range l.rows[g] {
			rh = max(rh, l.heights[id])
		}
		l.rowTop[g], l.rowH[g] = y, rh
		for _, id := range l.rows[g] {
			p := l.pos[id]
			p.Y = y + (rh-p.H)/2
			l.pos[id] = p
		}
		y += rh
	}
	l.height = y
	trackY := func(g, owner int) int {
		t := track[[2]int{g, owner}]
		return l.gapTop[g] + lane*(t+1) + t/bundle*lane
	}
	endY := func(e end, sideOf int) int {
		switch {
		case e.node == "" && e.row < 0:
			return l.top
		case e.node == "":
			return l.gapTop[n] + l.gapH[n]
		case sideOf < 0:
			return l.pos[e.node].Y
		}
		return l.pos[e.node].Y + l.pos[e.node].H
	}
	points := func(i int) []pt {
		p := plans[i]
		r := reqs[i]
		owner := i
		if p.bus > 0 {
			owner = -p.bus
		}
		pts := []pt{{p.xs[0], endY(r.from, side(r.from, p.up, true, p.same))}}
		if p.fan && leader[p.bus] != i {
			// A branch starts where it leaves the trunk.
			pts[0].y = trackY(p.gaps[0], owner)
		}
		for j, g := range p.gaps {
			a, b := p.xs[j], p.xs[j+1]
			if a != b || p.same {
				ty := trackY(g, owner)
				pts = append(pts, pt{a, ty}, pt{b, ty})
			}
		}
		if p.bus > 0 && !p.fan && leader[p.bus] != i {
			if len(pts) == 1 {
				pts = append(pts, pt{p.xs[0], trackY(p.gaps[0], owner)})
			}
			return pts
		}
		return append(pts, pt{p.xs[len(p.xs)-1], endY(r.to, side(r.to, p.up, false, p.same))})
	}
	var out []drawn
	done := map[int]bool{}
	for i := range reqs {
		p := plans[i]
		if p.bus == 0 {
			pts := points(i)
			out = append(out, drawn{D: polyline(pts), parts: [][]pt{pts}, reqs: []int{i}, starts: []pt{pts[0]}})
			continue
		}
		if p.fan {
			pts := points(i)
			d := drawn{D: polyline(pts), parts: [][]pt{pts}, reqs: []int{i}, starts: []pt{pts[0]}, net: p.bus}
			if leader[p.bus] == i {
				d.count, d.label = len(buses[p.bus]), pt{pts[0].x + 5, pts[0].y + 14}
				if p.up {
					d.label.y = pts[0].y - 6
				}
			}
			out = append(out, d)
			continue
		}
		if done[p.bus] {
			continue
		}
		done[p.bus] = true
		lead := leader[p.bus]
		var parts []string
		var all [][]pt
		var starts []pt
		var lpts []pt
		for _, j := range buses[p.bus] {
			pts := points(j)
			starts = append(starts, pts[0])
			if j == lead {
				lpts = pts
				continue
			}
			parts = append(parts, polyline(pts))
			all = append(all, pts)
		}
		// The leader goes last, so the arrowhead marks the trunk's end.
		parts = append(parts, polyline(lpts))
		all = append(all, lpts)
		a, b := lpts[len(lpts)-2], lpts[len(lpts)-1]
		out = append(out, drawn{D: strings.Join(parts, " "), parts: all, reqs: buses[p.bus], starts: starts, label: pt{b.x + 5, (a.y+b.y)/2 + 4}, net: p.bus, count: len(buses[p.bus])})
	}
	return out
}

func polyline(pts []pt) string {
	var clean []pt
	for _, p := range pts {
		if len(clean) > 0 && clean[len(clean)-1] == p {
			continue
		}
		if k := len(clean); k >= 2 {
			a, b := clean[k-2], clean[k-1]
			if (a.x == b.x && b.x == p.x) || (a.y == b.y && b.y == p.y) {
				clean[k-1] = p
				continue
			}
		}
		clean = append(clean, p)
	}
	var d strings.Builder
	for i, p := range clean {
		if i == 0 {
			fmt.Fprintf(&d, "M%d %d", p.x, p.y)
		} else {
			fmt.Fprintf(&d, " L%d %d", p.x, p.y)
		}
	}
	return d.String()
}

// gutter picks the space in row r nearest to x for a line passing through
// it, and the lane in that space the line takes.
func (l *layout) gutter(r, x int) int {
	ids := l.rows[r]
	type gap struct{ from, to int }
	var gaps []gap
	edge := 0
	for _, id := range ids {
		p := l.pos[id]
		gaps = append(gaps, gap{edge, p.X})
		edge = p.X + p.W
	}
	gaps = append(gaps, gap{edge, l.width})
	slots := func(i int) int {
		if i == 0 || i == len(gaps)-1 {
			return 1 << 20
		}
		return max(0, (gaps[i].to-gaps[i].from-12)/lane+1)
	}
	// A space costs the detour to reach it plus the lines already in it, so
	// lines spread over the spaces instead of crowding into the nearest one.
	cost := func(i int) int {
		g := gaps[i]
		used := l.lanes[[2]int{r, i}]
		c := abs(clamp(x, g.from+6, g.to-6)-x) + 40*used
		if used >= slots(i) {
			c += 1 << 20
		}
		return c
	}
	best := 0
	for i := range gaps {
		if cost(i) < cost(best) {
			best = i
		}
	}
	g := gaps[best]
	k := l.lanes[[2]int{r, best}]
	l.lanes[[2]int{r, best}]++
	step := k*lane + k/bundle*lane
	switch best {
	case 0:
		return g.to - 12 - step
	case len(gaps) - 1:
		return g.from + 12 + step
	}
	return g.from + (g.to-g.from)/2 + offset(k, slots(best))*lane
}

// offset spreads lanes outward from a space's centre: 0, 1, -1, 2, -2, ...
func offset(n, slots int) int {
	n %= max(1, slots)
	if n%2 == 1 {
		return (n + 1) / 2
	}
	return -n / 2
}

func clamp(x, lo, hi int) int {
	if hi < lo {
		return (lo + hi) / 2
	}
	return max(lo, min(hi, x))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// textWidth estimates how wide a label renders: wide for Hangul and other
// East Asian letters, narrow for the rest. It errs wide, so a label sized by
// it never runs past its box.
func textWidth(s string, px int) int {
	w := 0
	for _, r := range s {
		switch {
		case r >= 0x1100:
			w += px + 1
		case r >= 'A' && r <= 'Z' || r == 'm' || r == 'w' || r == 'M' || r == 'W' || r == '@':
			w += px*3/4 + 1
		default:
			w += px*3/5 + 1
		}
	}
	return w
}

// fit cuts s to the width given, ending with an ellipsis when it had to cut.
func fit(s string, width, px int) string {
	if textWidth(s, px) <= width {
		return s
	}
	for s != "" {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
		if textWidth(s+"…", px) <= width {
			return strings.TrimSpace(s) + "…"
		}
	}
	return ""
}

// setName sets a name in a box at most maxW wide: on one line when it fits,
// else on two lines broken at the space or separator that balances them,
// and cut short only when two lines at the widest box still cannot hold it.
// It returns the lines and the width the box needs.
func setName(name string, px, pad, minW, maxW int) ([]string, int) {
	if w := textWidth(name, px) + pad; w <= maxW {
		return []string{name}, max(minW, w)
	}
	room := maxW - pad
	best, bestW := -1, 1<<30
	for i, r := range name {
		if i == 0 || !(r == ' ' || r == '/' || r == '-' || r == '_' || r == '.' || r == '(') {
			continue
		}
		cut := i
		if r != ' ' && r != '(' {
			cut = i + 1
		}
		a, b := strings.TrimSpace(name[:cut]), strings.TrimSpace(name[cut:])
		if w := max(textWidth(a, px), textWidth(b, px)); w < bestW {
			best, bestW = cut, w
		}
	}
	if best < 0 {
		// One long word: break it where the first line is full.
		for i := range name {
			if textWidth(name[:i], px) > room {
				break
			}
			best = i
		}
	}
	a, b := strings.TrimSpace(name[:best]), strings.TrimSpace(name[best:])
	fa, fb := fit(a, room, px), fit(b, room, px)
	if fa != a || fb != b {
		return []string{fa, fb}, maxW
	}
	return []string{a, b}, max(minW, max(textWidth(a, px), textWidth(b, px))+pad)
}
