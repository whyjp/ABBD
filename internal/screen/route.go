package screen

import (
	"fmt"
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
//   - Boxes that the same two or more places all reach may share one
//     arrival bus instead (share), kept only where it spares a reader
//     turns at no other cost (route).
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
	// via is where a line that leaves its departure trunk for an arrival
	// trunk below that trunk's top steps off the departure track, or -1.
	via int
	// onto says the line's stub drops straight onto its arrival bus's
	// track: every line its box sends that way rides that one bus.
	onto bool
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
	// lead is the line whose chain the trunk runs down, len(reqs) for none
	// and -1 before it is chosen.
	lead int
	// fed is, for an arrival bus into a box a shared bus also feeds, that
	// shared bus: it draws the drop, and this bus's track ends on it. -1
	// for none.
	fed int
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

// route draws every request. Where lines could share arrival buses, it
// draws the view both ways and keeps the shared buses only when they spare
// the reader turns and cost nothing else, as weigh measures it. weigh
// compares every pair of strands, so a view with more than shareMost
// requests keeps the plain drawing rather than pay for that.
func (l *layout) route(reqs []wireReq) []drawn {
	plain := l.lay(reqs, false)
	if !plain.shares || len(reqs) > shareMost {
		return plain.drawn
	}
	if shared := l.lay(reqs, true); weigh(shared, reqs).beats(weigh(plain, reqs)) {
		return shared.drawn
	}
	return l.lay(reqs, false).drawn
}

const shareMost = 200

// routing is one way to draw a view's lines.
type routing struct {
	drawn []drawn
	// ways are the points each line turns at on its way from its start to
	// its end, along the ink a reader follows, its buses' included.
	ways [][]pt
	// shares says some lines could share an arrival bus.
	shares bool
}

// lay draws every request, with shared arrival buses where shared allows
// them.
func (l *layout) lay(reqs []wireReq, shared bool) routing {
	l.lanes, l.beside, l.passes, l.stubs = map[[2]int]int{}, map[int][]int{}, map[int][]int{}, map[int][]int{}
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
		p.dep, p.arr, p.via = -1, -1, -1
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
		// Frame ports folded into one chip are one place: they share its
		// slots and its buses.
		at := func(e end, k slotKey) slotKey {
			k.key = place(e)
			return k
		}
		members[at(r.from, fk)] = append(members[at(r.from, fk)], i)
		ends[at(r.to, tk)] = append(ends[at(r.to, tk)], i)
		if p.same {
			continue
		}
		// A box's lines bus by the box, whichever of its chips they use,
		// and a chip's by the chip, whichever folded ports it stands for.
		dk, ak := at(r.from, fk), at(r.to, tk)
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
	// A line between places more than a row apart runs down its chain:
	// the dummies the layout set straight for it.
	chainOf := make([]int, len(reqs))
	{
		byPair := map[[2]string]int{}
		for c, ch := range l.chains {
			byPair[pairKey(ch.upper, ch.lower)] = c
		}
		id := func(e end) string {
			switch {
			case e.node != "":
				return e.node
			case e.row < 0:
				return frameTopNode
			}
			return frameBottomNode
		}
		for i, r := range reqs {
			chainOf[i] = -1
			c, ok := byPair[pairKey(id(r.from), id(r.to))]
			if !ok {
				continue
			}
			ch := l.chains[c]
			if top, bottom := min(r.from.row, r.to.row)+1, max(r.from.row, r.to.row)-1; ch.top == top && ch.bottom == bottom {
				chainOf[i] = c
			}
		}
		// A frame's chips are set out after the layout, each over the
		// mean of its lines, so a line to a chip may find its chain far off
		// the way between its ends; it passes the rows on its own instead.
		for i, r := range reqs {
			c := chainOf[i]
			if c < 0 {
				continue
			}
			if r.from.node != "" && r.to.node != "" {
				continue
			}
			ch := l.chains[c]
			a, b := (r.from.lo+r.from.hi)/2, (r.to.lo+r.to.hi)/2
			for row := ch.top; row <= ch.bottom; row++ {
				x := l.pass[[2]int{c, row}]
				if abs(x-a)+abs(x-b)-abs(a-b) > detourOffer {
					chainOf[i] = -1
					break
				}
			}
		}
		// A chain no line runs down keeps no room: a line that left it
		// passes the rows on its own and may take its place.
		l.reserved = map[int][]int{}
		for c, ch := range l.chains {
			if !slices.Contains(chainOf, c) {
				continue
			}
			for r := ch.top; r <= ch.bottom; r++ {
				for k := 0; k < ch.lanes; k++ {
					l.reserved[r] = append(l.reserved[r], l.pass[[2]int{c, r}]+k*chainLane)
				}
			}
		}
	}
	// laneOf is the lane each line takes down its chain: the first lane
	// while trunks choose their leads, then as handed out below.
	laneOf := map[int]int{}
	for i, c := range chainOf {
		if c >= 0 {
			laneOf[i] = 0
		}
	}
	// near is the x where line i runs down its chain in the row next to its
	// start, or its end.
	near := func(i int, start bool) (int, bool) {
		c := chainOf[i]
		if k, ok := laneOf[i]; c < 0 || !ok || k >= l.chains[c].lanes {
			return 0, false
		}
		ch := l.chains[c]
		r := reqs[i]
		row := ch.bottom
		if start == (r.from.row < r.to.row) {
			row = ch.top
		}
		return l.pass[[2]int{c, row}] + laneOf[i]*chainLane, true
	}
	// held says who passes each row at each x: a line by its index, a bus
	// by -1-b. Two of them at one x in one row, or in rows next to each
	// other, would meet and read as one.
	held := map[[2]int]int{}
	// runs is where line i, or the bus owner draws it for, runs down i's
	// chain in row r, if it has room there.
	runs := func(i, r, owner int) (int, bool) {
		if i >= len(reqs) || chainOf[i] < 0 {
			return 0, false
		}
		c := chainOf[i]
		k, ok := laneOf[i]
		if !ok || k >= l.chains[c].lanes {
			return 0, false
		}
		x, ok := l.pass[[2]int{c, r}]
		if !ok {
			return 0, false
		}
		x += k * chainLane
		for _, q := range []int{r - 1, r, r + 1} {
			if o, ok := held[[2]int{q, x}]; ok && o != owner {
				return 0, false
			}
		}
		held[[2]int{r, x}] = owner
		l.passes[r] = append(l.passes[r], x)
		return x, true
	}
	shareOf, shares := share(reqs, arrive)
	can := len(shares) > 0
	if shared = shared && can; !shared {
		for i := range shareOf {
			shareOf[i] = -1
		}
		shares = nil
	}
	var buses []*bus
	for _, k := range sortedKeys(leave) {
		if is := leave[k]; len(is) >= 2 {
			for _, i := range is {
				plans[i].dep = len(buses)
			}
			// A box whose lines all ride one shared bus drops its stub
			// straight onto that bus's track.
			if s := shareOf[is[0]]; s >= 0 && !slices.ContainsFunc(is, func(i int) bool { return shareOf[i] != s }) {
				for _, i := range is {
					plans[i].onto = true
				}
				buses = append(buses, &bus{members: is, passage: map[int]int{}, lead: -1})
				continue
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
			buses = append(buses, &bus{members: is, passage: map[int]int{}, deep: deep, lead: -1})
		}
	}
	// Each shared bus gathers its places' lines into one track and feeds
	// every box on it. Every other line into one of those boxes joins the
	// bus's drop into it, on a bus of its own whose track ends on the drop.
	fedBy := map[string]int{}
	for _, is := range shares {
		for _, i := range is {
			plans[i].arr = len(buses)
			fedBy[place(reqs[i].to)+"|"+reqs[i].kind] = len(buses)
		}
		buses = append(buses, &bus{members: is, arrive: true, passage: map[int]int{}, lead: -1, fed: -1})
	}
	// Lines that all ride one departure bus, one into each box, already
	// travel as one; they split off its track straight into their boxes,
	// without a second bus.
	oneBus := func(is []int) bool {
		seen := map[string]bool{}
		for _, i := range is {
			if d := plans[i].dep; d < 0 || d != plans[is[0]].dep || seen[place(reqs[i].to)] {
				return false
			}
			seen[place(reqs[i].to)] = true
		}
		return true
	}
	for _, is := range gather(reqs, arrive, shared) {
		fed := -1
		is = slices.DeleteFunc(is, func(i int) bool { return shareOf[i] >= 0 })
		for _, i := range is {
			if b, ok := fedBy[place(reqs[i].to)+"|"+reqs[i].kind]; ok && !plans[i].up {
				fed = b
			}
		}
		if len(is) > 0 && (fed >= 0 || len(is) >= busAt && !oneBus(is)) {
			for _, i := range is {
				plans[i].arr = len(buses)
			}
			buses = append(buses, &bus{members: is, arrive: true, passage: map[int]int{}, lead: -1, fed: fed})
		}
	}
	// drop is the bus that draws the drop line i ends on: the shared bus
	// that feeds the place it reaches, else its own.
	drop := func(i int) int {
		if b := plans[i].arr; b >= 0 && buses[b].fed >= 0 {
			if f, ok := fedBy[place(reqs[i].to)+"|"+reqs[i].kind]; ok && !plans[i].up {
				return f
			}
		}
		return plans[i].arr
	}
	center := func(e end) int { return (e.lo + e.hi) / 2 }
	// A trunk runs down the chain of one of its lines, its lead: for a
	// departure bus the deepest line whose chain starts nearest the box,
	// for an arrival bus the line that joins it first, whose chain ends
	// nearest the boxes it reaches. The bus's stub, or its drop, stands
	// right above or below that chain, so the trunk runs straight.
	for _, b := range buses {
		b.lead = len(reqs)
		var at int
		if b.arrive {
			lo, hi := 1<<30, -1<<30
			for _, m := range b.members {
				lo, hi = min(lo, center(reqs[m].to)), max(hi, center(reqs[m].to))
			}
			at = (lo + hi) / 2
		} else {
			at = center(reqs[b.members[0]].from)
		}
		// own is how many rows the line passes on this bus's trunk.
		own := func(m int) int {
			q := &plans[m]
			if !b.arrive {
				return q.ride
			}
			if q.dep >= 0 {
				return len(q.gaps) - 1 - q.ride
			}
			return len(q.gaps) - 1
		}
		for _, m := range b.members {
			x, ok := near(m, !b.arrive)
			if !ok || own(m) == 0 || !b.arrive && own(m) < b.deep {
				continue
			}
			if b.lead == len(reqs) {
				b.lead = m
				continue
			}
			lx, _ := near(b.lead, !b.arrive)
			if b.arrive && own(m) != own(b.lead) {
				if own(m) > own(b.lead) {
					b.lead = m
				}
				continue
			}
			if abs(x-at) < abs(lx-at) {
				b.lead = m
			}
		}
	}
	// A chain's lanes go to the lines that run down it themselves: each
	// trunk's lead first, then every line that passes rows off any trunk.
	// A line that rides trunks all the way needs no lane of its own.
	{
		used := map[int]int{}
		laneOf = map[int]int{}
		claim := func(i int) {
			if c := chainOf[i]; c >= 0 {
				if _, ok := laneOf[i]; !ok {
					laneOf[i] = used[c]
					used[c]++
				}
			}
		}
		for _, b := range buses {
			if b.lead < len(reqs) {
				claim(b.lead)
			}
		}
		for i := range reqs {
			if p := &plans[i]; p.arr < 0 && (p.dep < 0 || p.ride < len(p.gaps)-1) {
				claim(i)
			}
		}
	}
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
	// used holds the x every line takes on each edge of each box: a box's
	// chips and the box itself spread their lines apart, and a line of one
	// must not stand on a line of the other.
	used := map[[2]string][]int{}
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
			add(i, drop(i), false)
		}
		// A line alone in its slot that runs down a chain, or a bus whose
		// trunk does, wants to leave or reach the box right above or below
		// that chain, so it drops straight.
		pin := func(sl slot) (int, bool) {
			i := sl.is[0]
			if b := plans[i].dep; sl.leave && b >= 0 {
				i = buses[b].lead
			} else if b := drop(i); !sl.leave && b >= 0 {
				i = buses[b].lead
			} else if len(sl.is) != 1 {
				return 0, false
			}
			if i >= len(reqs) {
				return 0, false
			}
			return near(i, sl.leave)
		}
		other := func(sl slot) int {
			if x, ok := pin(sl); ok {
				return x
			}
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
		// Slots spread evenly, a pinned one moves over its chain, and every
		// slot keeps its order, a gap from the next, and clear of the
		// chains passing the row next to the edge, which its stub would
		// otherwise run into.
		adj := r.row + k.side
		edgeOf := [2]string{r.node, fmt.Sprint(k.side)}
		apart := func(x int) bool {
			for _, y := range used[edgeOf] {
				if abs(x-y) < slotGap {
					return false
				}
			}
			return true
		}
		xs := make([]int, len(slots))
		pinned := make([]bool, len(slots))
		for s, sl := range slots {
			xs[s] = r.lo + (r.hi-r.lo)*(2*s+1)/(2*len(slots))
			if x, ok := pin(sl); ok && x >= r.lo && x <= r.hi && (r.node == "" || apart(x)) {
				xs[s], pinned[s] = x, true
			}
		}
		clear := func(s, x int) bool {
			if r.node != "" && !apart(x) {
				return false
			}
			for _, y := range l.reserved[adj] {
				if abs(x-y) < lane && !(pinned[s] && x == xs[s]) {
					return false
				}
			}
			return true
		}
		for s := range slots {
			if pinned[s] {
				continue
			}
			for d := 0; d <= r.hi-r.lo; d += 2 {
				if x := xs[s] + d; x <= r.hi && clear(s, x) {
					xs[s] = x
					break
				}
				if x := xs[s] - d; x >= r.lo && clear(s, x) {
					xs[s] = x
					break
				}
			}
		}
		gap := min(slotGap, (r.hi-r.lo)/max(1, len(xs)))
		edge := r.lo - gap
		for s := range xs {
			xs[s] = max(xs[s], edge+gap)
			edge = xs[s]
		}
		edge = r.hi + gap
		for s := len(xs) - 1; s >= 0; s-- {
			xs[s] = min(xs[s], edge-gap)
			edge = xs[s]
		}
		if r.node != "" {
			used[edgeOf] = append(used[edgeOf], xs...)
		}
		l.stubs[adj] = append(l.stubs[adj], xs...)
		for s, sl := range slots {
			x := xs[s]
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
					if cx, ok := runs(b.lead, r, -1-p.dep); ok {
						x = cx
					} else {
						sum, n := 0, 0
						for _, m := range b.members {
							if plans[m].ride >= j {
								sum, n = sum+plans[m].tx, n+1
							}
						}
						x = l.gutter(r, p.fx+(sum/n-p.fx)*j/(b.deep+1))
						held[[2]int{r, x}] = -1 - p.dep
					}
					b.passage[r] = x
				}
				p.xs = append(p.xs, x)
				continue
			}
			if p.arr >= 0 {
				b := buses[p.arr]
				x, ok := b.passage[r]
				if !ok {
					// A shared bus's trunk heads for the boxes it still
					// reaches past this row.
					lo, hi := 1<<30, -1<<30
					for _, m := range b.members {
						if to := reqs[m].to.row; !shared || plans[m].up && to < r || !plans[m].up && to > r {
							lo, hi = min(lo, plans[m].tx), max(hi, plans[m].tx)
						}
					}
					if cx, ok := runs(b.lead, r, -1-p.arr); ok {
						x = cx
					} else {
						x = l.gutter(r, (lo+hi)/2)
						held[[2]int{r, x}] = -1 - p.arr
					}
					b.passage[r] = x
				}
				p.xs = append(p.xs, x)
				continue
			}
			if x, ok := runs(i, r, i); ok {
				p.xs = append(p.xs, x)
				continue
			}
			x := l.gutterTo(r, p.xs[len(p.xs)-1], p.tx)
			held[[2]int{r, x}] = i
			p.xs = append(p.xs, x)
		}
		p.xs = append(p.xs, p.tx)
	}
	// A line that rides its departure trunk into its last gap steps down
	// from the trunk's track onto its arrival bus's track. Lines of one
	// departure bus bound for one arrival bus step down together, and each
	// such pair of buses steps down at an x of its own, clear of the lines
	// passing the rows on either side: two steps at one x would overlap,
	// and a reader could follow one bus's lines down the other's step into
	// places they never reach. Where the trunk ends, the first pair steps
	// straight on down it and the others a lane or more beside it, toward
	// their ends. Where the trunk runs on below, every pair steps down
	// above its end instead: an arrival track that ended on that trunk
	// would join the two the same way.
	{
		type step struct{ dep, arr, gap int }
		var steps []step
		for i := range plans {
			p := &plans[i]
			if k := len(p.gaps); p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == k-1 {
				st := step{p.dep, p.arr, p.gaps[k-1]}
				if !slices.Contains(steps, st) {
					steps = append(steps, st)
				}
			}
		}
		at := map[step]int{}
		for _, st := range steps {
			lead := -1
			for i := range plans {
				if p, k := &plans[i], len(plans[i].gaps); p.dep == st.dep && p.arr == st.arr && p.ride == k-1 && p.gaps[k-1] == st.gap {
					lead = i
					break
				}
			}
			p := &plans[lead]
			k := len(p.gaps)
			g := p.gaps[k-1]
			free := func(x int) bool {
				for _, r := range []int{g - 1, g} {
					for _, y := range l.passes[r] {
						if abs(x-y) < lane && y != p.xs[k-1] {
							return false
						}
					}
				}
				// A step down beside another box's stub would run into it.
				for _, y := range l.stubs[g] {
					if shared && abs(x-y) < lane && y != p.xs[k-1] {
						return false
					}
				}
				for o, y := range at {
					if o != st && abs(x-y) < lane {
						return false
					}
				}
				return true
			}
			base := p.xs[k-1]
			if buses[st.dep].deep > p.ride {
				base = p.tx
			}
			at[st] = stepX(base, p.tx, free)
		}
		for i := range plans {
			p := &plans[i]
			if k := len(p.gaps); p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == k-1 {
				p.xs[k] = at[step{p.dep, p.arr, p.gaps[k-1]}]
			}
		}
	}

	// Only one departure track may meet an arrival trunk, at the trunk's
	// top: a track that met it lower down would let a reader follow the
	// trunk's lines from above onto that track and into the other places
	// its bus reaches. Every other line that leaves a departure trunk for
	// the arrival trunk steps down off its departure track a little before
	// the trunk, onto a track of its own that ends on the trunk; a reader
	// coming along the trunk cannot climb that step.
	vias := map[int][]int{}
	for _, b := range buses {
		if !b.arrive {
			continue
		}
		joinAt := func(m int) (int, bool) {
			q := &plans[m]
			switch {
			case q.dep < 0 || q.onto:
				return q.gaps[0], true
			case q.ride < len(q.gaps)-1:
				return q.gaps[q.ride], true
			}
			return 0, false
		}
		first, set := 0, false
		for _, m := range b.members {
			if g, ok := joinAt(m); ok && (!set || plans[m].up && g > first || !plans[m].up && g < first) {
				first, set = g, true
			}
		}
		top := -1
		for _, m := range b.members {
			q := &plans[m]
			g, ok := joinAt(m)
			if !ok || q.dep < 0 || q.onto {
				continue
			}
			if g == first && (top < 0 || top == q.dep) {
				top = q.dep
				continue
			}
			t, d := q.xs[q.ride+1], q.xs[q.ride]
			free := func(x int) bool {
				for _, r := range []int{g - 1, g} {
					for _, y := range l.passes[r] {
						if abs(x-y) < lane {
							return false
						}
					}
				}
				for _, y := range l.stubs[g] {
					if shared && abs(x-y) < lane && y != d {
						return false
					}
				}
				for _, y := range vias[g] {
					if abs(x-y) < lane {
						return false
					}
				}
				return true
			}
			q.via = viaX(t, d, free)
			vias[g] = append(vias[g], q.via)
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
		case p.via >= 0 && j == p.ride:
			return i
		case p.onto:
			return busOwner(p.arr)
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
			if p.via >= 0 && j == p.ride {
				d := busOwner(p.dep)
				addSpan(g, d, a, p.via, p.up, false)
				addSpan(g, i, p.via, b, p.up, false)
				if p.up {
					above[g][[2]int{i, d}] = true
				} else {
					above[g][[2]int{d, i}] = true
				}
				continue
			}
			// A line bound for a trunk keeps a track even when it runs
			// straight down to it: the trunk starts there.
			if a == b && !p.same && o == i && p.arr < 0 {
				continue
			}
			addSpan(g, o, a, b, p.up, p.same)
		}
		if p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == len(p.gaps)-1 {
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
		if p.dep < 0 || p.onto || p.arr < 0 || p.up || p.ride >= len(p.gaps)-1 || p.via >= 0 {
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
	// A shared bus's drop runs down from its track, so a bus whose track
	// ends on that drop takes a track below it.
	for b, bs := range buses {
		if !bs.arrive || bs.fed < 0 {
			continue
		}
		p := &plans[bs.members[0]]
		if g := p.gaps[len(p.gaps)-1]; above[g] != nil {
			above[g][[2]int{busOwner(bs.fed), busOwner(b)}] = true
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
	for b, bs := range buses {
		if !bs.arrive {
			p := &plans[bs.members[0]]
			room[p.gaps[0]][map[bool]int{true: 1}[p.up]] = countRoom
			continue
		}
		reach := map[string]int{}
		for i := range reqs {
			if plans[i].arr >= 0 && drop(i) == b {
				reach[reqs[i].to.key]++
			}
		}
		for _, i := range bs.members {
			if p := &plans[i]; drop(i) == b && reach[reqs[i].to.key] > 1 {
				room[p.gaps[len(p.gaps)-1]][map[bool]int{false: 1}[p.up]] = countRoom
			}
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
	ways := make([][]pt, len(reqs))
	for i := range reqs {
		p := &plans[i]
		k := len(p.gaps)
		w := []pt{startOf(i)}
		for j := 0; j < k; j++ {
			y := trackY(p.gaps[j], owner(i, j))
			if p.dep >= 0 && p.via >= 0 && j == p.ride {
				d := trackY(p.gaps[j], busOwner(p.dep))
				w = append(w, pt{p.xs[j], d}, pt{p.via, d}, pt{p.via, y})
			} else {
				w = append(w, pt{p.xs[j], y})
			}
			w = append(w, pt{p.xs[j+1], y})
		}
		if p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == k-1 {
			y := trackY(p.gaps[k-1], busOwner(p.arr))
			w = append(w, pt{p.xs[k], y}, pt{p.tx, y})
		}
		ways[i] = straight(append(w, endOf(i)))
	}
	var out []drawn
	nets := 0
	// A line's own part: from its start, or where it leaves its departure
	// bus, to its end, or where it joins its arrival bus.
	for i := range reqs {
		p := &plans[i]
		if p.onto {
			continue
		}
		var pts []pt
		st := noStart
		k := len(p.gaps)
		j := 0
		if p.dep >= 0 && p.via >= 0 {
			j = p.ride + 1
			g := p.gaps[p.ride]
			pts = append(pts, pt{p.via, trackY(g, busOwner(p.dep))}, pt{p.via, trackY(g, i)}, pt{p.xs[j], trackY(g, i)})
		} else if p.dep >= 0 {
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
		if first.onto {
			o = busOwner(first.arr)
		}
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
			for j := 0; j <= p.ride && !p.onto; j++ {
				next := p.xs[j+1]
				if j == p.ride && p.via >= 0 {
					next = p.via
				}
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
		// The trunk passes each row from the highest line that joins it
		// there to the bus's track in the gap after it. In each gap the
		// track runs through every point where a line joins it, the trunk
		// meets it, or a drop leaves it.
		type passage struct{ x, reach, ty, top int }
		trunkAt := map[int]*passage{}
		var rows []int
		stops := map[int][]int{}
		var gaps []int
		stop := func(g, x int) {
			if _, ok := stops[g]; !ok {
				gaps = append(gaps, g)
			}
			if !slices.Contains(stops[g], x) {
				stops[g] = append(stops[g], x)
			}
		}
		for _, i := range bs.members {
			p := &plans[i]
			k := len(p.gaps)
			for j := 0; j < k; j++ {
				if owner(i, j) != o {
					continue
				}
				stop(p.gaps[j], p.xs[j])
				stop(p.gaps[j], p.xs[j+1])
				if j == 0 {
					continue
				}
				r := passRow(p, j)
				from, to := trackY(p.gaps[j-1], owner(i, j-1)), trackY(p.gaps[j], o)
				ps, ok := trunkAt[r]
				if !ok {
					ps = &passage{x: p.xs[j], reach: from, ty: to, top: p.gaps[j-1]}
					trunkAt[r] = ps
					rows = append(rows, r)
				}
				if abs(from-to) > abs(ps.reach-to) {
					ps.reach = from
				}
			}
			if owner(i, k-1) != o {
				stop(p.gaps[k-1], p.xs[k])
				stop(p.gaps[k-1], p.tx)
			}
		}
		slices.Sort(rows)
		var parts [][]pt
		for _, r := range rows {
			ps := trunkAt[r]
			part := []pt{{ps.x, ps.reach}}
			// A trunk that runs down through its own track meets it there.
			if y := trackY(ps.top, o); slices.Contains(stops[ps.top], ps.x) && (y-ps.reach)*(ps.ty-y) > 0 {
				part = append(part, pt{ps.x, y})
			}
			parts = append(parts, append(part, pt{ps.x, ps.ty}))
		}
		for _, g := range gaps {
			xs := slices.Sorted(slices.Values(stops[g]))
			if len(xs) < 2 {
				continue
			}
			y := trackY(g, o)
			var part []pt
			for _, x := range xs {
				part = append(part, pt{x, y})
			}
			parts = append(parts, part)
		}
		none := make([]pt, len(bs.members))
		for m := range none {
			none[m] = noStart
		}
		if len(parts) > 0 {
			var ds []string
			for _, pp := range parts {
				ds = append(ds, polyline(pp))
			}
			out = append(out, drawn{D: strings.Join(ds, " "), parts: parts, reqs: bs.members, starts: none, net: nets, joins: true, back: back(lead)})
		}
		// Each box the bus feeds shows how many lines go there, its own and
		// those of the buses whose tracks end on its drop.
		reach := map[string]int{}
		for i := range reqs {
			if plans[i].arr >= 0 && drop(i) == b {
				reach[reqs[i].to.key]++
			}
		}
		drops := map[int][]int{}
		var xs []int
		for _, i := range bs.members {
			p := &plans[i]
			if drop(i) != b {
				continue
			}
			if _, ok := drops[p.tx]; !ok {
				xs = append(xs, p.tx)
			}
			drops[p.tx] = append(drops[p.tx], i)
		}
		slices.Sort(xs)
		for _, x := range xs {
			is := drops[x]
			p := &plans[is[0]]
			y := trackY(p.gaps[len(p.gaps)-1], o)
			e := endOf(is[0])
			line := []pt{{x, y}, e}
			dd := drawn{D: polyline(line), parts: [][]pt{line}, reqs: is, starts: none[:len(is)], net: nets, back: back(is[0])}
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
		if p := &plans[i]; p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride < len(p.gaps)-1 && p.via < 0 && len(out) > 0 {
			at := pt{p.xs[p.ride+1], trackY(p.gaps[p.ride], busOwner(p.dep))}
			out[0].joinsAt = append(out[0].joinsAt, join{at, hue(reqs[i].kind)})
		}
	}
	var boxes []box
	for _, r := range l.pos {
		boxes = append(boxes, box{r.X, r.Y, r.W, r.H})
	}
	// A frame's chips are boxes to a count too: it must not sit on one.
	seen := map[box]bool{}
	for _, r := range reqs {
		for _, e := range []end{r.from, r.to} {
			if e.node != "" {
				continue
			}
			b := box{e.lo - 4, l.top - l.chips[0], e.hi - e.lo + 8, l.chips[0]}
			if e.row >= 0 {
				b.y, b.h = endY(e, 1)-3, l.chips[1]
			}
			if !seen[b] {
				seen[b] = true
				boxes = append(boxes, b)
			}
		}
	}
	sort.Slice(boxes, func(i, j int) bool {
		return boxes[i].y < boxes[j].y || boxes[i].y == boxes[j].y && boxes[i].x < boxes[j].x
	})
	anchor(out, pieces(out), boxes)
	return routing{out, ways, can}
}

// share picks the shared buses. Two or more boxes that the same two or more
// places all reach, by lines of one kind running down, get one bus for
// those lines: one track gathers them and feeds every one of the boxes, so
// a place that reaches them all does not split in two right below itself.
// Every place on the bus reaches every box it feeds, so the bus joins no
// two places that no line joins. It returns the shared bus each line rides,
// -1 for none, and each bus's lines.
func share(reqs []wireReq, arrive map[slotKey][]int) ([]int, [][]int) {
	of := make([]int, len(reqs))
	for i := range of {
		of[i] = -1
	}
	type target struct {
		kind string
		// from holds the line from each place that reaches the box, and
		// lines all its lines.
		from  map[string]int
		lines []int
	}
	var ts []target
	for _, k := range sortedKeys(arrive) {
		is := arrive[k]
		if k.side > 0 || reqs[is[0]].to.node == "" {
			continue
		}
		t := target{reqs[is[0]].kind, map[string]int{}, is}
		for _, i := range is {
			if n := reqs[i].from.node; n != "" {
				t.from[n] = i
			}
		}
		ts = append(ts, t)
	}
	used := make([]bool, len(ts))
	taken := map[string]bool{}
	both := func(a, b int) []string {
		var out []string
		for s := range ts[a].from {
			if _, ok := ts[b].from[s]; ok && !taken[s+"|"+ts[a].kind] {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return out
	}
	// fits says the bus can feed box t from places core: every other line
	// into t ends where a line of the bus does, so it joins the bus's drop
	// there and the box keeps one bundle of the kind (rule 4).
	fits := func(t int, core []string) bool {
		fed := map[string]bool{}
		for _, s := range core {
			fed[place(reqs[ts[t].from[s]].to)] = true
		}
		return !slices.ContainsFunc(ts[t].lines, func(i int) bool { return !fed[place(reqs[i].to)] })
	}
	var shares [][]int
	for {
		a, b := -1, -1
		var core []string
		for x := range ts {
			for y := x + 1; y < len(ts); y++ {
				if used[x] || used[y] || ts[x].kind != ts[y].kind {
					continue
				}
				if c := both(x, y); len(c) > len(core) && fits(x, c) && fits(y, c) {
					a, b, core = x, y, c
				}
			}
		}
		if len(core) < 2 {
			break
		}
		fed := []int{a, b}
		for c := range ts {
			if !used[c] && c != a && c != b && ts[c].kind == ts[a].kind && !slices.ContainsFunc(core, func(s string) bool { _, ok := ts[c].from[s]; return !ok }) && fits(c, core) {
				fed = append(fed, c)
			}
		}
		var is []int
		for _, t := range fed {
			used[t] = true
			for _, s := range core {
				is = append(is, ts[t].from[s])
			}
		}
		for _, s := range core {
			taken[s+"|"+ts[a].kind] = true
		}
		sort.Ints(is)
		for _, i := range is {
			of[i] = len(shares)
		}
		shares = append(shares, is)
	}
	return of, shares
}

// gather groups the lines of each arrival slot into the arrival buses they
// may share. Without shared buses, lines bound for boxes in one row, of one
// kind and from exactly the same places, share one arrival trunk that
// splits only in the gap above those boxes. A trunk shared by lines from
// different places would draw a path from each of them to every box it
// splits to. With shared buses, a shared bus covers that case.
func gather(reqs []wireReq, arrive map[slotKey][]int, shared bool) [][]int {
	type group struct {
		members  []int
		from     map[string]bool
		row, way int
		kind     string
	}
	var groups []*group
	for _, k := range sortedKeys(arrive) {
		is := slices.Clone(arrive[k])
		to := reqs[is[0]].to
		g := &group{members: is, from: map[string]bool{}, row: to.row, way: k.side, kind: reqs[is[0]].kind}
		for _, i := range is {
			g.from[reqs[i].from.key] = true
		}
		joined := false
		for _, h := range groups {
			if shared || to.node == "" || reqs[h.members[0]].to.node == "" || h.row != g.row || h.way != g.way || h.kind != g.kind {
				continue
			}
			if maps.Equal(g.from, h.from) {
				h.members = append(h.members, is...)
				joined = true
				break
			}
		}
		if !joined {
			groups = append(groups, g)
		}
	}
	out := make([][]int, len(groups))
	for k, g := range groups {
		out[k] = g.members
	}
	return out
}

// place names where an end is drawn: its key, or for a frame port the
// chip it shares with the ports folded into it.
func place(e end) string {
	if e.node == "" {
		return fmt.Sprint("chip|", e.row, "|", e.lo, "|", e.hi)
	}
	return e.key
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

// detourOffer is how much longer than the direct way a chain may make a
// line to a frame's chip before the line passes the rows on its own.
const detourOffer = 160

// slotGap is the least room between two lines on one edge of a box.
const slotGap = 12

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

// walk is the first x that free says is clear, going a lane at a time in
// direction dir from a without passing b.
func walk(a, b, dir int, free func(int) bool) (int, bool) {
	for x := a; (x-b)*dir <= 0; x += dir * lane {
		if free(x) {
			return x, true
		}
	}
	return 0, false
}

// stepX is where a line that rides its departure trunk into its last gap
// steps down onto its arrival track: from base toward its end at x end,
// at the first x free says is clear. Past either of those it costs the
// line twice the overshoot, so it then takes the clear x nearest to them,
// up to 32 lanes out. With none clear it steps at base.
func stepX(base, end int, free func(int) bool) int {
	dir := 1
	if end < base {
		dir = -1
	}
	if x, ok := walk(base, end, dir, free); ok {
		return x
	}
	for m := 1; m <= 32; m++ {
		for _, x := range []int{end + dir*m*lane, base - dir*m*lane} {
			if free(x) {
				return x
			}
		}
	}
	return base
}

// viaX is where a line steps down off its departure track at x d onto a
// track of its own that ends on the arrival trunk at x t: two lanes or
// more off the trunk toward d, at the first x free says is clear, then
// one lane off it. It never reaches d, past which its track would hook
// back under the departure trunk. With no clear x between them it steps
// halfway.
func viaX(t, d int, free func(int) bool) int {
	dir := 1
	if d < t {
		dir = -1
	}
	if x, ok := walk(t+2*dir*lane, d-dir, dir, free); ok {
		return x
	}
	if x := t + dir*lane; (x-d)*dir < 0 && free(x) {
		return x
	}
	return (t + d) / 2
}
