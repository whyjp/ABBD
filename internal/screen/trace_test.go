package screen

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
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
	for _, c := range strings.Fields(class) {
		switch c {
		case "w", "joins", "back", "both":
		case "mutual":
			return "bypass"
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
	seen := map[[2]inkKey]bool{}
	for i, r := range raws {
		cuts := []fpt{r.a, r.b}
		// A dot where two lines cross says they join there.
		for _, p := range s.junctions {
			if segDist(p, r.a, r.b) < 1 {
				cuts = append(cuts, p)
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
			if ka == kb || seen[[2]inkKey{ka, kb}] {
				continue
			}
			seen[[2]inkKey{ka, kb}] = true
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
	// Relations is how many drawn relations there are, Lost how many of them
	// no path of ink carries, and Phantom how many pairs of places the ink
	// joins that no relation joins.
	Relations, Lost, Phantom int
	// Bends and Crossings are, per traced relation, how often its path turns
	// and how many other lines it crosses on the way.
	Bends, Crossings []int
	// UnmarkedT counts the points where ink splits or merges without a
	// junction dot, CountAmbiguity the ×N labels that sit beside no line or
	// beside more than one.
	UnmarkedT, CountAmbiguity int
	// SizeSpread is the tallest box's height over the median box's, and
	// Beside the share of the ink that runs alongside a row of boxes
	// instead of through the gaps between rows.
	SizeSpread, Beside float64
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
	return fmt.Sprintf("relations %d, lost %d, phantom %d, bends median %.1f max %d, path crossings median %.1f max %d, unmarked T %d, count ambiguity %d, size spread %.1f, beside %.0f%%",
		t.Relations, t.Lost, t.Phantom, median(t.Bends), maxOf(t.Bends), median(t.Crossings), maxOf(t.Crossings), t.UnmarkedT, t.CountAmbiguity, t.SizeSpread, 100*t.Beside)
}

// shapeOf is the place a point on a drawing belongs to: the box it lies on,
// or a chip on no box, by its id.
func shapeOf(s sheet, p fpt) string {
	for _, sh := range s.shapes {
		if sh.kind == "box" && sh.r.grow(5).contains(p, 0) {
			return sh.id
		}
	}
	for _, sh := range s.shapes {
		if sh.kind == "port" && sh.r.grow(5).contains(p, 0) {
			return sh.id
		}
	}
	return ""
}

// trace measures one drawing against the relations it should draw, each a
// pair of place ids.
func trace(s sheet, rels [][2]string) Trace {
	g := inkGraph(s)
	var t Trace
	// Where lines leave and reach each place.
	starts, heads := map[string][]inkKey{}, map[string][]inkKey{}
	for _, w := range s.strokes {
		if len(w.pts) < 2 {
			continue
		}
		c := colour(w.class)
		first, last := w.pts[0], w.pts[len(w.pts)-1]
		both := strings.Contains(" "+w.class+" ", " mutual ")
		if id := shapeOf(s, first); id != "" {
			starts[id] = append(starts[id], keyOf(c, first))
			if both {
				heads[id] = append(heads[id], keyOf(c, first))
			}
		}
		if id := shapeOf(s, last); id != "" && !strings.Contains(" "+w.class+" ", " joins ") {
			heads[id] = append(heads[id], keyOf(c, last))
			if both {
				starts[id] = append(starts[id], keyOf(c, last))
			}
		}
	}
	headAt := map[inkKey]string{}
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
	}
	reach := func(from string) map[string]best {
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
		found := map[string]best{}
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
					for st := cur; ; {
						p, ok := prev[st]
						if !ok {
							break
						}
						path = append([]int{p.seg}, path...)
						st = p.s
					}
					found[id] = best{d, path}
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
				if id := shapeOf(s, g.pos[cur.k]); id != "" && id != from && cur.dir != (fpt{}) {
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
	shapes := map[string]bool{}
	for _, sh := range s.shapes {
		shapes[sh.id] = true
	}
	var froms []string
	for id := range starts {
		froms = append(froms, id)
	}
	sort.Strings(froms)
	traced := map[[2]string]bool{}
	for _, f := range froms {
		for to, b := range reach(f) {
			pair := [2]string{f, to}
			if !want[pair] {
				t.Phantom++
				continue
			}
			traced[pair] = true
			t.Bends = append(t.Bends, b.bends)
			t.Crossings = append(t.Crossings, pathCrossings(s, g, b.path))
		}
	}
	for r := range want {
		if !shapes[r[0]] || !shapes[r[1]] {
			continue
		}
		t.Relations++
		if !traced[r] {
			t.Lost++
		}
	}
	// A point where ink of one colour goes three or more ways is a split or
	// a merge, and needs its dot.
	var marked []fpt
	for k, arms := range g.arms {
		if len(arms) < 3 {
			continue
		}
		p := g.pos[k]
		if shapeOf(s, p) != "" {
			continue
		}
		dup := false
		for _, q := range marked {
			dup = dup || dist(p, q) < 4
		}
		if dup {
			continue
		}
		marked = append(marked, p)
		dot := false
		for _, j := range s.junctions {
			dot = dot || dist(p, j) < 3
		}
		if !dot {
			t.UnmarkedT++
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
		if len(ds) == 0 || ds[0] > countReach || len(ds) > 1 && ds[1]-ds[0] < countClear {
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

// relations are the pairs of places a drawing's lines stand for: a line
// between two modules that use each other stands for both directions.
func relations(d Drawing) [][2]string {
	var out [][2]string
	for _, w := range d.Wires {
		for _, e := range w.Ends {
			out = append(out, e)
			if w.Kind == "mutual" {
				out = append(out, [2]string{e[1], e[0]})
			}
		}
	}
	return out
}

func traced(t *testing.T, d Drawing) Trace {
	t.Helper()
	var b bytes.Buffer
	if err := page.ExecuteTemplate(&b, "drawing", d); err != nil {
		t.Fatal(err)
	}
	s, err := readSVG(b.String())
	if err != nil {
		t.Fatal(err)
	}
	return trace(s, relations(d))
}

// Every relation a drawing stands for can be traced along its ink. With
// ABBD_METRICS set to a file, the path measures of every view go there as a
// table.
func TestTraceEveryRealDrawing(t *testing.T) {
	var rows []string
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, v := range views(m) {
			got := traced(t, v.d)
			if got.Lost > 0 {
				t.Errorf("%s %s: %d relations no ink carries", name, v.name, got.Lost)
			}
			if got.UnmarkedT > 0 {
				t.Errorf("%s %s: rule 17: %d splits or merges without a dot", name, v.name, got.UnmarkedT)
			}
			if got.SizeSpread > maxSpread {
				t.Errorf("%s %s: rule 21: the tallest box is %.2f times the median, limit %.1f", name, v.name, got.SizeSpread, maxSpread)
			}
			t.Logf("%s %s: %s", name, v.name, got)
			rows = append(rows, fmt.Sprintf("%s\t%s\t%d\t%d\t%d\t%.1f\t%d\t%.1f\t%d\t%d\t%d\t%.2f\t%.3f", name, v.name, got.Relations, got.Lost, got.Phantom, median(got.Bends), maxOf(got.Bends), median(got.Crossings), maxOf(got.Crossings), got.UnmarkedT, got.CountAmbiguity, got.SizeSpread, got.Beside))
		}
	}
	if f := os.Getenv("ABBD_METRICS"); f != "" {
		head := "fixture\tview\trelations\tlost\tphantom\tbends_med\tbends_max\tcross_med\tcross_max\tunmarked_t\tcount_amb\tsize_spread\tbeside\n"
		if err := os.WriteFile(f, []byte(head+strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The hand-laid reference drawing of this repository's overview,
// eval/render-r1/reference-self.svg, lays out the same boxes and relations
// the way a person would in draw.io. The path measures must rate it at
// least as well as the renderer does, or they measure the wrong thing.
func TestReferenceRanksFirst(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "eval", "render-r1", "reference-self.svg"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := readSVG(string(b))
	if err != nil {
		t.Fatal(err)
	}
	m := fixture(t, "self")
	ref, got := trace(s, relations(m.Overview)), traced(t, m.Overview)
	t.Logf("reference: %s", ref)
	t.Logf("renderer:  %s", got)
	if ref.Lost > 0 || ref.Relations != got.Relations {
		t.Fatalf("the reference draws %d of %d relations, %d lost", ref.Relations, got.Relations, ref.Lost)
	}
	for _, c := range []struct {
		name     string
		ref, got float64
	}{
		{"phantom", float64(ref.Phantom), float64(got.Phantom)},
		{"bends median", median(ref.Bends), median(got.Bends)},
		{"bends max", float64(maxOf(ref.Bends)), float64(maxOf(got.Bends))},
		{"path crossings median", median(ref.Crossings), median(got.Crossings)},
		{"path crossings max", float64(maxOf(ref.Crossings)), float64(maxOf(got.Crossings))},
		{"unmarked T", float64(ref.UnmarkedT), float64(got.UnmarkedT)},
		{"count ambiguity", float64(ref.CountAmbiguity), float64(got.CountAmbiguity)},
		{"size spread", ref.SizeSpread, got.SizeSpread},
		{"beside", ref.Beside, got.Beside},
	} {
		if c.ref > c.got {
			t.Errorf("%s rates the renderer (%.2f) above the hand-laid reference (%.2f)", c.name, c.got, c.ref)
		}
	}
}
