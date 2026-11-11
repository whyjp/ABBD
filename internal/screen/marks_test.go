package screen

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
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
				case mask == "" && !hasClass(e, "frame") && !hasClass(e, "band"):
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

// pstack's MOD-0010 at 486,206: a line drops onto a bus whose right arm
// a line of another kind cuts 8px on, leaving a 4px stub. Left a T, it
// reads as a corner and the bus beyond the gap starts from nothing, so
// the T keeps its dot, and J2 reports it without one (rule 17).
func TestATeeCutCloseByKeepsItsDot(t *testing.T) {
	paths := []drawn{
		{kind: "depends_on", parts: [][]pt{{{486, 78}, {486, 206}}}},
		{kind: "depends_on", parts: [][]pt{{{486, 206}, {412, 206}}}},
		{kind: "depends_on", parts: [][]pt{{{486, 206}, {559, 206}}}},
		{kind: "contract", parts: [][]pt{{{494, 150}, {494, 260}}}},
	}
	d := Drawing{Width: 600, Height: 300}
	for _, p := range paths {
		d.Wires = append(d.Wires, Wire{D: polyline(p.parts[0]), Kind: p.kind, Hue: "0", Head: "dep-0"})
	}
	d.gaps(segsOf(paths))
	if want := []Gap{{494, 494, 206}}; !slices.Equal(d.Wires[2].Gaps, want) {
		t.Fatalf("cuts %v, want %v", d.Wires[2].Gaps, want)
	}
	j2 := func(d Drawing) []Breach {
		s, err := sheetOf(d)
		if err != nil {
			t.Fatal(err)
		}
		return newLook(view{name: "sketch", d: d}, s).dotted()
	}
	if got := j2(d); len(got) != 1 || !strings.Contains(got[0].Text, "T at 486,206") {
		t.Errorf("J2 on the undotted T: %+v; want one breach at 486,206", got)
	}
	d.dotCutTees(paths)
	if want := []Dot{{X: 486, Y: 206, Kind: hue("depends_on")}}; !slices.Equal(d.Dots, want) {
		t.Errorf("dots %+v, want %+v", d.Dots, want)
	}
	if got := j2(d); len(got) != 0 {
		t.Errorf("J2 on the dotted T: %+v; want none", got)
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
//
// With ABBD_LABEL_ROOT set to a project root, it reads that project's
// .abbd too, such as this repository's own, whose couplings carry phrases.
func TestLabelsReadAlongTheirLine(t *testing.T) {
	type tally struct{ rels, worded, shown, phrased, named, views, labelled, total, beside int }
	roots := map[string]string{}
	names := slices.Clone(fixtures)
	for _, name := range fixtures {
		roots[name] = filepath.Join("testdata", name)
	}
	if root := os.Getenv("ABBD_LABEL_ROOT"); root != "" {
		roots["root"], names = root, append(names, "root")
	}
	for _, name := range names {
		units, err := store.Open(roots[name]).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		m := Build(Input{Units: units})
		tr := newTree(units)
		var n tally
		for _, v := range views(m) {
			d := v.d
			r := render(t, d)
			hs, vs := r.pieces()
			n.views++
			drawn, worded, shown, phrased, named := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
			for _, w := range d.Wires {
				for _, rel := range w.Rels {
					drawn[rel] = true
					if s, _ := word(tr, w.lines); s != "" {
						worded[rel] = true
					}
					if w.Label != nil {
						shown[rel] = true
					}
					if tr.phrases[rel].text != "" {
						phrased[rel] = true
						if w.Label != nil && w.Label.Said && phraseNames(tr.phrases[rel].text)[0] == w.Label.S {
							named[rel] = true
						}
					}
				}
			}
			n.rels += len(drawn)
			n.worded += len(worded)
			n.shown += len(shown)
			n.phrased += len(phrased)
			n.named += len(named)
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
						if s.flat() && d.Wires[s.line].Hue == d.Wires[w.wire].Hue && lb.x < max(s.a.x, s.b.x) && min(s.a.x, s.b.x) < lb.x+lb.w &&
							(math.Abs(lb.y+lb.h-s.a.y) <= labelOff+2 || math.Abs(lb.y-s.a.y) <= labelOff+2) {
							t.Errorf("%s %s: label %q stands over a track of another line of its colour, and reads as that line's", name, v.name, w.s)
						}
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
		t.Logf("%s: summed over views, %d drawn relations, %d on a line with one word, %d on a labelled line; %d with a phrase, %d on a line its phrase labels; %d labels, %d beside a vertical stretch; %d of %d views carry a label", name, n.rels, n.worded, n.shown, n.phrased, n.named, n.total, n.beside, n.labelled, n.views)
		labels := 0
		for _, w := range m.Overview.Wires {
			if w.Label != nil {
				labels++
			}
		}
		t.Logf("%s overview: %d labels on %d paths", name, labels, len(m.Overview.Wires))
	}
}

// inkTouches says, of two wires of d, whether a straight run of one ends
// on a run of the other, read off the paths as drawn.
func inkTouches(d Drawing) func(i, j int) bool {
	var rs [][2]pt
	var of []int
	for i, w := range d.Wires {
		for _, r := range runs(w.D) {
			rs, of = append(rs, r), append(of, i)
		}
	}
	n := len(d.Wires)
	touch := make([]bool, n*n)
	for x, a := range rs {
		for y, b := range rs {
			if of[x] != of[y] && (on(a[0], b[0], b[1]) || on(a[1], b[0], b[1])) {
				touch[of[x]*n+of[y]], touch[of[y]*n+of[x]] = true, true
			}
		}
	}
	return func(i, j int) bool { return touch[i*n+j] }
}

// readsFor says a word on wire b reads for wire a too: paint gives both
// one colour.
func readsFor(a, b Wire) bool {
	return a.Hue == b.Hue
}

// oneSource says wires a and b each leave one place, the same.
func oneSource(a, b Wire) bool {
	var one model.ID
	for _, n := range append(slices.Clone(a.srcs), b.srcs...) {
		if b := base(model.ID(n)); one == "" {
			one = b
		} else if b != one {
			return false
		}
	}
	return one != ""
}

// A word is said once where lines whose ink touches carry it: no two labels
// of one word stand within repeatNear on lines that leave one place and
// whose ink touches, as the five 호출 on one fan
// of self's FEAT-0001 did.
func TestAWordIsSaidOnceOnInkThatTouches(t *testing.T) {
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			d := v.d
			touches := inkTouches(d)
			for i, a := range d.Wires {
				for j, b := range d.Wires[:i] {
					if a.Label == nil || b.Label == nil || a.Label.S != b.Label.S || !touches(i, j) {
						continue
					}
					dx, dy := a.Label.X-b.Label.X, a.Label.Y-b.Label.Y
					if dx*dx+dy*dy <= repeatNear*repeatNear && oneSource(a, b) {
						t.Errorf("%s %s: %q said at %d,%d and again at %d,%d on touching ink", name, v.name, a.Label.S, b.Label.X, b.Label.Y, a.Label.X, a.Label.Y)
					}
				}
			}
		}
	}
}

// A line with room for its word loses it only to a line whose ink touches
// its own and says the word within repeatNear, and not to one it reaches
// only through other lines. Room is where the word stands when no word is
// dropped.
func TestAWordIsDroppedOnlyBesideALineThatSaysIt(t *testing.T) {
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		sayOnce = false
		all := views(Build(Input{Units: units}))
		sayOnce = true
		for k, v := range views(fixture(t, name)) {
			d, room := v.d, all[k].d
			touches := inkTouches(d)
			for i, w := range d.Wires {
				r := room.Wires[i].Label
				if w.Label != nil || r == nil {
					continue
				}
				said := false
				for j, o := range d.Wires {
					if o.Label != nil && o.Label.S == r.S && touches(i, j) && readsFor(w, o) {
						dx, dy := o.Label.X-r.X, o.Label.Y-r.Y
						said = said || dx*dx+dy*dy <= repeatNear*repeatNear
					}
				}
				if !said {
					t.Errorf("%s %s: wire %d lost %q at %d,%d with no touching line saying it nearby", name, v.name, i, r.S, r.X, r.Y)
				}
			}
		}
	}
}

// The green line from internal/screen into store staleness hashes on self's
// MOD-0041 keeps its 호출 though the cyan line from the abbd CLI it touches
// says 호출 beside it: the two wear different colours.
func TestAWordStaysBesideALineOfAnotherColour(t *testing.T) {
	for _, v := range views(fixture(t, "self")) {
		if v.name != "MOD-0041 internal/store" {
			continue
		}
		at := slices.IndexFunc(v.d.Wires, func(w Wire) bool { return w.D == "M556 98 L710 98" })
		if at < 0 {
			t.Fatal("MOD-0041 draws no line M556 98 L710 98")
		}
		if l := v.d.Wires[at].Label; l == nil || l.S != "호출" {
			t.Errorf("the line from internal/screen on MOD-0041 says %v, want 호출", l)
		}
		return
	}
	t.Fatal("self draws no MOD-0041 internal/store")
}
