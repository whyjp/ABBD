package screen

import (
	"cmp"
	"fmt"
	"hash/fnv"
	"html/template"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Marks are what the page adds to routed ink so a reader can read it: a gap
// where one line passes over another (rule 24), and a word along a line that
// says how its relations arise (rule 25).

const (
	// gapHalf is how far a gap reaches each side of the line that passes
	// over it, the over line's stroke included, and gapEnd how far a
	// crossing must stand from a turn of either piece to be gapped: closer
	// in, it reads as a line that ends on the other. gapKeep is the least
	// ink a cut leaves, so no sliver reads as a stray mark. A crossing
	// keeps gapHalf+gapKeep from the end of a line, and two crossings on
	// one stretch closer than gapJoin share one cut.
	gapHalf, gapEnd, gapKeep = 4, 6, 4
	gapJoin                  = 2*gapHalf + gapKeep
)

// Gap is where a horizontal stretch of a line is cut, at height Y, because
// vertical lines cross over it from X0 to X1.
type Gap struct{ X0, X1, Y int }

// Left and Width are the cut's extent along the line.
func (g Gap) Left() int  { return g.X0 - gapHalf }
func (g Gap) Width() int { return g.X1 - g.X0 + 2*gapHalf }

// seg is one straight stretch of a wire's ink. aEnd and bEnd say a or b
// is where a piece of the line ends, not where it turns.
type seg struct {
	a, b       pt
	wire       int
	aEnd, bEnd bool
}

// room is how far a crossing must stand from the stretch's low end and
// from its high end to be gapped.
func (s seg) room() (int, int) {
	r := [2]int{gapEnd, gapEnd}
	for i, e := range [2]bool{s.aEnd, s.bEnd} {
		if e {
			r[i] = gapHalf + gapKeep
		}
	}
	if s.a.x+s.a.y > s.b.x+s.b.y {
		return r[1], r[0]
	}
	return r[0], r[1]
}

func (s seg) flat() bool { return s.a.y == s.b.y }

// lo and hi are the stretch's extent along its own direction.
func (s seg) lo() int {
	if s.flat() {
		return min(s.a.x, s.b.x)
	}
	return min(s.a.y, s.b.y)
}

func (s seg) hi() int {
	if s.flat() {
		return max(s.a.x, s.b.x)
	}
	return max(s.a.y, s.b.y)
}

func (s seg) bounds() box {
	return box{min(s.a.x, s.b.x), min(s.a.y, s.b.y), abs(s.a.x - s.b.x), abs(s.a.y - s.b.y)}
}

func segsOf(paths []drawn) []seg {
	var out []seg
	for i, p := range paths {
		for _, part := range p.parts {
			part = straight(part)
			for k := 1; k < len(part); k++ {
				if part[k-1] != part[k] {
					out = append(out, seg{part[k-1], part[k], i, k == 1, k == len(part)-1})
				}
			}
		}
	}
	return out
}

// mark adds the marks to d, whose wires were built from paths in order:
// the gaps, the labels, and the places each line leaves, which paint
// colours it by.
func (d *Drawing) mark(paths []drawn, reqs []wireReq, t tree) {
	for i, p := range paths {
		for _, k := range p.reqs {
			e := reqs[k].from
			if e.node == "" {
				e = reqs[k].to
			}
			d.Wires[i].srcs = append(d.Wires[i].srcs, e.node)
		}
	}
	segs := segsOf(paths)
	d.gaps(segs)
	d.labels(segs, heads(paths), t)
}

// shapes are the arrowhead shapes that say a line's kind: a filled head for
// a use through a contract, an open one for a direct dependency, and a
// hollow one for a unit that provides a contract.
var shapes = map[string]string{
	"contract": "use", "consumes": "use",
	"depends_on": "dep", "bypass": "dep", "mutual": "dep",
	"provides": "impl",
}

// warns says a kind is a direct dependency that skips the contract
// boundary the overview shows. Its line wears the colour of the box it
// leaves, like any other, and its open head marks it.
func warns(kind string) bool { return kind == "bypass" || kind == "mutual" }

// paint colours each box by its unit, each line by the boxes its lines
// leave, and each dot by the lines through it (rule 11). A line that
// leaves a frame's chip takes the colour of the box it reaches. Ink whose
// lines leave boxes of different colours is neutral, a line that skips the
// contract boundary included.
func (d *Drawing) paint(hues map[model.ID]int) {
	colour := func(node string) string {
		if h, ok := hues[base(model.ID(node))]; ok && node != "" {
			return hueKey(h)
		}
		return hueKey(-1)
	}
	for i, b := range d.Boxes {
		if coloured(b) {
			d.Boxes[i].Hue = colour(b.ID)
		}
	}
	used := map[string]bool{}
	for i := range d.Wires {
		w := &d.Wires[i]
		w.Hue = ""
		for _, n := range w.srcs {
			if c := colour(n); w.Hue == "" {
				w.Hue = c
			} else if c != w.Hue {
				w.Hue = hueKey(-1)
			}
		}
		if w.Hue == "" {
			w.Hue = hueKey(-1)
		}
		if !w.Joins {
			w.Head = shapes[w.Kind] + "-" + w.Hue
			used[w.Head] = true
		}
	}
	boxHue := map[int]string{}
	for _, b := range d.Boxes {
		if j, ok := d.numbers[string(base(model.ID(b.ID)))]; ok && b.Hue != "" {
			boxHue[j] = b.Hue
		}
	}
	for i := range d.Routes {
		r := &d.Routes[i]
		r.Hue = colour(r.src)
		// A pointed-at box lights the line in the box's colour, whatever
		// its kind; a chip or a box of no colour leaves it its own.
		var vars []string
		for _, j := range r.at {
			h, ok := boxHue[j]
			if !ok {
				h = r.Hue
			}
			vars = append(vars, fmt.Sprintf("--k%d:var(--h%s)", j, h))
		}
		r.Vars = template.CSS(strings.Join(vars, ";"))
	}
	d.Heads = slices.Sorted(maps.Keys(used))
	for i, dot := range d.Dots {
		p, got := pt{dot.X, dot.Y}, ""
		for _, w := range d.Wires {
			if w.inkKey() != dot.Kind || !through(w.D, p) {
				continue
			}
			if got == "" {
				got = w.Hue
			} else if got != w.Hue {
				got = hueKey(-1)
			}
		}
		if got == "" {
			got = hueKey(-1)
		}
		d.Dots[i].Hue = got
	}
}

// gaps cuts every horizontal piece where a vertical piece of another line
// crosses it away from both pieces' ends, unless a dot joins them there: a
// dot of the kind both lines are. The horizontal line is always the one
// broken, so the choice never depends on the order the lines are drawn in.
// Crossings on one piece closer than gapJoin share one cut.
func (d *Drawing) gaps(segs []seg) {
	dot := map[string]bool{}
	for _, j := range d.Dots {
		dot[fmt.Sprint(j.Kind, pt{j.X, j.Y})] = true
	}
	var hs, vs []seg
	for _, s := range segs {
		if s.flat() {
			hs = append(hs, s)
		} else {
			vs = append(vs, s)
		}
	}
	inside := func(v, lo, hi, room int) bool { return lo+room <= v && v <= hi-room }
	for _, h := range hs {
		var xs []int
		y, hk := h.a.y, d.Wires[h.wire].inkKey()
		for _, v := range vs {
			x := v.a.x
			lo, hi := h.room()
			if v.wire == h.wire || hk == d.Wires[v.wire].inkKey() && dot[fmt.Sprint(hk, pt{x, y})] || x < h.lo()+lo || x > h.hi()-hi || !inside(y, v.lo(), v.hi(), gapEnd) {
				continue
			}
			// The hole would cut the horizontal line's own vertical ink.
			own := slices.ContainsFunc(vs, func(o seg) bool {
				return o.wire == h.wire && abs(o.a.x-x) <= gapHalf && inside(y, o.lo(), o.hi(), -gapHalf)
			})
			if !own {
				xs = append(xs, x)
			}
		}
		slices.Sort(xs)
		w := &d.Wires[h.wire]
		for k, x := range xs {
			if k > 0 && x-xs[k-1] < gapJoin {
				w.Gaps[len(w.Gaps)-1].X1 = x
				continue
			}
			w.Gaps = append(w.Gaps, Gap{x, x, y})
		}
	}
	for i := range d.Wires {
		w := &d.Wires[i]
		if len(w.Gaps) == 0 {
			continue
		}
		slices.SortFunc(w.Gaps, func(a, b Gap) int {
			if a.Y != b.Y {
				return a.Y - b.Y
			}
			return a.X0 - b.X0
		})
		w.Gaps = slices.Compact(w.Gaps)
		// A mask's id names what it cuts: its line and its holes on a canvas
		// of its size. So a change to one line renames only that line's mask,
		// and two masks on one page share an id only when they are the same.
		f := fnv.New64a()
		fmt.Fprint(f, d.Width, d.Height, w.D, w.Gaps)
		w.Mask = fmt.Sprintf("x%x", f.Sum64())
	}
}

// heads are where the arrowheads stand. A marker sits at the last point of
// a whole path, and at its first too for a line that reads both ways, not
// at each piece's ends. Its tip is on the point and it reaches 9px back
// along the line, 5px to each side.
func heads(paths []drawn) []box {
	var out []box
	at := func(tip, from pt) {
		dx, dy := sign(tip.x-from.x), sign(tip.y-from.y)
		back := pt{tip.x - 9*dx, tip.y - 9*dy}
		b := box{min(tip.x, back.x), min(tip.y, back.y), abs(tip.x - back.x), abs(tip.y - back.y)}
		if dx == 0 {
			b.x, b.w = b.x-5, 10
		} else {
			b.y, b.h = b.y-5, 10
		}
		out = append(out, b)
	}
	for _, p := range paths {
		ps := p.parts
		if p.joins || len(ps) == 0 {
			continue
		}
		if last := straight(ps[len(ps)-1]); len(last) > 1 {
			at(last[len(last)-1], last[len(last)-2])
		}
		if first := straight(ps[0]); len(first) > 1 && p.kind == "mutual" && p.starts[0] != noStart {
			at(first[0], first[1])
		}
	}
	return out
}

const (
	// labelPx is a label's font size, and labelH the height its glyphs
	// take, from 9px above its baseline to 1px below. labelPad is how far
	// a label keeps from its stretch's ends, where a turn stands, labelOff
	// how far it stands off its line, and labelRoom how far it keeps from
	// every other mark.
	labelPx, labelH, labelPad, labelOff, labelRoom = 11, 10, 10, 3, 2
	// labelMax is the widest a label may be, about 21 letters with 등.
	labelMax = 170
	// most is the mark after a phrase that names at least half of a line's
	// relations but not all, or that keeps only its leading names.
	most = " 등"
)

// Label is a word set upright beside one straight stretch of a line,
// centred on X with its baseline at Y.
type Label struct {
	S    string
	X, Y int
	// Said marks a phrase, which is set in italic so it reads apart from a
	// via word.
	Said bool `json:",omitempty"`
}

// area is where a label's glyphs stand. They are centred on X, so a word of
// odd width reaches half a pixel past X - tw/2, and the box takes that pixel.
func (l Label) area() box {
	half := (textWidth(l.S, labelPx) + 1) / 2
	return box{l.X - half, l.Y - labelH + 1, 2 * half, labelH}
}

// units are what a line's label counts, given the relations of each line it
// speaks for: the lines when there are several, so that its count agrees
// with the bus's ×N, else the one line's relations.
func units(lines [][]string) [][]string {
	if len(lines) != 1 {
		return lines
	}
	out := make([][]string, len(lines[0]))
	for i, r := range lines[0] {
		out[i] = []string{r}
	}
	return out
}

// carried counts, for each key that key gives a relation, the units with a
// relation of that key.
func carried(us [][]string, key func(string) string) map[string]int {
	n := map[string]int{}
	for _, u := range us {
		seen := map[string]bool{}
		for _, r := range u {
			if k := key(r); k != "" && !seen[k] {
				seen[k] = true
				n[k]++
			}
		}
	}
	return n
}

// phraseNames splits a phrase at its commas, but not at those inside brackets,
// so f(a, b) stays one name.
func phraseNames(p string) []string {
	var out []string
	depth, from := 0, 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 && strings.HasPrefix(p[i:], ", ") {
				out, from = append(out, p[from:i]), i+2
			}
		}
	}
	return append(out, p[from:])
}

