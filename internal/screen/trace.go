package screen

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
)

// This file measures what a reader meets when they trace one relation along
// the drawn strokes, docs/blueprint-svg.md rules 16 to 23. The strokes form
// an ink graph: two strokes of one colour join where one ends on the other,
// and lines that only cross stay apart. A reader follows the ink the way the
// drawing flows: a vertical piece in the direction it was drawn, which is
// the line's direction, and a horizontal piece either way, since a bus's
// track carries lines both ways. A traced relation is the path through that
// graph, with the fewest bends, from where its line leaves its start to an
// arrowhead on its end.

// inkSeg is one straight piece of a stroke between two ink nodes.
type inkSeg struct {
	a, b   fpt
	colour string
	// free says a reader may follow the piece either way.
	free bool
}

type inkKey struct {
	colour string
	x, y   int
}

func keyOf(colour string, p fpt) inkKey {
	return inkKey{colour, int(math.Round(p.x * 2)), int(math.Round(p.y * 2))}
}

// colour is the kind a stroke's colour shows; a line between two modules
// that use each other is drawn in the direct-dependency colour.
func colour(class string) string {
	if k := kindOf(class); k != "mutual" {
		return k
	}
	return "bypass"
}

// kindOf is the kind of line a stroke's class names, without the marks
// that say how it is drawn or the classes that light it on hover.
func kindOf(class string) string {
	for _, c := range strings.Fields(class) {
		switch c {
		case "w", "joins", "back", "both":
		default:
			return c
		}
	}
	return ""
}

type ink struct {
	segs []inkSeg
	// out lists, per node, the pieces a reader may follow away from it.
	out map[inkKey][]int
	// arms are the directions ink leaves each node in, either way.
	arms map[inkKey]map[fpt]bool
	pos  map[inkKey]fpt
}

func dirOf(a, b fpt) fpt {
	l := dist(a, b)
	return fpt{math.Round((b.x - a.x) / l * 100), math.Round((b.y - a.y) / l * 100)}
}

// inkGraph cuts every stroke at each point where another stroke of its
// colour ends on it, so the pieces meet only at nodes.
func inkGraph(s sheet) ink {
	type raw struct {
		a, b        fpt
		colour      string
		both, horiz bool
	}
	var raws []raw
	for _, w := range s.strokes {
		c := colour(w.class)
		both := strings.Contains(" "+w.class+" ", " mutual ")
		for i := 1; i < len(w.pts); i++ {
			a, b := w.pts[i-1], w.pts[i]
			if dist(a, b) < 0.5 {
				continue
			}
			raws = append(raws, raw{a, b, c, both, math.Abs(a.y-b.y) < 0.5})
		}
	}
	g := ink{out: map[inkKey][]int{}, arms: map[inkKey]map[fpt]bool{}, pos: map[inkKey]fpt{}}
	seen := map[[2]inkKey]int{}
	for i, r := range raws {
		cuts := []fpt{r.a, r.b}
		// A dot where two lines of its colour cross says they join there.
		for _, j := range s.junctions {
			if j.kind == r.colour && segDist(j.at, r.a, r.b) < 1 {
				cuts = append(cuts, j.at)
			}
		}
		for j, o := range raws {
			if j == i || o.colour != r.colour {
				continue
			}
			for _, p := range []fpt{o.a, o.b} {
				if segDist(p, r.a, r.b) < 1 {
					cuts = append(cuts, p)
				}
			}
		}
		sort.Slice(cuts, func(x, y int) bool { return dist(r.a, cuts[x]) < dist(r.a, cuts[y]) })
		for k := 1; k < len(cuts); k++ {
			a, b := cuts[k-1], cuts[k]
			ka, kb := keyOf(r.colour, a), keyOf(r.colour, b)
			if ka == kb {
				continue
			}
			// A piece a one-way stroke and a two-way stroke both draw may
			// be followed either way.
			if id, ok := seen[[2]inkKey{ka, kb}]; ok {
				if (r.both || r.horiz) && !g.segs[id].free {
					g.segs[id].free = true
					g.out[kb] = append(g.out[kb], id)
				}
				continue
			}
			seen[[2]inkKey{ka, kb}] = len(g.segs)
			g.pos[ka], g.pos[kb] = a, b
			id := len(g.segs)
			g.segs = append(g.segs, inkSeg{a, b, r.colour, r.both || r.horiz})
			g.out[ka] = append(g.out[ka], id)
			if r.both || r.horiz {
				g.out[kb] = append(g.out[kb], id)
			}
			for _, e := range [][2]fpt{{a, b}, {b, a}} {
				k := keyOf(r.colour, e[0])
				if g.arms[k] == nil {
					g.arms[k] = map[fpt]bool{}
				}
				g.arms[k][dirOf(e[0], e[1])] = true
			}
		}
	}
	return g
}

