package screen

import (
	"encoding/xml"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// This file measures a rendered drawing against docs/blueprint-svg.md. It
// reads the SVG the page embeds, not the Drawing that produced it, so what it
// judges is what a reader sees. Its thresholds are its own constants, so a
// change to the renderer cannot loosen them.

// The rules' numbers are docs/blueprint-svg.md's.
const (
	// 1: boxes per view.
	viewBudget = 16
	// 3: width budgets.
	overviewWidth, insideWidth = 1320, 1320
	// 5: lines side by side within channelGap share a channel.
	maxPerChannel = 4
	channelGap    = 12.0
	// 6: two lines closer than minGap for runLimit or longer overlap.
	minGap, runLimit = 5.0, 24.0
	// 7: crossings per drawn line, after bundling.
	crossingsPerLine = 2
	// 8: the widths past which a label may be cut, its full text in the tooltip.
	hardBox, hardUnit, hardLoose = 280, 220, 236
	// endZone is how close to a line's end two lines may meet or run
	// together: lines that share a port converge there by design.
	endZone = 26.0
	// braidGap is how close two lines of different bundles run before they
	// read as one braid: the width of a channel.
	braidGap = channelGap
	// 14: how far, per drawn line, lines of different bundles may run
	// side by side in one channel.
	braidPerLine = 200
)

type fpt struct{ x, y float64 }

type frect struct{ x, y, w, h float64 }

func (r frect) contains(p fpt, inset float64) bool {
	return p.x > r.x+inset && p.x < r.x+r.w-inset && p.y > r.y+inset && p.y < r.y+r.h-inset
}

func (r frect) holds(o frect) bool {
	return o.x >= r.x && o.y >= r.y && o.x+o.w <= r.x+r.w && o.y+o.h <= r.y+r.h
}

func (r frect) grow(d float64) frect { return frect{r.x - d, r.y - d, r.w + 2*d, r.h + 2*d} }

func (r frect) overlaps(o frect) bool {
	return r.x < o.x+o.w && o.x < r.x+r.w && r.y < o.y+o.h && o.y < r.y+r.h
}

func (r frect) center() fpt { return fpt{r.x + r.w/2, r.y + r.h/2} }

// shape is a box or a port chip.
type shape struct {
	kind  string // "box" or "port"
	class string
	r     frect
	lines []string
	px    []int
	title string
	id    string
}

func (s shape) is(c string) bool { return strings.Contains(" "+s.class+" ", " "+c+" ") }

// stroke is one subpath of a drawn line. A bundle is one line drawn as
// several subpaths; they share a group.
type stroke struct {
	// path is the index of the drawn path.w the stroke is a subpath of,
	// and group its whole data-net.
	path, group   int
	class, origin string
	pts           []fpt
}

type sheet struct {
	width, height float64
	frame         *frect
	shapes        []shape
	strokes       []stroke
	paths, groups int
	markers       []fpt
	junctions     []junction
	counts        []countLabel
	// routed says the drawing holds each line's route, which the pointer
	// lights whole, so ink that joins two places no relation joins misleads
	// no reader who points at it (rule 18), and routes are those routes.
	routed bool
	routes []stroke
	// roads says the drawing's lines ride roads (confluence.go), as its
	// svg's data-roads says.
	roads bool
}

// junction is a dot that marks a split or merge of one kind's ink.
type junction struct {
	at   fpt
	kind string
}

// countLabel is a ×N drawn beside a bundle.
type countLabel struct {
	at   fpt
	text string
	// group is the stroke group of the wire the count is drawn with.
	group int
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func hasClass(e xml.StartElement, c string) bool {
	return strings.Contains(" "+attr(e, "class")+" ", " "+c+" ")
}

// readSVG parses one drawing. It understands the elements page.html emits.
func readSVG(src string) (sheet, error) {
	var s sheet
	dec := xml.NewDecoder(strings.NewReader(src))
	var group *shape
	nets := map[string]int{}
	var inText bool
	var title *string
	var count *countLabel
	// route is the class of the route whose way is read next.
	var route string
	// last is the group of the path read last, its whole data-net when it
	// has one: a count follows its wire, and its wire is that path's net.
	last := -1
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// A count follows its wire's path, past only the path's title
			// and origin marks; anything else between them leaves it unowned.
			switch {
			case t.Name.Local == "path", t.Name.Local == "title", t.Name.Local == "circle" && hasClass(t, "origin"), t.Name.Local == "text" && hasClass(t, "count"):
			default:
				last = -1
			}
			switch t.Name.Local {
			case "svg":
				s.width, s.height = num(attr(t, "width")), num(attr(t, "height"))
				s.roads = s.roads || attr(t, "data-roads") != ""
			case "g":
				if hasClass(t, "route") {
					s.routed, route = true, attr(t, "class")
				}
				for _, k := range []string{"box", "port"} {
					if hasClass(t, k) {
						group = &shape{kind: k, class: attr(t, "class"), id: attr(t, "data-id")}
					}
				}
			case "rect":
				r := frect{num(attr(t, "x")), num(attr(t, "y")), num(attr(t, "width")), num(attr(t, "height"))}
				switch {
				case hasClass(t, "frame"):
					s.frame = &r
				case group != nil && group.r.w == 0:
					group.r = r
				}
			case "text":
				if hasClass(t, "count") {
					s.counts = append(s.counts, countLabel{at: fpt{num(attr(t, "x")), num(attr(t, "y"))}, group: last})
					count = &s.counts[len(s.counts)-1]
				}
				inText = group != nil && !hasClass(t, "sub")
				if inText {
					px := 13
					if hasClass(t, "chip") || group.is("loose") {
						px = chipPx
					}
					group.lines = append(group.lines, "")
					group.px = append(group.px, px)
				}
			case "path":
				if hasClass(t, "way") && route != "" {
					parts, err := flatten(attr(t, "d"))
					if err != nil {
						return s, err
					}
					for _, p := range parts {
						s.routes = append(s.routes, stroke{class: route, pts: p})
					}
					route = ""
				}
				if hasClass(t, "w") {
					parts, err := flatten(attr(t, "d"))
					if err != nil {
						return s, err
					}
					g := s.groups
					if n := attr(t, "data-net"); n != "" {
						if k, ok := nets[n]; ok {
							g = k
						} else {
							nets[n] = g
							s.groups++
						}
					} else {
						s.groups++
					}
					last = g
					for _, p := range parts {
						s.strokes = append(s.strokes, stroke{path: s.paths, group: g, class: attr(t, "class"), origin: attr(t, "data-origin"), pts: p})
					}
					s.paths++
				}
			case "circle":
				switch {
				case hasClass(t, "origin"):
					s.markers = append(s.markers, fpt{num(attr(t, "cx")), num(attr(t, "cy"))})
				case hasClass(t, "junction"):
					j := junction{at: fpt{num(attr(t, "cx")), num(attr(t, "cy"))}}
					if cs := strings.Fields(attr(t, "class")); len(cs) > 1 {
						j.kind = cs[1]
					}
					s.junctions = append(s.junctions, j)
				}
			case "title":
				if group != nil {
					title = &group.title
				}
			}
		case xml.CharData:
			switch {
			case title != nil:
				*title += string(t)
			case count != nil:
				count.text += string(t)
			case inText:
				group.lines[len(group.lines)-1] += string(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "g":
				if group != nil {
					s.shapes = append(s.shapes, *group)
					group = nil
				}
			case "text":
				inText = false
				count = nil
			case "title":
				title = nil
			}
		}
	}
	return s, nil
}

// flatten turns a path made of M, L, H, V and C commands into polylines,
// one per subpath.
func flatten(d string) ([][]fpt, error) {
	fields := strings.FieldsFunc(d, func(r rune) bool { return r == ' ' || r == ',' })
	var toks []string
	for _, f := range fields {
		for f != "" && unicode.IsLetter(rune(f[0])) {
			toks = append(toks, f[:1])
			f = f[1:]
		}
		if f != "" {
			toks = append(toks, f)
		}
	}
	var out [][]fpt
	var cur fpt
	i := 0
	next := func() float64 { i++; return num(toks[i-1]) }
	add := func(p fpt) { out[len(out)-1] = append(out[len(out)-1], p) }
	for i < len(toks) {
		cmd := toks[i]
		i++
		switch cmd {
		case "M":
			cur = fpt{next(), next()}
			out = append(out, []fpt{cur})
		case "L":
			cur = fpt{next(), next()}
			add(cur)
		case "H":
			cur = fpt{next(), cur.y}
			add(cur)
		case "V":
			cur = fpt{cur.x, next()}
			add(cur)
		case "C":
			a, b, c := fpt{next(), next()}, fpt{next(), next()}, fpt{next(), next()}
			for k := 1; k <= 16; k++ {
				t := float64(k) / 16
				u := 1 - t
				add(fpt{
					u*u*u*cur.x + 3*u*u*t*a.x + 3*u*t*t*b.x + t*t*t*c.x,
					u*u*u*cur.y + 3*u*u*t*a.y + 3*u*t*t*b.y + t*t*t*c.y,
				})
			}
			cur = c
		default:
			return nil, fmt.Errorf("path command %q in %q", cmd, d)
		}
	}
	return out, nil
}

func dist(a, b fpt) float64 { return math.Hypot(a.x-b.x, a.y-b.y) }

func segDist(p, a, b fpt) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	l := dx*dx + dy*dy
	if l == 0 {
		return dist(p, a)
	}
	t := math.Max(0, math.Min(1, ((p.x-a.x)*dx+(p.y-a.y)*dy)/l))
	return dist(p, fpt{a.x + t*dx, a.y + t*dy})
}

