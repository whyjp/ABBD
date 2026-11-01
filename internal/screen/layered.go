package screen

import (
	"fmt"
	"slices"
	"sort"
)

// The rows are ordered and placed as a proper layered graph (Sugiyama): a
// line between two places more than one row apart gets a dummy in every row
// it passes, the chain of those dummies is ordered with the boxes, and the
// chain is placed straight, so the line has a slot of its own in each row
// and runs down through all of them at one x. A frame's top sits above the
// first row and its bottom below the last.

// chain is the run of dummies that the lines between two places take
// through the rows between them.
type chain struct {
	// upper and lower are the places it joins: node ids, or frameTopNode
	// and frameBottomNode for a frame's edges.
	upper, lower string
	// top and bottom are the first and last row it passes.
	top, bottom int
	// lanes is how many lines it has room for, side by side.
	lanes int
}

const (
	// chainLane is how far apart the lines of one chain run, and chainSep
	// how far a chain keeps from a box or another chain: both wider than a
	// channel, so lines of different chains never read as one braid.
	chainLane = 16
	chainSep  = 16
)

// layered is the graph the rows are ordered and placed on.
type layered struct {
	rows [][]string
	w    map[string]int
	// chainOf names the chain a dummy belongs to; a box is not in it.
	chainOf map[string]int
	// up and down are each item's neighbours in the row above and below,
	// with how many lines the join stands for.
	up, down map[string]map[string]int
	chains   []chain
}

func dummy(c, r int) string { return fmt.Sprintf("~%d@%d", c, r) }

// pairKey names the lines between two places whichever way they run.
func pairKey(a, b string) [2]string {
	if b < a {
		a, b = b, a
	}
	return [2]string{a, b}
}

// proper builds the layered graph of boxes in rows, joined as pairs say:
// each pair with how many lines join it. Pairs come sorted.
func proper(rows [][]string, w map[string]int, pairs [][2]string, lines map[[2]string]int) layered {
	g := layered{w: map[string]int{}, chainOf: map[string]int{}, up: map[string]map[string]int{}, down: map[string]map[string]int{}}
	rowOf := map[string]int{frameTopNode: -1, frameBottomNode: len(rows)}
	for r, ids := range rows {
		g.rows = append(g.rows, slices.Clone(ids))
		for _, id := range ids {
			rowOf[id] = r
			g.w[id] = w[id]
		}
	}
	join := func(a, b string, n int) {
		if g.down[a] == nil {
			g.down[a] = map[string]int{}
		}
		if g.up[b] == nil {
			g.up[b] = map[string]int{}
		}
		g.down[a][b] += n
		g.up[b][a] += n
	}
	isBox := func(id string) bool { return id != frameTopNode && id != frameBottomNode }
	for _, p := range pairs {
		a, b := p[0], p[1]
		ra, okA := rowOf[a]
		rb, okB := rowOf[b]
		if !okA || !okB || ra == rb {
			continue
		}
		if ra > rb {
			a, b, ra, rb = b, a, rb, ra
		}
		n := lines[pairKey(a, b)]
		if rb == ra+1 {
			if isBox(a) && isBox(b) {
				join(a, b, n)
			}
			continue
		}
		c := len(g.chains)
		g.chains = append(g.chains, chain{upper: a, lower: b, top: ra + 1, bottom: rb - 1, lanes: min(n, 4)})
		prev := a
		for r := ra + 1; r < rb; r++ {
			d := dummy(c, r)
			g.rows[r] = append(g.rows[r], d)
			g.chainOf[d] = c
			g.w[d] = chainLane * (min(n, 4) - 1)
			if r > ra+1 || isBox(a) {
				join(prev, d, n)
			}
			prev = d
		}
		if isBox(b) {
			join(prev, b, n)
		}
	}
	return g
}

