package screen

import (
	"maps"
	"slices"
	"sort"
	"strings"
)

// Lines run on a grid, rule 4. A line leaves its box, crosses the gap below
// on a track, passes each row between two boxes, crosses the next gap, and
// so on into the box it reaches. Lines converge on buses:
//
//   - Lines of one kind that leave one edge of a box share a departure bus:
//     one stub out of the box, then one trunk through every row two of them
//     still pass. A line leaves the trunk in the gap above the first row it
//     would pass alone.
//   - Lines of one kind that reach one edge of a box share an arrival bus:
//     one trunk through every row they pass, one track in the gap above the
//     box, and a drop into each chip they reach. A line joins the trunk
//     once it is off its departure trunk, so lines from different places
//     merge midway and split only at the end. Boxes side by side in one
//     row that are reached mostly from the same places share one arrival
//     bus, which splits only in the gap above them.
//
// A bus is drawn once and carries the count of its lines. What is left of a
// line between its buses is drawn on a track of its own.

// drawn is one path on the page: a line's own part, or the shared part of a
// bus.
type drawn struct {
	D    string
	reqs []int
	// starts are where each request's line leaves its end, for the origin
	// mark, or noStart when that end is on another path.
	starts []pt
	label  pt
	// net groups the paths of one bus; count is how many lines a bus
	// carries, on the path that shows the count.
	net, count int
	parts      [][]pt
	// joins says the path ends on a bus, not at a box, so it has no
	// arrowhead.
	joins bool
	// back says the line runs against the reading direction, rule 13: up,
	// or left within a row. A line that provides a contract reads from the
	// contract to its unit, so it runs the other way.
	back bool
	// joinsAt are where a line steps off its departure track onto an
	// arrival trunk.
	joinsAt []join
	kind    string
	// own is the stretch of ink the count stands beside.
	own [2]pt
}

type pt struct{ x, y int }

var noStart = pt{-1, -1}

type plan struct {
	gaps []int
	// xs are where the line crosses each gap: xs[0] leaves the start,
	// xs[j] passes the row between gaps j-1 and j, the last reaches the end.
	xs []int
	// up says the line climbs: it leaves a box's top and enters one's bottom.
	up, same bool
	fx, tx   int
	// dep and arr are the line's departure and arrival bus, -1 for none.
	dep, arr int
	// ride is how many rows the line passes on its departure bus's trunk.
	ride int
}

type bus struct {
	members []int
	// arrive says the bus gathers lines into one place; else it sends
	// lines out of one.
	arrive bool
	// passage is the trunk's x in each row it passes, and deep how many
	// rows a departure trunk passes.
	passage map[int]int
	deep    int
}

type slotKey struct {
	key  string
	side int
}

func sortedKeys(m map[slotKey][]int) []slotKey {
	var keys []slotKey
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].key != keys[j].key {
			return keys[i].key < keys[j].key
		}
		return keys[i].side < keys[j].side
	})
	return keys
}

