package screen

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// inked is one drawn line as the page draws it: its pieces, how wide its
// stroke is, the holes its mask cuts, and the arrowheads it ends in.
type inked struct {
	kind, net  string
	width      float64
	parts      [][]fpt
	holes      []frect
	start, end bool
}

// lettered is a word drawn beside a line, and the line it belongs to.
type lettered struct {
	s     string
	at    fpt
	wire  int
	turns bool
}

// circled is a dot or an origin mark.
type circled struct {
	at   fpt
	r    float64
	kind string
}

// page is what one rendered drawing shows a reader of its lines and marks.
type rendered struct {
	w, h    float64
	lines   []inked
	words   []lettered
	circles []circled
	rects   []frect
	counts  []lettered
}

// strokeWidth is the width page.css draws a line of the given classes in.
func strokeWidth(class string) float64 {
	cs := " " + class + " "
	switch {
	case strings.Contains(cs, " back "):
		return 1.2
	case strings.Contains(cs, " bypass "), strings.Contains(cs, " mutual "):
		return 2.6
	case strings.Contains(cs, " provides "):
		return 2
	}
	return 1.5
}

func render(t *testing.T, d Drawing) rendered {
	t.Helper()
	var b bytes.Buffer
	if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
		t.Fatal(err)
	}
	var r rendered
	masks := map[string][]frect{}
	var mask string
	maskOf := map[int]string{}
	var word *lettered
	dec := xml.NewDecoder(strings.NewReader(b.String()))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			rect := func() frect {
				return frect{num(attr(e, "x")), num(attr(e, "y")), num(attr(e, "width")), num(attr(e, "height"))}
			}
			switch e.Name.Local {
			case "svg":
				r.w, r.h = num(attr(e, "width")), num(attr(e, "height"))
			case "mask":
				mask = attr(e, "id")
			case "rect":
				switch {
				case mask != "" && attr(e, "fill") != "#fff":
					masks[mask] = append(masks[mask], rect())
				case mask == "" && !hasClass(e, "frame"):
					r.rects = append(r.rects, rect())
				}
			case "path":
				if !hasClass(e, "w") {
					continue
				}
				parts, err := flatten(attr(e, "d"))
				if err != nil {
					t.Fatal(err)
				}
				cs := strings.Fields(attr(e, "class"))
				id := strings.TrimSuffix(strings.TrimPrefix(attr(e, "mask"), "url(#"), ")")
				maskOf[len(r.lines)] = id
				r.lines = append(r.lines, inked{kind: hue(cs[1]), net: attr(e, "data-net"), width: strokeWidth(attr(e, "class")), parts: parts,
					start: attr(e, "marker-start") != "", end: attr(e, "marker-end") != ""})
			case "circle":
				k := ""
				if cs := strings.Fields(attr(e, "class")); hasClass(e, "junction") && len(cs) > 1 {
					k = cs[1]
				}
				r.circles = append(r.circles, circled{fpt{num(attr(e, "cx")), num(attr(e, "cy"))}, num(attr(e, "r")), k})
			case "text":
				switch {
				case hasClass(e, "lab"):
					r.words = append(r.words, lettered{at: fpt{num(attr(e, "x")), num(attr(e, "y"))}, wire: len(r.lines) - 1, turns: attr(e, "transform") != ""})
					word = &r.words[len(r.words)-1]
				case hasClass(e, "count"):
					r.counts = append(r.counts, lettered{at: fpt{num(attr(e, "x")), num(attr(e, "y"))}})
					word = &r.counts[len(r.counts)-1]
				}
			}
		case xml.CharData:
			if word != nil {
				word.s += string(e)
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "mask":
				mask = ""
			case "text":
				word = nil
			}
		}
	}
	for i, id := range maskOf {
		r.lines[i].holes = masks[id]
	}
	return r
}

// piece is one straight stretch of a rendered line.
type piece2 struct {
	a, b fpt
	line int
	// free says each end is an end of its whole piece of ink, not a turn.
	freeA, freeB bool
}

func (p piece2) flat() bool { return p.a.y == p.b.y }

