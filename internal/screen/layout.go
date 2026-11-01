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
// bottom. Within a layer, nodes follow the order of their neighbours to keep
// lines short. Everything is sorted before it is placed, so the same graph
// always gives the same picture.

type lnode struct {
	id   string
	w, h int
}

// ledge runs from a node to one it uses.
type ledge struct{ from, to string }

type rect struct{ X, Y, W, H int }

type layout struct {
	pos   map[string]rect
	rank  map[string]int
	ranks [][]string
	// top and bottom are each layer's vertical extent.
	top, bottom []int
	width       int
	height      int
	// lanes counts the lines already routed through each gap, so the next one
	// takes its own lane instead of drawing over them.
	lanes map[[2]int]int
}

const (
	gapX, gapY = 40, 42
	padX, padY = 20, 20
)

func arrange(nodes []lnode, edges []ledge, top int) layout {
	slices.SortFunc(nodes, func(a, b lnode) int { return strings.Compare(a.id, b.id) })
	byID := map[string]lnode{}
	for _, n := range nodes {
		byID[n.id] = n
	}
	out := map[string][]string{}
	for _, e := range edges {
		if _, ok := byID[e.from]; !ok {
			continue
		}
		if _, ok := byID[e.to]; !ok || e.from == e.to || slices.Contains(out[e.from], e.to) {
			continue
		}
		out[e.from] = append(out[e.from], e.to)
	}
	for k := range out {
		slices.Sort(out[k])
	}
	// A cycle cannot be layered; the edge that closes it is left out of the ranking only.
	const white, grey, black = 0, 1, 2
	color := map[string]int{}
	keep := map[ledge]bool{}
	var visit func(string)
	visit = func(v string) {
		color[v] = grey
		for _, w := range out[v] {
			switch color[w] {
			case white:
				keep[ledge{v, w}] = true
				visit(w)
			case black:
				keep[ledge{v, w}] = true
			}
		}
		color[v] = black
	}
	for _, n := range nodes {
		if color[n.id] == white {
			visit(n.id)
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
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
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
	n := 0
	for _, r := range rank {
		n = max(n, r+1)
	}
	if len(nodes) > 0 {
		n = max(n, 1)
	}
	ranks := make([][]string, n)
	for _, nd := range nodes {
		ranks[rank[nd.id]] = append(ranks[rank[nd.id]], nd.id)
	}
	neighbours := map[string][]string{}
	for _, e := range edges {
		if _, ok := byID[e.from]; !ok {
			continue
		}
		if _, ok := byID[e.to]; !ok || e.from == e.to {
			continue
		}
		neighbours[e.from] = append(neighbours[e.from], e.to)
		neighbours[e.to] = append(neighbours[e.to], e.from)
	}
	place := func() map[string]float64 {
		at := map[string]float64{}
		for _, r := range ranks {
			for i, id := range r {
				at[id] = (float64(i) + 0.5) / float64(len(r))
			}
		}
		return at
	}
	sweep := func(r int, above bool) {
		at := place()
		key := map[string]float64{}
		for _, id := range ranks[r] {
			sum, k := 0.0, 0
			for _, o := range neighbours[id] {
				if (above && rank[o] < r) || (!above && rank[o] > r) {
					sum += at[o]
					k++
				}
			}
			key[id] = at[id]
			if k > 0 {
				key[id] = sum / float64(k)
			}
		}
		sort.SliceStable(ranks[r], func(i, j int) bool { return key[ranks[r][i]] < key[ranks[r][j]] })
	}
	for i := 0; i < 4; i++ {
		for r := 1; r < n; r++ {
			sweep(r, true)
		}
		for r := n - 2; r >= 0; r-- {
			sweep(r, false)
		}
	}
	l := layout{pos: map[string]rect{}, rank: rank, ranks: ranks, lanes: map[[2]int]int{}}
	widths := make([]int, n)
	for r, ids := range ranks {
		for i, id := range ids {
			if i > 0 {
				widths[r] += gapX
			}
			widths[r] += byID[id].w
		}
		l.width = max(l.width, widths[r])
	}
	l.width += 2 * padX
	y := top
	for r, ids := range ranks {
		h := 0
		for _, id := range ids {
			h = max(h, byID[id].h)
		}
		x := (l.width - widths[r]) / 2
		for _, id := range ids {
			nd := byID[id]
			l.pos[id] = rect{X: x, Y: y + (h-nd.h)/2, W: nd.w, H: nd.h}
			x += nd.w + gapX
		}
		l.top = append(l.top, y)
		l.bottom = append(l.bottom, y+h)
		y += h + gapY
	}
	l.height = y - gapY
	if n == 0 {
		l.height = top
	}
	return l
}

// gutter picks the gap in layer r nearest to x for a line passing through it,
// and the lane in that gap the line takes.
func (l *layout) gutter(r, x int) int {
	ids := l.ranks[r]
	type gap struct{ from, to int }
	var gaps []gap
	edge := 0
	for _, id := range ids {
		p := l.pos[id]
		gaps = append(gaps, gap{edge, p.X})
		edge = p.X + p.W
	}
	gaps = append(gaps, gap{edge, l.width})
	// A gap costs the detour to reach it plus the lines already in it, so
	// lines spread over the gaps instead of crowding into the nearest one.
	cost := func(i int) int {
		g := gaps[i]
		used := l.lanes[[2]int{r, i}]
		if inner := i > 0 && i < len(gaps)-1; inner && used*7+12 > g.to-g.from {
			used += 20
		}
		return abs(clamp(x, g.from+6, g.to-6)-x) + 28*used
	}
	best := 0
	for i := range gaps {
		if cost(i) < cost(best) {
			best = i
		}
	}
	g := gaps[best]
	n := l.lanes[[2]int{r, best}]
	l.lanes[[2]int{r, best}]++
	width := g.to - g.from
	if best == 0 {
		return g.to - 10 - n*7
	}
	if best == len(gaps)-1 {
		return g.from + 10 + n*7
	}
	slots := max(1, (width-12)/7)
	return g.from + width/2 + offset(n, slots)*7
}

// offset spreads lanes outward from a gap's centre: 0, 1, -1, 2, -2, ...
func offset(n, slots int) int {
	n %= slots
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

type pt struct{ x, y int }

// route draws a line from a point on layer a to a point on layer b. A line
// that skips layers passes the ones between through their gaps. Layer -1 is
// above the drawing and len(ranks) below it, for a frame's ports.
func (l *layout) route(from pt, a int, to pt, b int) string {
	points := []pt{from}
	switch {
	case b > a+1:
		for r := a + 1; r < b; r++ {
			x := l.gutter(r, from.x+(to.x-from.x)*(r-a)/(b-a))
			points = append(points, pt{x, l.top[r] - 8}, pt{x, l.bottom[r] + 8})
		}
	case b < a-1:
		for r := a - 1; r > b; r-- {
			x := l.gutter(r, from.x+(to.x-from.x)*(a-r)/(a-b))
			points = append(points, pt{x, l.bottom[r] + 8}, pt{x, l.top[r] - 8})
		}
	}
	if a == b {
		// Both ends face the same way, so the line dips below the layer and back.
		return fmt.Sprintf("M%d %d C%d %d %d %d %d %d", from.x, from.y, from.x, from.y+34, to.x, to.y+34, to.x, to.y)
	}
	points = append(points, to)
	var d strings.Builder
	fmt.Fprintf(&d, "M%d %d", points[0].x, points[0].y)
	for i := 1; i < len(points); i++ {
		p, q := points[i-1], points[i]
		if p.x == q.x {
			fmt.Fprintf(&d, " L%d %d", q.x, q.y)
			continue
		}
		bend := (q.y - p.y) / 2
		fmt.Fprintf(&d, " C%d %d %d %d %d %d", p.x, p.y+bend, q.x, q.y-bend, q.x, q.y)
	}
	return d.String()
}

// textWidth estimates how wide a label renders at 12px: wide for Hangul and
// other East Asian letters, narrow for the rest. It errs wide, so a label cut
// to fit never runs past its box.
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