func cross(a, b, c, d fpt) (fpt, bool) {
	r := fpt{b.x - a.x, b.y - a.y}
	s := fpt{d.x - c.x, d.y - c.y}
	den := r.x*s.y - r.y*s.x
	if den == 0 {
		return fpt{}, false
	}
	t := ((c.x-a.x)*s.y - (c.y-a.y)*s.x) / den
	u := ((c.x-a.x)*r.y - (c.y-a.y)*r.x) / den
	if t < 0 || t >= 1 || u < 0 || u >= 1 {
		return fpt{}, false
	}
	return fpt{a.x + t*r.x, a.y + t*r.y}, true
}

// sample is a point on a polyline with the direction the line runs there
// and its distance along the line.
type sample struct {
	p, dir fpt
	at     float64
}

func samples(pts []fpt, step float64) []sample {
	var out []sample
	along := 0.0
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		l := dist(a, b)
		if l == 0 {
			continue
		}
		dir := fpt{(b.x - a.x) / l, (b.y - a.y) / l}
		for t := 0.0; t < l; t += step {
			out = append(out, sample{fpt{a.x + dir.x*t, a.y + dir.y*t}, dir, along + t})
		}
		along += l
	}
	return out
}

func nearEnds(p fpt, w stroke, r float64) bool {
	return dist(p, w.pts[0]) < r || dist(p, w.pts[len(w.pts)-1]) < r
}