func (r rendered) pieces() (hs, vs []piece2) {
	for i, l := range r.lines {
		for _, part := range l.parts {
			for k := 1; k < len(part); k++ {
				p := piece2{part[k-1], part[k], i, k == 1, k == len(part)-1}
				switch {
				case p.a == p.b:
				case p.flat():
					hs = append(hs, p)
				case p.a.x == p.b.x:
					vs = append(vs, p)
				}
			}
		}
	}
	return hs, vs
}

func amid(v, a, b, room float64) bool { return min(a, b)+room <= v && v <= max(a, b)-room }

func covers(h frect, x0, x1, y0, y1 float64) bool {
	return h.x <= x0 && x1 <= h.x+h.w && h.y <= y0 && y1 <= h.y+h.h
}

// gapFaults are the ways a rendered drawing breaks rule 24, with how many
// holes it cuts and how many crossings of two lines' strokes it draws.
// Where a vertical stroke crosses a horizontal one of another line away
// from both their turns, and no dot of their one kind joins them, the
// horizontal stroke has a hole through its whole width that reaches 2px
// past the vertical stroke on each side. A crossing near the end of the
// horizontal line, where a cut would leave a sliver, and one whose hole
// would cut the horizontal line's own vertical ink, need none. Every hole
// sits on a crossing, and the holes leave no stretch of ink under gapKeep.
func (r rendered) gapFaults() (faults []string, holes, crossings int) {
	hs, vs := r.pieces()
	joined := map[fpt]map[string]bool{}
	for _, c := range r.circles {
		if c.kind != "" {
			if joined[c.at] == nil {
				joined[c.at] = map[string]bool{}
			}
			joined[c.at][c.kind] = true
		}
	}
	for _, h := range hs {
		under := r.lines[h.line]
		y := h.a.y
		for _, o := range vs {
			x := o.a.x
			if o.line == h.line || !amid(x, h.a.x, h.b.x, 0) || !amid(y, o.a.y, o.b.y, 0) || x == h.a.x || x == h.b.x || y == o.a.y || y == o.b.y {
				continue
			}
			crossings++
			over := r.lines[o.line]
			if under.kind == over.kind && joined[fpt{x, y}][under.kind] {
				continue
			}
			if !amid(x, h.a.x, h.b.x, gapEnd) || !amid(y, o.a.y, o.b.y, gapEnd) || h.freeA && math.Abs(x-h.a.x) < gapHalf+gapKeep || h.freeB && math.Abs(x-h.b.x) < gapHalf+gapKeep {
				continue
			}
			own := slices.ContainsFunc(vs, func(s piece2) bool {
				return s.line == h.line && math.Abs(s.a.x-x) <= gapHalf && amid(y, s.a.y, s.b.y, -gapHalf)
			})
			if own {
				continue
			}
			reach := over.width/2 + 2
			if !slices.ContainsFunc(under.holes, func(g frect) bool {
				return covers(g, x-reach, x+reach, y-under.width/2, y+under.width/2)
			}) {
				faults = append(faults, fmt.Sprintf("line %d crosses over line %d at (%v, %v) with no hole beneath", o.line, h.line, x, y))
			}
		}
	}
	for i, l := range r.lines {
		for _, g := range l.holes {
			holes++
			on := slices.ContainsFunc(hs, func(h piece2) bool {
				return h.line == i && h.a.y > g.y && h.a.y < g.y+g.h && slices.ContainsFunc(vs, func(o piece2) bool {
					return o.line != i && o.a.x > g.x && o.a.x < g.x+g.w && amid(h.a.y, o.a.y, o.b.y, 0)
				})
			})
			if !on {
				faults = append(faults, fmt.Sprintf("line %d has a hole at %v that sits on no crossing", i, g))
			}
		}
	}
	for _, h := range hs {
		lo, hi := min(h.a.x, h.b.x), max(h.a.x, h.b.x)
		var cut [][2]float64
		for _, g := range r.lines[h.line].holes {
			if g.y < h.a.y && h.a.y < g.y+g.h && g.x < hi && lo < g.x+g.w {
				cut = append(cut, [2]float64{g.x, g.x + g.w})
			}
		}
		if len(cut) == 0 {
			continue
		}
		slices.SortFunc(cut, func(a, b [2]float64) int { return int(a[0] - b[0]) })
		// The ink left at a turn runs on into the next stretch, so only a
		// line's own end can leave a sliver beside a hole.
		freeLo, freeHi := h.freeA, h.freeB
		if h.a.x > h.b.x {
			freeLo, freeHi = h.freeB, h.freeA
		}
		left, right := lo-gapKeep, hi+gapKeep
		if freeLo {
			left = lo
		}
		if freeHi {
			right = hi
		}
		edges := append(append([][2]float64{{left, left}}, cut...), [2]float64{right, right})
		for k := 1; k < len(edges); k++ {
			if gap := edges[k][0] - edges[k-1][1]; gap > 0 && gap < gapKeep {
				faults = append(faults, fmt.Sprintf("line %d keeps a %.0fpx sliver at y %v between x %v and %v", h.line, gap, h.a.y, edges[k-1][1], edges[k][0]))
			}
		}
	}
	return faults, holes, crossings
}

