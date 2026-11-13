package screen

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// The drawings place nodes in layers by what uses what: a node sits above
// everything it uses, so a binary is on top and its leaf libraries at the
// bottom. A layer wider than the drawing's budget wraps into several rows.
// A line between rows further apart than one has a chain of dummies, one in
// each row it passes (layered.go), so the rows are ordered and placed with
// room for it. Lines run on a grid: down out of a box, across in the gap
// between two rows on a track of their own, down through a row on their
// chain or in the space between boxes, and into the box they reach. docs/blueprint-svg.md states the rules this
// layout keeps; legible.go measures them. Everything is sorted before it
// is placed, so the same graph always gives the same picture.

type lnode struct {
	id   string
	w, h int
	// tier is the stored stack's tier the node stands in, from 1 at the
	// top, 0 for none (tiers).
	tier int
	// step is where the node first comes in a stored flow, from 1, 0 for
	// none: a tier's boxes stand in its row in that order (huddle).
	step int
	// stage is where the node stands on the primary flow, from 1, 0 off
	// it. Callers state it, and arrange alone decides how the stages are
	// laid out (flowOf).
	stage int
}

// ledge runs from a node to one it uses.
type ledge struct{ from, to string }

type rect struct{ X, Y, W, H int }

type layout struct {
	pos map[string]rect
	// held is the track each crossing took in a drawing route lays this
	// one against, by gap and owner: no crossing takes a higher one.
	held map[[3]int]int
	row  map[string]int
	// rows are the wrapped layers, top to bottom.
	rows  [][]string
	width int
	// top is where gap 0 begins. Gap g lies above row g; gap len(rows) lies
	// below the last row.
	top          int
	gapTop, gapH []int
	rowTop, rowH []int
	height       int
	lanes        map[[2]int]int
	// beside are where lines pass beside each row, left or right of it.
	beside map[int][]int
	// passes are where lines pass through each row, beside or between its
	// boxes. Two lines that pass rows next to each other at one x would
	// meet in the gap between those rows and read as one.
	passes map[int][]int
	// stubs are the x where lines leave or reach a box on the edge that
	// faces each row: a line passing that row there would run on the stub
	// in the gap between them.
	stubs          map[int][]int
	heights        map[string]int
	minGap, endGap int
	// chains are the long lines' runs of dummies, and pass the left x of
	// each chain in each row it passes.
	chains []chain
	pass   map[[2]int]int
	// reserved are the x of every chain's lanes in each row, which a line
	// with no chain keeps clear of.
	reserved map[int][]int
	// chips are how tall the bands of chips on a frame's top and bottom
	// edges are (chipBand), for an inside.
	chips [2]int
	// seats are the centre x of each frame chip the layout seated, by its
	// node id (chipNode).
	seats map[string]int
	// ring says the rows stand about a focus (ring), so every line has the
	// focus at one end and a bus would only gather what drops straight.
	ring bool
	// idle says the tactic's fold or entry flag changed nothing (arrange).
	idle bool
	// counted and roads are the tactic's flags.
	counted, roads bool
	// axis is what the rows were ranked by.
	axis axis
}

const (
	gapX       = 40
	padX, padY = 24, 20
	// lane is the spacing between parallel lines; bundle is how many lines
	// share a channel before the next one starts a new channel, rule 2.
	lane   = 8
	bundle = 4
	// channel is how close lines run before they read as one channel.
	channel = 12
	// busAt is how many lines into one box are drawn as one bundle.
	busAt = 2
	// frameTopNode and frameBottomNode stand for a frame's top and bottom
	// edge while a view is layered; they are never placed.
	frameTopNode, frameBottomNode = "frame:top", "frame:bottom"
	// Width budgets, rule 3. Every drawing has the page's whole width, with
	// its text below it, and 1320px is about what a desktop window shows.
	// narrowBudget is what a unit's view kept when its text stood beside it;
	// a narrow tactic still draws it that way.
	overviewBudget = 1320
	insideBudget   = 1320
	narrowBudget   = 920
	// minWidth is the narrowest an overview drawing is drawn.
	minWidth = 320
)

// chipNode names the node a frame chip is seated as: the frame edge it
// stands on, frameTopNode or frameBottomNode, then a bar and its key.
func chipNode(edge, key string) string { return edge + "|" + key }

// isChip says id is a seated frame chip.
func isChip(id string) bool {
	return strings.HasPrefix(id, frameTopNode+"|") || strings.HasPrefix(id, frameBottomNode+"|")
}

// focusSeat is what a focus view keeps of its full view: the focus, set at
// the centre, and the x of every box there, which orders the ring.
type focusSeat struct {
	focus string
	at    map[string]int
}