// word is what a wire's label says about the lines it speaks for (units),
// and whether it is a phrase. It names the phrase the most units carry, the
// first by name among equals: alone when every relation carries it, with 등
// when at least half the units do, and otherwise by its first name with 외
// and how many units do not carry it, so a phrase that few lines carry
// never passes for the bus, and the lines that carry it and N add up to
// the bus's ×N. A phrase too wide for labelMax keeps the names before its
// commas that fit, with 등. When it still does not fit the next phrase
// speaks, but only one at least half the units carry; else the via word
// all the relations share, else "".
func word(t tree, lines [][]string) (string, bool) {
	us := units(lines)
	n := carried(us, func(r string) string { return t.phrases[r].text })
	for i, p := range slices.SortedFunc(maps.Keys(n), func(a, b string) int { return cmp.Or(n[b]-n[a], strings.Compare(a, b)) }) {
		c := n[p]
		if i > 0 && 2*c < len(us) {
			break
		}
		ns := phraseNames(p)
		tries := []string{fmt.Sprintf("%s 외 %d", ns[0], len(us)-c)}
		if 2*c >= len(us) {
			rest := most
			if every(lines, func(r string) bool { return t.phrases[r].text == p }) {
				rest = ""
			}
			tries = []string{p + rest}
			for k := len(ns) - 1; k > 0; k-- {
				tries = append(tries, strings.Join(ns[:k], ", ")+most)
			}
		}
		for _, s := range tries {
			if textWidth(s, labelPx) <= labelMax {
				return s, true
			}
		}
	}
	via := ""
	for _, line := range lines {
		for _, r := range line {
			w := t.vias[r].Word()
			if w == "" || via != "" && w != via {
				return "", false
			}
			via = w
		}
	}
	return via, false
}