// route draws every request.
func (l *layout) route(reqs []wireReq) []drawn {
	l.lanes, l.beside, l.passes = map[[2]int]int{}, map[int][]int{}, map[int][]int{}
	n := len(l.rows)
	plans := make([]plan, len(reqs))
	// side is +1 for a box's bottom, -1 for its top.
	side := func(up, isFrom, same bool) int {
		if !same && isFrom == up {
			return -1
		}
		return 1
	}
	members, ends := map[slotKey][]int{}, map[slotKey][]int{}
	leave, arrive := map[slotKey][]int{}, map[slotKey][]int{}
	for i, r := range reqs {
		p := &plans[i]
		p.dep, p.arr = -1, -1
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
		fk := slotKey{r.from.key, side(p.up, true, p.same)}
		tk := slotKey{r.to.key, side(p.up, false, p.same)}
		members[fk] = append(members[fk], i)
		ends[tk] = append(ends[tk], i)
		if p.same {
			continue
		}
		// A box's lines bus by the box, whichever of its chips they use.
		dk, ak := fk, tk
		if r.from.node != "" {
			dk.key = "node|" + r.from.node
		}
		if r.to.node != "" {
			ak.key = "node|" + r.to.node
		}
		dk.key += "|" + r.kind
		ak.key += "|" + r.kind
		leave[dk] = append(leave[dk], i)
		arrive[ak] = append(arrive[ak], i)
	}
	var buses []*bus
	for _, k := range sortedKeys(leave) {
		if is := leave[k]; len(is) >= 2 {
			for _, i := range is {
				plans[i].dep = len(buses)
			}
			// The trunk runs on through every row two of its lines still
			// pass, and each line leaves it in the gap above the first row
			// it would pass alone.
			deep := 0
			for j := 1; ; j++ {
				c := 0
				for _, i := range is {
					if len(plans[i].gaps) > j {
						c++
					}
				}
				if c < 2 {
					break
				}
				deep = j
			}
			for _, i := range is {
				plans[i].ride = min(deep, len(plans[i].gaps)-1)
			}
			buses = append(buses, &bus{members: is, passage: map[int]int{}, deep: deep})
		}
	}
	// Lines bound for boxes in one row, of one kind and from exactly the
	// same places, share one arrival trunk that splits only in the gap
	// above those boxes. A trunk shared by lines from different places
	// would draw a path from each of them to every box it splits to.
	type gather struct {
		members  []int
		from     map[string]bool
		row, way int
		kind     string
	}
	var gathers []*gather
	for _, k := range sortedKeys(arrive) {
		is := arrive[k]
		to := reqs[is[0]].to
		g := &gather{members: is, from: map[string]bool{}, row: to.row, way: k.side, kind: reqs[is[0]].kind}
		for _, i := range is {
			g.from[reqs[i].from.key] = true
		}
		joined := false
		for _, h := range gathers {
			if to.node == "" || reqs[h.members[0]].to.node == "" || h.row != g.row || h.way != g.way || h.kind != g.kind {
				continue
			}
			if maps.Equal(g.from, h.from) {
				h.members = append(h.members, is...)
				joined = true
				break
			}
		}
		if !joined {
			gathers = append(gathers, g)
		}
	}
	for _, g := range gathers {
		if len(g.members) >= busAt {
			for _, i := range g.members {
				plans[i].arr = len(buses)
			}
			buses = append(buses, &bus{members: g.members, arrive: true, passage: map[int]int{}})
		}
	}
	center := func(e end) int { return (e.lo + e.hi) / 2 }
	// place spreads the lines over each end's range: one slot per bus, one
	// per other line, ordered by where they lead. Lines that leave an edge
	// and lines that reach it share the edge, so no two of them meet there.
	type slot struct {
		is    []int
		leave bool
	}
	edges := map[slotKey][]int{}
	for k := range members {
		edges[k] = nil
	}
	for k := range ends {
		edges[k] = nil
	}
	for _, k := range sortedKeys(edges) {
		var slots []slot
		seen := map[[2]int]int{}
		add := func(i, b int, leave bool) {
			if b >= 0 {
				if j, ok := seen[[2]int{b, map[bool]int{true: 1}[leave]}]; ok {
					slots[j].is = append(slots[j].is, i)
					return
				}
				seen[[2]int{b, map[bool]int{true: 1}[leave]}] = len(slots)
			}
			slots = append(slots, slot{[]int{i}, leave})
		}
		for _, i := range members[k] {
			add(i, plans[i].dep, true)
		}
		for _, i := range ends[k] {
			add(i, plans[i].arr, false)
		}
		other := func(sl slot) int {
			sum := 0
			for _, i := range sl.is {
				if sl.leave {
					sum += center(reqs[i].to)
				} else {
					sum += center(reqs[i].from)
				}
			}
			return sum / len(sl.is)
		}
		sort.SliceStable(slots, func(a, b int) bool {
			if oa, ob := other(slots[a]), other(slots[b]); oa != ob {
				return oa < ob
			}
			return slots[a].is[0] < slots[b].is[0]
		})
		r := reqs[slots[0].is[0]].to
		if slots[0].leave {
			r = reqs[slots[0].is[0]].from
		}
		for s, sl := range slots {
			x := r.lo + (r.hi-r.lo)*(2*s+1)/(2*len(slots))
			for _, i := range sl.is {
				if sl.leave {
					plans[i].fx = x
				} else {
					plans[i].tx = x
				}
			}
		}
	}

	// The x of each passage through a row. A line on a departure trunk
	// passes the rows it rides on the trunk, which heads row by row from
	// where it leaves toward the middle of the places its lines still
	// reach. A line on an arrival bus passes
	// every other row on the bus's trunk, which runs straight above the
	// middle of the places it reaches.
	passRow := func(p *plan, j int) int {
		if p.up {
			return p.gaps[j-1] - 1
		}
		return p.gaps[j-1]
	}
	for i := range reqs {
		p := &plans[i]
		p.xs = []int{p.fx}
		k := len(p.gaps)
		for j := 1; j < k; j++ {
			r := passRow(p, j)
			if p.dep >= 0 && j <= p.ride {
				b := buses[p.dep]
				x, ok := b.passage[r]
				if !ok {
					sum, n := 0, 0
					for _, m := range b.members {
						if plans[m].ride >= j {
							sum, n = sum+plans[m].tx, n+1
						}
					}
					x = l.gutter(r, p.fx+(sum/n-p.fx)*j/(b.deep+1))
					b.passage[r] = x
				}
				p.xs = append(p.xs, x)
				continue
			}
			if p.arr >= 0 {
				b := buses[p.arr]
				x, ok := b.passage[r]
				if !ok {
					lo, hi := 1<<30, -1<<30
					for _, m := range b.members {
						lo, hi = min(lo, plans[m].tx), max(hi, plans[m].tx)
					}
					x = l.gutter(r, (lo+hi)/2)
					b.passage[r] = x
				}
				p.xs = append(p.xs, x)
				continue
			}
			p.xs = append(p.xs, l.gutter(r, p.fx+(p.tx-p.fx)*j/k))
		}
		p.xs = append(p.xs, p.tx)
		// A line that rides its departure trunk into its last gap leaves
		// it where the trunk crosses that gap and steps down onto its
		// arrival bus's track there, so lines from different trunks never
		// share one drop.
		if p.dep >= 0 && p.arr >= 0 && p.ride == k-1 {
			p.xs[k] = p.xs[k-1]
		}
	}

	// owner says whose track crossing j of line i runs on: its departure
	// bus while it rides that trunk, its arrival bus after it has joined
	// that trunk, else its own. A line that rides its departure trunk into
	// its last gap crosses on it to the drop and meets the arrival bus
	// there.
	busOwner := func(b int) int { return -1 - b }
	owner := func(i, j int) int {
		p := &plans[i]
		switch {
		case p.dep >= 0 && j <= p.ride:
			return busOwner(p.dep)
		case p.arr >= 0 && (j > 0 || len(p.gaps) == 1):
			return busOwner(p.arr)
		}
		return i
	}
	type span struct {
		lo, hi      int
		owner       int
		tops, downs []int
	}
	spans := make([]map[int]*span, n+1)
	owners := make([][]int, n+1)
	// above[g] holds owner pairs whose first must lie above the second.
	above := make([]map[[2]int]bool, n+1)
	addSpan := func(g, o, a, b int, up, same bool) {
		if spans[g] == nil {
			spans[g] = map[int]*span{}
			above[g] = map[[2]int]bool{}
		}
		sp, ok := spans[g][o]
		if !ok {
			sp = &span{lo: min(a, b), hi: max(a, b), owner: o}
			spans[g][o] = sp
			owners[g] = append(owners[g], o)
		}
		sp.lo, sp.hi = min(sp.lo, a, b), max(sp.hi, a, b)
		switch {
		case same:
			sp.tops = append(sp.tops, a, b)
		case up:
			sp.downs, sp.tops = append(sp.downs, a), append(sp.tops, b)
		default:
			sp.tops, sp.downs = append(sp.tops, a), append(sp.downs, b)
		}
	}
	for i := range reqs {
		p := &plans[i]
		for j, g := range p.gaps {
			a, b := p.xs[j], p.xs[j+1]
			o := owner(i, j)
			// A line bound for a trunk keeps a track even when it runs
			// straight down to it: the trunk starts there.
			if a == b && !p.same && o == i && p.arr < 0 {
				continue
			}
			addSpan(g, o, a, b, p.up, p.same)
		}
		if p.dep >= 0 && p.arr >= 0 && p.ride == len(p.gaps)-1 {
			g := p.gaps[p.ride]
			addSpan(g, busOwner(p.arr), p.xs[p.ride], p.tx, p.up, false)
			d, a := busOwner(p.dep), busOwner(p.arr)
			if p.up {
				d, a = a, d
			}
			above[g][[2]int{d, a}] = true
		}
	}
	// A line that leaves its departure trunk for an arrival trunk joins
	// that trunk at its top: every other line that joins the trunk in the
	// same gap takes a track below it. A trunk that ran on above the join
	// would let a reader follow those other lines back along the departure
	// track into places they never reach.
	for i := range reqs {
		p := &plans[i]
		if p.dep < 0 || p.arr < 0 || p.up || p.ride >= len(p.gaps)-1 {
			continue
		}
		g, r := p.gaps[p.ride], passRow(p, p.ride+1)
		for _, m := range buses[p.arr].members {
			q := &plans[m]
			for j := 1; j < len(q.gaps); j++ {
				if m != i && q.gaps[j-1] == g && passRow(q, j) == r && owner(m, j) == busOwner(p.arr) && owner(m, j-1) != owner(i, p.ride) {
					above[g][[2]int{owner(i, p.ride), owner(m, j-1)}] = true
				}
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
	track := map[[2]int]int{}
	tracks := make([]int, n+1)
	for g := range spans {
		// cost is how many crossings putting a above b makes between them.
		cost := func(a, b *span) int {
			c := inside(a.downs, b) + inside(b.tops, a)
			if above[g][[2]int{b.owner, a.owner}] {
				c += 1 << 16
			}
			return c
		}
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
	// A bus's count stands beside its stub or its drop, rule 22, so the gap
	// it stands in keeps room for it between the box and the nearest track.
	room := make([][2]int, n+1)
	for _, bs := range buses {
		p := &plans[bs.members[0]]
		g, top := p.gaps[0], !p.up
		if bs.arrive {
			reach, counted := map[string]int{}, false
			for _, i := range bs.members {
				reach[reqs[i].to.key]++
				counted = counted || reach[reqs[i].to.key] > 1
				if len(plans[i].gaps) > len(p.gaps) {
					p = &plans[i]
				}
			}
			if !counted {
				continue
			}
			g, top = p.gaps[len(p.gaps)-1], p.up
		}
		if top {
			room[g][0] = countRoom
		} else {
			room[g][1] = countRoom
		}
	}
	l.place(tracks, room)
	trackY := func(g, o int) int {
		t := track[[2]int{g, o}]
		return l.gapTop[g] + room[g][0] + lane*(t+1) + t/bundle*lane
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
	startOf := func(i int) pt {
		p := &plans[i]
		return pt{p.fx, endY(reqs[i].from, side(p.up, true, p.same))}
	}
	endOf := func(i int) pt {
		p := &plans[i]
		return pt{p.tx, endY(reqs[i].to, side(p.up, false, p.same))}
	}

	back := func(i int) bool {
		p := &plans[i]
		against := p.up
		if p.same {
			against = p.tx < p.fx
		}
		return against != (reqs[i].kind == "provides")
	}
	var out []drawn
	nets := 0
	// A line's own part: from its start, or where it leaves its departure
	// bus, to its end, or where it joins its arrival bus.
	for i := range reqs {
		p := &plans[i]
		var pts []pt
		st := noStart
		k := len(p.gaps)
		j := 0
		if p.dep >= 0 {
			j = p.ride + 1
			pts = append(pts, pt{p.xs[j], trackY(p.gaps[j-1], busOwner(p.dep))})
		} else {
			st = startOf(i)
			pts = append(pts, st)
		}
		joins := false
		for ; j < k; j++ {
			o := owner(i, j)
			if o != i {
				// The trunk draws the way down through a row; a one-gap
				// line steps onto the track itself.
				if k == 1 {
					pts = append(pts, pt{p.fx, trackY(p.gaps[j], o)})
				}
				joins = true
				break
			}
			if a, b := p.xs[j], p.xs[j+1]; a != b || p.same || p.arr >= 0 {
				ty := trackY(p.gaps[j], i)
				pts = append(pts, pt{a, ty}, pt{b, ty})
			}
		}
		if p.dep >= 0 && p.arr >= 0 && p.ride == k-1 {
			pts = append(pts, pt{p.xs[k], trackY(p.gaps[k-1], busOwner(p.arr))})
			joins = true
		}
		if !joins {
			pts = append(pts, endOf(i))
		}
		if len(pts) < 2 || pts[0] == pts[len(pts)-1] {
			continue
		}
		out = append(out, drawn{D: polyline(pts), parts: [][]pt{pts}, reqs: []int{i}, starts: []pt{st}, joins: joins, back: back(i)})
	}
	// Departure buses: one stub out of the box, the trunk down through the
	// rows its lines share, and a track in each gap where some leave it.
	for b, bs := range buses {
		if bs.arrive {
			continue
		}
		nets++
		o := busOwner(b)
		first := plans[bs.members[0]]
		s := startOf(bs.members[0])
		var starts []pt
		var parts [][]pt
		span := map[int][2]int{}
		var gaps []int
		rows := map[int]bool{}
		for _, i := range bs.members {
			p := &plans[i]
			st := startOf(i)
			if !slices.Contains(starts, st) {
				parts = append(parts, []pt{st, {st.x, trackY(p.gaps[0], o)}})
			}
			starts = append(starts, st)
			for j := 0; j <= p.ride; j++ {
				next := p.xs[j+1]
				g := p.gaps[j]
				lh, ok := span[g]
				if !ok {
					lh = [2]int{next, next}
					gaps = append(gaps, g)
				}
				span[g] = [2]int{min(lh[0], p.xs[j], next), max(lh[1], p.xs[j], next)}
				if j > 0 && !rows[passRow(p, j)] {
					rows[passRow(p, j)] = true
					parts = append(parts, []pt{{p.xs[j], trackY(p.gaps[j-1], o)}, {p.xs[j], trackY(g, o)}})
				}
			}
		}
		for _, g := range gaps {
			if lh := span[g]; lh[0] < lh[1] {
				y := trackY(g, o)
				parts = append(parts, []pt{{lh[0], y}, {lh[1], y}})
			}
		}
		var ds []string
		for _, pp := range parts {
			ds = append(ds, polyline(pp))
		}
		lb := pt{s.x + 5, s.y + 14}
		if first.up {
			lb.y = s.y - 6
		}
		out = append(out, drawn{D: strings.Join(ds, " "), parts: parts, reqs: bs.members, starts: starts, net: nets, count: len(bs.members), label: lb, joins: true, back: back(bs.members[0]), own: [2]pt{s, {s.x, trackY(first.gaps[0], o)}}})
	}
	// Arrival buses: the trunk and the track, then a drop into each place.
	for b, bs := range buses {
		if !bs.arrive {
			continue
		}
		nets++
		o := busOwner(b)
		lead := bs.members[0]
		for _, i := range bs.members {
			if len(plans[i].gaps) > len(plans[lead].gaps) {
				lead = i
			}
		}
		lp := &plans[lead]
		y := trackY(lp.gaps[len(lp.gaps)-1], o)
		// The trunk passes each row from the highest line that joins it
		// there to the bus's track in the gap after it, and steps across
		// that gap to its passage through the next row.
		type passage struct{ x, reach, ty, next int }
		trunkAt := map[int]*passage{}
		var rows []int
		for _, i := range bs.members {
			p := &plans[i]
			for j := 1; j < len(p.gaps); j++ {
				if owner(i, j) != o {
					continue
				}
				r := passRow(p, j)
				from, to := trackY(p.gaps[j-1], owner(i, j-1)), trackY(p.gaps[j], o)

				ps, ok := trunkAt[r]
				if !ok {
					ps = &passage{x: p.xs[j], reach: from, ty: to, next: p.xs[j+1]}
					trunkAt[r] = ps
					rows = append(rows, r)
				}
				if abs(from-to) > abs(ps.reach-to) {
					ps.reach = from
				}
			}
		}
		slices.Sort(rows)
		var trunk [][]pt

		lo, hi := 1<<30, -1<<30
		for _, r := range rows {
			ps := trunkAt[r]
			trunk = append(trunk, []pt{{ps.x, ps.reach}, {ps.x, ps.ty}})
			if ps.ty == y {
				lo, hi = min(lo, ps.x), max(hi, ps.x)
			} else if ps.next != ps.x {
				trunk = append(trunk, []pt{{ps.x, ps.ty}, {ps.next, ps.ty}})
			}
		}
		drops := map[int][]int{}
		var xs []int
		none := make([]pt, len(bs.members))
		for m, i := range bs.members {
			p := &plans[i]
			none[m] = noStart
			entry := p.xs[len(p.xs)-2]
			if owner(i, len(p.gaps)-1) != o {
				entry = p.xs[len(p.xs)-1]
			}
			lo, hi = min(lo, entry, p.tx), max(hi, entry, p.tx)
			if _, ok := drops[p.tx]; !ok {
				xs = append(xs, p.tx)
			}
			drops[p.tx] = append(drops[p.tx], i)
		}
		parts := trunk
		if lo < hi {
			parts = append(parts, []pt{{lo, y}, {hi, y}})
		}
		if len(parts) > 0 {
			var ds []string
			for _, pp := range parts {
				ds = append(ds, polyline(pp))
			}
			out = append(out, drawn{D: strings.Join(ds, " "), parts: parts, reqs: bs.members, starts: none, net: nets, joins: true, back: back(lead)})
		}
		slices.Sort(xs)
		// Each box the bus reaches shows how many of its lines go there.
		reach := map[string]int{}
		for _, i := range bs.members {
			reach[reqs[i].to.key]++
		}
		for _, x := range xs {
			is := drops[x]
			e := endOf(is[0])
			drop := []pt{{x, y}, e}
			dd := drawn{D: polyline(drop), parts: [][]pt{drop}, reqs: is, starts: none[:len(is)], net: nets, back: back(is[0])}
			if n := reach[reqs[is[0]].to.key]; n > 0 {
				dd.count, dd.label, dd.own = n, pt{x + 5, (y+e.y)/2 + 4}, [2]pt{{x, y}, e}
				delete(reach, reqs[is[0]].to.key)
			}
			out = append(out, dd)
		}
	}
	for k := range out {
		out[k].kind = reqs[out[k].reqs[0]].kind
	}
	for i := range reqs {
		if p := &plans[i]; p.dep >= 0 && p.arr >= 0 && p.ride < len(p.gaps)-1 && len(out) > 0 {
			at := pt{p.xs[p.ride+1], trackY(p.gaps[p.ride], busOwner(p.dep))}
			out[0].joinsAt = append(out[0].joinsAt, join{at, hue(reqs[i].kind)})
		}
	}
	var boxes []box
	for _, r := range l.pos {
		boxes = append(boxes, box{r.X, r.Y, r.W, r.H})
	}
	sort.Slice(boxes, func(i, j int) bool {
		return boxes[i].y < boxes[j].y || boxes[i].y == boxes[j].y && boxes[i].x < boxes[j].x
	})
	anchor(out, pieces(out), boxes)
	return out
}

// wanted are the pairs of places the requests join: a box, or a frame
// port. A line between two modules that use each other stands for both
// directions.
func wanted(reqs []wireReq) [][2]string {
	id := func(e end) string {
		if e.node != "" {
			return e.node
		}
		return e.key
	}
	var out [][2]string
	for _, r := range reqs {
		out = append(out, [2]string{id(r.from), id(r.to)})
		if r.kind == "mutual" {
			out = append(out, [2]string{id(r.to), id(r.from)})
		}
	}
	return out
}

// countRoom is how far a bus's track keeps from the box whose stub or drop
// carries the bus's count: the count's height and a clear margin.
const countRoom = 16

// place sets each gap's height from its tracks and the room its counts
// need above and below them, and each row below it.
func (l *layout) place(tracks []int, room [][2]int) {
	n := len(l.rows)
	l.gapTop, l.gapH = make([]int, n+1), make([]int, n+1)
	l.rowTop, l.rowH = make([]int, n), make([]int, n)
	y := l.top
	for g := 0; g <= n; g++ {
		h := l.minGap
		if g == 0 || g == n {
			h = l.endGap
		}
		if tracks[g] > 0 {
			h = max(h, lane*(tracks[g]+1)+lane/2+(tracks[g]-1)/bundle*lane+room[g][0]+room[g][1])
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
}