// Metrics is what docs/blueprint-svg.md bounds, measured on one drawing.
type Metrics struct {
	Boxes, Split                                  int
	Width, Wires, Crossings, TooClose, MaxChannel int
	Truncated, Overflow                           int
	// Escaped counts the chips that leave the canvas, or the frame or box
	// whose edge they sit on.
	Escaped                                  int
	OriginDashed, Unmarked                   int
	UnwiredOnFrame, NodeOverlaps, ThroughBox int
	// Forward and Back count the lines that arrive at a box or a chip by
	// the way they run, BackPlain the back lines drawn like forward ones,
	// and SplitIn the lines of one kind that arrive at one edge of a box
	// apart from the bundle of their kind.
	Forward, Back, BackPlain, SplitIn int
	// Braid is how far, in pixels, lines of different bundles run side by
	// side within braidGap of each other, away from their ends: a run a
	// shared trunk would draw once. Detour is how much longer, in pixels,
	// the lines are than the shortest grid path between their ends, and
	// MaxDetour the most any one line is.
	Braid, Detour, MaxDetour int
	// Bends is how many times the lines turn, and MaxBends the most any
	// one line does.
	Bends, MaxBends int
	// Length is how long, in pixels, the lines run in all, and BackLength
	// how much of that is drawn dashed: the ink of the lines that climb.
	Length, BackLength int
	// crossed are the points R1 counts as crossings.
	crossed []fpt
}

