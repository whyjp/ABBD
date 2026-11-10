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
//   - In a focus view (ring), a line between rows next to each other
//     whose two boxes' spans meet drops straight and joins no bus, except
//     into a hub, which takes its lines as one trunk (H2).
//     Where its box's other lines fork into that row anyway, such lines
//     join the fork instead, the stem standing over one of them.
//     Elsewhere such a line drops straight too, unless it reaches a hub
//     or rides a shared bus, where the view reads better for it (layDrops).
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
	// road says the path is a piece of road (confluence.go), whose ink
	// joins every kind's: a highway carries lines of several kinds.
	road bool
	// starts are where each request's line leaves its end, for the origin
	// mark, or noStart when that end is on another path.
	starts []pt
	label  pt
	// net groups the paths of one bus; count is how many lines a bus
	// carries, on the path that shows the count, and counted are the
	// requests it counts.
	net, count int
	counted    []int
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
	// late says the line passes its one row down its own chain and joins
	// its arrival bus only on the bus's track.
	late bool
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

// route draws every request. Lines between modules that use each other,
// bundled, read both ways and join every pair of their far ends (rule 18),
// so they are drawn apart, on buses of their own pair, unless that runs a
// line out of its way past rule 15 or back past rule 13, which bundling
// spares. Where the layout's straighten moves a chain, the view is drawn
// again with the chains moved, and that is kept where weigh reads it
// better, unless the view has more than shareMost requests, as for shared
// buses. Where it is not, it is
// drawn once more with every crossing held at or under its first track: a
// line that drops straight leaves its track empty rather than pull the
// tracks below it up, which would shorten the steps beneath into jogs.
// Only the kept drawing's counts are anchored: route may lay a view many
// times to keep one, and no measure it compares them by reads where a
// count stands, while placing the counts was most of a lay.
func (l *layout) route(reqs []wireReq) routing {
	before := l.keep()
	r := l.routeApart(reqs)
	after := l.keep()
	*l = before
	if len(reqs) <= shareMost && l.straighten(reqs) {
		moved := l.keep()
		for _, held := range []map[[3]int]int{nil, r.tracks} {
			*l = moved
			l.held = held
			s := l.routeApart(reqs)
			l.held = nil
			if weigh(s, reqs).beats(weigh(r, reqs)) {
				r, after = s, l.keep()
				break
			}
		}
	}
	*l = after
	if l.roads {
		r.confluent(reqs, l.position, len(l.rows))
	}
	anchor(r.drawn, pieces(r.drawn), r.boxes)
	return r
}

// position is where height y stands among the rows, as the lint reads it:
// 2k in the gap above row k, 2k+1 within it.
func (l *layout) position(y int) int {
	for k := range l.rowTop {
		switch {
		case y < l.rowTop[k]:
			return 2 * k
		case y <= l.rowTop[k]+l.rowH[k]:
			return 2*k + 1
		}
	}
	return 2 * len(l.rowTop)
}

// straighten moves each chain that passes its rows outside the span its
// lines' two ends share onto that span, where every row it passes has room,
// so its lines leave their box over the place they reach and drop straight.
// The layout seats a chain under what it joins, but a frame's chips are set
// out after it, packed along the frame, and a row's boxes pull a chain to a
// median, so a line may leave its box far from where it would drop into
// the other, then turn twice to reach it. It says whether it moved a chain.
func (l *layout) straighten(reqs []wireReq) bool {
	chainOf := l.chainsOf(reqs)
	moved := false
	for c, ch := range l.chains {
		lo, hi, used := 0, l.width, false
		for i, k := range chainOf {
			if k != c {
				continue
			}
			f, t := reqs[i].from, reqs[i].to
			used = true
			lo, hi = max(lo, f.lo, t.lo), min(hi, f.hi, t.hi)
		}
		w := (ch.lanes - 1) * chainLane
		if !used || hi-lo < w {
			continue
		}
		straight := true
		for r := ch.top; r <= ch.bottom; r++ {
			x := l.pass[[2]int{c, r}]
			straight = straight && x >= lo && x+w <= hi
		}
		if straight {
			continue
		}
		room := func(x int) bool {
			for r := ch.top; r <= ch.bottom; r++ {
				for _, id := range l.rows[r] {
					if p := l.pos[id]; x < p.X+p.W+chainSep && x+w > p.X-chainSep {
						return false
					}
				}
				for o, och := range l.chains {
					y, ok := l.pass[[2]int{o, r}]
					if o != c && ok && x < y+(och.lanes-1)*chainLane+chainSep && x+w > y-chainSep {
						return false
					}
				}
			}
			return true
		}
		want := (lo + hi - w) / 2
		for d := 0; d <= hi-lo-w; d++ {
			x := want + d
			if x+w > hi || !room(x) {
				if x = want - d; x < lo || !room(x) {
					continue
				}
			}
			for r := ch.top; r <= ch.bottom; r++ {
				l.pass[[2]int{c, r}] = x
			}
			moved = true
			break
		}
	}
	return moved
}

func (l *layout) routeApart(reqs []wireReq) routing {
	if !bundlesPairs(reqs) {
		return l.routeOnce(reqs, false)
	}
	before := l.keep()
	apart := l.routeOnce(reqs, true)
	after := l.keep()
	*l = before
	bundled := l.routeOnce(reqs, false)
	if costOf(apart, reqs).breaks <= costOf(bundled, reqs).breaks {
		*l = after
		return apart
	}
	return bundled
}