func every(lines [][]string, f func(string) bool) bool {
	return !slices.ContainsFunc(lines, func(l []string) bool { return slices.ContainsFunc(l, func(r string) bool { return !f(r) }) })
}

// said lists the phrases of the lines a wire's label speaks for, each once
// with whose words it is and how many of its units (see units) carry it,
// for its tooltip.
func said(t tree, lines [][]string) []string {
	us := units(lines)
	n := carried(us, func(r string) string {
		if p := t.phrases[r]; p.text != "" {
			return fmt.Sprintf("%s (%s)", p.text, labels[Badge(p.origin)])
		}
		return ""
	})
	out := make([]string, 0, len(n))
	for s, c := range n {
		out = append(out, fmt.Sprintf("%s %d/%d", s, c, len(us)))
	}
	slices.Sort(out)
	return out
}

func hit(a, b box) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

func grow(b box, n int) box { return box{b.x - n, b.y - n, b.w + 2*n, b.h + 2*n} }

// marked is everything a label keeps off besides lines: boxes, chips, the
// frame, counts, dots, origin marks and arrowheads.
func (d *Drawing) marked(heads []box) []box {
	var out []box
	for _, b := range d.Boxes {
		out = append(out, box{b.X, b.Y, b.W, b.H})
	}
	for _, p := range d.Ports {
		out = append(out, box{p.X, p.RectY(), p.W, p.H})
	}
	if f := d.Frame; f != nil {
		out = append(out, box{f.X, f.Y, f.W, 0}, box{f.X, f.Y + f.H, f.W, 0}, box{f.X, f.Y, 0, f.H}, box{f.X + f.W, f.Y, 0, f.H})
	}
	for _, w := range d.Wires {
		if w.Count > 1 {
			out = append(out, box{w.LX, w.LY - 9, textWidth(fmt.Sprintf("×%d", w.Count), 11) + 2, 11})
		}
		for _, m := range w.Marks {
			out = append(out, box{m.X - 4, m.Y - 4, 8, 8})
		}
	}
	for _, j := range d.Dots {
		out = append(out, box{j.X - 3, j.Y - 3, 6, 6})
	}
	return append(out, heads...)
}