func (m Metrics) String() string {
	return fmt.Sprintf("forward %d, back %d, back drawn plain %d, split arrivals %d, ", m.Forward, m.Back, m.BackPlain, m.SplitIn) + fmt.Sprintf("boxes %d, split bundles %d, width %d, lines %d, crossings %d, too-close pairs %d, max per channel %d, cut labels %d, overflowing labels %d, escaped chips %d, origin-dashed %d, unmarked %d, unwired on frame %d, node overlaps %d, through-box %d",
		m.Boxes, m.Split, m.Width, m.Wires, m.Crossings, m.TooClose, m.MaxChannel, m.Truncated, m.Overflow, m.Escaped, m.OriginDashed, m.Unmarked, m.UnwiredOnFrame, m.NodeOverlaps, m.ThroughBox) + fmt.Sprintf(", braid %dpx, detour %dpx, longest detour %dpx, bends %d, most bends %d, length %dpx", m.Braid, m.Detour, m.MaxDetour, m.Bends, m.MaxBends, m.Length)
}

func measure(s sheet) Metrics {
	m := Metrics{Width: int(s.width), Wires: s.groups}
	for _, sh := range s.shapes {
		// A twin is a unit the focus view draws a second time.
		if sh.kind == "box" && !strings.HasSuffix(sh.id, twinMark) {
			m.Boxes++
		}
	}
	var ws []stroke
	for _, w := range s.strokes {
		if len(w.pts) > 1 {
			ws = append(ws, w)
		}
	}
	for _, w := range ws {
		n := bends(w.pts)
		m.Bends += n
		m.MaxBends = max(m.MaxBends, n)
		for i := 1; i < len(w.pts); i++ {
			d := int(math.Abs(w.pts[i].x-w.pts[i-1].x) + math.Abs(w.pts[i].y-w.pts[i-1].y))
			m.Length += d
			if strings.Contains(" "+w.class+" ", " back ") {
				m.BackLength += d
			}
		}
	}
	box := bounds(ws)
	// R1: crossings between two lines, away from the ends where lines meet
	// on purpose.
	for i := range ws {
		for j := i + 1; j < len(ws); j++ {
			if ws[i].group == ws[j].group || apart(box[i], box[j], 0) {
				continue
			}
			var seen []fpt
			for a := 1; a < len(ws[i].pts); a++ {
				for b := 1; b < len(ws[j].pts); b++ {
					p, ok := cross(ws[i].pts[a-1], ws[i].pts[a], ws[j].pts[b-1], ws[j].pts[b])
					if !ok || nearEnds(p, ws[i], endZone) || nearEnds(p, ws[j], endZone) {
						continue
					}
					dup := false
					for _, q := range seen {
						dup = dup || dist(p, q) < 6
					}
					if !dup {
						seen = append(seen, p)
					}
				}
			}
			m.Crossings += len(seen)
			m.crossed = append(m.crossed, seen...)
		}
	}
	// R3: two lines running side by side closer than minGap.
	sampled := make([][]sample, len(ws))
	for i, w := range ws {
		sampled[i] = samples(w.pts, 2)
	}
	close := func(i, j int) bool {
		si := sampled[i]
		run := 0.0
		for k, sm := range si {
			if nearEnds(sm.p, ws[i], endZone) || nearEnds(sm.p, ws[j], endZone) {
				run = 0
				continue
			}
			near := false
			for b := 1; b < len(ws[j].pts) && !near; b++ {
				a, c := ws[j].pts[b-1], ws[j].pts[b]
				l := dist(a, c)
				near = l > 0 && segDist(sm.p, a, c) < minGap && math.Abs(sm.dir.x*(c.x-a.x)/l+sm.dir.y*(c.y-a.y)/l) > 0.9
			}
			if !near || k == 0 {
				run = 0
				continue
			}
			if run += sm.at - si[k-1].at; run >= runLimit {
				return true
			}
		}
		return false
	}
	pairs := map[[2]int]bool{}
	for i := range ws {
		for j := i + 1; j < len(ws); j++ {
			g := [2]int{ws[i].group, ws[j].group}
			if g[0] != g[1] && !pairs[g] && !apart(box[i], box[j], minGap) && (close(i, j) || close(j, i)) {
				pairs[g] = true
				m.TooClose++
			}
		}
	}
	// R2: lines that share a vertical channel, read on scan lines.
	for y := 0.0; y < s.height; y += 6 {
		var xs []float64
		for _, w := range ws {
			for a := 1; a < len(w.pts); a++ {
				p, q := w.pts[a-1], w.pts[a]
				if (p.y-y)*(q.y-y) > 0 || p.y == q.y || math.Abs(q.y-p.y) < 2*math.Abs(q.x-p.x) {
					continue
				}
				x := p.x + (q.x-p.x)*(y-p.y)/(q.y-p.y)
				if !nearEnds(fpt{x, y}, w, endZone) {
					xs = append(xs, x)
				}
				break
			}
		}
		sort.Float64s(xs)
		run := 1
		for k := range xs {
			if k > 0 && xs[k]-xs[k-1] <= channelGap {
				run++
			} else {
				run = 1
			}
			m.MaxChannel = max(m.MaxChannel, run)
		}
	}
	// R4: labels whole, and inside their shapes.
	for _, sh := range s.shapes {
		pad, hard := 12.0, float64(hardBox)
		switch {
		case sh.kind == "port":
			pad, hard = 6, math.Inf(1)
		case sh.is("inner"):
			hard = hardUnit
		case sh.is("loose"):
			pad, hard = 6, hardLoose
		}
		for k, line := range sh.lines {
			if strings.HasSuffix(line, "…") && (sh.r.w < hard || !strings.Contains(sh.title, strings.TrimSuffix(line, "…"))) {
				m.Truncated++
			}
			if float64(textWidth(line, sh.px[k])) > sh.r.w-pad {
				m.Overflow++
			}
		}
	}
	canvas := frect{0, 0, s.width, s.height}
	for _, sh := range s.shapes {
		if sh.kind != "port" {
			continue
		}
		hosted := s.frame != nil && s.frame.holds(sh.r)
		for _, b := range s.shapes {
			hosted = hosted || b.kind == "box" && b.r.holds(sh.r)
		}
		if !hosted || !canvas.holds(sh.r) {
			m.Escaped++
		}
	}
	// R10: nothing drawn on top of anything else.
	for i := range s.shapes {
		for j := i + 1; j < len(s.shapes); j++ {
			a, b := s.shapes[i], s.shapes[j]
			if a.kind == b.kind && a.r.overlaps(b.r) {
				m.NodeOverlaps++
			}
		}
	}
	through := map[int]bool{}
	for i, w := range ws {
		var si []sample
		for _, sh := range s.shapes {
			if apart(box[i], sh.r, 2) || sh.r.grow(5).contains(w.pts[0], 0) || sh.r.grow(5).contains(w.pts[len(w.pts)-1], 0) {
				continue
			}
			if si == nil {
				si = samples(w.pts, 3)
			}
			for _, sm := range si {
				if sh.r.contains(sm.p, 2) {
					through[w.group] = true
					break
				}
			}
		}
	}
	m.ThroughBox = len(through)
	m.Braid = braid(ws)
	for _, w := range ws {
		run := 0.0
		for a := 1; a < len(w.pts); a++ {
			run += dist(w.pts[a-1], w.pts[a])
		}
		a, b := w.pts[0], w.pts[len(w.pts)-1]
		d := int(run - math.Abs(b.x-a.x) - math.Abs(b.y-a.y))
		m.Detour += d
		m.MaxDetour = max(m.MaxDetour, d)
	}
	// 12: an unwired contract is listed under the drawing, not on the frame.
	if s.frame != nil {
		for _, sh := range s.shapes {
			if sh.kind == "port" && sh.is("unwired") {
				m.UnwiredOnFrame++
			}
		}
	}
	// 4: lines of one kind that leave one edge of a box are one bundle,
	// but for a line that drops straight from one box into the next, which
	// needs no bundle.
	onBox := func(p fpt) bool {
		for _, b := range s.shapes {
			if b.kind == "box" && p.x > b.r.x && p.x < b.r.x+b.r.w && (math.Abs(p.y-b.r.y) < 6 || math.Abs(p.y-b.r.y-b.r.h) < 6) {
				return true
			}
		}
		return false
	}
	// A line between two modules that use each other reads both ways, so
	// a bundle of two of them would join their far ends (rule 18): it is a
	// bundle of its own, as a straight drop is.
	straight := func(w stroke) bool {
		return kindOf(w.class) == "mutual" || len(w.pts) == 2 && w.pts[0].x == w.pts[1].x && onBox(w.pts[0]) && onBox(w.pts[1])
	}
	leaving := map[string]map[int]bool{}
	for _, w := range ws {
		if straight(w) {
			continue
		}
		st := w.pts[0]
		for k, b := range s.shapes {
			if b.kind != "box" || st.x <= b.r.x || st.x >= b.r.x+b.r.w {
				continue
			}
			edge := ""
			switch {
			case math.Abs(st.y-b.r.y) < 1:
				edge = "top"
			case math.Abs(st.y-b.r.y-b.r.h) < 1:
				edge = "bottom"
			default:
				continue
			}
			kk := fmt.Sprint(k, edge, kindOf(w.class))
			if leaving[kk] == nil {
				leaving[kk] = map[int]bool{}
			}
			leaving[kk][w.group] = true
		}
	}
	for _, gs := range leaving {
		m.Split += len(gs) - 1
	}
	// 13: a line reads top to bottom and left to right. Each stroke that
	// ends on a box or a chip is one arriving line; it runs forward when its
	// end lies below its start, or level with it and to the right. A line
	// that provides a contract reads from the contract to the unit that
	// meets it, so it counts the other way round.
	on := func(p fpt) (frect, bool) {
		for _, sh := range s.shapes {
			if sh.r.grow(5).contains(p, 0) {
				return sh.r, true
			}
		}
		return frect{}, false
	}
	// forwardOf says the line a-b reads down or right.
	forwardOf := func(class string, a, b fpt) bool {
		dy, dx := b.y-a.y, b.x-a.x
		if strings.Contains(" "+class+" ", " provides ") {
			dy, dx = -dy, -dx
		}
		return dy > 2 || math.Abs(dy) <= 2 && dx >= 0
	}
	arrivals := map[string]map[int]bool{}
	for _, w := range ws {
		a, b := w.pts[0], w.pts[len(w.pts)-1]
		r, ok := on(b)
		if !ok {
			continue
		}
		forward := forwardOf(w.class, a, b)
		// A stroke of a road many lines ride may end going up into a box
		// that a line it carries reaches going across: the line's route,
		// where the drawing holds it, says which way that line reads.
		for _, c := range strings.Fields(w.class) {
			if k, err := strconv.Atoi(strings.TrimPrefix(c, "r")); err == nil && c[0] == 'r' && k < len(s.routes) {
				way := s.routes[k].pts
				forward = forward || dist(way[len(way)-1], b) < 1 && forwardOf(w.class, way[0], b)
			}
		}
		switch {
		case forward:
			m.Forward++
		default:
			m.Back++
			if !strings.Contains(" "+w.class+" ", " back ") {
				m.BackPlain++
			}
		}
		edge := "top"
		if math.Abs(b.y-r.y-r.h) < math.Abs(b.y-r.y) {
			edge = "bottom"
		}
		k := fmt.Sprint(r, edge, kindOf(w.class))
		if straight(w) {
			k = fmt.Sprint(k, "straight", w.group)
		}
		if arrivals[k] == nil {
			arrivals[k] = map[int]bool{}
		}
		arrivals[k][w.group] = true
	}
	// Rule 7 counts a drawn line by where it arrives: a bundle is one line
	// however many pieces draw it, and a piece that ends on a trunk is part
	// of the line that trunk carries.
	m.Wires = 0
	counted := map[int]bool{}
	for _, gs := range arrivals {
		m.SplitIn += len(gs) - 1
		for g := range gs {
			if !counted[g] {
				counted[g] = true
				m.Wires++
			}
		}
	}
	// R7: a line's dash says its kind, never its origin, and a line a reader
	// or a human gave carries a mark where it leaves its unit.
	dashed, unmarked := map[int]bool{}, map[int]bool{}
	marked := map[int]bool{}
	for _, w := range ws {
		if strings.Contains(" "+w.class+" ", " guess ") {
			dashed[w.group] = true
		}
		for _, mk := range s.markers {
			if dist(mk, w.pts[0]) < 6 || dist(mk, w.pts[len(w.pts)-1]) < 6 {
				marked[w.group] = true
			}
		}
	}
	for _, w := range ws {
		if w.origin != "" && w.origin != "extracted" && !marked[w.group] {
			unmarked[w.group] = true
		}
	}
	m.OriginDashed, m.Unmarked = len(dashed), len(unmarked)
	return m
}