// chainsOf is the chain each line runs down, or -1: a line between places
// more than a row apart runs down the dummies the layout set straight for
// it.
func (l *layout) chainsOf(reqs []wireReq) []int {
	chainOf := make([]int, len(reqs))
	byPair := map[[2]string]int{}
	// lanes are the lanes of the chips set out after the layout (proper).
	lanes := map[string]int{}
	for c, ch := range l.chains {
		byPair[pairKey(ch.upper, ch.lower)] = c
		switch {
		case ch.lower == "":
			lanes[ch.upper] = c
		case ch.upper == "":
			lanes[ch.lower] = c
		}
	}
	// A line to a seated chip runs down the chip's own chain.
	id := func(e end) string {
		edge := frameBottomNode
		switch {
		case e.node != "":
			return e.node
		case e.row < 0:
			edge = frameTopNode
		}
		c := chipNode(edge, strings.TrimPrefix(e.key, "port|"))
		if _, ok := l.seats[c]; ok {
			return c
		}
		return edge
	}
	for i, r := range reqs {
		chainOf[i] = -1
		if abs(r.from.row-r.to.row) > 1 {
			if c, ok := lanes[id(r.from)]; ok {
				chainOf[i] = c
				continue
			}
			if c, ok := lanes[id(r.to)]; ok {
				chainOf[i] = c
				continue
			}
		}
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
		a, b := (r.from.lo+r.from.hi)/2, (r.to.lo+r.to.hi)/2
		for row := min(r.from.row, r.to.row) + 1; row < max(r.from.row, r.to.row); row++ {
			x := l.pass[[2]int{c, row}]
			if abs(x-a)+abs(x-b)-abs(a-b) > detourOffer {
				chainOf[i] = -1
				break
			}
		}
	}
	return chainOf
}

// bundlesPairs says some two lines between modules that use each other
// leave or reach one box toward different places, so bundling them would
// join those places.
func bundlesPairs(reqs []wireReq) bool {
	far := map[string]string{}
	for _, r := range reqs {
		if r.kind != "mutual" {
			continue
		}
		for _, e := range [][2]string{{place(r.from), place(r.to)}, {place(r.to), place(r.from)}} {
			if o, ok := far[e[0]]; ok && o != e[1] {
				return true
			}
			far[e[0]] = e[1]
		}
	}
	return false
}

// keep is a copy of the layout to restore: lay sets the gaps and rows
// afresh, and the boxes' y in place.
func (l *layout) keep() layout {
	k := *l
	k.pos, k.pass = maps.Clone(l.pos), maps.Clone(l.pass)
	return k
}

// routeOnce draws every request. Where lines could share arrival buses, it
// draws the view both ways and keeps the shared buses only when they spare
// the reader turns and cost nothing else, as weigh measures it. weigh
// compares every pair of strands, so a view with more than shareMost
// requests keeps the plain drawing rather than pay for that.
func (l *layout) routeOnce(reqs []wireReq, apart bool) routing {
	plain := l.layDrops(reqs, false, apart)
	if !plain.shares || len(reqs) > shareMost {
		return plain
	}
	kept := l.keep()
	if shared := l.layDrops(reqs, true, apart); weigh(shared, reqs).beats(weigh(plain, reqs)) {
		return shared
	}
	*l = kept
	return plain
}

const shareMost = 200

// layDrops lays the view, then lays it again with every line that drops()
// took off its buses but could not stand straight kept on them, since such
// a line leaves or reaches its box beside its bundle (rule 4).
func (l *layout) layDrops(reqs []wireReq, shared, apart bool) routing {
	before := l.keep()
	bent := map[int]bool{}
	var r routing
	for {
		kept := l.keep()
		r = l.layStraight(reqs, shared, apart, bent)
		grew := false
		for _, i := range r.bent {
			grew = grew || !bent[i]
			bent[i] = true
		}
		if !grew {
			break
		}
		*l = kept
	}
	if !r.dropped {
		return r
	}
	after := l.keep()
	*l = before
	bundled := l.layStraight(reqs, shared, apart, nil)
	if weigh(r, reqs).beats(weigh(bundled, reqs)) {
		*l = after
		return r
	}
	return bundled
}

// layStraight lays the view with the lines that would cross over the box
// they reach and come back set straight onto their arrival trunks, and
// keeps that only where it costs the reader less than the plain drawing.
func (l *layout) layStraight(reqs []wireReq, shared, apart bool, bent map[int]bool) routing {
	before := l.keep()
	r := l.layJoined(reqs, shared, apart, bent, true)
	if !r.tweaked {
		return r
	}
	after := l.keep()
	*l = before
	plain := l.layJoined(reqs, shared, apart, bent, false)
	if costOf(r, reqs).beats(costOf(plain, reqs)) {
		*l = after
		return r
	}
	return plain
}

// layJoined lays the view with a line that steps off its departure trunk
// leading its arrival bus's drop (lay's early), and keeps that only where
// weigh says it reads better than the plain drawing.
func (l *layout) layJoined(reqs []wireReq, shared, apart bool, bent map[int]bool, tweak bool) routing {
	before := l.keep()
	r := l.lay(reqs, shared, apart, bent, tweak, true)
	if !r.joined {
		return r
	}
	after := l.keep()
	*l = before
	plain := l.lay(reqs, shared, apart, bent, tweak, false)
	if weigh(r, reqs).beats(weigh(plain, reqs)) {
		*l = after
		return r
	}
	return plain
}

// routing is one way to draw a view's lines.
type routing struct {
	drawn []drawn
	// ways are the points each line turns at on its way from its start to
	// its end, along the ink a reader follows, its buses' included.
	ways [][]pt
	// back says each line runs against the reading direction, rule 13.
	back []bool
	// shares says some lines could share an arrival bus.
	shares bool
	// roomy says the counted tactic would give some ×N room (lay).
	roomy bool
	// bent are the lines lay took off their buses to drop straight that
	// do not stand straight.
	bent []int
	// dropped says some line of a view other than a focus view drops
	// straight off its buses.
	dropped bool
	// boxes are what a ×N must not stand on: the boxes and the frame's
	// chips.
	boxes []box
	// tweaked says lay set some line straight onto its arrival trunk:
	// a stub over the trunk's lead, or a line joining on the track.
	tweaked bool
	// joined says lay let a line that steps off its departure trunk lead
	// its arrival bus, the bus's other lines joining on the track.
	joined bool
	// tracks is the track each crossing took, by gap and owner.
	tracks map[[3]int]int
	// roads says the lines ride roads (confluent), where ink may join
	// places no relation joins (J3).
	roads bool
}