// A line that passes over another breaks the one beneath, and nowhere else
// (rule 24), in every drawing of every real state.
func TestEveryCrossingIsAGap(t *testing.T) {
	holes, crossings := 0, 0
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			faults, h, c := render(t, v.d).gapFaults()
			for _, f := range faults {
				t.Errorf("%s %s: %s", name, v.name, f)
			}
			holes += h
			crossings += c
		}
	}
	t.Logf("%d holes across the fixtures, at %d crossings of two lines' strokes", holes, crossings)
}

// A dot joins only ink of its own kind, so a line of another kind that
// crosses there still passes over and breaks the line beneath. Two
// crossings closer than gapJoin share one cut, and a crossing near a
// line's end is not cut (rule 24, rule 17).
func TestADotJoinsOnlyItsOwnKind(t *testing.T) {
	under := drawn{parts: [][]pt{{{0, 50}, {200, 50}}}}
	paths := []drawn{
		under,
		{parts: [][]pt{{{60, 0}, {60, 100}}}},
		{parts: [][]pt{{{140, 0}, {140, 100}}}},
		{parts: [][]pt{{{146, 0}, {146, 100}}}},
		{parts: [][]pt{{{195, 0}, {195, 100}}}},
	}
	d := Drawing{Width: 220, Height: 120, Dots: []Dot{{X: 60, Y: 50, Kind: "contract"}}}
	for i, p := range paths {
		kind := "depends_on"
		if i == 0 {
			kind = "contract"
		}
		d.Wires = append(d.Wires, Wire{D: polyline(p.parts[0]), Kind: kind, Hue: "0", Head: "dep-0"})
	}
	d.gaps(segsOf(paths))
	faults, holes, _ := render(t, d).gapFaults()
	for _, f := range faults {
		t.Error(f)
	}
	if want := []Gap{{60, 60, 50}, {140, 146, 50}}; !slices.Equal(d.Wires[0].Gaps, want) {
		t.Errorf("cuts %v, want %v", d.Wires[0].Gaps, want)
	}
	if holes != 2 {
		t.Errorf("%d holes, want 2", holes)
	}
}

// lineOf says which lines a label may name: its own, and every path of a
// bus it names whole.
func (r rendered) lineOf(w lettered, i int) bool {
	return i == w.wire || r.lines[w.wire].net != "" && r.lines[i].net == r.lines[w.wire].net
}

