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
	// contract to its unit, so it runs the other way. A piece of road is
	// back where every line it carries is.
	back bool
	// joinsAt are where a line steps off its departure track onto an
	// arrival trunk.
	joinsAt []join
	kind    string
	// own is the stretch of ink the count stands beside.
	own [2]pt
	// both says the path carries a line that reads both ways, folded
	// (fold.go): it has a head where it leaves its box too.
	both bool
}

type pt struct{ x, y int }

var noStart = pt{-1, -1}

type plan struct {
	gaps []int
	// xs are where the line crosses each gap: xs[0] leaves the start,
	// xs[j] passes the row between gaps j-1 and j, the last reaches the end.
	xs []int
	// up says the line climbs: it leaves a box's top and enters one's bottom.
	// over says a line within one row runs in the gap above it, from top to
	// top, as a line between two stages of a spine does.
	up, same, over bool
	fx, tx         int
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
//
// Lines between stages that stand side by side in one row (hops) take no
// part in any of that: they run straight across between the two boxes.
func (l *layout) route(reqs []wireReq) routing {
	var r routing
	if hop := l.hops(reqs); hop != nil {
		r = l.withHops(reqs, hop)
	} else {
		r = l.routeLines(reqs)
	}
	anchor(r.drawn, pieces(r.drawn), r.boxes)
	return r
}

// routeLines is route but for the counts and the hops: the built-in
// policy routes the lines (router.go), and the lines of the drawing it keeps
// ride roads where the view has them.
func (l *layout) routeLines(reqs []wireReq) routing {
	if f := l.hooks.onRouting(); f != nil {
		f(l, reqs)
	}
	s := builtin{}.route(newRouter(*l, reqs))
	*l = s.at
	r := s.r
	if !l.roads {
		return r
	}
	// A folded pair rides the roads as the one line it is drawn as.
	if folded, f, ok := r.fold(); ok {
		fr := f.reqs(reqs)
		for i := range fr {
			fr[i].fan = fr[i].both && l.fan
		}
		folded.confluent(fr, l.position, len(l.rows), l.apart, l.hooks.onRoads())
		return f.unfold(folded)
	}
	r.confluent(reqs, l.position, len(l.rows), l.apart, l.hooks.onRoads())
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
	// With through, so does a line between two boxes whose chain the
	// layout set down past a full row's end (tactic.through).
	for i, r := range reqs {
		if c := chainOf[i]; c >= 0 && (r.from.node == "" || r.to.node == "" || l.through) && l.astray(c, r) {
			chainOf[i] = -1
		}
	}
	return chainOf
}

// astray says chain c passes some row between r's ends more than
// detourOffer off the way between them.
func (l *layout) astray(c int, r wireReq) bool {
	a, b := (r.from.lo+r.from.hi)/2, (r.to.lo+r.to.hi)/2
	for row := min(r.from.row, r.to.row) + 1; row < max(r.from.row, r.to.row); row++ {
		if x := l.pass[[2]int{c, row}]; abs(x-a)+abs(x-b)-abs(a-b) > detourOffer {
			return true
		}
	}
	return false
}

// strays says some line between two boxes runs down a chain astray, so
// drawing through the rows' gaps would draw something else.
func (l *layout) strays(reqs []wireReq, chainOf []int) bool {
	for i, r := range reqs {
		if c := chainOf[i]; c >= 0 && r.from.node != "" && r.to.node != "" && l.astray(c, r) {
			return true
		}
	}
	return false
}

// bundlesPairs says some two lines between modules that use each other
// leave or reach one box toward different places, so bundling them would
// join those places.
func bundlesPairs(reqs []wireReq) bool {
	far := map[string]string{}
	for _, r := range reqs {
		if r.kind != kindMutual {
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

// keep is a copy of the layout to lay on, leaving the layout as it was:
// lay sets the gaps and rows afresh, and the boxes' y in place.
func (l *layout) keep() layout {
	k := *l
	k.pos, k.pass = maps.Clone(l.pos), maps.Clone(l.pass)
	return k
}

const shareMost = 200

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
	// strays says the through tactic would set some line off its chain.
	strays bool
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
	// rider holds, for each line, the line the router folded it into
	// (fold.go), or -1; nil where it folded none.
	rider []int
}

// layOpts are the modes lay draws a view's lines in, the ones route turns
// to keep the drawing that reads best.
type layOpts struct {
	// Shared lets lines share arrival buses where they could (share).
	Shared bool `json:"shared,omitempty"`
	// Apart puts each line between two modules that use each other on
	// buses of its own pair (rule 18).
	Apart bool `json:"apart,omitempty"`
	// Tweak sets lines straight onto their arrival trunks (layStraight).
	Tweak bool `json:"tweak,omitempty"`
	// Early lets a line that steps off its departure trunk lead its arrival
	// bus (layJoined).
	Early bool `json:"early,omitempty"`
	// Drops takes a line that can drop straight off its buses in any view,
	// not only in a focus view (layDrops). Bent are the lines kept on their
	// buses anyway, since they did not stand straight; only Drops sets any.
	Drops bool         `json:"drops,omitempty"`
	Bent  map[int]bool `json:"bent,omitempty"`
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
		// A folded pair's line rides no bus but its own (fold.go).
		if k.side > 0 || reqs[is[0]].to.node == "" || reqs[is[0]].both {
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
			if shared || to.node == "" || reqs[h.members[0]].to.node == "" || h.row != g.row || h.way != g.way || h.kind != g.kind || reqs[is[0]].both || reqs[h.members[0]].both {
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
		if r.twoWay() {
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
		if r.twoWay() {
			out = append(out, [2]string{endID(r.to), endID(r.from)})
		}
	}
	return out
}

// detourOffer is how much longer than the direct way a chain may make a
// line to a frame's chip before the line passes the rows on its own, and,
// with tactic.through, any line before it passes a row off its chain.
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
// order, so a gap holds no more tracks than its crossings need. Two kept
// apart stand a track or more clear of each other. No crossing takes a
// track above least.
func shelve(m int, meet, held, apart func(a, b int) bool, least func(k int) int) []int {
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
				if meet(q, s) && t[q] == t[s] || apart(q, s) && abs(t[q]-t[s]) == 1 {
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