// other is the far end of piece s from node k.
func (g ink) other(s int, k inkKey) fpt {
	if keyOf(g.segs[s].colour, g.segs[s].a) == k {
		return g.segs[s].b
	}
	return g.segs[s].a
}

// 21: how many times the median box's height the tallest box may be.
const maxSpread = 1.5

// Trace is what tracing every relation of one drawing measures.
type Trace struct {
	// Relations is how many relations the view stands for, Lost how many
	// of them no path of ink carries, and Phantom how many pairs of places
	// the ink joins that no relation joins.
	Relations, Lost, Phantom int
	// Bends, Jogs and Crossings are, per traced relation, how often its path
	// turns, how many of its stretches between two turns are shorter than
	// jogLen, and how many other lines it crosses on the way.
	Bends, Jogs, Crossings []int
	// Ways are the traced paths, by their turning points.
	Ways []tracedWay
	// Missing are the relations no ink carries, and Phantoms the paths of
	// ink between places that no relation joins.
	Missing  [][2]string
	Phantoms []tracedWay
	// UnmarkedT counts the points where ink splits or merges without a
	// junction dot, at the points Unmarked lists, and CountAmbiguity the ×N
	// labels that sit beside no line or beside more than one.
	UnmarkedT, CountAmbiguity int
	Unmarked                  []fpt
	// SizeSpread is the tallest box's height over the median box's, and
	// Beside the share of the ink that runs alongside a row of boxes
	// instead of through the gaps between rows.
	SizeSpread, Beside float64
}

// tracedWay is one traced relation's path from the place it leaves to the
// place it reaches.
type tracedWay struct {
	from, to string
	pts      []fpt
	// kind is the colour of the ink the path runs on.
	kind string
}

// jogLen is how short a stretch between two turns may be before it reads as
// a jog: a sideways step a person drawing the line by hand would not make.
const jogLen = 24.0

// turns keeps the points of a path where it turns, and its two ends.
func turns(pts []fpt) []fpt {
	out := []fpt{pts[0]}
	for i := 1; i < len(pts)-1; i++ {
		if dirOf(out[len(out)-1], pts[i]) != dirOf(pts[i], pts[i+1]) {
			out = append(out, pts[i])
		}
	}
	return append(out, pts[len(pts)-1])
}

// jogs counts the stretches of a path between two of its turns that are
// shorter than jogLen.
func jogs(pts []fpt) int {
	n := 0
	for i := 2; i < len(pts)-1; i++ {
		if dist(pts[i-1], pts[i]) < jogLen {
			n++
		}
	}
	return n
}

func sum(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}

func median(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	if len(s)%2 == 1 {
		return float64(s[len(s)/2])
	}
	return float64(s[len(s)/2-1]+s[len(s)/2]) / 2
}

func maxOf(xs []int) int {
	m := 0
	for _, x := range xs {
		m = max(m, x)
	}
	return m
}

func (t Trace) String() string {
	over := 0
	for _, b := range t.Bends {
		if b > 2 {
			over++
		}
	}
	return fmt.Sprintf("relations %d, lost %d, phantom %d, bends median %.1f max %d total %d, over two bends %d, jogs %d, path crossings median %.1f max %d, unmarked T %d, count ambiguity %d, size spread %.1f, beside %.0f%%",
		t.Relations, t.Lost, t.Phantom, median(t.Bends), maxOf(t.Bends), sum(t.Bends), over, sum(t.Jogs), median(t.Crossings), maxOf(t.Crossings), t.UnmarkedT, t.CountAmbiguity, t.SizeSpread, 100*t.Beside)
}

// shapeOf is the place a point on a drawing belongs to: the box it lies on,
// or a chip on no box, by its index in s.shapes, or -1 for none. A chip
// with no id is no place.
func shapeOf(s sheet, p fpt) int {
	for _, kind := range []string{"box", "port"} {
		for i, sh := range s.shapes {
			if sh.kind == kind && sh.r.grow(5).contains(p, 0) {
				if sh.id == "" {
					return -1
				}
				return i
			}
		}
	}
	return -1
}