// arrange lays the boxes out in rows, ordered by tactic t. A focus view
// orders its rows by its seat instead. Frame chips among the nodes
// (chipNode) are seated. They take no part in the layering, which the
// frame's edges still steer. They stand in a row above the first and a row
// below the last, ordered and placed with the boxes, so a line to a chip
// runs down a chain that starts under it.
func arrange(nodes []lnode, edges []ledge, budget int, seat *focusSeat, t tactic) layout {
	slices.SortFunc(nodes, func(a, b lnode) int { return strings.Compare(a.id, b.id) })
	byID := map[string]lnode{}
	var tops, bottoms []string
	for _, n := range nodes {
		byID[n.id] = n
		switch {
		case strings.HasPrefix(n.id, frameTopNode+"|"):
			tops = append(tops, n.id)
		case strings.HasPrefix(n.id, frameBottomNode+"|"):
			bottoms = append(bottoms, n.id)
		}
	}
	nodes = slices.DeleteFunc(nodes, func(n lnode) bool { return isChip(n.id) })
	chips := len(tops)+len(bottoms) > 0
	// Chips not seated in rows of their own get lanes instead (proper).
	if !t.seated {
		tops, bottoms = nil, nil
	}
	out := map[string][]string{}
	touched := map[string]bool{}
	// lines counts the edges between each pair of places, either way: each
	// stands for a line the drawing will route between them. A line to a
	// seated chip comes as two edges, one to the frame's edge for the
	// layering and one to the chip, and runs to the chip.
	lines := map[[2]string]int{}
	for _, e := range edges {
		if _, ok := byID[e.from]; !ok {
			continue
		}
		if _, ok := byID[e.to]; ok && e.from != e.to {
			lines[pairKey(e.from, e.to)]++
		}
		switch {
		case isChip(e.from):
			lines[pairKey(frameTopNode, e.to)]--
			continue
		case isChip(e.to):
			lines[pairKey(e.from, frameBottomNode)]--
			continue
		}
		if _, ok := byID[e.to]; !ok || e.from == e.to || slices.Contains(out[e.from], e.to) {
			continue
		}
		out[e.from] = append(out[e.from], e.to)
		touched[e.from], touched[e.to] = true, true
	}
	maps.DeleteFunc(lines, func(_ [2]string, n int) bool { return n <= 0 })
	for k := range out {
		slices.Sort(out[k])
	}
	// The pairs of places lines join, frame edges included.
	var pairs [][2]string
	for k := range lines {
		pairs = append(pairs, k)
	}
	slices.SortFunc(pairs, func(a, b [2]string) int {
		if c := strings.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return strings.Compare(a[1], b[1])
	})
	// A cycle cannot be layered (feedback). A folding tactic leaves out of
	// the ranking the edges of two places that use each other: one line with
	// a head at each end joins them and reads both ways, so neither has to
	// stand a row above the other. idle says the tactic's flags changed
	// nothing here.
	idle := t.fold || t.entry
	both := func(v, w string) bool {
		if !t.fold || !slices.Contains(out[w], v) {
			return false
		}
		idle = false
		return true
	}
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.id
	}
	var pick func(ids, front []string, score func(string) int, best string) string
	// An entering tactic breaks the cycle at the node the places already
	// ranked use most, where a reader comes into it.
	if t.entry {
		pick = func(ids, front []string, score func(string) int, best string) string {
			placed := map[string]bool{}
			for _, f := range front {
				placed[f] = true
			}
			from := func(v string) int {
				n := 0
				for u, ws := range out {
					if placed[u] && slices.Contains(ws, v) {
						n++
					}
				}
				return n
			}
			entry := ids[0]
			for _, v := range ids {
				if fv, fe := from(v), from(entry); fv > fe || fv == fe && score(v) > score(entry) {
					entry = v
				}
			}
			if entry != best {
				idle = false
			}
			return entry
		}
	}
	keep := feedback(ids, out, both, pick)
	rank, order := longest(ids, out, keep)
	// A node that uses what sits below the frame sinks to just above the
	// highest other thing it uses, so its line down stays short.
	for i := len(order) - 1; i >= 0; i-- {
		v := order[i]
		if !keep[ledge{v, frameBottomNode}] {
			continue
		}
		low := rank[frameBottomNode]
		for _, w := range out[v] {
			if keep[ledge{v, w}] && w != frameBottomNode {
				low = min(low, rank[w])
			}
		}
		rank[v] = max(rank[v], low-1)
	}
	fa := flowOf(nodes)
	if fa != nil {
		pinStages(nodes, fa)
	}
	switch {
	case fa != nil:
		flowRank(nodes, order, out, keep, rank, fa)
	case seat == nil:
		tier(nodes, order, out, keep, rank)
	}
	// The frame's edges leave the layering, and the layers they alone held
	// close up.
	if _, ok := byID[frameTopNode]; ok {
		nodes = slices.DeleteFunc(nodes, func(nd lnode) bool { return nd.id == frameTopNode || nd.id == frameBottomNode })
		delete(rank, frameTopNode)
		delete(rank, frameBottomNode)
		used := map[int]bool{}
		for _, nd := range nodes {
			used[rank[nd.id]] = true
		}
		var levels []int
		for r := range used {
			levels = append(levels, r)
		}
		slices.Sort(levels)
		at := map[int]int{}
		for i, r := range levels {
			at[r] = i
		}
		for _, nd := range nodes {
			rank[nd.id] = at[rank[nd.id]]
		}
		for k := range out {
			out[k] = slices.DeleteFunc(out[k], func(w string) bool { return w == frameTopNode || w == frameBottomNode })
		}
		delete(out, frameTopNode)
		delete(touched, frameTopNode)
		delete(touched, frameBottomNode)
	}
	ax := axis{basis: byDependency, cycle: cyclic(out), flow: fa}
	switch {
	case fa != nil:
		ax.basis = byFlow
	case seat == nil && slices.ContainsFunc(nodes, func(nd lnode) bool { return nd.tier > 0 }):
		ax.basis = byStack
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
	ranks := make([][]string, n)
	for _, nd := range nodes {
		ranks[rank[nd.id]] = append(ranks[rank[nd.id]], nd.id)
	}
	w := map[string]int{}
	for _, nd := range nodes {
		w[nd.id] = nd.w
	}
	// Order the layers with their chains, wrap a layer wider than the room
	// into rows in that order, and order the rows again with the chains the
	// wrapping adds.
	sortRows, split := func(g *layered) { g.order(t) }, wrap
	if seat != nil {
		sortRows = func(g *layered) { g.seat(seat.at, seat.focus) }
		split = brick
	}
	// lead is how many rows of chips stand above the boxes' rows.
	lead := 0
	if len(tops) > 0 {
		lead = 1
	}
	withChips := func(rows [][]string) [][]string {
		if len(tops) > 0 {
			rows = append([][]string{tops}, rows...)
		}
		if len(bottoms) > 0 {
			rows = append(rows, bottoms)
		}
		return rows
	}
	for _, id := range append(slices.Clone(tops), bottoms...) {
		w[id] = byID[id].w
	}
	tiers, steps := map[string]int{}, map[string]int{}
	for _, nd := range nodes {
		if nd.tier > 0 {
			tiers[nd.id] = nd.tier
		}
		if nd.step > 0 {
			steps[nd.id] = nd.step
		}
	}
	if len(tiers) > 0 {
		sorted := sortRows
		sortRows = func(g *layered) { sorted(g); g.huddle(tiers, steps) }
	}
	// The stages set the order of a row that holds two or more of them, so
	// every other row is ordered again around those: what the first sweeps
	// chose stood against an order the stages then overrode. A row with one
	// stage sets no order and is ordered again too.
	if fa != nil {
		huddled := sortRows
		sortRows = func(g *layered) {
			huddled(g)
			g.fixed = make([]bool, len(g.rows))
			for r, row := range g.rows {
				n := 0
				for _, id := range row {
					if tiers[id] > 0 {
						n++
					}
				}
				g.fixed[r] = n > 1
			}
			g.order(t)
			g.fixed = nil
		}
	}
	g := proper(withChips(ranks), w, pairs, lines)
	sortRows(&g)
	room := budget - 2*padX
	var rows [][]string
	wrapped := false
	for _, row := range g.rows {
		if slices.ContainsFunc(row, isChip) {
			rows = append(rows, row)
			continue
		}
		ids := slices.DeleteFunc(slices.Clone(row), func(id string) bool { _, d := g.chainOf[id]; return d })
		rs := split(ids, byID, room)
		if fa.spine(ids) {
			rs = [][]string{ids}
		}
		if seat == nil && len(rs) > 1 {
			rs = stack(rs, keep, byID, room)
		}
		wrapped = wrapped || len(rs) > 1
		rows = append(rows, rs...)
	}
	if wrapped {
		g = proper(rows, w, pairs, lines)
		sortRows(&g)
	}

	l := layout{pos: map[string]rect{}, row: map[string]int{}, lanes: map[[2]int]int{}, heights: map[string]int{}, minGap: 36, endGap: 12, pass: map[[2]int]int{}, idle: idle, counted: t.counted, roads: t.roads, axis: ax}
	// Where even crooked chains leave the drawing wider than its budget,
	// the shortest give up their slots.
	g.shed(budget)
	var x map[string]int
	var width int
	if seat != nil {
		l.ring = true
		x, width = g.ring(seat.focus, budget)
	} else {
		room := budget
		if t.narrow {
			room = 0
		}
		x, width = g.place(budget, room)
		rows := make([][]string, len(g.rows))
		for r, row := range g.rows {
			rows[r] = slices.Clone(row)
		}
		// A move changes the gaps in its row, so a row may come out wider:
		// past the budget, the drawing keeps the rows as they were.
		if g.shorten(x) {
			g.huddle(tiers, steps)
			if x2, w2 := g.place(budget, room); w2 <= max(width, budget) {
				x, width = x2, w2
			} else {
				g.rows = rows
			}
		}
	}
	// The chips leave the rows, each keeping where it was seated.
	l.width = width
	for _, ch := range g.chains {
		ch.top, ch.bottom = ch.top-lead, ch.bottom-lead
		l.chains = append(l.chains, ch)
	}
	if chips {
		l.seats = map[string]int{}
	}
	for r, row := range g.rows {
		if slices.ContainsFunc(row, isChip) {
			for _, id := range row {
				l.seats[id] = x[id] + w[id]/2
			}
			continue
		}
		var ids []string
		for _, id := range row {
			if c, d := g.chainOf[id]; d {
				l.pass[[2]int{c, r - lead}] = x[id]
				continue
			}
			ids = append(ids, id)
			l.row[id] = r - lead
			l.heights[id] = byID[id].h
			l.pos[id] = rect{X: x[id], W: byID[id].w, H: byID[id].h}
		}
		l.rows = append(l.rows, ids)
	}
	// A chip with a lane stands over it, where it leaves the frame.
	for c, ch := range l.chains {
		switch {
		case ch.lower == "":
			l.seats[ch.upper] = l.pass[[2]int{c, ch.top}] + g.w[dummy(c, ch.top+lead)]/2
		case ch.upper == "":
			l.seats[ch.lower] = l.pass[[2]int{c, ch.bottom}] + g.w[dummy(c, ch.bottom+lead)]/2
		}
	}
	return l
}