// labels sets each line's word (see word) upright beside a straight stretch of its own
// ink long enough to hold it, on the canvas and clear of every other mark
// and line. A horizontal stretch holds the word above or below it, a
// vertical one beside it. Horizontal stretches are tried first, nearest the
// line's start first. A bus whose lines all say the same word is labelled
// once, anywhere along it. Otherwise a line is labelled only where its ink
// is its own, such as the drop into its box, since a word on a shared
// trunk would name lines it does not stand for. A line with no room keeps
// its word in its tooltip only.
func (d *Drawing) labels(segs []seg, heads []box, t tree) {
	taken := d.marked(heads)
	canvas := box{0, 0, d.Width, d.Height}
	words, saids := make([]string, len(d.Wires)), make([]bool, len(d.Wires))
	for i, w := range d.Wires {
		words[i], saids[i] = word(t, w.lines)
		if ps := said(t, w.lines); len(ps) > 0 {
			d.Wires[i].Title += "\n건너는 것: " + strings.Join(ps, ", ")
		}
	}
	// whole says every path of a net says one word, so the net is labelled
	// as one line.
	whole := map[int]bool{}
	for i, w := range d.Wires {
		if w.Net == 0 {
			continue
		}
		if _, seen := whole[w.Net]; !seen {
			whole[w.Net] = true
		}
		whole[w.Net] = whole[w.Net] && words[i] == words[slices.IndexFunc(d.Wires, func(o Wire) bool { return o.Net == w.Net })]
	}
	mate := func(i, j int) bool {
		n := d.Wires[i].Net
		return i == j || n != 0 && whole[n] && d.Wires[j].Net == n
	}
	// ink is the colour paint will give a wire, as far as labels need it:
	// its one source's, or neutral for several.
	ink := func(w Wire) string {
		var one model.ID
		for _, n := range w.srcs {
			if b := base(model.ID(n)); one == "" {
				one = b
			} else if b != one {
				return ""
			}
		}
		return string(one)
	}
	kin := func(i, j int) bool { return ink(d.Wires[i]) == ink(d.Wires[j]) }
	done := map[int]bool{}
	for i := range d.Wires {
		s, n := words[i], d.Wires[i].Net
		if s == "" || n != 0 && whole[n] && done[n] {
			continue
		}
		tw := textWidth(s, labelPx)
		said := saids[i]
		var mine []seg
		for _, g := range segs {
			if mate(i, g.wire) {
				mine = append(mine, g)
			}
		}
		// Horizontal stretches come first, and among them the one nearest
		// where the line leaves, as the stretches run in its order.
		slices.SortStableFunc(mine, func(a, b seg) int {
			if a.flat() == b.flat() {
				return 0
			}
			if a.flat() {
				return -1
			}
			return 1
		})
		// clear says the label at l, set beside g between lo and hi, stands
		// on the canvas and keeps off every other mark and line, and no
		// other line's ink runs along g there.
		clear := func(g seg, l Label, lo, hi int) bool {
			lb := l.area()
			if lb.x < 0 || lb.y < 0 || lb.x+lb.w > canvas.w || lb.y+lb.h > canvas.h {
				return false
			}
			for _, o := range taken {
				if hit(lb, grow(o, labelRoom)) {
					return false
				}
			}
			for _, o := range segs {
				if o == g {
					continue
				}
				// A line's stroke reaches 1.3px off its centre at most. Another
				// line parallel to g keeps more than twice the label's offset
				// from it, so the label reads as g's and not the other's. So
				// does a flat line of g's colour: a word beside a drop right
				// over such a track reads as the track's.
				room := labelRoom + 1
				if (o.flat() == g.flat() || o.flat() && kin(i, o.wire)) && !mate(i, o.wire) {
					room = 2*labelOff + 2
				}
				if hit(lb, grow(o.bounds(), room)) {
					return false
				}
				along := o.flat() == g.flat() && (g.flat() && o.a.y == g.a.y || !g.flat() && o.a.x == g.a.x)
				if along && !mate(i, o.wire) && max(o.lo(), lo) < min(o.hi(), hi) {
					return false
				}
			}
			return true
		}
		var got *Label
	search:
		for _, g := range mine {
			half := tw / 2
			if !g.flat() {
				half = labelH / 2
			}
			lo, hi := g.lo()+labelPad+half, g.hi()-labelPad-half
			if lo > hi {
				continue
			}
			mid := (lo + hi) / 2
			for off := 0; mid-off >= lo || mid+off <= hi; off += 4 {
				for _, c := range []int{mid - off, mid + off} {
					if c < lo || c > hi {
						continue
					}
					var opts []Label
					if g.flat() {
						y := g.a.y
						opts = []Label{{s, c, y - labelOff - 1, said}, {s, c, y + labelOff + labelH - 1, said}}
					} else {
						x, y := g.a.x, c+labelH/2-1
						opts = []Label{{s, x + labelOff + tw/2 + 1, y, said}, {s, x - labelOff - tw/2 - 1, y, said}}
					}
					for _, l := range opts {
						if clear(g, l, c-half, c+half) {
							got = &l
							break search
						}
					}
				}
			}
		}
		if got == nil {
			continue
		}
		d.Wires[i].Label = got
		taken = append(taken, got.area())
		if n != 0 && whole[n] {
			done[n] = true
		}
	}
}
