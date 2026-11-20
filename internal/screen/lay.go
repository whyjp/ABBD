package screen

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// layRun is one run of lay: the plans, buses and tracks it settles, each
// stage of lay reading what the stages before it left and adding its own.
type layRun struct {
	l    *layout
	reqs []wireReq
	o    layOpts
	// n is the number of rows; the gaps are numbered 0 to n.
	n int
	// shared is o.Shared kept only where lines could share an arrival bus.
	shared bool
	// can says some lines could share an arrival bus; shares are those
	// lines, grouped by the bus they would share, and shareOf says which.
	can     bool
	shares  [][]int
	shareOf []int
	// tweaked, joined and strays are the facts routing reports of the run,
	// and roomy says some ×N got room.
	tweaked, joined, roomy bool
	strays                 bool

	plans []plan
	// members and ends hold the lines by the edge each leaves and reaches;
	// leave and arrive hold them by the place each leaves and reaches (the
	// buses they would share), and keys each line's two such keys.
	members, ends, leave, arrive map[slotKey][]int
	keys                         [][2]slotKey

	chainOf []int
	// laneOf is the lane each line takes down its chain: the first lane
	// while trunks choose their leads, then as handed out.
	laneOf map[int]int
	// held says who passes each row at each x: a line by its index, a bus
	// by -1-b. Two of them at one x in one row, or in rows next to each
	// other, would meet and read as one.
	held  map[[2]int]int
	hubs  map[string][]string
	buses []*bus
	// fedBy is the shared bus that feeds each place.
	fedBy map[string]int
	// offChain holds the lines set off their chain to pass rows on their own.
	offChain map[int]bool

	// used holds the x every line takes on each edge of each box: a box's
	// chips and the box itself spread their lines apart, and a line of one
	// must not stand on a line of the other.
	used map[[2]string][]int
	// set is where each line's end already stands on its edge, by line and
	// whether it is the line's start.
	set map[int]map[bool]int
	// reach is how many lines of each arrival bus drop into each place, the
	// ×N its drop shows (draw).
	reach map[into]int

	spans  []map[int]*trackSpan
	owners [][]int
	// above[g] holds owner pairs whose first must lie above the second.
	above  []map[[2]int]bool
	track  map[[2]int]int
	took   map[[3]int]int
	tracks []int
	room   [][2]int

	ways [][]pt
	out  []drawn
	nets int
}

// into is a place an arrival bus drops into.
type into struct {
	bus   int
	place string
}

// slot is the share of an edge one bus, or one line, takes.
type slot struct {
	is    []int
	leave bool
}

type trackSpan struct {
	lo, hi      int
	owner       int
	tops, downs []int
	// styles holds 1 where a line on it runs with the flow and 2
	// where one runs against it, as back says.
	styles int
}

// lay draws every request in the modes o turns on: with shared arrival
// buses where o.Shared allows them, and with each line between modules that
// use each other on buses of its own pair where o.Apart says so.
func (l *layout) lay(reqs []wireReq, o layOpts) routing {
	l.lanes, l.beside, l.passes, l.stubs = map[[2]int]int{}, map[int][]int{}, map[int][]int{}, map[int][]int{}
	ly := &layRun{l: l, reqs: reqs, o: o, shared: o.Shared, n: len(l.rows), held: map[[2]int]int{}, fedBy: map[string]int{}, offChain: map[int]bool{}}
	ly.plan()
	ly.chains()
	ly.sharing()
	ly.departureBuses()
	ly.arrivalBuses()
	ly.leads()
	ly.stepOffs()
	ly.lateJoins()
	ly.lanesOut()
	ly.slots()
	ly.passRows()
	ly.stepDowns()
	ly.viaSteps()
	ly.buildSpans()
	ly.orderAbove()
	ly.assignTracks()
	ly.makeRoom()
	ly.drawWays()
	ly.drawOwn()
	ly.drawDepartures()
	ly.drawArrivals()
	return ly.routing()
}

// sideOf is +1 for a box's bottom, -1 for its top.
func (ly *layRun) sideOf(p *plan, isFrom bool) int {
	if p.over {
		return -1
	}
	if !p.same && isFrom == p.up {
		return -1
	}
	return 1
}