// braid sums, over each pair of lines of different bundles, how far one
// runs beside the other within braidGap, away from both lines' ends.
// bounds bounds each stroke, so pairs too far apart to meet are skipped.
func bounds(ws []stroke) []frect {
	out := make([]frect, len(ws))
	for i, w := range ws {
		lo, hi := w.pts[0], w.pts[0]
		for _, p := range w.pts {
			lo, hi = fpt{math.Min(lo.x, p.x), math.Min(lo.y, p.y)}, fpt{math.Max(hi.x, p.x), math.Max(hi.y, p.y)}
		}
		out[i] = frect{lo.x, lo.y, hi.x - lo.x, hi.y - lo.y}
	}
	return out
}

// apart says no point of a comes within gap of a point of b.
func apart(a, b frect, gap float64) bool {
	return a.x > b.x+b.w+gap || b.x > a.x+a.w+gap || a.y > b.y+b.h+gap || b.y > a.y+a.h+gap
}

func braid(ws []stroke) int {
	total := 0.0
	box := bounds(ws)
	for i := range ws {
		si := samples(ws[i].pts, 2)
		for j := i + 1; j < len(ws); j++ {
			if ws[i].group == ws[j].group || apart(box[i], box[j], braidGap) {
				continue
			}
			for k, sm := range si {
				if k == 0 || nearEnds(sm.p, ws[i], endZone) || nearEnds(sm.p, ws[j], endZone) {
					continue
				}
				for b := 1; b < len(ws[j].pts); b++ {
					a, c := ws[j].pts[b-1], ws[j].pts[b]
					l := dist(a, c)
					if l > 0 && segDist(sm.p, a, c) < braidGap && math.Abs(sm.dir.x*(c.x-a.x)/l+sm.dir.y*(c.y-a.y)/l) > 0.9 {
						total += sm.at - si[k-1].at
						break
					}
				}
			}
		}
	}
	return int(total)
}