// order sorts each row to cut the crossings between adjacent rows: sweeps
// by the weighted mean position of each item's neighbours, down and up,
// each followed by swapping neighbours while that crosses less, keeping the
// best order seen.
func (g *layered) order() {
	pos := map[string]float64{}
	index := func() {
		for _, row := range g.rows {
			for i, id := range row {
				pos[id] = float64(i)
			}
		}
	}
	crossings := func(r int) int {
		type e struct {
			a, b float64
			n    int
		}
		var es []e
		for _, v := range g.rows[r] {
			for w, n := range g.down[v] {
				es = append(es, e{pos[v], pos[w], n})
			}
		}
		total := 0
		for i := range es {
			for j := i + 1; j < len(es); j++ {
				if (es[i].a-es[j].a)*(es[i].b-es[j].b) < 0 {
					total += es[i].n * es[j].n
				}
			}
		}
		return total
	}
	all := func() int {
		index()
		t := 0
		for r := 0; r+1 < len(g.rows); r++ {
			t += crossings(r)
		}
		return t
	}
	sweep := func(r int, side, other map[string]map[string]int) {
		key := map[string]float64{}
		for _, id := range g.rows[r] {
			sum, k := 0.0, 0
			// An item joined only the other way, as a chain from the
			// frame is, follows that side.
			near := side[id]
			if len(near) == 0 {
				near = other[id]
			}
			for o, n := range near {
				sum += pos[o] * float64(n)
				k += n
			}
			key[id] = pos[id]
			if k > 0 {
				key[id] = sum / float64(k)
			}
		}
		sort.SliceStable(g.rows[r], func(i, j int) bool { return key[g.rows[r][i]] < key[g.rows[r][j]] })
		for i, id := range g.rows[r] {
			pos[id] = float64(i)
		}
	}
	// pair is how many crossings the lines of u and v make when u lies
	// left of v.
	pair := func(u, v string) int {
		c := 0
		for _, side := range []map[string]map[string]int{g.up, g.down} {
			for a, na := range side[u] {
				for b, nb := range side[v] {
					if pos[a] > pos[b] {
						c += na * nb
					}
				}
			}
		}
		return c
	}
	transpose := func() {
		for again, rounds := true, 0; again && rounds < 8; rounds++ {
			again = false
			for _, row := range g.rows {
				for i := 0; i+1 < len(row); i++ {
					u, v := row[i], row[i+1]
					if pair(v, u) < pair(u, v) {
						row[i], row[i+1] = v, u
						pos[u], pos[v] = float64(i+1), float64(i)
						again = true
					}
				}
			}
		}
	}
	snapshot := func() [][]string {
		out := make([][]string, len(g.rows))
		for i, r := range g.rows {
			out[i] = slices.Clone(r)
		}
		return out
	}
	index()
	transpose()
	best, bestN := snapshot(), all()
	for i := 0; i < 12; i++ {
		if i%2 == 0 {
			for r := 1; r < len(g.rows); r++ {
				sweep(r, g.up, g.down)
			}
		} else {
			for r := len(g.rows) - 2; r >= 0; r-- {
				sweep(r, g.down, g.up)
			}
		}
		transpose()
		if c := all(); c < bestN {
			best, bestN = snapshot(), c
		}
	}
	g.rows = best
}

// drop takes chain c out of the rows: its lines will pass them in the
// gaps between boxes, as they would with no chain.
func (g *layered) drop(c int) {
	ch := &g.chains[c]
	for r := ch.top; r <= ch.bottom; r++ {
		d := dummy(c, r)
		g.rows[r] = slices.DeleteFunc(g.rows[r], func(id string) bool { return id == d })
		for o := range g.up[d] {
			delete(g.down[o], d)
		}
		for o := range g.down[d] {
			delete(g.up[o], d)
		}
		delete(g.up, d)
		delete(g.down, d)
		delete(g.chainOf, d)
		delete(g.w, d)
	}
	ch.lanes = 0
}

// sep is how far apart two adjacent items of a row stand.
func (g *layered) sep(a, b string) int {
	_, da := g.chainOf[a]
	_, db := g.chainOf[b]
	if da || db {
		return chainSep
	}
	return gapX
}

