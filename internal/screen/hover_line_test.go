package screen

import (
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A pointed-at line is what a reader follows from one shape to the other,
// so what the pointer lights is each line whole: one unbroken stroke whose
// two ends touch the boxes or chips it joins, above every other line. These
// tests read the drawing as the page draws it and point at every piece of
// ink and every shape.

// drawnRoute is a route as the page draws it: its classes and colours, its
// way on the ink.
type drawnRoute struct {
	class, hue string
	vars       map[string]string
	way        drawnWay
}

// drawnWay is a polyline and where its arrowheads stand.
type drawnWay struct {
	pts  []fpt
	tips []drawnTip
}

type drawnTip struct {
	shape string
	at    fpt
	deg   float64
}

type drawnInk struct {
	class string
	parts [][]fpt
}

type drawnPage struct {
	shapes []shape
	hues   map[string]string
	ink    []drawnInk
	routes []drawnRoute
	// heads are the colours each arrowhead shape paints in.
	heads map[string]string
}

func classIs(class, c string) bool { return strings.Contains(" "+class+" ", " "+c+" ") }

// numbered are the numbers of a class list's classes that start with p.
func numbered(class, p string) []string {
	var out []string
	for _, c := range strings.Fields(class) {
		if len(c) > len(p) && strings.HasPrefix(c, p) && c[len(p)] >= '0' && c[len(p)] <= '9' {
			out = append(out, c[len(p):])
		}
	}
	return out
}

var tipAt = regexp.MustCompile(`^translate\(([-\d.]+) ([-\d.]+)\) rotate\(([-\d.]+)\)$`)

func readDrawn(t *testing.T, d Drawing) drawnPage {
	t.Helper()
	var b strings.Builder
	if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(strings.NewReader(b.String()))
	out := drawnPage{hues: map[string]string{}, heads: map[string]string{}}
	var group *shape
	var route *drawnRoute
	var way *drawnWay
	head := ""
	var stack []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.EndElement:
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if e.Name.Local != "g" {
				continue
			}
			switch {
			case top == "shape":
				out.shapes = append(out.shapes, *group)
				group = nil
			case top == "route":
				out.routes = append(out.routes, *route)
				route = nil
			case top == "head":
				head = ""
			}
		case xml.StartElement:
			kind := e.Name.Local
			class := attr(e, "class")
			switch e.Name.Local {
			case "g":
				switch {
				case classIs(class, "box") || classIs(class, "port"):
					kind = "shape"
					group = &shape{kind: strings.Fields(class)[0], class: class, id: attr(e, "data-id")}
					out.hues[class] = attr(e, "data-h")
				case classIs(class, "route"):
					kind = "route"
					route = &drawnRoute{class: class, hue: attr(e, "data-h"), vars: map[string]string{}}
					for _, v := range strings.Split(attr(e, "style"), ";") {
						if k, val, ok := strings.Cut(v, ":"); ok {
							route.vars[k] = val
						}
					}
					way = &route.way
				case strings.HasPrefix(attr(e, "id"), "t-"):
					kind = "head"
					head = strings.TrimPrefix(attr(e, "id"), "t-")
				}
			case "rect":
				if group != nil && group.r.w == 0 {
					group.r = frect{num(attr(e, "x")), num(attr(e, "y")), num(attr(e, "width")), num(attr(e, "height"))}
				}
			case "path":
				switch {
				case head != "":
					out.heads[head] = attr(e, "style")
				case classIs(class, "w"):
					parts, err := flatten(attr(e, "d"))
					if err != nil {
						t.Fatal(err)
					}
					out.ink = append(out.ink, drawnInk{class: class, parts: parts})
				case classIs(class, "way"):
					parts, err := flatten(attr(e, "d"))
					if err != nil || len(parts) != 1 {
						t.Fatalf("a route draws %q", attr(e, "d"))
					}
					way.pts = parts[0]
				}
			case "use":
				if route == nil {
					break
				}
				m := tipAt.FindStringSubmatch(attr(e, "transform"))
				if m == nil {
					t.Fatalf("an arrowhead stands at %q", attr(e, "transform"))
				}
				way.tips = append(way.tips, drawnTip{strings.TrimPrefix(attr(e, "href"), "#t-"), fpt{num(m[1]), num(m[2])}, num(m[3])})
			}
			stack = append(stack, kind)
		}
	}
	return out
}

