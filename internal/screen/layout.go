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
	passes         map[int][]int
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
	if best > 0 && best < len(gaps)-1 {
		at := func(k int) int { return g.from + (g.to-g.from)/2 + offset(k, slots(best))*lane }
		for k+1 < slots(best) && l.nextTo(r, at(k)) {
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
	for !l.free(r, at) {
		at += out * lane
	}
	l.beside[r] = append(l.beside[r], at)
	l.passes[r] = append(l.passes[r], at)
	return at
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