func (m Metrics) violations(width int) []string {
	var out []string
	check := func(ok bool, rule, msg string, args ...any) {
		if !ok {
			out = append(out, rule+": "+fmt.Sprintf(msg, args...))
		}
	}
	check(m.Boxes <= viewBudget, "1", "%d boxes, budget %d", m.Boxes, viewBudget)
	check(m.Width <= width, "3", "%dpx wide, budget %dpx", m.Width, width)
	check(m.Split == 0, "4", "%d lines leave a box beside a bundle of their kind instead of in it", m.Split)
	check(m.SplitIn == 0, "4", "%d lines reach a box beside a bundle of their kind instead of in it", m.SplitIn)
	check(m.MaxChannel <= maxPerChannel, "5", "%d lines share one channel, limit %d", m.MaxChannel, maxPerChannel)
	check(m.TooClose == 0, "6", "%d pairs of lines run closer than %vpx for %vpx", m.TooClose, minGap, runLimit)
	check(m.Crossings <= crossingsPerLine*m.Wires, "7", "%d crossings for %d drawn lines, limit %d", m.Crossings, m.Wires, crossingsPerLine*m.Wires)
	check(m.Truncated == 0, "8", "%d labels cut short below the hard width", m.Truncated)
	check(m.Overflow == 0, "8", "%d labels wider than their box", m.Overflow)
	check(m.Escaped == 0, "8", "%d chips run off their edge or the canvas", m.Escaped)
	check(m.NodeOverlaps == 0, "9", "%d boxes or chips overlap", m.NodeOverlaps)
	check(m.ThroughBox == 0, "9", "%d lines pass through a box they do not join", m.ThroughBox)
	check(m.OriginDashed == 0, "11", "%d lines dashed for their origin", m.OriginDashed)
	check(m.Unmarked == 0, "11", "%d lines from a reader or a human without an origin mark", m.Unmarked)
	check(m.UnwiredOnFrame == 0, "12", "%d unwired contracts spread on the frame", m.UnwiredOnFrame)
	check(m.Back <= max(1, int(backShare*float64(m.Forward+m.Back))), "13", "%d of %d arriving lines run up or left, limit %d", m.Back, m.Forward+m.Back, max(1, int(backShare*float64(m.Forward+m.Back))))
	check(m.BackPlain == 0, "13", "%d lines run up or left drawn like forward ones", m.BackPlain)
	check(m.Braid <= braidPerLine*max(1, m.Wires), "14", "lines run %dpx side by side for %d drawn lines, limit %dpx", m.Braid, m.Wires, braidPerLine*max(1, m.Wires))
	check(m.MaxDetour <= detourLimit, "15", "a line runs %dpx longer than the shortest way between its ends, limit %dpx", m.MaxDetour, detourLimit)
	return out
}

// bends counts where a line turns: a point between two runs that head
// different ways.
func bends(pts []fpt) int {
	n := 0
	var last fpt
	for i := 1; i < len(pts); i++ {
		d := fpt{fsign(pts[i].x - pts[i-1].x), fsign(pts[i].y - pts[i-1].y)}
		if d == (fpt{}) {
			continue
		}
		if last != (fpt{}) && d != last {
			n++
		}
		last = d
	}
	return n
}

func fsign(f float64) float64 {
	switch {
	case f > 0.5:
		return 1
	case f < -0.5:
		return -1
	}
	return 0
}