func fpieces(pts []fpt) [][2]fpt {
	var out [][2]fpt
	for i := 1; i < len(pts); i++ {
		out = append(out, [2]fpt{pts[i-1], pts[i]})
	}
	return out
}

// along says some piece of a runs along some piece of b on one track for
// a length.
func along(a, b [][2]fpt) bool {
	for _, p := range a {
		for _, q := range b {
			if stretch(p, q, 0) > 0.5 {
				return true
			}
		}
	}
	return false
}

// stretch is how far two pieces run along each other on tracks at most tol
// apart.
func stretch(p, q [2]fpt, tol float64) float64 {
	switch {
	case p[0].y == p[1].y && q[0].y == q[1].y && math.Abs(p[0].y-q[0].y) <= tol:
		return math.Min(math.Max(p[0].x, p[1].x), math.Max(q[0].x, q[1].x)) - math.Max(math.Min(p[0].x, p[1].x), math.Min(q[0].x, q[1].x))
	case p[0].x == p[1].x && q[0].x == q[1].x && math.Abs(p[0].x-q[0].x) <= tol:
		return math.Min(math.Max(p[0].y, p[1].y), math.Max(q[0].y, q[1].y)) - math.Max(math.Min(p[0].y, p[1].y), math.Min(q[0].y, q[1].y))
	}
	return 0
}

func inkPieces(ink drawnInk) [][2]fpt {
	var out [][2]fpt
	for _, part := range ink.parts {
		out = append(out, fpieces(part)...)
	}
	return out
}

// touches says p lies on the edge of a shape whose classes include one of
// cls, or just outside it where an arrowhead's tip stops.
func (pg drawnPage) touches(p fpt, cls []string) bool {
	for _, s := range pg.shapes {
		if s.r.grow(2).contains(p, 0) && !s.r.contains(p, 2) && slices.ContainsFunc(cls, s.is) {
			return true
		}
	}
	return false
}

// broken says what is wrong with a way the pointer lights, or "".
func (pg drawnPage) broken(r drawnRoute, w drawnWay) string {
	ends := []string{}
	for _, j := range numbered(r.class, "e") {
		ends = append(ends, "p"+j)
	}
	n := len(w.pts)
	switch {
	case n < 2:
		return "lights no stroke"
	case !pg.touches(w.pts[0], ends) || !pg.touches(w.pts[n-1], ends):
		return "lights a stroke that stops short of the boxes or chips it joins"
	case len(w.tips) == 0:
		return "lights a line with no arrowhead"
	}
	for i, tp := range w.tips {
		at, from := w.pts[n-1], w.pts[n-2]
		if i == 1 {
			at, from = w.pts[0], w.pts[1]
		}
		if dist(tp.at, at) > 0.01 || math.Abs(math.Remainder(tp.deg-math.Atan2(at.y-from.y, at.x-from.x)*180/math.Pi, 360)) > 0.01 {
			return "lights an arrowhead away from its line's end"
		}
	}
	return ""
}

// phantoms is how many stretches of bus ink no line's way runs along, a
// shared track drawn on between two lines' drops (self overview, y=468 from
// x=594 to 628). Drawing those tracks
// as their lines' spans moves the layout's weight past its ceilings
// (eval/lint/decisions.tsv rows 58 and 59), so it stays a known count that
// may only fall. Pointing at one lights the lines its whole bus carries.
const phantoms = 14