// toward is the x at which the lines from box id to place other leave
// id's row: down or up their chain when they have one, else at other's
// centre. A frame's top and bottom are places too.
func (l *layout) toward(id, other string) int {
	for c, ch := range l.chains {
		if pairKey(ch.upper, ch.lower) == pairKey(id, other) {
			if ch.upper == id {
				return l.pass[[2]int{c, ch.top}]
			}
			return l.pass[[2]int{c, ch.bottom}]
		}
	}
	p, ok := l.pos[other]
	if !ok {
		p = l.pos[id]
	}
	return p.X + p.W/2
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

// stack wraps a layer the way a person stacks its rows when the boxes
// with a line to what lies below fit in the last row: those boxes go
// last, so their lines leave from the bottom row and pass no box of the
// layer on the way down, and the boxes with no line below fill the rows
// above, near the lines that reach them. Each group keeps the order the
// crossing count chose. When the boxes with a line below spill out of the
// last row, some of their lines pass a row of the layer anyway, so the
// layer keeps rows as wrap made them.
func stack(rs [][]string, keep map[ledge]bool, byID map[string]lnode, room int) [][]string {
	down := map[string]bool{}
	for e := range keep {
		down[e.from] = true
	}
	group := func(id string) int {
		if down[id] {
			return 1
		}
		return 0
	}
	order := slices.Concat(rs...)
	slices.SortStableFunc(order, func(a, b string) int { return group(a) - group(b) })
	out := wrap(order, byID, room)
	if slices.ContainsFunc(slices.Concat(out[:len(out)-1]...), func(id string) bool { return down[id] }) {
		return rs
	}
	return out
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

// brick splits a ring layer wider than room into rows that each take every
// k-th box, so each row spans the layer from left to right in its order and
// a box of one row stands over a gap of the next, where the line of the box
// above passes down.
func brick(ids []string, byID map[string]lnode, room int) [][]string {
	for k := 1; ; k++ {
		rows := make([][]string, k)
		for i, id := range ids {
			rows[i%k] = append(rows[i%k], id)
		}
		fits := true
		for _, r := range rows {
			fits = fits && rowWidth(r, byID) <= room
		}
		if fits || k >= len(ids) {
			return rows
		}
	}
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
	// kind keeps lines of different kinds out of one bundle, and plan says
	// every relation the line stands for arises only in design, so it is
	// drawn dotted and rides no road a solid line rides (confluence.go).
	// lead says the view's lead rides the line (leadLines): its hop paints
	// the line in solid flow ink at rest, so the roads never keep it apart
	// to show its dots.
	kind string
	plan bool
	lead bool
}

func polyline(pts []pt) string {
	var d strings.Builder
	for i, p := range straight(pts) {
		if i == 0 {
			fmt.Fprintf(&d, "M%d %d", p.x, p.y)
		} else {
			fmt.Fprintf(&d, " L%d %d", p.x, p.y)
		}
	}
	return d.String()
}

// straight drops repeated points and the points within a straight run.
func straight(pts []pt) []pt { return straightInto(nil, pts) }

// straightInto is straight written over dst, which must not share pts's
// points.
func straightInto(dst, pts []pt) []pt {
	clean := dst[:0]
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
	return clean
}

// gutter picks the space in row r nearest to x for a line passing through
// it, and the lane in that space the line takes.
func (l *layout) gutter(r, x int) int { return l.gutterTo(r, x, x, -1, nil) }

// gutterTo is gutter for a line headed for to: of two spaces as far from x,
// it takes the one nearer to, so a line that must step aside steps the way
// it is going. own is the lane the line's own chain holds in row r, or -1,
// and mine says the line passing row q at y is this line or a bus it rides.
func (l *layout) gutterTo(r, x, to, own int, mine func(q, y int) bool) int {
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
		at := clamp(x, g.from+6, g.to-6)
		c := abs(at-x) + abs(at-to)/2 - abs(x-to)/2 + 40*used
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
	if best > 0 && best < len(gaps)-1 {
		// A line already running down its own chain's lane keeps straight
		// on it. runs refuses the chain where the line's own trunk passed
		// the row before at that x, the one line it may meet there; set in
		// the space's middle, the line would ride past its turn and back.
		if x == own && x >= g.from+12 && x <= g.to-12 && l.meetsOnly(r, x, mine) && !l.takenBut(r, x, own) && !l.onStub(r, x) &&
			!slices.ContainsFunc(l.passes[r], func(y int) bool { return abs(y-x) < lane }) {
			l.passes[r] = append(l.passes[r], x)
			return x
		}
	}
	l.lanes[[2]int{r, best}]++
	if best > 0 && best < len(gaps)-1 {
		at := func(k int) int { return g.from + (g.to-g.from)/2 + offset(k, slots(best))*lane }
		for k+1 < slots(best) && (l.nextTo(r, at(k)) || l.taken(r, at(k)) || l.onStub(r, at(k))) {
			k++
			l.lanes[[2]int{r, best}]++
		}
		l.passes[r] = append(l.passes[r], at(k))
		return at(k)
	}
	// Beside the row a line passes straight on where it is headed. It
	// steps a lane outward from any line already there, and once more
	// past a channel that already holds bundle lines, rule 5.
	out := -1
	if best > 0 {
		out = 1
	}
	at := clamp(x, g.from+12, g.to-12)
	for !l.free(r, at) || l.taken(r, at) || l.onStub(r, at) {
		at += out * lane
	}
	l.beside[r] = append(l.beside[r], at)
	l.passes[r] = append(l.passes[r], at)
	return at
}

// onStub says a line passing row r at x would run on a line's stub into a
// box beside that row.
func (l *layout) onStub(r, x int) bool {
	for _, y := range l.stubs[r] {
		if abs(x-y) < lane {
			return true
		}
	}
	return false
}

// clearOf says a line passing row r at x stands 12px or more off every
// shape in the row.
func (l *layout) clearOf(r, x int) bool {
	for _, id := range l.rows[r] {
		if p := l.pos[id]; x > p.X-12 && x < p.X+p.W+12 {
			return false
		}
	}
	return true
}

// taken says x in row r lies on a chain's lane, or in its channel.
func (l *layout) taken(r, x int) bool { return l.takenBut(r, x, -1) }

// takenBut is taken leaving out the chain lane at own.
func (l *layout) takenBut(r, x, own int) bool {
	for _, y := range l.reserved[r] {
		if y != own && abs(x-y) <= channel {
			return true
		}
	}
	return false
}

// meets counts the lines passing a row next to r at x.
func (l *layout) meets(r, x int) int {
	n := 0
	for _, q := range []int{r - 1, r + 1} {
		for _, y := range l.passes[q] {
			if abs(x-y) < 3 {
				n++
			}
		}
	}
	return n
}

// meetsOnly says a line passing row r at x would meet at most one line
// passing a row next to it, and that one is the line's own (mine).
func (l *layout) meetsOnly(r, x int, mine func(q, y int) bool) bool {
	n := 0
	for _, q := range []int{r - 1, r + 1} {
		for _, y := range l.passes[q] {
			if abs(x-y) < 3 {
				if n++; n > 1 || !mine(q, y) {
					return false
				}
			}
		}
	}
	return true
}

// nextTo says a line passing row r at x would meet one that passes a row
// next to it at the same x.
func (l *layout) nextTo(r, x int) bool { return l.meets(r, x) > 0 }

// free says a line may pass beside row r at x: no line passes within a
// lane of it, and the channel it would join holds fewer than bundle lines.
func (l *layout) free(r, x int) bool {
	xs := append(slices.Clone(l.beside[r]), x)
	slices.Sort(xs)
	i := slices.Index(xs, x)
	if i > 0 && xs[i]-xs[i-1] < lane || i+1 < len(xs) && xs[i+1]-xs[i] < lane {
		return false
	}
	lo, hi := i, i
	for lo > 0 && xs[lo]-xs[lo-1] <= channel {
		lo--
	}
	for hi+1 < len(xs) && xs[hi+1]-xs[hi] <= channel {
		hi++
	}
	return hi-lo+1 <= bundle
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

// feedback says which edges of out, between ids, rank: those that point
// forward in an order that leaves as few of them as it can pointing
// backwards (Eades, Lin and Smyth: peel sinks to the back and sources to the
// front, else the node with the most more outgoing than incoming edges, or
// the one pick names). An edge skip names takes no part. A cycle cannot be
// layered, so the edges left pointing backwards are left out of the ranking
// only.
func feedback(ids []string, out map[string][]string, skip func(v, w string) bool, pick func(ids, front []string, score func(string) int, best string) string) map[ledge]bool {
	in := map[string]map[string]bool{}
	outs := map[string]map[string]bool{}
	left := map[string]bool{}
	for _, v := range ids {
		left[v] = true
		in[v], outs[v] = map[string]bool{}, map[string]bool{}
	}
	for _, v := range ids {
		for _, w := range out[v] {
			if left[w] && !skip(v, w) {
				outs[v][w], in[w][v] = true, true
			}
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
		rest := slices.Sorted(maps.Keys(left))
		moved := false
		for _, v := range rest {
			if len(outs[v]) == 0 {
				back = append([]string{v}, back...)
				remove(v)
				moved = true
			}
		}
		for _, v := range rest {
			if left[v] && len(in[v]) == 0 {
				front = append(front, v)
				remove(v)
				moved = true
			}
		}
		if moved {
			continue
		}
		score := func(v string) int { return len(outs[v]) - len(in[v]) }
		best := rest[0]
		for _, v := range rest {
			if score(v) > score(best) {
				best = v
			}
		}
		if pick != nil {
			best = pick(rest, front, score, best)
		}
		front = append(front, best)
		remove(best)
	}
	at := map[string]int{}
	for i, v := range append(front, back...) {
		at[v] = i
	}
	keep := map[ledge]bool{}
	for _, v := range ids {
		for _, w := range out[v] {
			if _, ok := at[w]; ok && at[v] < at[w] && !skip(v, w) {
				keep[ledge{v, w}] = true
			}
		}
	}
	return keep
}

// longest ranks ids by the longest path down the kept edges, and gives the
// order it ranked them in. A node nothing uses sits just above the highest
// thing it uses rather than on the top layer, so the top layer does not
// collect every root.
func longest(ids []string, out map[string][]string, keep map[ledge]bool) (map[string]int, []string) {
	indeg := map[string]int{}
	used := map[string]bool{}
	for e := range keep {
		indeg[e.to]++
		used[e.to] = true
	}
	rank := map[string]int{}
	var queue, order []string
	for _, v := range ids {
		if indeg[v] == 0 {
			queue = append(queue, v)
		}
	}
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
	for i := len(order) - 1; i >= 0; i-- {
		v := order[i]
		low := -1
		for _, w := range out[v] {
			if keep[ledge{v, w}] && (low < 0 || rank[w] < low) {
				low = rank[w]
			}
		}
		if !used[v] && low > 0 {
			rank[v] = low - 1
		}
	}
	return rank, order
}

// flowAxis is how a view laid the primary flow out. The layout records it,
// and lint and graph.json read it. across says the stages read left to right
// along one row, the spine; else each stage has rows of its own, top down.
// stage is each staged box's stage, by id.
type flowAxis struct {
	across bool
	stage  map[string]int
}

// spineRoom is the room a spine reads in on one screen, whatever room a
// tactic narrows rows to, so every tactic of a view picks the same geometry.
const spineRoom = insideBudget - 12 - 2*padX

// flowOf reads the stages off nodes: across when the staged nodes, side by
// side with gapX between them, fit spineRoom, else down. It is nil when the
// nodes show fewer than two stages.
func flowOf(nodes []lnode) *flowAxis {
	f := &flowAxis{stage: map[string]int{}}
	shown := map[int]bool{}
	width := -gapX
	for _, n := range nodes {
		if n.stage > 0 {
			f.stage[n.id], shown[n.stage] = n.stage, true
			width += n.w + gapX
		}
	}
	if len(shown) < 2 {
		return nil
	}
	f.across = width <= spineRoom
	return f
}

// spine says ids hold a staged node of a flow laid across: their row is the
// spine, which never wraps.
func (f *flowAxis) spine(ids []string) bool {
	return f != nil && f.across && slices.ContainsFunc(ids, func(id string) bool { return f.stage[id] > 0 })
}

// pinStages turns stages into the pins tier and huddle keep, in place of any
// tier. Across, every staged node stands in one tier, ordered by stage, then
// by its flow step, then by id. Down, each stage is a tier of its own.
func pinStages(nodes []lnode, f *flowAxis) {
	var staged []int
	for i := range nodes {
		nodes[i].tier = 0
		if nodes[i].stage == 0 {
			continue
		}
		if !f.across {
			nodes[i].tier = nodes[i].stage
			continue
		}
		staged = append(staged, i)
	}
	slices.SortStableFunc(staged, func(a, b int) int {
		x, y := nodes[a], nodes[b]
		if x.stage != y.stage {
			return x.stage - y.stage
		}
		if x.step != y.step {
			return x.step - y.step
		}
		return strings.Compare(x.id, y.id)
	})
	for k, i := range staged {
		nodes[i].tier, nodes[i].step = 1, 2*k+2
	}
}

// flowRank ranks the boxes of a view that lays the primary flow out.
// Across, the stages stand in one row, the spine, and every other box is
// ranked against them as against one box (spineRank). Down, each stage
// holds a row of its own in stage order, and every other box stands by the
// lines as tier has it, against the stages too: below the boxes that use
// it, else just above the highest it uses, so it stands between the stages
// it joins.
func flowRank(nodes []lnode, order []string, out map[string][]string, keep map[ledge]bool, rank map[string]int, f *flowAxis) {
	if f.across {
		spineRank(order, out, keep, rank, f)
		if inline(nodes, out, rank, f) {
			closeRanks(order, rank)
			return
		}
		f.across = false
		pinStages(nodes, f)
	}
	gap := len(nodes) + 2
	last := 0
	for id, s := range f.stage {
		rank[id] = s * gap
		last = max(last, rank[id])
	}
	// The frame's edges hold the top and the bottom, so what the outside
	// uses stands above the stages and what uses the outside below them.
	fixed := func(v string) bool { return f.stage[v] > 0 || v == frameTopNode || v == frameBottomNode }
	for _, n := range nodes {
		switch n.id {
		case frameTopNode:
			rank[n.id] = 0
		case frameBottomNode:
			rank[n.id] = last + gap
		}
	}
	ins := map[string][]string{}
	linked := map[string]bool{}
	for _, v := range order {
		for _, w := range out[v] {
			if keep[ledge{v, w}] && (f.stage[v] == 0 || f.stage[w] == 0) {
				ins[w] = append(ins[w], v)
				linked[v], linked[w] = true, true
			}
		}
	}
	for range len(order) {
		moved := false
		for _, v := range order {
			if fixed(v) || !linked[v] {
				continue
			}
			r := 0
			if len(ins[v]) > 0 {
				for _, u := range ins[v] {
					r = max(r, rank[u]+1)
				}
			} else {
				r = 1 << 30
				for _, w := range out[v] {
					if keep[ledge{v, w}] {
						r = min(r, rank[w]-1)
					}
				}
			}
			if r != rank[v] {
				rank[v], moved = r, true
			}
		}
		if !moved {
			break
		}
	}
	closeRanks(order, rank)
}

// closeRanks closes up the ranks no box holds. A box no line ties to the
// others keeps its rank from the lines.
func closeRanks(order []string, rank map[string]int) {
	used := map[int]bool{}
	for _, v := range order {
		used[rank[v]] = true
	}
	at := map[int]int{}
	for i, r := range slices.Sorted(maps.Keys(used)) {
		at[r] = i
	}
	for _, v := range order {
		rank[v] = at[rank[v]]
	}
}

// spineRank ranks a view whose flow reads across as arrange ranks any view,
// but with the stages taken as one node, the spine: the cycles through the
// stages are broken against the spine, not by the order of the whole view,
// so a box a stage uses and that uses a stage stands on one side of the
// whole spine, and every box ranks by the rows it joins. A box stands below
// the spine when no stage uses it, beside the stages in its row only where
// the lines rank it there. Edges a folding tactic left out stay out. The
// ranks are doubled so that inline can stand a box just below the spine.
func spineRank(order []string, out map[string][]string, keep map[ledge]bool, rank map[string]int, f *flowAxis) {
	const spine = "flow:spine"
	of := func(v string) string {
		if f.stage[v] > 0 {
			return spine
		}
		return v
	}
	ids := []string{spine}
	cout := map[string][]string{}
	ranked := map[ledge]bool{}
	for _, v := range order {
		if f.stage[v] == 0 {
			ids = append(ids, v)
		}
		for _, w := range out[v] {
			e := ledge{of(v), of(w)}
			if e.from == e.to {
				continue
			}
			if !slices.Contains(cout[e.from], e.to) {
				cout[e.from] = append(cout[e.from], e.to)
			}
			if keep[ledge{v, w}] || !keep[ledge{w, v}] && !slices.Contains(out[w], v) {
				ranked[e] = true
			}
		}
	}
	for k := range cout {
		slices.Sort(cout[k])
	}
	skip := func(a, b string) bool { return !ranked[ledge{a, b}] }
	// The spine wins a tie to break a cycle at, so the boxes the stages use
	// stand below them.
	first := func(rest, _ []string, score func(string) int, best string) string {
		if slices.Contains(rest, spine) && score(spine) == score(best) {
			return spine
		}
		return best
	}
	r, _ := longest(ids, cout, feedback(ids, cout, skip, first))
	for _, v := range order {
		rank[v] = 2 * r[of(v)]
	}
}

// inline stands in the spine row each box below it that a stage uses and
// that uses a stage which does not reach it back, so its line would climb
// to close no cycle: just before the first such stage, its tier and step
// putting it there (huddle). Where the row would not fit spineRoom with
// them, it says false and moves none: the flow reads down instead. A box
// the lines rank into the spine's row stands just below it where the row
// would not fit spineRoom with it, since the spine never wraps.
func inline(nodes []lnode, out map[string][]string, rank map[string]int, f *flowAxis) bool {
	spine := 0
	for id := range f.stage {
		spine = rank[id]
	}
	usedBy := map[string]bool{}
	for u := range f.stage {
		for _, w := range out[u] {
			usedBy[w] = true
		}
	}
	reaches := func(from, to string) bool {
		seen := map[string]bool{from: true}
		queue := []string{from}
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			for _, n := range out[c] {
				if n == to {
					return true
				}
				if !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		return false
	}
	step := map[int]int{}
	width := -gapX
	for _, n := range nodes {
		if n.stage > 0 {
			width += n.w + gapX
			if step[n.stage] == 0 || n.step < step[n.stage] {
				step[n.stage] = n.step
			}
		}
	}
	at := map[int]int{}
	for i, n := range nodes {
		if n.stage > 0 || !usedBy[n.id] || rank[n.id] <= spine {
			continue
		}
		first := 0
		for _, w := range out[n.id] {
			if s := f.stage[w]; s > 0 && (first == 0 || s < first) && !reaches(w, n.id) {
				first = s
			}
		}
		if first > 0 {
			at[i] = first
			width += n.w + gapX
		}
	}
	if width > spineRoom {
		return false
	}
	for i, first := range at {
		rank[nodes[i].id] = spine
		nodes[i].tier, nodes[i].step = 1, step[first]-1
	}
	// A box tied to nothing in the view but the world outside stands off
	// the spine's row, as the overview keeps it.
	outside := func(v string) bool {
		for _, w := range out[v] {
			if w != frameTopNode && w != frameBottomNode {
				return false
			}
		}
		for u, ws := range out {
			if u != frameTopNode && u != frameBottomNode && slices.Contains(ws, v) {
				return false
			}
		}
		return true
	}
	var beside []string
	for _, n := range nodes {
		if rank[n.id] == spine && n.tier != 1 && n.id != frameTopNode && n.id != frameBottomNode {
			if outside(n.id) {
				rank[n.id] = spine + 1
				continue
			}
			beside = append(beside, n.id)
			width += n.w + gapX
		}
	}
	if width > spineRoom {
		for _, id := range beside {
			rank[id] = spine + 1
		}
	}
	return true
}

// tier ranks the boxes of a stored stack's tiers (lnode.tier) in the
// stack's order, one row to a tier, so a tier's boxes stand together and
// read top down as the stack does; a line between two boxes of one tier
// runs within its row. Every other node sits below what uses it, or just
// above what it uses when nothing uses it, and the rows close up.
func tier(nodes []lnode, order []string, out map[string][]string, keep map[ledge]bool, rank map[string]int) {
	of := map[string]int{}
	for _, n := range nodes {
		if n.tier > 0 {
			of[n.id] = n.tier
			rank[n.id] = n.tier - 1
		}
	}
	if len(of) == 0 {
		return
	}
	ins := map[string][]string{}
	linked := map[string]bool{}
	for _, v := range order {
		for _, w := range out[v] {
			if keep[ledge{v, w}] {
				ins[w] = append(ins[w], v)
				linked[v], linked[w] = true, true
			}
		}
	}
	for range len(order) {
		moved := false
		for _, v := range order {
			if of[v] > 0 || !linked[v] {
				continue
			}
			r := 0
			if len(ins[v]) > 0 {
				for _, u := range ins[v] {
					r = max(r, rank[u]+1)
				}
			} else {
				r = 1 << 30
				for _, w := range out[v] {
					if keep[ledge{v, w}] {
						r = min(r, rank[w]-1)
					}
				}
			}
			if r != rank[v] {
				rank[v], moved = r, true
			}
		}
		if !moved {
			break
		}
	}
	used := map[int]bool{}
	for _, v := range order {
		if of[v] > 0 || linked[v] {
			used[rank[v]] = true
		}
	}
	at := map[int]int{}
	for i, r := range slices.Sorted(maps.Keys(used)) {
		at[r] = i
	}
	for _, v := range order {
		if of[v] > 0 || linked[v] {
			rank[v] = at[rank[v]]
		}
	}
}