// plan lays out each line's gaps and sorts the lines into the edges they
// leave and reach, and the places whose buses they would share.
func (ly *layRun) plan() {
	l, reqs, o := ly.l, ly.reqs, ly.o
	plans := make([]plan, len(reqs))
	ly.plans = plans
	members, ends := map[slotKey][]int{}, map[slotKey][]int{}
	leave, arrive := map[slotKey][]int{}, map[slotKey][]int{}
	keys := make([][2]slotKey, len(reqs))
	// In a fanning view each folded line shares a slot only at its hub
	// (foldHubs), the end it shares with other folded lines of its kind
	// and dotting.
	var hub []string
	if l.fan {
		from, to := make([]string, len(reqs)), make([]string, len(reqs))
		fans := make([]wireReq, len(reqs))
		for i, r := range reqs {
			from[i], to[i] = place(r.from), place(r.to)
			fans[i] = r
			fans[i].fan = r.both
		}
		hub = foldHubs(fans, from, to)
	}
	for i, r := range reqs {
		p := &plans[i]
		p.dep, p.arr, p.via = -1, -1, -1
		a, b := r.from.row, r.to.row
		p.up, p.same = b < a, a == b
		p.over = p.same && l.over(r)
		switch {
		case p.over:
			p.gaps = []int{a}
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
		fk := slotKey{r.from.key, ly.sideOf(p, true)}
		tk := slotKey{r.to.key, ly.sideOf(p, false)}
		// Frame ports folded into one chip are one place: they share its
		// slots and its buses.
		at := func(e end, k slotKey) slotKey {
			k.key = place(e)
			return k
		}
		members[at(r.from, fk)] = append(members[at(r.from, fk)], i)
		ends[at(r.to, tk)] = append(ends[at(r.to, tk)], i)
		// A line within one row runs on its own under it, except on a
		// spine, whose stages send their lines down: there a line between
		// two stages that are not neighbours runs over the row, clear of
		// those lines, and rides its box's buses, so it never leaves or
		// reaches a box beside them (rule 4).
		if p.same && !l.staged(r) {
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
		// A folded pair rides buses of its own, but in a fanning view
		// (tactic.fan) folded pairs of one kind and dotting share their
		// hub's slot, a fan of lines that each read both ways, and no slot
		// of a line that reads one way; at the far end each keeps its own.
		switch {
		case r.both && hub != nil && hub[i] == place(r.from):
			dk.key += fmt.Sprint("|both|", r.plan)
			ak.key += "|" + place(r.from)
		case r.both && hub != nil && hub[i] == place(r.to):
			dk.key += "|" + place(r.to)
			ak.key += fmt.Sprint("|both|", r.plan)
		case r.both || o.Apart && r.kind == kindMutual:
			dk.key += "|" + place(r.to)
			ak.key += "|" + place(r.from)
		}
		keys[i] = [2]slotKey{dk, ak}
		leave[dk] = append(leave[dk], i)
		arrive[ak] = append(arrive[ak], i)
	}
	ly.members, ly.ends, ly.leave, ly.arrive, ly.keys = members, ends, leave, arrive, keys
}

// chains finds the chain each line runs down, and keeps the room it takes.
func (ly *layRun) chains() {
	l, reqs := ly.l, ly.reqs
	chainOf := l.chainsOf(reqs)
	strays := l.strays(reqs, chainOf)
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
	ly.chainOf, ly.strays, ly.laneOf = chainOf, strays, laneOf
}

// near is the x where line i runs down its chain in the row next to its
// start, or its end.
func (ly *layRun) near(i int, start bool) (int, bool) {
	l, reqs, chainOf, laneOf := ly.l, ly.reqs, ly.chainOf, ly.laneOf
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

// runsOn says line i, off its departure trunk, may pass a row at the x
// where its trunk o passed the row next to it: the trunk ended there
// and the line goes straight on, so the two read as the one line they
// are, where stepping aside would turn it twice for nothing. Not where
// another line leaves the trunk in the same gap for an arrival bus: it
// steps down onto that bus's track at the trunk's x, and the track
// would end on this line.
func (ly *layRun) runsOn(i, o int) bool {
	plans := ly.plans
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
func (ly *layRun) runs(i, r, owner int) (int, bool) {
	l, reqs, plans, chainOf, laneOf, held := ly.l, ly.reqs, ly.plans, ly.chainOf, ly.laneOf, ly.held
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
		if o, ok := held[[2]int{q, x}]; ok && o != owner && (q == r || owner != i && (plans[i].arr < 0 || owner != -1-plans[i].arr) || !ly.runsOn(i, o)) {
			return 0, false
		}
	}
	held[[2]int{r, x}] = owner
	l.passes[r] = append(l.passes[r], x)
	return x, true
}

// sharing settles which lines could share an arrival bus, and which boxes
// are hubs.
func (ly *layRun) sharing() {
	reqs := ly.reqs
	shareOf, shares := share(reqs, ly.arrive)
	can := len(shares) > 0
	ly.shareOf, ly.shares, ly.can = shareOf, shares, can
	if ly.shared = ly.shared && can; !ly.shared {
		for i := range shareOf {
			shareOf[i] = -1
		}
		ly.shares = nil
	}
	ly.hubs = hubsOf(drawnRels(reqs), func(k string) bool { return !strings.HasPrefix(k, chipPlace) })
}

// drops says line i can drop straight from box to box: they stand in
// rows next to each other and their spans meet, so it needs no bus.
func (ly *layRun) drops(i int) bool {
	l, o, reqs, plans := ly.l, ly.o, ly.reqs, ly.plans
	f, t := reqs[i].from, reqs[i].to
	_, hub := ly.hubs[t.node]
	return (l.ring && !o.Bent[i] || o.Drops && !o.Bent[i] && ly.shareOf[i] < 0 && !hub) && !plans[i].same && len(plans[i].gaps) == 1 && f.node != "" && t.node != "" && max(f.lo, t.lo) <= min(f.hi, t.hi)
}

func (ly *layRun) alone(is []int) []int { return slices.DeleteFunc(slices.Clone(is), ly.drops) }

// A hub, a box hubUsers or more places use, still takes its lines as
// one trunk (H2).
func (ly *layRun) free(is []int) []int {
	reqs := ly.reqs
	return slices.DeleteFunc(slices.Clone(is), func(i int) bool { _, hub := ly.hubs[reqs[i].to.node]; return ly.drops(i) && !hub })
}

// departureBuses gathers the lines each place sends the same way onto one
// bus each.
func (ly *layRun) departureBuses() {
	reqs, plans, leave, shareOf := ly.reqs, ly.plans, ly.leave, ly.shareOf
	for _, k := range sortedKeys(leave) {
		// Where a place's other lines fork into the row below it anyway,
		// its straight drops join the fork too: one stem, one track, a
		// drop into each box, and the stem over one of them (stemOver), so
		// no stub runs beside the stem through its track.
		is := ly.alone(leave[k])
		oneRow := !slices.ContainsFunc(leave[k], func(i int) bool {
			return len(plans[i].gaps) != 1 || place(reqs[i].from) != place(reqs[leave[k][0]].from)
		})
		if len(is) >= 2 && oneRow {
			is = leave[k]
		}
		if len(is) >= 2 {
			for _, i := range is {
				plans[i].dep = len(ly.buses)
			}
			// A box whose lines all ride one shared bus drops its stub
			// straight onto that bus's track.
			if s := shareOf[is[0]]; s >= 0 && !slices.ContainsFunc(is, func(i int) bool { return shareOf[i] != s }) {
				for _, i := range is {
					plans[i].onto = true
				}
				ly.buses = append(ly.buses, &bus{members: is, passage: map[int]int{}, lead: -1})
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
			ly.buses = append(ly.buses, &bus{members: is, passage: map[int]int{}, deep: deep, lead: -1})
		}
	}
}

// arrivalBuses gathers the lines into one place onto buses: a shared bus
// for the places that share one, and a bus of its own for the lines into
// each other box.
func (ly *layRun) arrivalBuses() {
	reqs, plans, shareOf, fedBy := ly.reqs, ly.plans, ly.shareOf, ly.fedBy
	// Each shared bus gathers its places' lines into one track and feeds
	// every box on it. Every other line into one of those boxes joins the
	// bus's drop into it, on a bus of its own whose track ends on the drop.
	for _, is := range ly.shares {
		for _, i := range is {
			plans[i].arr = len(ly.buses)
			fedBy[place(reqs[i].to)+"|"+reqs[i].kind] = len(ly.buses)
		}
		ly.buses = append(ly.buses, &bus{members: is, arrive: true, passage: map[int]int{}, lead: -1, fed: -1})
	}
	for _, is := range gather(reqs, ly.arrive, ly.shared) {
		fed := -1
		is = slices.DeleteFunc(ly.free(is), func(i int) bool { return shareOf[i] >= 0 })
		for _, i := range is {
			if b, ok := fedBy[place(reqs[i].to)+"|"+reqs[i].kind]; ok && !plans[i].up && !reqs[i].both {
				fed = b
			}
		}
		if len(is) > 0 && (fed >= 0 || len(is) >= busAt && !ly.oneBus(is)) {
			for _, i := range is {
				plans[i].arr = len(ly.buses)
			}
			ly.buses = append(ly.buses, &bus{members: is, arrive: true, passage: map[int]int{}, lead: -1, fed: fed})
		}
	}
}

// oneBus says lines that all ride one departure bus, one into each box,
// already travel as one; they split off its track straight into their
// boxes, without a second bus.
func (ly *layRun) oneBus(is []int) bool {
	reqs, plans := ly.reqs, ly.plans
	seen := map[string]bool{}
	for _, i := range is {
		if d := plans[i].dep; d < 0 || d != plans[is[0]].dep || seen[place(reqs[i].to)] {
			return false
		}
		seen[place(reqs[i].to)] = true
	}
	return true
}

// drop is the bus that draws the drop line i ends on: the shared bus
// that feeds the place it reaches, else its own.
func (ly *layRun) drop(i int) int {
	reqs, plans, buses, fedBy := ly.reqs, ly.plans, ly.buses, ly.fedBy
	if b := plans[i].arr; b >= 0 && buses[b].fed >= 0 {
		if f, ok := fedBy[place(reqs[i].to)+"|"+reqs[i].kind]; ok && !plans[i].up {
			return f
		}
	}
	return plans[i].arr
}

func (ly *layRun) center(e end) int { return (e.lo + e.hi) / 2 }

// leads gives each bus the line whose chain its trunk runs down.
func (ly *layRun) leads() {
	reqs, plans, buses := ly.reqs, ly.plans, ly.buses
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
				lo, hi = min(lo, ly.center(reqs[m].to)), max(hi, ly.center(reqs[m].to))
			}
			at = (lo + hi) / 2
		} else {
			at = ly.center(reqs[b.members[0]].from)
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
			x, ok := ly.near(m, !b.arrive)
			if !ok || own(m) == 0 || !b.arrive && own(m) < b.deep {
				continue
			}
			if b.lead == len(reqs) {
				b.lead = m
				continue
			}
			lx, _ := ly.near(b.lead, !b.arrive)
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
}

// stepOffs lets a line that steps off its departure trunk lead its arrival
// bus.
func (ly *layRun) stepOffs() {
	o, reqs, plans, buses, chainOf, offChain := ly.o, ly.reqs, ly.plans, ly.buses, ly.chainOf, ly.offChain
	// A line that steps down onto its arrival bus's track off a departure
	// trunk that passed the last row down a lane of the chain the bus's
	// lead runs down too would meet that lead's trunk a lane off its own
	// step and jog onto the drop. It leads the bus instead, so the drop
	// stands under its step and it runs straight on down; the bus's other
	// lines on that chain pass the rows on their own, off it, straight on
	// where they can, and join on the track.
	for _, b := range buses {
		if !o.Early || !b.arrive || b.lead >= len(reqs) || chainOf[b.lead] < 0 || plans[b.lead].dep >= 0 {
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
			b.lead, ly.joined = s, true
			break
		}
	}
}

// lateJoins sets a line that would run over a whole box to its bus down its
// own chain, joining the bus on the track.
func (ly *layRun) lateJoins() {
	l, o, reqs, plans, buses, chainOf := ly.l, ly.o, ly.reqs, ly.plans, ly.buses, ly.chainOf
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
			p.late = o.Tweak && ok && mine && (bx < t.lo && ox > t.hi || bx > t.hi && ox < t.lo)
			ly.tweaked = ly.tweaked || p.late
		}
	}
}

// lanesOut hands out the lanes of each chain.
func (ly *layRun) lanesOut() {
	l, reqs, plans, buses, chainOf := ly.l, ly.reqs, ly.plans, ly.buses, ly.chainOf
	// A chain's lanes go to the lines that run down it themselves: each
	// trunk's lead first, then every line that passes rows off any trunk.
	// A line that rides trunks all the way needs no lane of its own.
	{
		used := map[int]int{}
		laneOf := map[int]int{}
		ly.laneOf = laneOf
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
}

// slots spreads the lines over each end's range: one slot per bus, one
// per other line, ordered by where they lead. Lines that leave an edge
// and lines that reach it share the edge, so no two of them meet there.
func (ly *layRun) slots() {
	reqs, plans, members, ends := ly.reqs, ly.plans, ly.members, ly.ends

	edges := map[slotKey][]int{}
	for k := range members {
		edges[k] = nil
	}
	for k := range ends {
		edges[k] = nil
	}
	ly.used = map[[2]string][]int{}
	ly.set = map[int]map[bool]int{}
	ly.reach = map[into]int{}
	for i := range reqs {
		if plans[i].arr >= 0 {
			ly.reach[into{ly.drop(i), reqs[i].to.key}]++
		}
	}
	for _, k := range sortedKeys(edges) {
		ly.edge(k)
	}
}

// shown is the count a slot's stub or drop shows.
func (ly *layRun) shown(sl slot) int {
	reqs, plans, buses := ly.reqs, ly.plans, ly.buses
	i := sl.is[0]
	if b := plans[i].dep; sl.leave && b >= 0 {
		return len(buses[b].members)
	}
	if b := ly.drop(i); !sl.leave && b >= 0 {
		return ly.reach[into{b, reqs[i].to.key}]
	}
	return 0
}

// edge places the slots of one edge of a box.
func (ly *layRun) edge(k slotKey) {
	l, reqs, plans, members, ends, used, set := ly.l, ly.reqs, ly.plans, ly.members, ly.ends, ly.used, ly.set
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
		add(i, ly.drop(i), false)
	}
	other := func(sl slot) int {
		if x, ok := ly.pin(sl); ok {
			return x
		}
		sum := 0
		for _, i := range sl.is {
			if sl.leave {
				sum += ly.center(reqs[i].to)
			} else {
				sum += ly.center(reqs[i].from)
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
		x, ok := ly.pin(sl)
		if ok && x >= r.lo && x <= r.hi && (r.node == "" || apart(x)) {
			xs[s], pinned[s] = x, true
		} else if x, over := ly.stemOver(sl, xs[s]); !ok && over && (r.node == "" || apart(x)) {
			xs[s], pinned[s] = x, true
		} else if i := sl.is[0]; (!ok || !l.ring) && len(sl.is) == 1 && ly.drops(i) {
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
		n := ly.shown(slots[s])
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
			ly.roomy = ly.roomy || !slices.Equal(zs, ys)
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

// A line alone in its slot that runs down a chain, or a bus whose
// trunk does, wants to leave or reach the box right above or below
// that chain, so it drops straight.
func (ly *layRun) pin(sl slot) (int, bool) {
	o, reqs, plans, buses, set := ly.o, ly.reqs, ly.plans, ly.buses, ly.set
	i := sl.is[0]
	if b := plans[i].dep; sl.leave && b >= 0 {
		i = buses[b].lead
		// A stub with no trunk below it stands over a line of its
		// own that leads an arrival trunk down its chain, so the
		// stub runs straight on into that trunk.
		if o.Tweak && i == len(reqs) && buses[b].deep == 0 {
			for _, m := range sl.is {
				if a := plans[m].arr; a < 0 || buses[a].lead != m {
					continue
				}
				if x, ok := ly.near(m, true); ok {
					ly.tweaked = true
					return x, true
				}
			}
		}
	} else if b := ly.drop(i); !sl.leave && b >= 0 {
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
				if x, ok := ly.near(m, false); ok && x >= t.lo && x <= t.hi {
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
	if x, ok := ly.near(i, sl.leave); ok {
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
func (ly *layRun) stemOver(sl slot, want int) (int, bool) {
	reqs, set := ly.reqs, ly.set
	best, found := 0, false
	for _, i := range sl.is {
		if !sl.leave || len(sl.is) < 2 || !ly.drops(i) {
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

// passRow is the row the line passes between gaps j-1 and j.
func passRow(p *plan, j int) int {
	if p.up {
		return p.gaps[j-1] - 1
	}
	return p.gaps[j-1]
}

// passRows settles the x of each passage through a row. A line on a
// departure trunk passes the rows it rides on the trunk, which heads row by
// row from where it leaves toward the middle of the places its lines still
// reach. A line on an arrival bus passes every other row on the bus's
// trunk, which runs straight above the middle of the places it reaches.
func (ly *layRun) passRows() {
	l, reqs, plans, buses, chainOf, laneOf, held, offChain := ly.l, ly.reqs, ly.plans, ly.buses, ly.chainOf, ly.laneOf, ly.held, ly.offChain
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
					if cx, ok := ly.runs(b.lead, r, -1-p.dep); ok {
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
						if to := reqs[m].to.row; !ly.shared || plans[m].up && to < r || !plans[m].up && to > r {
							lo, hi = min(lo, plans[m].tx), max(hi, plans[m].tx)
						}
					}
					if cx, ok := ly.runs(b.lead, r, -1-p.arr); ok {
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
			if x, ok := ly.runs(i, r, i); ok {
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
}

// stepDowns settles where a line that rides its departure trunk into its
// last gap steps down onto its arrival bus's track. Lines of one
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
func (ly *layRun) stepDowns() {
	l, reqs, plans, buses := ly.l, ly.reqs, ly.plans, ly.buses
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

// viaSteps keeps one departure track meeting each arrival trunk, at the
// trunk's top: a track that met it lower down would let a reader follow the
// trunk's lines from above onto that track and into the other places
// its bus reaches. Every other line that leaves a departure trunk for
// the arrival trunk steps down off its departure track a little before
// the trunk, onto a track of its own that ends on the trunk; a reader
// coming along the trunk cannot climb that step. A line whose track
// would lead such a reader nowhere its lines do not go (joinsHarmless)
// needs no step and runs its track onto the trunk. It still holds the
// x its step would take, so the steps of the others stand as they did.
func (ly *layRun) viaSteps() {
	l, reqs, plans, buses := ly.l, ly.reqs, ly.plans, ly.buses
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
}

func busOwner(b int) int { return -1 - b }

// owner says whose track crossing j of line i runs on: its departure
// bus while it rides that trunk, its arrival bus after it has joined
// that trunk, else its own. A line that rides its departure trunk into
// its last gap crosses on it to the drop and meets the arrival bus
// there.
func (ly *layRun) owner(i, j int) int {
	plans := ly.plans
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

// back says line i runs against the reading direction, rule 13.
func (ly *layRun) back(i int) bool {
	reqs, plans := ly.reqs, ly.plans
	p := &plans[i]
	against := p.up
	if p.same {
		against = p.tx < p.fx
	}
	return against != (reqs[i].kind == kindProvides)
}

// buildSpans lays down the stretches each line, or bus, crosses each gap
// by.
func (ly *layRun) buildSpans() {
	n, reqs, plans := ly.n, ly.reqs, ly.plans
	spans := make([]map[int]*trackSpan, n+1)
	owners := make([][]int, n+1)
	// above[g] holds owner pairs whose first must lie above the second.
	above := make([]map[[2]int]bool, n+1)
	ly.spans, ly.owners, ly.above = spans, owners, above
	addSpan := func(g, o, i, a, b int, up, same, over bool) {
		if spans[g] == nil {
			spans[g] = map[int]*trackSpan{}
			above[g] = map[[2]int]bool{}
		}
		sp, ok := spans[g][o]
		if !ok {
			sp = &trackSpan{lo: min(a, b), hi: max(a, b), owner: o}
			spans[g][o] = sp
			owners[g] = append(owners[g], o)
		}
		sp.lo, sp.hi = min(sp.lo, a, b), max(sp.hi, a, b)
		sp.styles |= map[bool]int{false: 1, true: 2}[ly.back(i)]
		switch {
		case over:
			sp.downs = append(sp.downs, a, b)
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
			o := ly.owner(i, j)
			if p.via >= 0 && j == p.ride {
				d := busOwner(p.dep)
				addSpan(g, d, i, a, p.via, p.up, false, false)
				addSpan(g, i, i, p.via, b, p.up, false, false)
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
			addSpan(g, o, i, a, b, p.up, p.same, p.over)
		}
		if p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == len(p.gaps)-1 {
			g := p.gaps[p.ride]
			addSpan(g, busOwner(p.arr), i, p.xs[p.ride], p.tx, p.up, false, false)
			d, a := busOwner(p.dep), busOwner(p.arr)
			if p.up {
				d, a = a, d
			}
			above[g][[2]int{d, a}] = true
		}
	}
}

// orderAbove sets which owners' tracks must lie above which in a gap.
func (ly *layRun) orderAbove() {
	reqs, plans, buses, above := ly.reqs, ly.plans, ly.buses, ly.above
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
				if m != i && q.gaps[j-1] == g && passRow(q, j) == r && ly.owner(m, j) == busOwner(p.arr) && ly.owner(m, j-1) != ly.owner(i, p.ride) {
					above[g][[2]int{ly.owner(i, p.ride), ly.owner(m, j-1)}] = true
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
}

// crossing keys a crossing's track by what stays when the view is laid
// again: a line by its request, a bus by its first line and side.
func (ly *layRun) crossing(g, o int) [3]int {
	buses := ly.buses
	if o >= 0 {
		return [3]int{g, o, 0}
	}
	b := buses[-1-o]
	return [3]int{g, b.members[0], map[bool]int{false: 1, true: 2}[b.arrive]}
}

// assignTracks gives each owner's crossing of a gap a track.
func (ly *layRun) assignTracks() {
	n, l, spans, owners, above := ly.n, ly.l, ly.spans, ly.owners, ly.above
	inside := func(xs []int, s *trackSpan) int {
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
	tracks := make([]int, n+1)
	ly.track, ly.took, ly.tracks = track, took, tracks
	for g := range spans {
		// cost is how many crossings putting a above b makes between them,
		// then, on an overview drawn from its record, whether that sets a
		// line against the flow above one with it, which would part lines
		// with the flow that could share a road (layout.recorded).
		cost := func(a, b *trackSpan) int {
			c := (inside(a.downs, b) + inside(b.tops, a)) << 8
			if l.recorded && a.styles == 2 && b.styles == 1 && a.lo < b.hi && b.lo < a.hi {
				c++
			}
			if above[g][[2]int{b.owner, a.owner}] {
				c += 1 << 24
			}
			return c
		}
		left := slices.Clone(owners[g])
		var order []*trackSpan
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
		// On an overview drawn from its record, a line against the flow
		// beside one with it, on the next track, reads as one line drawn
		// twice, since the two never share ink: it keeps a track clear
		// between them (layout.recorded).
		clear := func(a, b int) bool {
			sa, sb := order[a].styles, order[b].styles
			return l.recorded && sa != sb && sa != 3 && sb != 3 && order[a].lo < order[b].hi && order[b].lo < order[a].hi
		}
		held := func(a, b int) bool { return cost(order[a], order[b]) >= 1<<8 || cost(order[b], order[a]) >= 1<<8 }
		least := func(k int) int { return l.held[ly.crossing(g, order[k].owner)] }
		for k, t := range shelve(len(order), meet, held, clear, least) {
			track[[2]int{g, order[k].owner}] = t
			took[ly.crossing(g, order[k].owner)] = t
			tracks[g] = max(tracks[g], t+1)
		}
	}
}

// trackAt is how far down its gap a track lies.
func (ly *layRun) trackAt(g, t int) int { return ly.room[g][0] + lane*(t+1) + t/bundle*lane }

// makeRoom sizes each gap for its tracks and the counts and arrowheads at
// its ends, and places the rows.
func (ly *layRun) makeRoom() {
	n, l, reqs, plans, buses, track, tracks := ly.n, ly.l, ly.reqs, ly.plans, ly.buses, ly.track, ly.tracks
	// A bus's count stands beside its stub or its drop, rule 22, so the gap
	// it stands in keeps room for it between the box and the nearest track.
	room := make([][2]int, n+1)
	ly.room = room
	for b, bs := range buses {
		if !bs.arrive {
			p := &plans[bs.members[0]]
			room[p.gaps[0]][map[bool]int{true: 1}[p.up]] = countRoom
			continue
		}
		reach := map[string]int{}
		for i := range reqs {
			if plans[i].arr >= 0 && ly.drop(i) == b {
				reach[reqs[i].to.key]++
			}
		}
		for _, i := range bs.members {
			if p := &plans[i]; ly.drop(i) == b && reach[reqs[i].to.key] > 1 {
				room[p.gaps[len(p.gaps)-1]][map[bool]int{false: 1}[p.up]] = countRoom
			}
		}
	}
	// A line runs straight into its arrowhead for at least straightIn, so
	// a gap keeps that room between a box and the track a line leaves for
	// it: room above its tracks for a head into the row above, and a gap
	// tall enough for a head into the row below. A box shorter than its
	// row stands that much further off.
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
		o := ly.owner(i, k-1)
		if p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == k-1 {
			o = busOwner(p.arr)
		}
		headRoom(p.gaps[k-1], o, r.to, ly.sideOf(p, false))
		if r.twoWay() {
			headRoom(p.gaps[0], ly.owner(i, 0), r.from, ly.sideOf(p, true))
		}
	}
	for _, h := range ins {
		if !h.below {
			room[h.g][0] += max(0, straightIn-h.off-ly.trackAt(h.g, h.t))
		}
	}
	for _, h := range ins {
		if want := ly.trackAt(h.g, h.t) + straightIn - h.off; h.below && want > l.gapHeight(h.g, tracks, room) {
			room[h.g][1] += want - tracksHeight(tracks[h.g], room[h.g])
		}
	}
	l.place(tracks, room)
}

func (ly *layRun) trackY(g, o int) int {
	return ly.l.gapTop[g] + ly.trackAt(g, ly.track[[2]int{g, o}])
}

func (ly *layRun) endY(e end, sideOf int) int {
	l, n := ly.l, ly.n
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

func (ly *layRun) startOf(i int) pt {
	p := &ly.plans[i]
	return pt{p.fx, ly.endY(ly.reqs[i].from, ly.sideOf(p, true))}
}

func (ly *layRun) endOf(i int) pt {
	p := &ly.plans[i]
	return pt{p.tx, ly.endY(ly.reqs[i].to, ly.sideOf(p, false))}
}

// drawWays works out the points each line turns at.
func (ly *layRun) drawWays() {
	reqs, plans := ly.reqs, ly.plans
	ways := make([][]pt, len(reqs))
	for i := range reqs {
		p := &plans[i]
		k := len(p.gaps)
		w := []pt{ly.startOf(i)}
		for j := 0; j < k; j++ {
			y := ly.trackY(p.gaps[j], ly.owner(i, j))
			if p.dep >= 0 && p.via >= 0 && j == p.ride {
				d := ly.trackY(p.gaps[j], busOwner(p.dep))
				w = append(w, pt{p.xs[j], d}, pt{p.via, d}, pt{p.via, y})
			} else {
				w = append(w, pt{p.xs[j], y})
			}
			w = append(w, pt{p.xs[j+1], y})
		}
		if p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride == k-1 {
			y := ly.trackY(p.gaps[k-1], busOwner(p.arr))
			w = append(w, pt{p.xs[k], y}, pt{p.tx, y})
		}
		ways[i] = straight(append(w, ly.endOf(i)))
	}
	ly.ways = ways
}

// drawOwn draws each line's own part.
func (ly *layRun) drawOwn() {
	reqs, plans := ly.reqs, ly.plans
	var out []drawn
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
			pts = append(pts, pt{p.via, ly.trackY(g, busOwner(p.dep))}, pt{p.via, ly.trackY(g, i)}, pt{p.xs[j], ly.trackY(g, i)})
		} else if p.dep >= 0 {
			j = p.ride + 1
			pts = append(pts, pt{p.xs[j], ly.trackY(p.gaps[j-1], busOwner(p.dep))})
		} else {
			st = ly.startOf(i)
			pts = append(pts, st)
		}
		joins := false
		for ; j < k; j++ {
			o := ly.owner(i, j)
			if o != i {
				// The trunk draws the way down through a row; a one-gap
				// line steps onto the track itself.
				if k == 1 || p.late {
					pts = append(pts, pt{p.xs[j], ly.trackY(p.gaps[j], o)})
				}
				joins = true
				break
			}
			if a, b := p.xs[j], p.xs[j+1]; a != b || p.same || p.arr >= 0 {
				ty := ly.trackY(p.gaps[j], i)
				pts = append(pts, pt{a, ty}, pt{b, ty})
			}
		}
		if p.dep >= 0 && p.arr >= 0 && p.ride == k-1 {
			pts = append(pts, pt{p.xs[k], ly.trackY(p.gaps[k-1], busOwner(p.arr))})
			joins = true
		}
		if !joins {
			pts = append(pts, ly.endOf(i))
		}
		if len(pts) < 2 || pts[0] == pts[len(pts)-1] {
			continue
		}
		d := polyline(pts)
		key := fmt.Sprint(d, reqs[i].kind, joins, ly.back(i))
		// A folded pair's line shares no path: its heads would stand on
		// another line's ink (fold.go).
		if reqs[i].both {
			key += fmt.Sprint("|both", i)
		}
		if o, ok := own[key]; ok {
			out[o].reqs, out[o].starts = append(out[o].reqs, i), append(out[o].starts, st)
			continue
		}
		own[key] = len(out)
		out = append(out, drawn{D: d, parts: [][]pt{pts}, reqs: []int{i}, starts: []pt{st}, joins: joins, back: ly.back(i)})
	}
	ly.out = out
}

// drawDepartures draws the departure buses.
func (ly *layRun) drawDepartures() {
	plans, buses, out, nets := ly.plans, ly.buses, ly.out, ly.nets
	// Departure buses: one stub out of the box, the trunk down through the
	// rows its lines share, and a track in each gap where some leave it.
	for b, bs := range buses {
		if bs.arrive {
			continue
		}
		nets++
		o := busOwner(b)
		first := plans[bs.members[0]]
		s := ly.startOf(bs.members[0])
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
			st := ly.startOf(i)
			if !slices.Contains(starts, st) {
				parts = append(parts, []pt{st, {st.x, ly.trackY(p.gaps[0], o)}})
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
					parts = append(parts, []pt{{p.xs[j], ly.trackY(p.gaps[j-1], o)}, {p.xs[j], ly.trackY(g, o)}})
				}
			}
		}
		for _, g := range gaps {
			if lh := span[g]; lh[0] < lh[1] {
				y := ly.trackY(g, o)
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
		out = append(out, drawn{D: strings.Join(ds, " "), parts: parts, reqs: bs.members, starts: starts, net: nets, count: len(bs.members), counted: bs.members, label: lb, joins: true, back: ly.back(bs.members[0]), own: [2]pt{s, {s.x, ly.trackY(first.gaps[0], o)}}})
	}
	ly.out, ly.nets = out, nets
}

// drawArrivals draws the arrival buses.
func (ly *layRun) drawArrivals() {
	reqs, plans, buses, out, nets := ly.reqs, ly.plans, ly.buses, ly.out, ly.nets
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
				if ly.owner(i, j) != o {
					continue
				}
				stop(p.gaps[j], p.xs[j])
				stop(p.gaps[j], p.xs[j+1])
				if j == 0 || p.late {
					continue
				}
				r := passRow(p, j)
				from, to := ly.trackY(p.gaps[j-1], ly.owner(i, j-1)), ly.trackY(p.gaps[j], o)
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
			if ly.owner(i, k-1) != o {
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
			if y := ly.trackY(ps.top, o); slices.Contains(stops[ps.top], ps.x) && (y-ps.reach)*(ps.ty-y) > 0 {
				part = append(part, pt{ps.x, y})
			}
			parts = append(parts, append(part, pt{ps.x, ps.ty}))
		}
		for _, g := range gaps {
			xs := slices.Sorted(slices.Values(stops[g]))
			if len(xs) < 2 {
				continue
			}
			y := ly.trackY(g, o)
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
			out = append(out, drawn{D: strings.Join(ds, " "), parts: parts, reqs: bs.members, starts: none, net: nets, joins: true, back: ly.back(lead)})
		}
		// Each box the bus feeds shows how many lines go there, its own and
		// those of the buses whose tracks end on its drop.
		reach := map[string][]int{}
		for i := range reqs {
			if plans[i].arr >= 0 && ly.drop(i) == b {
				reach[reqs[i].to.key] = append(reach[reqs[i].to.key], i)
			}
		}
		drops := map[int][]int{}
		var xs []int
		for _, i := range bs.members {
			p := &plans[i]
			if ly.drop(i) != b {
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
			y := ly.trackY(p.gaps[len(p.gaps)-1], o)
			e := ly.endOf(is[0])
			line := []pt{{x, y}, e}
			dd := drawn{D: polyline(line), parts: [][]pt{line}, reqs: is, starts: none[:len(is)], net: nets, back: ly.back(is[0])}
			if n := reach[reqs[is[0]].to.key]; len(n) > 0 {
				dd.count, dd.counted, dd.label, dd.own = len(n), n, pt{x + 5, (y+e.y)/2 + 4}, [2]pt{{x, y}, e}
				delete(reach, reqs[is[0]].to.key)
			}
			out = append(out, dd)
		}
	}
	ly.out, ly.nets = out, nets
}

// routing assembles what the run drew.
func (ly *layRun) routing() routing {
	l, reqs, plans, out, ways, leave, arrive, keys := ly.l, ly.reqs, ly.plans, ly.out, ly.ways, ly.leave, ly.arrive, ly.keys
	for k := range out {
		out[k].kind = reqs[out[k].reqs[0]].kind
		out[k].both = slices.ContainsFunc(out[k].reqs, func(i int) bool { return reqs[i].both })
	}
	for i := range reqs {
		if p := &plans[i]; p.dep >= 0 && !p.onto && p.arr >= 0 && p.ride < len(p.gaps)-1 && p.via < 0 && len(out) > 0 {
			at := pt{p.xs[p.ride+1], ly.trackY(p.gaps[p.ride], busOwner(p.dep))}
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
				b.y, b.h = ly.endY(e, 1)-3, l.chips[1]
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
		backs[i] = ly.back(i)
	}
	var crooked []int
	dropped := false
	for i := range reqs {
		dropped = dropped || !l.ring && ly.drops(i) && plans[i].dep < 0
		if ly.drops(i) && plans[i].dep < 0 && (!l.ring && plans[i].fx != plans[i].tx && (len(leave[keys[i][0]]) > 1 || len(arrive[keys[i][1]]) > 1) || teed(reqs, ways, i)) {
			crooked = append(crooked, i)
		}
	}
	return routing{drawn: out, ways: ways, back: backs, shares: ly.can, roomy: ly.roomy, strays: ly.strays, bent: crooked, dropped: dropped, boxes: boxes, tweaked: ly.tweaked, joined: ly.joined, tracks: ly.took}
}