// trace measures one drawing against the relations it should draw, each a
// pair of place ids.
func trace(s sheet, rels [][2]string) Trace {
	g := inkGraph(s)
	var t Trace
	// Where lines leave and reach each place.
	starts, heads := map[int][]inkKey{}, map[int][]inkKey{}
	for _, w := range s.strokes {
		if len(w.pts) < 2 {
			continue
		}
		c := colour(w.class)
		first, last := w.pts[0], w.pts[len(w.pts)-1]
		both := strings.Contains(" "+w.class+" ", " mutual ")
		if id := shapeOf(s, first); id >= 0 {
			starts[id] = append(starts[id], keyOf(c, first))
			if both {
				heads[id] = append(heads[id], keyOf(c, first))
			}
		}
		if id := shapeOf(s, last); id >= 0 && !strings.Contains(" "+w.class+" ", " joins ") {
			heads[id] = append(heads[id], keyOf(c, last))
			if both {
				starts[id] = append(starts[id], keyOf(c, last))
			}
		}
	}
	headAt := map[inkKey]int{}
	for id, ks := range heads {
		for _, k := range ks {
			headAt[k] = id
		}
	}
	want := map[[2]string]bool{}
	for _, r := range rels {
		if r[0] != r[1] {
			want[r] = true
		}
	}
	// Every place each start reaches, by the path with the fewest bends.
	type state struct {
		k   inkKey
		dir fpt
	}
	type best struct {
		bends int
		path  []int
		pts   []fpt
	}
	reach := func(from int) map[int]best {
		dist := map[state]int{}
		prev := map[state]struct {
			s   state
			seg int
		}{}
		var queue []state
		for _, k := range starts[from] {
			st := state{k, fpt{}}
			dist[st] = 0
			queue = append(queue, st)
		}
		found := map[int]best{}
		// Bends cost 1 and straight steps 0: a 0-1 search.
		for len(queue) > 0 {
			bi := 0
			for i, q := range queue {
				if dist[q] < dist[queue[bi]] {
					bi = i
				}
			}
			cur := queue[bi]
			queue = append(queue[:bi], queue[bi+1:]...)
			d := dist[cur]
			if id, ok := headAt[cur.k]; ok && id != from {
				if b, seen := found[id]; !seen || d < b.bends {
					var path []int
					pts := []fpt{g.pos[cur.k]}
					for st := cur; ; {
						p, ok := prev[st]
						if !ok {
							break
						}
						path = append([]int{p.seg}, path...)
						pts = append([]fpt{g.pos[p.s.k]}, pts...)
						st = p.s
					}
					found[id] = best{d, path, pts}
				}
				continue
			}
			for _, si := range g.out[cur.k] {
				nxt := g.other(si, cur.k)
				nk := keyOf(g.segs[si].colour, nxt)
				dir := dirOf(g.pos[cur.k], nxt)
				// Ink that turns back on itself is not a path.
				if dir.x == -cur.dir.x && dir.y == -cur.dir.y && dir != (fpt{}) {
					continue
				}
				nd := d
				if cur.dir != (fpt{}) && dir != cur.dir {
					nd++
				}
				// A line does not run on through a place it reaches.
				if id := shapeOf(s, g.pos[cur.k]); id >= 0 && id != from && cur.dir != (fpt{}) {
					continue
				}
				ns := state{nk, dir}
				if old, ok := dist[ns]; !ok || nd < old {
					dist[ns] = nd
					prev[ns] = struct {
						s   state
						seg int
					}{cur, si}
					queue = append(queue, ns)
				}
			}
		}
		return found
	}
	var froms []int
	for id := range starts {
		froms = append(froms, id)
	}
	sort.Ints(froms)
	traced := map[[2]string]bool{}
	for _, f := range froms {
		found := reach(f)
		for _, to := range slices.Sorted(maps.Keys(found)) {
			b := found[to]
			// A chip that stands for several ports joins each of them, so
			// ink between two places traces every relation between them.
			joined := false
			for _, fi := range strings.Fields(s.shapes[f].id) {
				for _, ti := range strings.Fields(s.shapes[to].id) {
					if pair := [2]string{fi, ti}; want[pair] {
						traced[pair], joined = true, true
					}
				}
			}
			if !joined {
				t.Phantom++
				t.Phantoms = append(t.Phantoms, tracedWay{from: s.shapes[f].id, to: s.shapes[to].id, pts: turns(b.pts)})
				continue
			}
			t.Bends = append(t.Bends, b.bends)
			way := turns(b.pts)
			t.Jogs = append(t.Jogs, jogs(way))
			kind := ""
			if len(b.path) > 0 {
				kind = g.segs[b.path[0]].colour
			}
			t.Ways = append(t.Ways, tracedWay{s.shapes[f].id, s.shapes[to].id, way, kind})
			t.Crossings = append(t.Crossings, pathCrossings(s, g, b.path))
		}
	}
	// On roads each line's route is the line: a road joins places no one
	// line does, and carries kinds the ink's colours keep apart, so a way
	// traced through the ink is no line's. The routes say each relation's
	// way, its bends, jogs and crossings.
	if s.roads && s.routed {
		traced = map[[2]string]bool{}
		t.Bends, t.Jogs, t.Crossings, t.Ways = nil, nil, nil, nil
		for _, r := range s.routes {
			if len(r.pts) < 2 {
				continue
			}
			f, to := shapeOf(s, r.pts[0]), shapeOf(s, r.pts[len(r.pts)-1])
			if f < 0 || to < 0 {
				continue
			}
			both := strings.Contains(" "+r.class+" ", " mutual ")
			joined := false
			for _, fi := range strings.Fields(s.shapes[f].id) {
				for _, ti := range strings.Fields(s.shapes[to].id) {
					for _, pair := range [][2]string{{fi, ti}, {ti, fi}} {
						if want[pair] && (pair[0] == fi || both) {
							traced[pair], joined = true, true
						}
					}
				}
			}
			if !joined {
				continue
			}
			way := turns(r.pts)
			t.Bends = append(t.Bends, len(way)-2)
			t.Jogs = append(t.Jogs, jogs(way))
			t.Ways = append(t.Ways, tracedWay{s.shapes[f].id, s.shapes[to].id, way, colour(strings.TrimPrefix(r.class, "route "))})
			t.Crossings = append(t.Crossings, routeCrossings(s, way))
		}
	}
	// A relation with an end the drawing has no shape for is lost too.
	t.Relations = len(want)
	for r := range want {
		if !traced[r] {
			t.Lost++
			t.Missing = append(t.Missing, r)
		}
	}
	// A point where ink of one colour goes three or more ways is a split or
	// a merge, and needs its dot.
	for k, arms := range g.arms {
		if len(arms) < 3 {
			continue
		}
		p := g.pos[k]
		dot := false
		for _, j := range s.junctions {
			dot = dot || j.kind == k.colour && dist(p, j.at) < 3
		}
		if !dot {
			t.UnmarkedT++
			t.Unmarked = append(t.Unmarked, p)
		}
	}
	t.CountAmbiguity = countAmbiguity(s, g)
	// Rule 21 bounds height, not width: a box may be wide so the lines
	// below it drop straight, but one made tall by its chips reads heavier
	// than the module it is.
	var heights []int
	for _, sh := range s.shapes {
		if sh.kind == "box" && !sh.is("loose") {
			heights = append(heights, int(sh.r.h))
		}
	}
	if m := median(heights); m > 0 {
		t.SizeSpread = float64(maxOf(heights)) / m
	}
	total, beside := 0.0, 0.0
	for _, sg := range g.segs {
		l := dist(sg.a, sg.b)
		total += l
		if math.Abs(sg.a.x-sg.b.x) > 0.5 {
			continue
		}
		lo, hi := math.Min(sg.a.y, sg.b.y), math.Max(sg.a.y, sg.b.y)
		var bands [][2]float64
		for _, sh := range s.shapes {
			if sh.kind == "box" {
				bands = append(bands, [2]float64{sh.r.y, sh.r.y + sh.r.h})
			}
		}
		for y := lo + 1; y < hi; y += 2 {
			for _, b := range bands {
				if y > b[0] && y < b[1] {
					beside += 2
					break
				}
			}
		}
	}
	if total > 0 {
		t.Beside = beside / total
	}
	return t
}