// Pointing at a piece of ink lights exactly the lines whose ways run along
// it, each on top and whole between the shapes it joins, and keeps lit the
// shapes at their ends.
func TestPointingAtInkLightsTheLinesThatRunAlongIt(t *testing.T) {
	t.Parallel()
	inks, lines, phantom := 0, 0, 0
	why := map[string]int{}
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			pg := readDrawn(t, v.d)
			if len(pg.routes) == 0 {
				continue
			}
			ways := make([][][2]fpt, len(pg.routes))
			for k, r := range pg.routes {
				ways[k] = fpieces(r.way.pts)
				if got := numbered(r.class, "r"); !slices.Equal(got, []string{fmt.Sprint(k)}) {
					t.Fatalf("%s %s: route %d is numbered %v", name, v.name, k, got)
				}
			}
			for _, ink := range pg.ink {
				inks++
				ps := inkPieces(ink)
				var want []string
				// Off roads, ink of another kind on the same track is another
				// line; a road carries every kind that rides it.
				for k, r := range pg.routes {
					if (v.d.roads || kindOf(strings.TrimPrefix(r.class, "route ")) == kindOf(ink.class)) && along(ps, ways[k]) {
						want = append(want, fmt.Sprint(k))
					}
				}
				got := numbered(ink.class, "r")
				slices.Sort(got)
				slices.Sort(want)
				if !slices.Equal(got, want) {
					why["lights other lines than those that run along it"]++
					t.Logf("%s %s: ink %q lights %v, along it run %v", name, v.name, ink.class, got, want)
				}
				if len(got) == 0 {
					why["lights nothing"]++
				}
				for _, p := range ps {
					if dist(p[0], p[1]) >= 2 {
						phantom += bare(p, slices.Concat(ways...))
					}
				}
				for _, k := range got {
					var i int
					fmt.Sscan(k, &i)
					r := pg.routes[i]
					lines++
					for _, e := range numbered(r.class, "e") {
						if !classIs(ink.class, "e"+e) {
							why["leaves a lit line's end box faded"]++
						}
					}
					if w := pg.broken(r, r.way); w != "" {
						why[w]++
					}
				}
			}
			// Each route lies on the ink of its own line all the way.
			for k := range pg.routes {
				var own [][2]fpt
				for _, ink := range pg.ink {
					if classIs(ink.class, fmt.Sprint("r", k)) {
						own = append(own, inkPieces(ink)...)
					}
				}
				for _, p := range ways[k] {
					if bare(p, own) > 0 {
						why["has a stretch off its own ink"]++
						break
					}
				}
			}
		}
	}
	t.Logf("pointed at %d pieces of ink, which light %d lines; %d stretches of bus ink carry no line: %v", inks, lines, phantom, why)
	if phantom > phantoms {
		t.Errorf("%d stretches of ink carry no line, more than the %d known", phantom, phantoms)
	}
	if len(why) > 0 {
		t.Errorf("pointing at ink lights what it should not: %v", why)
	}
}

// bare is how many stretches of p, longer than a pixel, no piece of own
// runs along.
func bare(p [2]fpt, own [][2]fpt) int {
	lo, hi, across := p[0].x, p[1].x, p[0].y == p[1].y
	if !across {
		lo, hi = p[0].y, p[1].y
	}
	lo, hi = math.Min(lo, hi), math.Max(lo, hi)
	var runs [][2]float64
	for _, q := range own {
		if stretch(p, q, 1) > 0 {
			a, b := q[0].x, q[1].x
			if !across {
				a, b = q[0].y, q[1].y
			}
			runs = append(runs, [2]float64{math.Min(a, b), math.Max(a, b)})
		}
	}
	slices.SortFunc(runs, func(a, b [2]float64) int { return int(a[0] - b[0]) })
	n, at := 0, lo
	for _, r := range runs {
		if r[0] > at+1 {
			n++
		}
		at = math.Max(at, r[1])
	}
	if at < hi-1 {
		n++
	}
	return n
}

// Pointing at a box or chip lights every line with an end there whole, on
// top, and in one colour: a box's own, whatever the line's kind; a chip,
// which wears no unit's colour, leaves each line its own.
func TestPointingAtAShapeLightsItsLinesWholeInOneColour(t *testing.T) {
	t.Parallel()
	shapes, lines := 0, 0
	why := map[string]int{}
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			pg := readDrawn(t, v.d)
			for _, sh := range pg.shapes {
				js := numbered(sh.class, "p")
				if len(js) == 0 {
					continue
				}
				j := js[0]
				shapes++
				box := pg.hues[sh.class]
				for _, o := range pg.shapes {
					if o.kind == "box" && classIs(o.class, "p"+j) {
						box = pg.hues[o.class]
					}
				}
				for _, r := range pg.routes {
					if !classIs(r.class, "e"+j) {
						continue
					}
					lines++
					if s := pg.broken(r, r.way); s != "" {
						why[s]++
					}
					want := "var(--h" + box + ")"
					if box == "" {
						want = "var(--h" + r.hue + ")"
					}
					if got := r.vars["--k"+j]; got != want {
						why["lights a line in another colour than it should take"]++
					}
				}
			}
		}
	}
	t.Logf("pointed at %d shapes, which light %d lines: %v", shapes, lines, why)
	if len(why) > 0 {
		t.Errorf("shapes light lines that are not whole or not in one colour: %v", why)
	}
}