// Every label stands upright on the canvas beside a straight stretch of
// its own line, and keeps 2px off every box, chip, count, dot, origin mark,
// arrowhead, other label and other line (rule 25).
func TestLabelsReadAlongTheirLine(t *testing.T) {
	type tally struct{ rels, worded, shown, views, labelled, total, beside int }
	for _, name := range fixtures {
		m := fixture(t, name)
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(units)
		var n tally
		for _, v := range views(m) {
			d := v.d
			r := render(t, d)
			hs, vs := r.pieces()
			n.views++
			drawn, word, said := map[string]bool{}, map[string]bool{}, map[string]bool{}
			for _, w := range d.Wires {
				for _, rel := range w.Rels {
					drawn[rel] = true
					if via(tr, w.Rels) != "" {
						word[rel] = true
					}
					if w.Label != nil {
						said[rel] = true
					}
				}
			}
			n.rels += len(drawn)
			n.worded += len(word)
			n.shown += len(said)
			if len(r.words) > 0 {
				n.labelled++
			}
			var areas []frect
			for _, w := range r.words {
				tw := float64(textWidth(w.s, labelPx))
				areas = append(areas, frect{w.at.x - tw/2, w.at.y - labelH + 1, tw, labelH})
			}
			var marks []frect
			marks = append(marks, r.rects...)
			for _, c := range r.circles {
				marks = append(marks, frect{c.at.x - c.r, c.at.y - c.r, 2 * c.r, 2 * c.r})
			}
			for _, c := range r.counts {
				marks = append(marks, frect{c.at.x, c.at.y - 9, float64(textWidth(c.s, 11)), 11})
			}
			for _, l := range r.lines {
				head := func(tip, from fpt) {
					dx, dy := math.Copysign(1, tip.x-from.x), math.Copysign(1, tip.y-from.y)
					if tip.x == from.x {
						marks = append(marks, frect{tip.x - 4.5, min(tip.y, tip.y-9*dy), 9, 9})
					} else {
						marks = append(marks, frect{min(tip.x, tip.x-9*dx), tip.y - 4.5, 9, 9})
					}
				}
				if p := l.parts; l.end && len(p) > 0 && len(p[len(p)-1]) > 1 {
					last := p[len(p)-1]
					head(last[len(last)-1], last[len(last)-2])
				}
				if p := l.parts; l.start && len(p) > 0 && len(p[0]) > 1 {
					head(p[0][0], p[0][1])
				}
			}
			over := func(a, b frect, room float64) bool {
				return a.x < b.x+b.w+room && b.x-room < a.x+a.w && a.y < b.y+b.h+room && b.y-room < a.y+a.h
			}
			for k, w := range r.words {
				n.total++
				lb := areas[k]
				if w.turns {
					t.Errorf("%s %s: label %q is turned", name, v.name, w.s)
				}
				if lb.x < 0 || lb.y < 0 || lb.x+lb.w > r.w || lb.y+lb.h > r.h {
					t.Errorf("%s %s: label %q leaves the canvas", name, v.name, w.s)
				}
				for _, o := range marks {
					if over(lb, o, labelRoom) {
						t.Errorf("%s %s: label %q at %v is within 2px of a mark at %v", name, v.name, w.s, w.at, o)
					}
				}
				for j, o := range areas {
					if j != k && over(lb, o, labelRoom) {
						t.Errorf("%s %s: labels %q and %q touch", name, v.name, w.s, r.words[j].s)
					}
				}
				near, upright := false, false
				for _, s := range append(slices.Clone(hs), vs...) {
					sw := r.lines[s.line].width / 2
					seg := frect{min(s.a.x, s.b.x) - sw, min(s.a.y, s.b.y) - sw, math.Abs(s.a.x-s.b.x) + 2*sw, math.Abs(s.a.y-s.b.y) + 2*sw}
					if over(lb, seg, 1) {
						t.Errorf("%s %s: label %q on a line", name, v.name, w.s)
					}
					if !r.lineOf(w, s.line) {
						continue
					}
					if s.flat() && amid(lb.x, s.a.x, s.b.x, 0) && amid(lb.x+lb.w, s.a.x, s.b.x, 0) && (math.Abs(lb.y+lb.h-s.a.y) <= labelOff+1 || math.Abs(lb.y-s.a.y) <= labelOff+1) ||
						!s.flat() && amid(lb.y, s.a.y, s.b.y, 0) && amid(lb.y+lb.h, s.a.y, s.b.y, 0) && (math.Abs(lb.x+lb.w-s.a.x) <= labelOff+2 || math.Abs(lb.x-s.a.x) <= labelOff+2) {
						near = true
						upright = upright || !s.flat()
					}
				}
				if !near {
					t.Errorf("%s %s: label %q beside no stretch of its line", name, v.name, w.s)
				}
				if upright {
					n.beside++
				}
			}
		}
		t.Logf("%s: summed over views, %d drawn relations, %d on a line with one word, %d on a labelled line; %d labels, %d beside a vertical stretch; %d of %d views carry a label", name, n.rels, n.worded, n.shown, n.total, n.beside, n.labelled, n.views)
	}
}