// lay draws every request, with shared arrival buses where shared allows
// them, and with each line between modules that use each other on buses of
// its own pair where apart says so.
func (l *layout) lay(reqs []wireReq, shared, apart bool, bent map[int]bool, tweak, early bool) routing {
	tweaked, joined := false, false
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
	keys := make([][2]slotKey, len(reqs))
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
		// A line between two modules that use each other reads both ways,
		// so apart keeps it off the buses of other pairs' lines (rule 18).
		if apart && r.kind == "mutual" {
			dk.key += "|" + place(r.to)
			ak.key += "|" + place(r.from)
		}
		keys[i] = [2]slotKey{dk, ak}
		leave[dk] = append(leave[dk], i)
		arrive[ak] = append(arrive[ak], i)
	}
	chainOf := l.chainsOf(reqs)
	{
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
		r := reqs[i]
		step := 1
		if r.to.row < r.from.row {
			step = -1
		}
		row := r.to.row - step
		if start {
			row = r.from.row + step
		}
		return l.pass[[2]int{c, row}] + laneOf[i]*chainLane, true
	}
	// held says who passes each row at each x: a line by its index, a bus
	// by -1-b. Two of them at one x in one row, or in rows next to each
	// other, would meet and read as one.
	held := map[[2]int]int{}
	// runsOn says line i, off its departure trunk, may pass a row at the x
	// where its trunk o passed the row next to it: the trunk ended there
	// and the line goes straight on, so the two read as the one line they
	// are, where stepping aside would turn it twice for nothing. Not where
	// another line leaves the trunk in the same gap for an arrival bus: it
	// steps down onto that bus's track at the trunk's x, and the track
	// would end on this line.
	runsOn := func(i, o int) bool {
		p := &plans[i]
		if p.dep < 0 || o != -1-p.dep {
			return false
		}
		for m := range plans {
			if q := &plans[m]; m != i && q.dep == p.dep && q.ride == p.ride && q.arr >= 0 {
				return false
			}
		}
		return true
	}
	// runs is where line i, or the bus owner draws it for, runs down i's
	// chain in row r, if it has room there. The arrival trunk i leads goes
	// straight on where i's departure trunk ended, as i would (runsOn):
	// refused there, it passed that row in the middle of the widest space
	// and came back to the chain in the next.
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
			if o, ok := held[[2]int{q, x}]; ok && o != owner && (q == r || owner != i && (plans[i].arr < 0 || owner != -1-plans[i].arr) || !runsOn(i, o)) {
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
	// drops says line i can drop straight from box to box: they stand in
	// rows next to each other and their spans meet, so it needs no bus.
	hubs := hubsOf(drawnRels(reqs), func(k string) bool { return !strings.HasPrefix(k, chipPlace) })
	drops := func(i int) bool {
		f, t := reqs[i].from, reqs[i].to
		_, hub := hubs[t.node]
		return (l.ring && !bent[i] || bent != nil && !bent[i] && shareOf[i] < 0 && !hub) && !plans[i].same && len(plans[i].gaps) == 1 && f.node != "" && t.node != "" && max(f.lo, t.lo) <= min(f.hi, t.hi)
	}
	alone := func(is []int) []int { return slices.DeleteFunc(slices.Clone(is), drops) }
	// A hub, a box hubUsers or more places use, still takes its lines as
	// one trunk (H2).
	free := func(is []int) []int {
		return slices.DeleteFunc(slices.Clone(is), func(i int) bool { _, hub := hubs[reqs[i].to.node]; return drops(i) && !hub })
	}
	var buses []*bus
	for _, k := range sortedKeys(leave) {
		// Where a place's other lines fork into the row below it anyway,
		// its straight drops join the fork too: one stem, one track, a
		// drop into each box, and the stem over one of them (stemOver), so
		// no stub runs beside the stem through its track.
		is := alone(leave[k])
		oneRow := !slices.ContainsFunc(leave[k], func(i int) bool {
			return len(plans[i].gaps) != 1 || place(reqs[i].from) != place(reqs[leave[k][0]].from)
		})
		if len(is) >= 2 && oneRow {
			is = leave[k]
		}
		if len(is) >= 2 {
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
		is = slices.DeleteFunc(free(is), func(i int) bool { return shareOf[i] >= 0 })
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
	// A line that steps down onto its arrival bus's track off a departure
	// trunk that passed the last row down a lane of the chain the bus's
	// lead runs down too would meet that lead's trunk a lane off its own
	// step and jog onto the drop. It leads the bus instead, so the drop
	// stands under its step and it runs straight on down; the bus's other
	// lines on that chain pass the rows on their own, off it, straight on
	// where they can, and join on the track.
	offChain := map[int]bool{}
	for _, b := range buses {
		if !early || !b.arrive || b.lead >= len(reqs) || chainOf[b.lead] < 0 || plans[b.lead].dep >= 0 {
			continue
		}
		c := chainOf[b.lead]
		for _, s := range b.members {
			if q := &plans[s]; q.dep < 0 || q.onto || q.ride != len(q.gaps)-1 || chainOf[s] != c || buses[q.dep].lead != s {
				continue
			}
			for _, m := range b.members {
				if q := &plans[m]; m != s && q.dep < 0 && chainOf[m] == c {
					q.late, chainOf[m], offChain[m] = true, -1, true
				}
			}
			b.lead, joined = s, true
			break
		}
	}
	// A line that passes one row before it reaches its arrival bus's
	// track, where the bus's trunk passes that row on the other side of
	// the box the line reaches from the line's own chain, would run over
	// the whole box and come back to it. It passes the row down its own
	// chain instead and joins the bus on the track.
	for _, b := range buses {
		if !b.arrive || b.lead >= len(reqs) || chainOf[b.lead] < 0 {
			continue
		}
		for _, m := range b.members {
			p := &plans[m]
			if m == b.lead || len(p.gaps) != 2 || p.dep >= 0 && p.ride > 0 || chainOf[m] < 0 {
				continue
			}
			r := p.gaps[0]
			if p.up {
				r--
			}
			bx, ok := l.pass[[2]int{chainOf[b.lead], r}]
			ox, mine := l.pass[[2]int{chainOf[m], r}]
			t := reqs[m].to
			p.late = tweak && ok && mine && (bx < t.lo && ox > t.hi || bx > t.hi && ox < t.lo)
			tweaked = tweaked || p.late
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
			if p := &plans[i]; (p.arr < 0 || p.late) && (p.dep < 0 || p.ride < len(p.gaps)-1) {
				claim(i)
			}
		}
		// Lines of more than one kind from one frame chip, down a chain past
		// rows, would run its whole length chainLane apart and read as one
		// doubled line. Where the chain has lanes to spare, they spread
		// over its width.
		on := map[int][]int{}
		for i, c := range chainOf {
			if _, ok := laneOf[i]; ok {
				on[c] = append(on[c], i)
			}
		}
		for c, is := range on {
			ch := l.chains[c]
			top := strings.HasPrefix(ch.upper, frameTopNode)
			if n := len(is); n < 2 || n >= ch.lanes || ch.bottom <= ch.top || !top && !strings.HasPrefix(ch.lower, frameBottomNode) {
				continue
			}
			chip := func(i int) string {
				if top != plans[i].up {
					return place(reqs[i].from)
				}
				return place(reqs[i].to)
			}
			if slices.ContainsFunc(is, func(i int) bool { return chip(i) != chip(is[0]) }) ||
				!slices.ContainsFunc(is, func(i int) bool { return reqs[i].kind != reqs[is[0]].kind }) {
				continue
			}
			for _, i := range is {
				laneOf[i] = (2*laneOf[i]*(ch.lanes-1) + len(is) - 1) / (2 * (len(is) - 1))
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
	// set is where each line's end already stands on its edge, by line and
	// whether it is the line's start.
	set := map[int]map[bool]int{}
	// reach is how many lines of each arrival bus drop into each place, the
	// ×N its drop shows (draw).
	type into struct {
		bus   int
		place string
	}
	reach := map[into]int{}
	for i := range reqs {
		if plans[i].arr >= 0 {
			reach[into{drop(i), reqs[i].to.key}]++
		}
	}
	shown := func(sl slot) int {
		i := sl.is[0]
		if b := plans[i].dep; sl.leave && b >= 0 {
			return len(buses[b].members)
		}
		if b := drop(i); !sl.leave && b >= 0 {
			return reach[into{b, reqs[i].to.key}]
		}
		return 0
	}
	roomy := false
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
				// A stub with no trunk below it stands over a line of its
				// own that leads an arrival trunk down its chain, so the
				// stub runs straight on into that trunk.
				if tweak && i == len(reqs) && buses[b].deep == 0 {
					for _, m := range sl.is {
						if a := plans[m].arr; a < 0 || buses[a].lead != m {
							continue
						}
						if x, ok := near(m, true); ok {
							tweaked = true
							return x, true
						}
					}
				}
			} else if b := drop(i); !sl.leave && b >= 0 {
				i = buses[b].lead
				// An arrival bus into a frame chip that no line leads
				// still drops where one of its lines runs down a chain
				// over the chip, so that line lands straight rather than
				// jogging a lane to the chip's middle. Unlike the stub
				// above, it trades nothing for that, so it holds without
				// tweak. Held to tweak and weighed against a plain drawing
				// without it, layStraight took the plain one in two of
				// pstack's views, each with 2 more excess turns.
				if t := reqs[sl.is[0]].to; i >= len(reqs) && t.node == "" {
					for _, m := range sl.is {
						if x, ok := near(m, false); ok && x >= t.lo && x <= t.hi {
							return x, true
						}
					}
				}
			} else if len(sl.is) != 1 {
				return 0, false
			}
			if i >= len(reqs) {
				return 0, false
			}
			if x, ok := near(i, sl.leave); ok {
				return x, true
			}
			// A line alone in its slot that passes no row, between boxes
			// in rows next to each other or between a frame chip and a
			// first- or last-row box, drops straight where its other end
			// already stands.
			x, ok := set[i][!sl.leave]
			return x, ok && len(sl.is) == 1 && len(plans[i].gaps) == 1
		}
		// A departure bus that carries lines which could drop straight
		// stands its stem over the one nearest its share of the edge, so
		// that line still drops straight and the others fork off the stem.
		stemOver := func(sl slot, want int) (int, bool) {
			best, found := 0, false
			for _, i := range sl.is {
				if !sl.leave || len(sl.is) < 2 || !drops(i) {
					continue
				}
				f, t := reqs[i].from, reqs[i].to
				lo, hi := max(f.lo, t.lo), min(f.hi, t.hi)
				x := clamp(want, lo, hi)
				if at, ok := set[i][false]; ok {
					if at < lo || at > hi {
						continue
					}
					x = at
				}
				if !found || abs(x-want) < abs(best-want) {
					best, found = x, true
				}
			}
			return best, found
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
			x, ok := pin(sl)
			if ok && x >= r.lo && x <= r.hi && (r.node == "" || apart(x)) {
				xs[s], pinned[s] = x, true
			} else if x, over := stemOver(sl, xs[s]); !ok && over && (r.node == "" || apart(x)) {
				xs[s], pinned[s] = x, true
			} else if i := sl.is[0]; (!ok || !l.ring) && len(sl.is) == 1 && drops(i) {
				// A line that drops straight stands as near its share of
				// the edge as the span both its ends meet in lets it.
				f, t := reqs[i].from, reqs[i].to
				if x := clamp(xs[s], max(f.lo, t.lo), min(f.hi, t.hi)); r.node == "" || apart(x) {
					xs[s], pinned[s] = x, true
				}
			}
		}
		gap := min(slotGap, (r.hi-r.lo)/max(1, len(xs)))
		// On a ring, where straight drops pin the slots at an edge's ends,
		// the slots between pins spread over the room between them.
		// Elsewhere they keep their even share where it fits between the
		// pins (below): spreading there trades steps for crossings.
		if l.ring {
			counts := make([]int, len(slots))
			for s, sl := range slots {
				counts[s] = len(sl.is)
			}
			order := spreadRuns(xs, pinned, counts, r.lo, r.hi, gap)
			slots, pinned, xs = permute(slots, order), permute(pinned, order), permute(xs, order)
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
			// A slot keeps between the pinned slots on either side of it,
			// with room for the slots between, so the order never pushes a
			// pinned line off its chain.
			lo, hi := r.lo, r.hi
			for p := range slots {
				switch {
				case pinned[p] && p < s:
					lo = max(lo, xs[p]+(s-p)*gap)
				case pinned[p] && p > s:
					hi = min(hi, xs[p]-(p-s)*gap)
				}
			}
			if lo > hi {
				lo, hi = r.lo, r.hi
			}
			xs[s] = clamp(xs[s], lo, hi)
			for d := 0; d <= hi-lo; d += 2 {
				if x := xs[s] + d; x <= hi && clear(s, x) {
					xs[s] = x
					break
				}
				if x := xs[s] - d; x >= lo && clear(s, x) {
					xs[s] = x
					break
				}
			}
		}
		// spread keeps each slot apartBy[s] from the slot before it and
		// apartBy[s+1] from the slot after it, within the edge.
		spread := func(apartBy []int) []int {
			ys := slices.Clone(xs)
			edge := r.lo - gap
			for s := range ys {
				ys[s] = max(ys[s], edge+apartBy[s])
				edge = ys[s]
			}
			edge = r.hi + gap
			for s := len(ys) - 1; s >= 0; s-- {
				ys[s] = min(ys[s], edge-apartBy[s+1])
				edge = ys[s]
			}
			return ys
		}
		apartBy := make([]int, len(xs)+1)
		for s := range apartBy {
			apartBy[s] = gap
		}
		// A ×N stands on whichever side of its stub or drop has the more
		// open ink (anchor), so a slot that shows one between two others
		// keeps the count's width on its wider side, rule 22. The end slots
		// have the edge's open end beside them. Each count in turn gets
		// that room where the edge still has it and no slot the room moves
		// stands on a line of the box's chips. A slot moved off a chain
		// stays: its jog is what judge weighs against the room. roomy says
		// some count got room, so counted draws something else.
		plain := spread(apartBy)
		ys := plain
		for s := 1; s+1 < len(xs); s++ {
			n := shown(slots[s])
			if n < 2 {
				continue
			}
			try := slices.Clone(apartBy)
			side := s + 1
			if ys[s+1]-ys[s] < ys[s]-ys[s-1] {
				side = s
			}
			try[side] = max(try[side], textWidth(fmt.Sprintf("×%d", n), 11)+9)
			if sum(try[1:len(xs)]) > r.hi-r.lo {
				continue
			}
			zs := spread(try)
			ok := true
			for t, z := range zs {
				ok = ok && (z == plain[t] || r.node == "" || apart(z))
			}
			if ok {
				roomy = roomy || !slices.Equal(zs, ys)
				apartBy, ys = try, zs
			}
		}
		if l.counted {
			xs = ys
		} else {
			xs = plain
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
				if set[i] == nil {
					set[i] = map[bool]int{}
				}
				set[i][sl.leave] = x
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
			if p.arr >= 0 && !p.late {
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
			own := -1
			if c := chainOf[i]; c >= 0 {
				if cx, ok := l.pass[[2]int{c, r}]; ok {
					own = cx + laneOf[i]*chainLane
				}
			}
			mine := func(q, y int) bool {
				o, ok := held[[2]int{q, y}]
				return ok && (o == i || p.dep >= 0 && o == -1-p.dep || p.arr >= 0 && o == -1-p.arr)
			}
			// A line set off its chain keeps straight on where nothing
			// stands within 12px and no other line passes.
			if x := p.xs[len(p.xs)-1]; offChain[i] && l.clearOf(r, x) && l.meetsOnly(r, x, mine) && !l.taken(r, x) &&
				!slices.ContainsFunc(l.passes[r], func(y int) bool { return abs(y-x) < lane }) {
				held[[2]int{r, x}] = i
				l.passes[r] = append(l.passes[r], x)
				p.xs = append(p.xs, x)
				continue
			}
			x := l.gutterTo(r, p.xs[len(p.xs)-1], p.tx, own, mine)
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
	// their ends. Where the trunk runs on below, an arrival track that
	// ended on the trunk would join the two the same way, so a pair steps
	// down on the trunk only when that join leads no reader anywhere its
	// lines do not go (joinsHarmless). Otherwise it steps down above its
	// end, or at the clear x nearest it on the trunk's side: a step past
	// the end would make the line ride beyond its turn and come back
	// beside its own track (B3, B4). Where the trunk ends there and every
	// clear x lies past the end, a pair still steps at the trunk's x unless
	// something in its own gap holds it: a pair in another gap that steps
	// straight down its own trunk there stands in the same column, a row
	// or more away, and shares no ink with it. Where the trunk runs on,
	// joinsHarmless alone decides whether a pair steps at its x.
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
		// down holds the pairs that step straight down their trunk's x.
		down := map[step]bool{}
		reach := map[[2]string]bool{}
		for _, w := range wanted(reqs) {
			reach[w] = true
		}
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
			mixed := slices.ContainsFunc(buses[st.arr].members, func(m int) bool { return m < len(reqs) && place(reqs[m].from) != place(reqs[lead].from) })
			column := false
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
					if abs(x-y) < lane && y != p.xs[k-1] {
						return false
					}
				}
				for o, y := range at {
					// A pair of the same trunk that steps down on the trunk
					// in this gap leaves the trunk's x to a line whose end
					// lies under it, which runs straight on down.
					if o.dep == st.dep && o.gap == st.gap && y == p.xs[k-1] && x == y && x == p.tx {
						continue
					}
					if o != st && abs(x-y) < lane && !(column && o.gap != st.gap && down[o]) {
						return false
					}
				}
				// A line of the same trunk that drops to its end in this gap
				// on no arrival bus would run on the step, and a reader
				// coming along the arrival track from another place could
				// turn down it.
				for _, q := range plans {
					if n := len(q.gaps); mixed && q.dep == st.dep && q.arr < 0 && q.ride == n-1 && q.gaps[n-1] == g && abs(x-q.tx) < lane {
						return false
					}
				}
				return true
			}
			base := p.xs[k-1]
			deep := buses[st.dep].deep > p.ride
			if deep {
				if joinsHarmless(reach, reqs, plans, buses, st.dep, st.arr, st.gap) {
					at[st], down[st] = base, k > 1
					continue
				}
				d := base
				base = p.tx
				if dir := sign(d - base); dir != 0 {
					if x, ok := walk(base, d-dir*lane, dir, free); ok {
						at[st] = x
						continue
					}
				}
			}
			at[st] = stepX(base, p.tx, free)
			if x := at[st]; !deep && (x-base)*(x-p.tx) > 0 {
				column = true
				if free(base) {
					at[st] = base
				}
			}
			down[st] = k > 1 && at[st] == p.xs[k-1]
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
	// coming along the trunk cannot climb that step. A line whose track
	// would lead such a reader nowhere its lines do not go (joinsHarmless)
	// needs no step and runs its track onto the trunk. It still holds the
	// x its step would take, so the steps of the others stand as they did.
	vias := map[int][]int{}
	wants := map[[2]string]bool{}
	for _, w := range wanted(reqs) {
		wants[w] = true
	}
	for bi, b := range buses {
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
		// A line that drops from its box straight onto the trunk's top holds
		// that top: a departure track meeting it there would let a reader
		// follow that line onto the track and to the track's other places.
		held := false
		for _, m := range b.members {
			if q := &plans[m]; (q.dep < 0 || q.onto) && len(q.xs) > 1 && q.fx == q.xs[1] {
				g, ok := joinAt(m)
				held = held || ok && g == first
			}
		}
		top := -1
		for _, m := range b.members {
			q := &plans[m]
			g, ok := joinAt(m)
			if !ok || q.dep < 0 || q.onto {
				continue
			}
			if g == first && !held && (top < 0 || top == q.dep) {
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
					if abs(x-y) < lane && y != d {
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
			x := viaX(t, d, free)
			vias[g] = append(vias[g], x)
			if !joinsHarmless(wants, reqs, plans, buses, q.dep, bi, g) {
				q.via = x
			}
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
	// took is each crossing's track keyed by what stays when the view is
	// laid again: a line by its request, a bus by its first line and side.
	took := map[[3]int]int{}
	crossing := func(g, o int) [3]int {
		if o >= 0 {
			return [3]int{g, o, 0}
		}
		b := buses[-1-o]
		return [3]int{g, b.members[0], map[bool]int{false: 1, true: 2}[b.arrive]}
	}
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
		meet := func(a, b int) bool { return order[a].lo <= order[b].hi+lane && order[b].lo <= order[a].hi+lane }
		held := func(a, b int) bool { return cost(order[a], order[b]) > 0 || cost(order[b], order[a]) > 0 }
		least := func(k int) int { return l.held[crossing(g, order[k].owner)] }
		for k, t := range shelve(len(order), meet, held, least) {
			track[[2]int{g, order[k].owner}] = t
			took[crossing(g, order[k].owner)] = t
			tracks[g] = max(tracks[g], t+1)
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
	// A line runs straight into its arrowhead for at least straightIn, so
	// a gap keeps that room between a box and the track a line leaves for
	// it: room above its tracks for a head into the row above, and a gap
	// tall enough for a head into the row below. A box shorter than its
	// row stands that much further off.
	trackAt := func(g, t int) int { return room[g][0] + lane*(t+1) + t/bundle*lane }
	type headIn struct {
		g, t, off int
		below     bool
	}
	var ins []headIn
	headRoom := func(g, o int, e end, sideOf int) {
		t, ok := track[[2]int{g, o}]
		if !ok {
			return
		}
		below := e.row >= 0
		if e.node != "" {
			below = sideOf < 0
		}
		off, r := 0, g-1
		if below {
			r = g
		}
		if e.node != "" && r >= 0 && r < n {
			rh := 0
			for _, id := range l.rows[r] {
				rh = max(rh, l.heights[id])
			}
			off = (rh - l.pos[e.node].H) / 2
		}
		ins = append(ins, headIn{g, t, off, below})
	}
	for i, r := range reqs {
		p := &plans[i]
		k := len(p.gaps)
		o := owner(i, k-1)
		if p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == k-1 {
			o = busOwner(p.arr)
		}
		headRoom(p.gaps[k-1], o, r.to, side(p.up, false, p.same))
		if r.kind == "mutual" {
			headRoom(p.gaps[0], owner(i, 0), r.from, side(p.up, true, p.same))
		}
	}
	for _, h := range ins {
		if !h.below {
			room[h.g][0] += max(0, straightIn-h.off-trackAt(h.g, h.t))
		}
	}
	for _, h := range ins {
		if want := trackAt(h.g, h.t) + straightIn - h.off; h.below && want > l.gapHeight(h.g, tracks, room) {
			room[h.g][1] += want - tracksHeight(tracks[h.g], room[h.g])
		}
	}
	l.place(tracks, room)
	trackY := func(g, o int) int {
		return l.gapTop[g] + trackAt(g, track[[2]int{g, o}])
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
	// bus, to its end, or where it joins its arrival bus. Lines of one kind
	// whose own parts run the same way, as two lines of one pair of buses
	// stepping between them at the pair's x do, share one path, so neither
	// hides the other from the pointer.
	own := map[string]int{}
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
				if k == 1 || p.late {
					pts = append(pts, pt{p.xs[j], trackY(p.gaps[j], o)})
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
		d := polyline(pts)
		key := fmt.Sprint(d, reqs[i].kind, joins, back(i))
		if o, ok := own[key]; ok {
			out[o].reqs, out[o].starts = append(out[o].reqs, i), append(out[o].starts, st)
			continue
		}
		own[key] = len(out)
		out = append(out, drawn{D: d, parts: [][]pt{pts}, reqs: []int{i}, starts: []pt{st}, joins: joins, back: back(i)})
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
		out = append(out, drawn{D: strings.Join(ds, " "), parts: parts, reqs: bs.members, starts: starts, net: nets, count: len(bs.members), counted: bs.members, label: lb, joins: true, back: back(bs.members[0]), own: [2]pt{s, {s.x, trackY(first.gaps[0], o)}}})
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
				if j == 0 || p.late {
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
		reach := map[string][]int{}
		for i := range reqs {
			if plans[i].arr >= 0 && drop(i) == b {
				reach[reqs[i].to.key] = append(reach[reqs[i].to.key], i)
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
			if n := reach[reqs[is[0]].to.key]; len(n) > 0 {
				dd.count, dd.counted, dd.label, dd.own = len(n), n, pt{x + 5, (y+e.y)/2 + 4}, [2]pt{{x, y}, e}
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
	backs := make([]bool, len(reqs))
	for i := range reqs {
		backs[i] = back(i)
	}
	var crooked []int
	dropped := false
	for i := range reqs {
		dropped = dropped || !l.ring && drops(i) && plans[i].dep < 0
		if drops(i) && plans[i].dep < 0 && (!l.ring && plans[i].fx != plans[i].tx && (len(leave[keys[i][0]]) > 1 || len(arrive[keys[i][1]]) > 1) || teed(reqs, ways, i)) {
			crooked = append(crooked, i)
		}
	}
	return routing{drawn: out, ways: ways, back: backs, shares: can, roomy: roomy, bent: crooked, dropped: dropped, boxes: boxes, tweaked: tweaked, joined: joined, tracks: took}
}

// teed says line i drops straight across a flat stretch of another line
// leaving its place, a T a reader takes for a branch.
func teed(reqs []wireReq, ways [][]pt, i int) bool {
	a := ways[i]
	if len(a) != 2 || a[0].x != a[1].x {
		return false
	}
	x, y0, y1 := a[0].x, min(a[0].y, a[1].y), max(a[0].y, a[1].y)
	for j, b := range ways {
		if j == i || place(reqs[j].from) != place(reqs[i].from) {
			continue
		}
		for k := 1; k < len(b); k++ {
			p, q := b[k-1], b[k]
			if p.y == q.y && y0 < p.y && p.y < y1 && min(p.x, q.x) < x && x < max(p.x, q.x) {
				return true
			}
		}
	}
	return false
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
		return fmt.Sprint(chipPlace, e.row, "|", e.lo, "|", e.hi)
	}
	return e.key
}

// chipPlace starts the place of a frame chip.
const chipPlace = "chip|"

// drawnRels are the relations the drawing records, as wanted lists them,
// between the shapes that draw their ends: a box, or the frame chip its
// ports fold into. Lint reads the same relations off those shapes.
func drawnRels(reqs []wireReq) [][2]string {
	shape := func(e end) string {
		if e.node != "" {
			return e.node
		}
		return place(e)
	}
	var out [][2]string
	for _, r := range reqs {
		out = append(out, [2]string{shape(r.from), shape(r.to)})
		if r.kind == "mutual" {
			out = append(out, [2]string{shape(r.to), shape(r.from)})
		}
	}
	return out
}

// endID names an end as the drawing names its shape: the box's id, or the
// frame port's key.
func endID(e end) string {
	if e.node != "" {
		return e.node
	}
	return e.key
}

// endsOf are the places the requests is join, each once.
func endsOf(reqs []wireReq, is []int) []string {
	var out []string
	for _, i := range is {
		for _, e := range []end{reqs[i].from, reqs[i].to} {
			if id := endID(e); !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

// wanted are the pairs of places the requests join: a box, or a frame
// port. A line between two modules that use each other stands for both
// directions.
func wanted(reqs []wireReq) [][2]string {
	var out [][2]string
	for _, r := range reqs {
		out = append(out, [2]string{endID(r.from), endID(r.to)})
		if r.kind == "mutual" {
			out = append(out, [2]string{endID(r.to), endID(r.from)})
		}
	}
	return out
}

// detourOffer is how much longer than the direct way a chain may make a
// line to a frame's chip before the line passes the rows on its own.
const detourOffer = 160

// slotGap is the least room between two lines on one edge of a box.
const slotGap = 12

// spreadRuns moves the slots of one edge, in order and spread over lo..hi
// at xs, that no pin holds. A run of them between two pins, or between a
// pin and an end, keeps its share of the edge unless that share comes
// within gap of a pin. Then it spreads evenly over the room between, and
// if that room leaves a bus no side for its ×N count, it moves into the
// empty room past the next pin. It sets each slot's x in xs and returns
// the slots' order by x.
func spreadRuns(xs []int, pinned []bool, counts []int, lo, hi, gap int) []int {
	n := len(xs)
	lay := func(k, l, h int, lp, hp bool) []int {
		a, b := 0.5, 0.5
		if lp {
			a = 1
		}
		if hp {
			b = 1
		}
		step := float64(h-l) / (float64(k-1) + a + b)
		out := make([]int, k)
		for j := range out {
			out[j] = l + int((float64(j)+a)*step)
		}
		return out
	}
	fits := func(ps, cs []int, l, h int, lp, hp bool) bool {
		for j, x := range ps {
			left, right := 1<<30, 1<<30
			if j > 0 {
				left = x - ps[j-1]
			} else if lp {
				left = x - l
			}
			if j < len(ps)-1 {
				right = ps[j+1] - x
			} else if hp {
				right = h - x
			}
			if min(left, right) < gap {
				return false
			}
			if w := textWidth(fmt.Sprintf("×%d", cs[j]), 11) + 9; cs[j] >= 2 && left < w && right < w {
				return false
			}
		}
		return true
	}
	// room is the open stretch between the pins at a and b, or an end
	// where a is -1 or b is n.
	room := func(a, b int) (int, int, bool, bool) {
		l, h := lo, hi
		if a >= 0 {
			l = xs[a]
		}
		if b < n {
			h = xs[b]
		}
		return l, h, a >= 0, b < n
	}
	for s := 0; s < n; {
		if pinned[s] {
			s++
			continue
		}
		e := s
		for e < n && !pinned[e] {
			e++
		}
		if !(s > 0 && xs[s] < xs[s-1]+gap || e < n && xs[e-1] > xs[e]-gap) {
			s = e
			continue
		}
		cs := counts[s:e]
		l, h, lp, hp := room(s-1, e)
		ps := lay(e-s, l, h, lp, hp)
		var past [][2]int
		if e < n && (e+1 == n || pinned[e+1]) {
			past = append(past, [2]int{e, e + 1})
		}
		if s > 0 && (s == 1 || pinned[s-2]) {
			past = append(past, [2]int{s - 2, s - 1})
		}
		for _, p := range past {
			if fits(ps, cs, l, h, lp, hp) {
				break
			}
			l, h, lp, hp = room(p[0], p[1])
			if q := lay(e-s, l, h, lp, hp); fits(q, cs, l, h, lp, hp) {
				ps = q
			}
		}
		copy(xs[s:e], ps)
		s = e
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return xs[order[a]] < xs[order[b]] })
	return order
}

func permute[T any](v []T, order []int) []T {
	out := make([]T, len(v))
	for i, j := range order {
		out[i] = v[j]
	}
	return out
}

// countRoom is how far a bus's track keeps from the box whose stub or drop
// carries the bus's count: the count's height and a clear margin.
const countRoom = 16

// straightIn is how far a line runs straight into its arrowhead: the
// chevron's depth and 12px of line a reader sees enter it.
const straightIn = 20

// place sets each gap's height from its tracks and the room its counts
// need above and below them, and each row below it.
func (l *layout) place(tracks []int, room [][2]int) {
	n := len(l.rows)
	l.gapTop, l.gapH = make([]int, n+1), make([]int, n+1)
	l.rowTop, l.rowH = make([]int, n), make([]int, n)
	y := l.top
	for g := 0; g <= n; g++ {
		h := l.gapHeight(g, tracks, room)
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

// gapHeight is how tall gap g stands: room for its tracks and counts, and
// never under the layout's least gap.
func (l *layout) gapHeight(g int, tracks []int, room [][2]int) int {
	h := l.minGap
	if g == 0 || g == len(l.rows) {
		h = l.endGap
	}
	if tracks[g] > 0 {
		h = max(h, tracksHeight(tracks[g], room[g]))
	}
	return h
}

// tracksHeight is how tall a gap of t tracks stands with the room given
// above and below them.
func tracksHeight(t int, room [2]int) int {
	return lane*(t+1) + lane/2 + (t-1)/bundle*lane + room[0] + room[1]
}

// shelve gives each of m crossings of one gap, in the order they were
// ranked, a track, 0 the highest. Each takes the highest track below every
// crossing ranked before it that it meets and is held by, as when their
// order crosses something either way, and clear of every other crossing it
// meets. Two that cross nothing whichever lies above may stand in either
// order, so a gap holds no more tracks than its crossings need. No crossing
// takes a track above least.
func shelve(m int, meet, held func(a, b int) bool, least func(k int) int) []int {
	t := make([]int, m)
	for s := range m {
		t[s] = least(s)
		for q := range s {
			if meet(q, s) && held(q, s) {
				t[s] = max(t[s], t[q]+1)
			}
		}
		taken := func() bool {
			for q := range s {
				if meet(q, s) && t[q] == t[s] {
					return true
				}
			}
			return false
		}
		for taken() {
			t[s]++
		}
	}
	return t
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

// joinsHarmless says the lines of departure bus dep bound for arrival bus
// arr may step down where dep's trunk runs on below gap: every place whose
// lines ride arr already reaches every place the trunk leads to below, so a
// reader who turns from arr's track down the trunk meets no place its
// lines do not reach.
func joinsHarmless(reach map[[2]string]bool, reqs []wireReq, plans []plan, buses []*bus, dep, arr, gap int) bool {
	var from, below []string
	for _, m := range buses[arr].members {
		if m < len(reqs) {
			from = append(from, endID(reqs[m].from))
		}
	}
	for i := range plans {
		q, n := &plans[i], len(plans[i].gaps)
		if q.dep != dep || q.arr == arr && q.ride == n-1 && q.gaps[n-1] == gap {
			continue
		}
		below = append(below, endID(reqs[i].to))
		if q.arr >= 0 {
			for _, m := range buses[q.arr].members {
				if m < len(reqs) {
					below = append(below, endID(reqs[m].to))
				}
			}
		}
	}
	for _, f := range from {
		for _, b := range below {
			if f != b && !reach[[2]string{f, b}] {
				return false
			}
		}
	}
	return true
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