// pathCrossings counts the other lines a traced path crosses, away from its
// two ends.
func pathCrossings(s sheet, g ink, path []int) int {
	if len(path) == 0 {
		return 0
	}
	first, last := g.segs[path[0]], g.segs[path[len(path)-1]]
	ends := []fpt{first.a, first.b, last.a, last.b}
	var seen []fpt
	for _, si := range path {
		a, b := g.segs[si].a, g.segs[si].b
		for _, w := range s.strokes {
			for k := 1; k < len(w.pts); k++ {
				c, d := w.pts[k-1], w.pts[k]
				p, ok := cross(a, b, c, d)
				if !ok || dist(p, c) < 1 || dist(p, d) < 1 || dist(p, a) < 1 || dist(p, b) < 1 {
					continue
				}
				near := false
				for _, e := range ends {
					near = near || dist(p, e) < 10
				}
				for _, q := range seen {
					near = near || dist(p, q) < 6
				}
				if !near {
					seen = append(seen, p)
				}
			}
		}
	}
	return len(seen)
}

// routeCrossings counts the drawn lines a route crosses, away from its
// two ends: where its own road is crossed by other ink.
func routeCrossings(s sheet, way []fpt) int {
	ends := []fpt{way[0], way[len(way)-1]}
	var seen []fpt
	for k := 1; k < len(way); k++ {
		a, b := way[k-1], way[k]
		for _, w := range s.strokes {
			for m := 1; m < len(w.pts); m++ {
				c, d := w.pts[m-1], w.pts[m]
				p, ok := cross(a, b, c, d)
				if !ok || dist(p, c) < 1 || dist(p, d) < 1 || dist(p, a) < 1 || dist(p, b) < 1 {
					continue
				}
				near := false
				for _, e := range ends {
					near = near || dist(p, e) < 10
				}
				for _, q := range seen {
					near = near || dist(p, q) < 6
				}
				if !near {
					seen = append(seen, p)
				}
			}
		}
	}
	return len(seen)
}