// place gives every item its left x and returns the drawing's width. It
// works on blocks, as Brandes and Köpf do: a box is a block, and so is
// each chain, so a chain is placed as one vertical line. Two items side by
// side in a row keep their order and spacing whatever rows their blocks
// span, so a box steps aside where a chain needs to pass. A chain that
// cannot be one block without contradicting the row order (two chains that
// cross) is left as one block per dummy. The blocks start halfway between
// their leftmost and rightmost places, then each moves, within what its
// neighbours leave it, toward the weighted median of what it is joined to:
// a chain toward an x it can run straight down from one of its ends.
func (g *layered) place(budget int) (map[string]int, int) {
	var blocks [][]string
	blockOf := map[string]int{}
	for _, row := range g.rows {
		for _, id := range row {
			blockOf[id] = len(blocks)
			blocks = append(blocks, []string{id})
		}
	}
	type edge struct{ to, d int }
	// constraints are, per block, the blocks that must lie to its right
	// and how far its left x must stay from theirs.
	constraints := func() []map[int]int {
		succ := make([]map[int]int, len(blocks))
		for b := range succ {
			succ[b] = map[int]int{}
		}
		for _, row := range g.rows {
			for i := 1; i < len(row); i++ {
				a, b := row[i-1], row[i]
				ba, bb := blockOf[a], blockOf[b]
				succ[ba][bb] = max(succ[ba][bb], g.w[a]+g.sep(a, b))
			}
		}
		return succ
	}
	acyclic := func(succ []map[int]int) bool {
		in := make([]int, len(blocks))
		for b := range succ {
			for c := range succ[b] {
				in[c]++
			}
		}
		var q []int
		for b := range in {
			if in[b] == 0 && len(blocks[b]) > 0 {
				q = append(q, b)
			}
		}
		seen := 0
		for len(q) > 0 {
			b := q[0]
			q = q[1:]
			seen++
			for c := range succ[b] {
				if in[c]--; in[c] == 0 {
					q = append(q, c)
				}
			}
		}
		live := 0
		for _, bl := range blocks {
			if len(bl) > 0 {
				live++
			}
		}
		return seen == live
	}
	cs := make([]int, len(g.chains))
	for c := range cs {
		cs[c] = c
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := g.chains[cs[i]], g.chains[cs[j]]
		return a.bottom-a.top > b.bottom-b.top
	})
	split := func(c int) {
		ch := g.chains[c]
		head := blockOf[dummy(c, ch.top)]
		blocks[head] = blocks[head][:1]
		for r := ch.top + 1; r <= ch.bottom; r++ {
			d := dummy(c, r)
			blockOf[d] = len(blocks)
			blocks = append(blocks, []string{d})
		}
	}
	var merged []int
	for _, c := range cs {
		ch := g.chains[c]
		if ch.bottom == ch.top {
			continue
		}
		head := blockOf[dummy(c, ch.top)]
		for r := ch.top + 1; r <= ch.bottom; r++ {
			d := dummy(c, r)
			blocks[blockOf[d]] = nil
			blockOf[d] = head
			blocks[head] = append(blocks[head], d)
		}
		if acyclic(constraints()) {
			merged = append(merged, c)
		} else {
			split(c)
		}
	}
	var succ, pred []map[int]int
	var topo, bw, left []int
	width := 0
	compact := func() {
		succ = constraints()
		pred = make([]map[int]int, len(blocks))
		for b := range pred {
			pred[b] = map[int]int{}
		}
		in := make([]int, len(blocks))
		for b := range succ {
			for c, d := range succ[b] {
				in[c]++
				pred[c][b] = d
			}
		}
		topo = nil
		for b := range in {
			if in[b] == 0 && len(blocks[b]) > 0 {
				topo = append(topo, b)
			}
		}
		for k := 0; k < len(topo); k++ {
			var next []int
			for c := range succ[topo[k]] {
				if in[c]--; in[c] == 0 {
					next = append(next, c)
				}
			}
			slices.Sort(next)
			topo = append(topo, next...)
		}
		bw = make([]int, len(blocks))
		for b, bl := range blocks {
			for _, id := range bl {
				bw[b] = max(bw[b], g.w[id])
			}
		}
		left = make([]int, len(blocks))
		width = 0
		for _, b := range topo {
			left[b] = padX
			for p, d := range pred[b] {
				left[b] = max(left[b], left[p]+d)
			}
			width = max(width, left[b]+bw[b]+padX)
		}
	}
	// A drawing kept straight past its width budget gives up its shortest
	// straight chains first.
	compact()
	for width > budget && len(merged) > 0 {
		split(merged[len(merged)-1])
		merged = merged[:len(merged)-1]
		compact()
	}
	right := make([]int, len(blocks))
	for k := len(topo) - 1; k >= 0; k-- {
		b := topo[k]
		right[b] = width - padX - bw[b]
		for s, d := range succ[b] {
			right[b] = min(right[b], right[s]-d)
		}
	}
	bx := make([]int, len(blocks))
	for b := range bx {
		bx[b] = (left[b] + right[b]) / 2
	}
	x := func(id string) int { return bx[blockOf[id]] }
	center := func(id string) int { return x(id) + g.w[id]/2 }
	// span is where a chain may meet a box and drop straight from it.
	span := func(id string) (int, int, bool) {
		if id == frameTopNode || id == frameBottomNode {
			return 0, 0, false
		}
		return x(id) + 12, x(id) + g.w[id] - 12, true
	}
	weight := func(a, b string) int {
		_, da := g.chainOf[a]
		_, db := g.chainOf[b]
		switch {
		case da && db:
			return 8
		case da || db:
			return 2
		}
		return 1
	}
	// want is the centre block b would like: the weighted median of what
	// its items are joined to outside it.
	want := func(b int) (int, bool) {
		type v struct{ x, w int }
		var vs []v
		total := 0
		for _, id := range blocks[b] {
			for _, side := range []map[string]map[string]int{g.up, g.down} {
				for o, n := range side[id] {
					if blockOf[o] == b {
						continue
					}
					w := n * weight(id, o)
					vs = append(vs, v{center(o), w})
					total += w
				}
			}
		}
		if total == 0 {
			return 0, false
		}
		sort.Slice(vs, func(i, j int) bool { return vs[i].x < vs[j].x || vs[i].x == vs[j].x && vs[i].w < vs[j].w })
		acc := 0
		for i, e := range vs {
			acc += e.w
			if 2*acc == total && i+1 < len(vs) {
				return (e.x + vs[i+1].x) / 2, true
			}
			if 2*acc > total {
				return e.x, true
			}
		}
		return vs[len(vs)-1].x, true
	}
	// A whole chain heads for an x it can drop straight from one of its
	// ends.
	chainWant := func(b int, cur int) int {
		c, ok := g.chainOf[blocks[b][0]]
		if !ok || len(blocks[b]) == 1 && g.chains[c].bottom > g.chains[c].top {
			return cur
		}
		ch := g.chains[c]
		alo, ahi, aok := span(ch.upper)
		blo, bhi, bok := span(ch.lower)
		switch {
		case aok && bok && max(alo, blo) <= min(ahi, bhi):
			return clamp(cur, max(alo, blo), min(ahi, bhi))
		case aok && bok:
			ca, cb := clamp(cur, alo, ahi), clamp(cur, blo, bhi)
			if abs(ca-cur) <= abs(cb-cur) {
				return ca
			}
			return cb
		case aok:
			return clamp(cur, alo, ahi)
		case bok:
			return clamp(cur, blo, bhi)
		}
		return cur
	}
	for pass := 0; pass < 24; pass++ {
		for k := range topo {
			b := topo[k]
			if pass%2 == 1 {
				b = topo[len(topo)-1-k]
			}
			m, ok := want(b)
			if !ok {
				continue
			}
			m = chainWant(b, m)
			lo, hi := padX, width-padX-bw[b]
			for p, d := range pred[b] {
				lo = max(lo, bx[p]+d)
			}
			for s, d := range succ[b] {
				hi = min(hi, bx[s]-d)
			}
			bx[b] = clamp(m-bw[b]/2, lo, hi)
		}
	}
	out := map[string]int{}
	for id, b := range blockOf {
		out[id] = bx[b]
	}
	return out, width
}