// Pointing at a box or chip lights exactly its own relations, the lines in
// and out with an end there, and the shapes at their other ends: the shapes
// one hop away, and nothing else. No lit stroke reaches a shape that is not
// one of them.
func TestPointingAtAShapeLightsOnlyItsOneHopNeighbours(t *testing.T) {
	t.Parallel()
	shapes := 0
	why := map[string]int{}
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			pg := readDrawn(t, v.d)
			for _, sh := range pg.shapes {
				js := numbered(sh.class, "p")
				if len(js) == 0 {
					continue
				}
				j := js[0]
				shapes++
				hop := numbered(sh.class, "n")
				far := []string{j}
				for _, r := range pg.routes {
					if !classIs(r.class, "e"+j) {
						continue
					}
					ends := numbered(r.class, "e")
					far = append(far, ends...)
					w := r.way
					for _, other := range pg.shapes {
						ks := numbered(other.class, "p")
						if len(ks) == 0 || slices.Contains(ends, ks[0]) {
							continue
						}
						if slices.ContainsFunc(fpieces(w.pts), func(p [2]fpt) bool { return enters(p, other.r.grow(1)) }) {
							why["lights a stroke that reaches a shape at neither of its ends"]++
							t.Logf("%s %s: pointing at %s lights %v (ink %v), which reaches %s %v", name, v.name, sh.id, w.pts, r.way.pts, other.id, other.r)
						}
					}
				}
				slices.Sort(far)
				far = slices.Compact(far)
				slices.Sort(hop)
				if !slices.Equal(far, hop) {
					why["lights other shapes than its one-hop neighbours"]++
					t.Logf("%s %s: pointing at %s lights lines to %v, its neighbours are %v", name, v.name, sh.id, far, hop)
				}
			}
		}
	}
	t.Logf("pointed at %d shapes: %v", shapes, why)
	if len(why) > 0 {
		t.Errorf("pointing at a shape lights more or less than its one-hop neighbours: %v", why)
	}
}

// enters says the piece p, across or down, meets the rect r.
func enters(p [2]fpt, r frect) bool {
	lx, hx := math.Min(p[0].x, p[1].x), math.Max(p[0].x, p[1].x)
	ly, hy := math.Min(p[0].y, p[1].y), math.Max(p[0].y, p[1].y)
	return lx <= r.x+r.w && hx >= r.x && ly <= r.y+r.h && hy >= r.y
}

// Every arrowhead a route ends in paints in the route's own colour, which
// the page sets on the route, so a head never shows in another colour or
// none.
func TestARouteHeadWearsItsRoutesColour(t *testing.T) {
	t.Parallel()
	pg := readDrawn(t, fixture(t, "ripgrep").Overview)
	for _, s := range routeHeads {
		if st := pg.heads[s]; !strings.Contains(st, "var(--c)") || strings.Contains(st, "context-") {
			t.Errorf("arrowhead %s paints with %q", s, st)
		}
	}
	for _, r := range pg.routes {
		for _, tp := range r.way.tips {
			if _, ok := pg.heads[tp.shape]; !ok {
				t.Errorf("a route ends in arrowhead %q, which the drawing does not define", tp.shape)
			}
		}
	}
}