// A ×N label belongs to the stretch of ink nearest it, between two splits:
// it must sit within countReach of that stretch and at least countClear
// closer to it than to any other, rule 22.
const countReach, countClear = 10.0, 5.0

// countAmbiguity counts the ×N labels a reader cannot tie to one line.
func countAmbiguity(s sheet, g ink) int {
	// Stretches: pieces joined through nodes where the ink only bends.
	parent := make([]int, len(g.segs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	at := map[inkKey][]int{}
	for i, sg := range g.segs {
		at[keyOf(sg.colour, sg.a)] = append(at[keyOf(sg.colour, sg.a)], i)
		at[keyOf(sg.colour, sg.b)] = append(at[keyOf(sg.colour, sg.b)], i)
	}
	for k, ss := range at {
		if len(g.arms[k]) == 2 {
			for _, i := range ss[1:] {
				parent[find(i)] = find(ss[0])
			}
		}
	}
	bad := 0
	for _, c := range s.counts {
		w := float64(textWidth(c.text, 11))
		box := frect{c.at.x, c.at.y - 9, w, 11}
		near := map[int]float64{}
		for i, sg := range g.segs {
			d := rectSegDist(box, sg.a, sg.b)
			if old, ok := near[find(i)]; !ok || d < old {
				near[find(i)] = d
			}
		}
		var ds []float64
		for _, d := range near {
			ds = append(ds, d)
		}
		sort.Float64s(ds)
		// It must also sit closer to the ink of its own wire than to any
		// other wire's: a count drawn across another line counts that one.
		own, other := 0.0, math.Inf(1)
		if c.group >= 0 {
			own = math.Inf(1)
		}
		for _, st := range s.strokes {
			if c.group < 0 {
				break
			}
			for i := 1; i < len(st.pts); i++ {
				d := rectSegDist(box, st.pts[i-1], st.pts[i])
				if st.group == c.group {
					own = math.Min(own, d)
				} else {
					other = math.Min(other, d)
				}
			}
		}
		if len(ds) == 0 || ds[0] > countReach || len(ds) > 1 && ds[1]-ds[0] < countClear || other-own < countClear {
			bad++
		}
	}
	return bad
}

// rectSegDist is how close segment a-b comes to rectangle r.
func rectSegDist(r frect, a, b fpt) float64 {
	if r.contains(a, 0) || r.contains(b, 0) {
		return 0
	}
	corners := []fpt{{r.x, r.y}, {r.x + r.w, r.y}, {r.x + r.w, r.y + r.h}, {r.x, r.y + r.h}}
	best := math.Inf(1)
	for i := range corners {
		c, d := corners[i], corners[(i+1)%4]
		if _, ok := cross(a, b, c, d); ok {
			return 0
		}
		best = math.Min(best, math.Min(segDist(c, a, b), segDist(a, c, d)))
		best = math.Min(best, segDist(b, c, d))
	}
	return best
}

// steps counts the staircase steps the chooser weighs (cost in weigh.go)
// on the traced paths: each turn past a path's second, and each jog.
func steps(got Trace) int {
	n := sum(got.Jogs)
	for _, b := range got.Bends {
		n += max(0, b-2)
	}
	return n
}

// over2 counts the traced paths that turn more than twice.
func over2(bends []int) int {
	n := 0
	for _, b := range bends {
		if b > 2 {
			n++
		}
	}
	return n
}

func pointList(pts []fpt) string {
	var ps []string
	for _, p := range pts {
		ps = append(ps, fmt.Sprintf("%g,%g", p.x, p.y))
	}
	return strings.Join(ps, " ")
}
