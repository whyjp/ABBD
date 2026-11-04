package screen

import (
	"fmt"
	"slices"
	"sort"
	"strings"
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

// A tactic is one way order searches the rows: the order each row starts
// in, and where a sweep pulls an item among the items it joins. Each finds
// orders the others miss, and the count order minimizes is not the one a
// reader sees: it leaves out the frame's chips, the buses and the lines
// that climb. So a unit's view is drawn with every tactic and keeps the
// drawing that reads best (choose). The overview takes the plain tactic.
type tactic struct {
	// reversed starts every row in reverse.
	reversed bool
	// median pulls an item to the weighted median of what it joins, not to
	// the weighted mean.
	median bool
	// narrow packs each row as tight as its order lets it, and keeps a
	// unit's view within narrowBudget. Otherwise the items spread over the
	// whole budget toward what they join, so more lines drop straight.
	narrow bool
	// seated sets a unit's frame chips in the layout as a row above the
	// boxes and a row below them (arrange), so each chip stands over the
	// chain its lines take and the chains order with the chips. Otherwise
	// the chips are set after the layout, over where their lines leave
	// their row. Only the whole budget seats chips: packed narrow, three
	// more drawings per view cut 1 jog and 3 crossings over 274 views.
	seated bool
}

var tactics = []tactic{{}, {median: true}, {reversed: true}, {narrow: true}, {narrow: true, median: true}, {narrow: true, reversed: true},
	{seated: true}, {seated: true, median: true}, {seated: true, reversed: true}}

// order sorts each row to cut the crossings between adjacent rows: sweeps
// by where each item's neighbours stand, down and up, each followed by
// swapping neighbours while that crosses less, keeping the best order seen.
func (g *layered) order(t tactic) {
	// The sweeps work on item numbers, not names: they look up every
	// item's neighbours many times over.
	var ids []string
	num := map[string]int{}
	rows := make([][]int, len(g.rows))
	for r, row := range g.rows {
		for _, id := range row {
			num[id] = len(ids)
			rows[r] = append(rows[r], len(ids))
			ids = append(ids, id)
		}
	}
	type link struct{ o, n int }
	links := func(side map[string]map[string]int) [][]link {
		out := make([][]link, len(ids))
		for i, id := range ids {
			for o, n := range side[id] {
				out[i] = append(out[i], link{num[o], n})
			}
		}
		return out
	}
	up, down := links(g.up), links(g.down)
	pos := make([]float64, len(ids))
	index := func() {
		for _, row := range rows {
			for i, v := range row {
				pos[v] = float64(i)
			}
		}
	}
	crossings := func(r int) int {
		type e struct {
			a, b float64
			n    int
		}
		var es []e
		for _, v := range rows[r] {
			for _, l := range down[v] {
				es = append(es, e{pos[v], pos[l.o], l.n})
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
		for r := 0; r+1 < len(rows); r++ {
			t += crossings(r)
		}
		return t
	}
	key := make([]float64, len(ids))
	sweep := func(r int, side, other [][]link) {
		for _, v := range rows[r] {
			sum, k := 0.0, 0
			// An item joined only the other way, as a chain from the
			// frame is, follows that side.
			near := side[v]
			if len(near) == 0 {
				near = other[v]
			}
			for _, l := range near {
				sum += pos[l.o] * float64(l.n)
				k += l.n
			}
			key[v] = pos[v]
			switch {
			case k > 0 && t.median:
				var at []float64
				for _, l := range near {
					for range l.n {
						at = append(at, pos[l.o])
					}
				}
				slices.Sort(at)
				key[v] = (at[(k-1)/2] + at[k/2]) / 2
			case k > 0:
				key[v] = sum / float64(k)
			}
		}
		sort.SliceStable(rows[r], func(i, j int) bool { return key[rows[r][i]] < key[rows[r][j]] })
		for i, v := range rows[r] {
			pos[v] = float64(i)
		}
	}
	// pair is how many crossings the lines of u and v make when u lies
	// left of v.
	pair := func(u, v int) int {
		c := 0
		for _, side := range [][][]link{up, down} {
			for _, a := range side[u] {
				for _, b := range side[v] {
					if pos[a.o] > pos[b.o] {
						c += a.n * b.n
					}
				}
			}
		}
		return c
	}
	transpose := func() {
		for again, rounds := true, 0; again && rounds < 8; rounds++ {
			again = false
			for _, row := range rows {
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
	snapshot := func() [][]int {
		out := make([][]int, len(rows))
		for i, r := range rows {
			out[i] = slices.Clone(r)
		}
		return out
	}
	if t.reversed {
		for _, row := range rows {
			slices.Reverse(row)
		}
	}
	index()
	transpose()
	best, bestN := snapshot(), all()
	for i := 0; i < 12; i++ {
		if i%2 == 0 {
			for r := 1; r < len(rows); r++ {
				sweep(r, up, down)
			}
		} else {
			for r := len(rows) - 2; r >= 0; r-- {
				sweep(r, down, up)
			}
		}
		transpose()
		if c := all(); c < bestN {
			best, bestN = snapshot(), c
		}
	}
	// A box joined to nothing in the rows next to its own crosses no line
	// wherever it stands, so it stands last, out of the way of the lines
	// that pass its row: left in front of them, it would push them aside.
	alone := func(v int) bool { return len(up[v])+len(down[v]) == 0 }
	for r, row := range best {
		slices.SortStableFunc(row, func(a, b int) int {
			switch {
			case alone(a) == alone(b):
				return 0
			case alone(a):
				return 1
			}
			return -1
		})
		for i, v := range row {
			g.rows[r][i] = ids[v]
		}
	}
}

// crossCost is how many pixels of line a crossing is worth: a move that
// adds one crossing must save a reader more line than this.
const crossCost = 120

// moveMost is the most items one move passes, which keeps shorten linear
// in a row's length.
const moveMost = 8

// shorten moves an item along its row while that makes the lines shorter
// by more than crossCost, each crossing counted at crossCost: a line that
// crosses the whole drawing to dodge a few crossings reads worse than the
// crossings, and a swap that saves a few pixels is not worth the churn. It
// measures the lines where place set every item, at x, and reports whether
// it moved any, so the rows are placed again. An item that moves takes the
// room of the items it passes, and they give up the room it took. A
// box joined to nothing stays where order left it.
func (g *layered) shorten(x map[string]int) bool {
	at := make([]map[string]int, len(g.rows))
	for r, row := range g.rows {
		at[r] = map[string]int{}
		for _, id := range row {
			at[r][id] = x[id] + g.w[id]/2
		}
	}
	swapped := false
	alone := func(id string) bool { return len(g.up[id])+len(g.down[id]) == 0 }
	// gain is what moving u past the items passed in row r saves, moved
	// holding the new centre of each item that moves: the line it shortens
	// less the crossings it adds. Only u changes sides with the items it
	// passes, so only a line of u and a line of one of them can cross or
	// uncross. right says u moves right.
	gain := func(r int, u string, passed []string, right bool, moved map[string]int) int {
		saved := 0
		for k, side := range []map[string]map[string]int{g.up, g.down} {
			near := at[max(0, r-1)]
			if k == 1 {
				near = at[min(len(at)-1, r+1)]
			}
			for id, x := range moved {
				for o, n := range side[id] {
					saved += n * (abs(at[r][id]-near[o]) - abs(x-near[o]))
				}
			}
			for _, v := range passed {
				for a, na := range side[u] {
					for b, nb := range side[v] {
						xa, xb := near[a], near[b]
						if !right {
							xa, xb = xb, xa
						}
						switch {
						case xa < xb:
							saved -= crossCost * na * nb
						case xa > xb:
							saved += crossCost * na * nb
						}
					}
				}
			}
		}
		return saved
	}
	// moveTo is where u, at i in row r, would stand at j and where the items
	// it passes would, each giving up the room u takes.
	moveTo := func(r, i, j int) ([]string, map[string]int) {
		row := g.rows[r]
		u := row[i]
		moved := map[string]int{}
		var passed []string
		if j > i {
			passed = row[i+1 : j+1]
			for _, v := range passed {
				moved[v] = at[r][v] - g.w[u] - g.sep(u, v)
			}
			last := passed[len(passed)-1]
			moved[u] = at[r][last] + g.w[last]/2 - g.w[u]/2
		} else {
			passed = row[j:i]
			for _, v := range passed {
				moved[v] = at[r][v] + g.w[u] + g.sep(v, u)
			}
			first := passed[0]
			moved[u] = at[r][first] - g.w[first]/2 + g.w[u]/2
		}
		return passed, moved
	}
	for again, rounds := true, 0; again && rounds < 8; rounds++ {
		again = false
		for r := range g.rows {
			for i := 0; i < len(g.rows[r]); i++ {
				u := g.rows[r][i]
				if alone(u) {
					continue
				}
				best, bestGain := -1, crossCost
				for _, step := range []int{-1, 1} {
					for j := i + step; j >= 0 && j < len(g.rows[r]) && abs(j-i) <= moveMost && !alone(g.rows[r][j]); j += step {
						passed, moved := moveTo(r, i, j)
						if gn := gain(r, u, passed, j > i, moved); gn > bestGain {
							best, bestGain = j, gn
						}
					}
				}
				if best < 0 {
					continue
				}
				_, moved := moveTo(r, i, best)
				for id, x := range moved {
					at[r][id] = x
				}
				row := slices.Delete(g.rows[r], i, i+1)
				g.rows[r] = slices.Insert(row, best, u)
				again, swapped = true, true
			}
		}
	}
	return swapped
}

// shed takes the shortest chains out of the rows, one at a time, while the
// rows with every chain crooked are still wider than budget: their lines
// pass those rows in the gaps between boxes, as they would with no chain.
// Setting chains straight only widens a drawing, so the rows with every
// chain crooked are the narrowest place can make them. The chains left
// are numbered afresh, so every chain the graph holds has its dummies.
func (g *layered) shed(budget int) {
	width := func(row []string) int {
		w := 2 * padX
		for i, id := range row {
			w += g.w[id]
			if i > 0 {
				w += g.sep(row[i-1], id)
			}
		}
		return w
	}
	widths := make([]int, len(g.rows))
	for r, row := range g.rows {
		widths[r] = width(row)
	}
	byLength := make([]int, len(g.chains))
	for c := range byLength {
		byLength[c] = c
	}
	sort.SliceStable(byLength, func(i, j int) bool {
		a, b := g.chains[byLength[i]], g.chains[byLength[j]]
		return a.bottom-a.top < b.bottom-b.top
	})
	gone := make([]bool, len(g.chains))
	shed := false
	for _, c := range byLength {
		if slices.Max(append([]int{0}, widths...)) <= budget {
			break
		}
		gone[c], shed = true, true
		ch := g.chains[c]
		for r := ch.top; r <= ch.bottom; r++ {
			d := dummy(c, r)
			g.rows[r] = slices.DeleteFunc(g.rows[r], func(id string) bool { return id == d })
			widths[r] = width(g.rows[r])
		}
	}
	if !shed {
		return
	}
	rename, chainOf := map[string]string{}, map[string]int{}
	var kept []chain
	for c, ch := range g.chains {
		for r := ch.top; r <= ch.bottom; r++ {
			rename[dummy(c, r)] = ""
			if !gone[c] {
				rename[dummy(c, r)] = dummy(len(kept), r)
				chainOf[dummy(len(kept), r)] = len(kept)
			}
		}
		if !gone[c] {
			kept = append(kept, ch)
		}
	}
	name := func(id string) (string, bool) {
		n, ok := rename[id]
		if !ok {
			return id, true
		}
		return n, n != ""
	}
	relink := func(side map[string]map[string]int) map[string]map[string]int {
		out := map[string]map[string]int{}
		for a, near := range side {
			na, ok := name(a)
			if !ok {
				continue
			}
			out[na] = map[string]int{}
			for b, n := range near {
				if nb, ok := name(b); ok {
					out[na][nb] = n
				}
			}
		}
		return out
	}
	w := map[string]int{}
	for _, row := range g.rows {
		for i, id := range row {
			row[i], _ = name(id)
			w[row[i]] = g.w[id]
		}
	}
	g.w, g.chainOf, g.chains = w, chainOf, kept
	g.up, g.down = relink(g.up), relink(g.down)
}

// sep is how far apart two adjacent items of a row stand.
func (g *layered) sep(a, b string) int {
	_, da := g.chainOf[a]
	_, db := g.chainOf[b]
	switch {
	case da || db:
		return chainSep
	case isChip(a) && isChip(b):
		return 4
	}
	return gapX
}

// place gives every item its left x and returns the drawing's width. It
// works on blocks, as Brandes and Köpf do: a box is a block, and so is each
// straight run of a chain, so a run is placed as one vertical line. Two
// items side by side in a row keep their order and spacing whatever rows
// their blocks span, so a box steps aside where a run needs to pass. The
// blocks start halfway between their leftmost and rightmost places, then
// each moves, within what its neighbours leave it, toward the weighted
// median of what it is joined to: a run that reaches an end of its chain
// toward an x it can drop straight from that end. The blocks may spread up
// to room wide, and the drawing is then trimmed to what they cover.
func (g *layered) place(budget, room int) (map[string]int, int) {
	// run is the dummies of chain c in rows from to to, set as one block.
	type run struct{ c, from, to int }
	cs := make([]int, len(g.chains))
	for c := range cs {
		cs[c] = c
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := g.chains[cs[i]], g.chains[cs[j]]
		return a.bottom-a.top > b.bottom-b.top
	})
	var blocks [][]string
	var blockOf map[string]int
	var runOf map[int]run
	var succ, pred []map[int]int
	var topo, bw, left []int
	width := 0
	// compact makes the blocks, every item one but the dummies of each run,
	// and packs them as far left as their order lets them: succ holds, per
	// block, the blocks that must lie to its right and how far its left x
	// must stay from theirs. It reports false when the runs contradict the
	// row order, two of them swapping sides, so no x can keep them all.
	compact := func(runs []run) bool {
		blocks, blockOf, runOf = nil, map[string]int{}, map[int]run{}
		for r, row := range g.rows {
			for _, id := range row {
				blockOf[id] = len(blocks)
				if c, d := g.chainOf[id]; d {
					runOf[len(blocks)] = run{c, r, r}
				}
				blocks = append(blocks, []string{id})
			}
		}
		for _, u := range runs {
			head := blockOf[dummy(u.c, u.from)]
			runOf[head] = u
			for r := u.from + 1; r <= u.to; r++ {
				d := dummy(u.c, r)
				blocks[blockOf[d]] = nil
				blockOf[d] = head
				blocks[head] = append(blocks[head], d)
			}
		}
		succ = make([]map[int]int, len(blocks))
		pred = make([]map[int]int, len(blocks))
		for b := range succ {
			succ[b], pred[b] = map[int]int{}, map[int]int{}
		}
		for _, row := range g.rows {
			for i := 1; i < len(row); i++ {
				a, b := row[i-1], row[i]
				ba, bb := blockOf[a], blockOf[b]
				succ[ba][bb] = max(succ[ba][bb], g.w[a]+g.sep(a, b))
			}
		}
		in := make([]int, len(blocks))
		for b := range succ {
			for c, d := range succ[b] {
				in[c]++
				pred[c][b] = d
			}
		}
		topo = nil
		live := 0
		for b := range in {
			if len(blocks[b]) > 0 {
				live++
				if in[b] == 0 {
					topo = append(topo, b)
				}
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
		return len(topo) == live
	}
	// Each chain, longest first, is set as few straight runs as the drawing
	// lets it: a run grows down as far as the order still holds and the
	// drawing still fits its budget, so a line that cannot run straight all
	// the way turns once where it must, not in every row it passes. A run
	// only adds to what the blocks must keep, so how far one can grow is
	// found by halving.
	var runs []run
	fits := func(u run) bool {
		runs = append(runs, u)
		ok := compact(runs) && width <= budget
		runs = runs[:len(runs)-1]
		return ok
	}
	// Most drawings with room to spare take every chain straight at once.
	for _, c := range cs {
		runs = append(runs, run{c, g.chains[c].top, g.chains[c].bottom})
	}
	if compact(runs) && width <= budget {
		cs = nil
	} else {
		runs = nil
	}
	for _, c := range cs {
		ch := g.chains[c]
		if fits(run{c, ch.top, ch.bottom}) {
			if ch.bottom > ch.top {
				runs = append(runs, run{c, ch.top, ch.bottom})
			}
			continue
		}
		for from := ch.top; from <= ch.bottom; {
			lo, hi := from, ch.bottom
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if fits(run{c, from, mid}) {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
			if lo > from {
				runs = append(runs, run{c, from, lo})
			}
			from = lo + 1
		}
	}
	compact(runs)
	room = max(width, room)
	right := make([]int, len(blocks))
	for k := len(topo) - 1; k >= 0; k-- {
		b := topo[k]
		right[b] = room - padX - bw[b]
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
		switch {
		case id == frameTopNode || id == frameBottomNode:
			return 0, 0, false
		case isChip(id):
			return x(id) + 4, x(id) + g.w[id] - 4, true
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
	// A run that reaches an end of its chain heads for an x it can drop
	// straight from that end.
	chainWant := func(b int, cur int) int {
		u, ok := runOf[b]
		if !ok {
			return cur
		}
		ch := g.chains[u.c]
		alo, ahi, aok := span(ch.upper)
		blo, bhi, bok := span(ch.lower)
		aok = aok && u.from == ch.top
		bok = bok && u.to == ch.bottom
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
			lo, hi := padX, room-padX-bw[b]
			for p, d := range pred[b] {
				lo = max(lo, bx[p]+d)
			}
			for s, d := range succ[b] {
				hi = min(hi, bx[s]-d)
			}
			bx[b] = clamp(m-bw[b]/2, lo, hi)
		}
	}
	lo, hi := room, 0
	for _, b := range topo {
		lo, hi = min(lo, bx[b]), max(hi, bx[b]+bw[b])
	}
	out := map[string]int{}
	for id, b := range blockOf {
		out[id] = bx[b] - lo + padX
	}
	return out, max(width, hi-lo+2*padX)
}

// seat orders every row of a focus view by where its items stand in the
// full view, so the ring keeps the reader's mental map: a box left of
// another there stays left of it here. A chain stands where its box end
// does. A chain with no box end but the focus, a line to the frame, stands
// mid-row, over or under the focus. Every line of a focus view has the
// focus at one end, so no order crosses fewer of them.
func (g *layered) seat(at map[string]int, focus string) {
	mid := func(row []string) int {
		var xs []int
		for _, id := range row {
			if x, ok := at[id]; ok && id != focus {
				xs = append(xs, x)
			}
		}
		if len(xs) == 0 {
			return 0
		}
		slices.Sort(xs)
		return (xs[(len(xs)-1)/2] + xs[len(xs)/2]) / 2
	}
	for r, row := range g.rows {
		key := map[string]int{}
		for _, id := range row {
			x, ok := at[id]
			if c, d := g.chainOf[id]; d {
				ch := g.chains[c]
				x, ok = mid(row), true
				for _, end := range []string{ch.upper, ch.lower} {
					if e, has := at[end]; has && end != focus {
						x = e
					}
				}
			}
			if !ok {
				x = 1 << 30
			}
			key[id] = x
		}
		slices.SortStableFunc(g.rows[r], func(a, b string) int {
			if key[a] != key[b] {
				return key[a] - key[b]
			}
			return strings.Compare(a, b)
		})
	}
}

// ring places a focus view's rows about one vertical axis: the focus sits
// on it, and so do the lines that reach the focus from the frame, so they
// drop straight into it. Each row packs tight around those lines, or
// around the axis when it has none, keeping the order seat gave it. A row
// that would reach past budget that way moves in only as far as it must.
func (g *layered) ring(focus string, budget int) (map[string]int, int) {
	axial := func(id string) bool {
		c, d := g.chainOf[id]
		if !d {
			return id == focus
		}
		ch := g.chains[c]
		return (ch.upper == focus || ch.lower == focus) && (ch.upper == frameTopNode || ch.lower == frameBottomNode)
	}
	// extent is how far row reaches left of the axis, and how wide it is:
	// the axis runs through the middle of its axial items when it has
	// them, else through its own middle.
	extent := func(row []string) (int, int) {
		x, lo, hi := 0, -1, -1
		for i, id := range row {
			if i > 0 {
				x += g.sep(row[i-1], id)
			}
			if axial(id) {
				if lo < 0 {
					lo = x
				}
				hi = x + g.w[id]
			}
			x += g.w[id]
		}
		if lo < 0 {
			lo, hi = 0, x
		}
		return (lo + hi) / 2, x
	}
	half, widest := 0, 0
	for _, row := range g.rows {
		l, w := extent(row)
		half, widest = max(half, l, w-l), max(widest, w)
	}
	width := max(min(2*half, max(budget-2*padX, widest)), minWidth-2*padX) + 2*padX
	out := map[string]int{}
	for _, row := range g.rows {
		l, w := extent(row)
		x := clamp(width/2-l, padX, width-padX-w)
		for i, id := range row {
			if i > 0 {
				x += g.sep(row[i-1], id)
			}
			out[id] = x
			x += g.w[id]
		}
	}
	return out, width
}