// Every real drawing holds few enough shapes and lines to take part.
func TestEveryRealDrawingTakesPart(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			if len(v.d.Wires) > 0 && len(v.d.Routes) == 0 {
				t.Errorf("%s %s: has lines but routes none of them, so pointing lights nothing", name, v.name)
			}
			if len(v.d.Routes) > routeMax {
				t.Errorf("%s %s: routes %d lines, more than %d", name, v.name, len(v.d.Routes), routeMax)
			}
			for _, p := range v.d.Ports {
				if len(v.d.Routes) > 0 && p.Lit == "" {
					t.Errorf("%s %s: chip %q stays lit while its box dims", name, v.name, p.Label)
				}
			}
		}
	}
}

// hairpins is how many lines ride a bus past the stub or drop they leave
// it by, step to another track and come back along it, so their ink and
// their route double back. Most are a chip's line that steps 8 down beside
// the chip and comes back (pstack's /swarm focus). In orca's shared types
// page CFG-0006's line runs right along y=62 to x=385 and back left along
// y=82 to x=181. It is a routing defect that hover shows rather than
// hides, a known count that may only fall.
const hairpins = 14

func TestFewLinesDoubleBack(t *testing.T) {
	t.Parallel()
	n := 0
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			for _, r := range v.d.Routes {
				if doublesBack(r.runs) {
					n++
				}
			}
		}
	}
	t.Logf("%d lines double back on a bus", n)
	if n > hairpins {
		t.Errorf("%d lines double back on a bus, more than the %d known", n, hairpins)
	}
}

// doublesBack says a line's runs step a short way to another track and
// come back along the one they rode.
func doublesBack(rs [][2]pt) bool {
	for i := 2; i < len(rs); i++ {
		a, step, b := rs[i-2], rs[i-1], rs[i]
		short := abs(step[1].x-step[0].x)+abs(step[1].y-step[0].y) <= 24
		back := sign(a[1].x-a[0].x) == -sign(b[1].x-b[0].x) && sign(a[1].y-a[0].y) == -sign(b[1].y-b[0].y)
		if short && back && shared(a, b) > 0 {
			return true
		}
	}
	return false
}

// In pstack's "understand code" page two lines ride a departure trunk at
// x=163 that stands in one column with another trunk stepping down two
// gaps lower. They step straight down their own trunk rather than riding
// past their drop and coming back.
func TestALineStepsDownItsTrunkBesideAnotherGapsStep(t *testing.T) {
	t.Parallel()
	seen := false
	for _, v := range views(fixture(t, "pstack")) {
		if v.name != "MOD-0007 understand code" {
			continue
		}
		seen = true
		for _, r := range v.d.Routes {
			if doublesBack(r.runs) {
				t.Errorf("a line doubles back on a bus: %v", r.runs)
			}
		}
	}
	if !seen {
		t.Fatal("no MOD-0007 understand code view in pstack")
	}
}

// In pstack's focus pages, such as MOD-0007's /teach, a box's lines to the
// modules outside, gathered into one chip under it, run down a chain whose
// x stands over the chip. They land on the chip at that x rather than jog
// a lane aside to the chip's middle just above it.
func TestALineIntoAGatheredChipLandsAtItsOwnX(t *testing.T) {
	t.Parallel()
	n := 0
	for _, v := range views(fixture(t, "pstack")) {
		if !strings.Contains(v.name, " focus ") {
			continue
		}
		for _, r := range v.d.Routes {
			if len(r.ends) != 2 || !strings.Contains(r.ends[1], gatherPort) {
				continue
			}
			n++
			if k := len(r.runs); k >= 3 {
				if a, b := r.runs[k-2][0], r.runs[k-2][1]; a.y == b.y && abs(b.x-a.x) <= lane {
					t.Errorf("%s: a line jogs %dpx into its gathered chip: %v", v.name, abs(b.x-a.x), r.runs)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no line into a gathered chip in pstack")
	}
}

// shared is how far two parallel pieces span the same stretch, on any two
// tracks.
func shared(a, b [2]pt) int {
	if a[0].y == a[1].y {
		return min(max(a[0].x, a[1].x), max(b[0].x, b[1].x)) - max(min(a[0].x, a[1].x), min(b[0].x, b[1].x))
	}
	return min(max(a[0].y, a[1].y), max(b[0].y, b[1].y)) - max(min(a[0].y, a[1].y), min(b[0].y, b[1].y))
}
