package screen

import (
	"fmt"
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
	// Width budgets, rule 5: what the page shows of a drawing at 1440px wide.
	overviewBudget = 1320
	insideBudget   = 920
	// minWidth is the narrowest an overview drawing is drawn.
	minWidth = 320
)

// focusSeat is what a focus view keeps of its full view: the focus, set at
// the centre, and the x of every box there, which orders the ring.
type focusSeat struct {
	focus string
	at    map[string]int
}

func arrange(nodes []lnode, edges []ledge, budget int, seat *focusSeat) layout {
	slices.SortFunc(nodes, func(a, b lnode) int { return strings.Compare(a.id, b.id) })
	byID := map[string]lnode{}
	for _, n := range nodes {
		byID[n.id] = n
	}
	out := map[string][]string{}
	touched := map[string]bool{}
	// lines counts the edges between each pair of places, either way: each
	// stands for a line the drawing will route between them.
	lines := map[[2]string]int{}
	for _, e := range edges {
		if _, ok := byID[e.from]; !ok {
			continue
		}
		if _, ok := byID[e.to]; ok && e.from != e.to {
			lines[pairKey(e.from, e.to)]++
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
	sortRows, split := (*layered).order, wrap
	if seat != nil {
		sortRows = func(g *layered) { g.seat(seat.at, seat.focus) }
		split = brick
	}
	g := proper(ranks, w, pairs, lines)
	sortRows(&g)
	room := budget - 2*padX
	var rows [][]string
	wrapped := false
	for _, row := range g.rows {
		ids := slices.DeleteFunc(slices.Clone(row), func(id string) bool { _, d := g.chainOf[id]; return d })
		if seat == nil {
			ids = sides(ids, keep)
		}
		rs := split(ids, byID, room)
		wrapped = wrapped || len(rs) > 1
		rows = append(rows, rs...)
	}
	if wrapped {
		g = proper(rows, w, pairs, lines)
		sortRows(&g)
	}

	l := layout{pos: map[string]rect{}, row: map[string]int{}, lanes: map[[2]int]int{}, heights: map[string]int{}, minGap: 36, endGap: 12, pass: map[[2]int]int{}}
	// Where even crooked chains leave the drawing wider than its budget,
	// the shortest give up their slots.
	g.shed(budget)
	var x map[string]int
	var width int
	if seat != nil {
		x, width = g.ring(seat.focus, budget)
	} else {
		x, width = g.place(budget)
		rows := make([][]string, len(g.rows))
		for r, row := range g.rows {
			rows[r] = slices.Clone(row)
		}
		// A move changes the gaps in its row, so a row may come out wider:
		// past the budget, the drawing keeps the rows as they were.
		if g.shorten(x) {
			if x2, w2 := g.place(budget); w2 <= max(width, budget) {
				x, width = x2, w2
			} else {
				g.rows = rows
			}
		}
	}
	l.width, l.chains = width, g.chains
	for r, row := range g.rows {
		var ids []string
		for _, id := range row {
			if _, d := g.chainOf[id]; !d {
				ids = append(ids, id)
			}
		}
		l.rows = append(l.rows, ids)
		for _, id := range ids {
			l.row[id] = r
			l.heights[id] = byID[id].h
		}
	}
	for r, row := range g.rows {
		for _, id := range row {
			if c, d := g.chainOf[id]; d {
				l.pass[[2]int{c, r}] = x[id]
				continue
			}
			l.pos[id] = rect{X: x[id], W: byID[id].w, H: byID[id].h}
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

// sides orders a layer before it wraps the way a person stacks the rows of
// one layer: the boxes joined only to what lies above go first, so they
// land in the top row near their lines, and the boxes joined only to what
// lies below go last, so their lines leave from the bottom row and pass no
// box of the layer on the way down. Each group keeps the order the
// crossing count chose.
func sides(ids []string, keep map[ledge]bool) []string {
	up, down := map[string]bool{}, map[string]bool{}
	for e := range keep {
		down[e.from], up[e.to] = true, true
	}
	group := func(id string) int {
		if down[id] {
			return 1
		}
		return 0
	}
	out := slices.Clone(ids)
	slices.SortStableFunc(out, func(a, b string) int { return group(a) - group(b) })
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
	// kind keeps lines of different kinds out of one bundle.
	kind string
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
func straight(pts []pt) []pt {
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
	return clean
}

// gutter picks the space in row r nearest to x for a line passing through
// it, and the lane in that space the line takes.
func (l *layout) gutter(r, x int) int { return l.gutterTo(r, x, x) }

// gutterTo is gutter for a line headed for to: of two spaces as far from x,
// it takes the one nearer to, so a line that must step aside steps the way
// it is going.
func (l *layout) gutterTo(r, x, to int) int {
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

// taken says x in row r lies on a chain's lane, or in its channel.
func (l *layout) taken(r, x int) bool {
	for _, y := range l.reserved[r] {
		if abs(x-y) <= channel {
			return true
		}
	}
	return false
}

// nextTo says a line passing row r at x would meet one that passes a row
// next to it at the same x.
func (l *layout) nextTo(r, x int) bool {
	for _, q := range []int{r - 1, r + 1} {
		for _, y := range l.passes[q] {
			if abs(x-y) < 3 {
				return true
			}
		}
	}
	return false
}

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
