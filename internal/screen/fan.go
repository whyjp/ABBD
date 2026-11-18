package screen

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A fan draws the lines that leave one place on one trunk as strands, one
// per line, spread along the edge they leave and gathered into the trunk
// a few px out, so the drawing shows at rest how many lines left there
// and merged. It is a trial the project turns on (store.Project's
// OverviewStrands), drawn on the overview after everything else is set:
// it moves no box, route, word, number or key, and a path it has no clear
// room for keeps its plain start. The path's D stays whole, so every
// measure and the route a pointer lights read it as before (Fan).

const (
	// fanReach is how far out the strands gather, and plyReach how far the
	// twist runs past that.
	fanReach, plyReach = 13, 12
	// fanRest is the least trunk the first stretch keeps past the strands.
	fanRest = 7
	// fanMost is the most strands a reader counts at rest; more lines draw
	// that many strands and keep their ×N.
	fanMost = 4
	// fanPitch is the widest step between strands, for up to three, and
	// fanPitch4 for four; the step narrows by 2px toward a neighbour down
	// to fanPitchMin, where three strands still read apart.
	fanPitch, fanPitch4, fanPitchMin = 10, 8, 6
)

// fanShapes are the ends a fan may take.
var fanShapes = map[string]bool{"fan": true, "twist": true}

// fan gives each path that may start as strands its Fan, in shape. A
// twist with no room on its first stretch, too short or too near another
// mark, gathers only.
func (d *Drawing) fan(shape string) {
	if !fanShapes[shape] {
		return
	}
	lead := map[int]bool{}
	if d.Lead != nil {
		for _, h := range d.Lead.Hops {
			lead[h.way] = true
		}
	}
	for i := range d.Wires {
		w := &d.Wires[i]
		if !w.fans(lead) {
			continue
		}
		ps := pointsOf(w.D)
		a, b := ps[0], ps[1]
		long := abs(b.x-a.x) + abs(b.y-a.y)
		reaches := []int{fanReach}
		if shape == "twist" {
			reaches = []int{fanReach + plyReach, fanReach}
		}
		for _, reach := range reaches {
			if long < reach+fanRest {
				continue
			}
			f, ok := d.fanOf(i, ps, reach)
			if !ok {
				continue
			}
			if reach > fanReach {
				f.Plies = plies(a, b)
			}
			d.Wires[i].Fan = f
			d.fanned = true
			break
		}
	}
}

// fans says w may start as strands: a road path of two or more lines that
// all leave its start, toward the reading direction, of a kind that reads
// one way, on no hop of the primary flow, and drawn as one straight-cut
// path.
func (w *Wire) fans(lead map[int]bool) bool {
	if w.Fan != nil || len(w.rides) < 2 || len(w.leave) != len(w.rides) || slices.Contains(w.leave, "") {
		return false
	}
	if !w.Joins || w.Back || w.Both || w.Kind == "mutual" || slices.ContainsFunc(w.rides, func(k int) bool { return lead[k] }) {
		return false
	}
	ps := pointsOf(w.D)
	return len(ps) >= 2 && (ps[0].x == ps[1].x) != (ps[0].y == ps[1].y)
}

// fanOf sets the strands of wire i, whose points are ps, gathering reach
// px out: as wide as fanPitch allows and narrower toward a neighbour, or
// none where even fanPitchMin meets another mark.
func (d *Drawing) fanOf(i int, ps []pt, reach int) (*Fan, bool) {
	w := d.Wires[i]
	a, b := ps[0], ps[1]
	u := pt{sign(b.x - a.x), sign(b.y - a.y)}
	// across is a point's place along the edge the strands leave.
	across := func(p pt) int {
		if u.x == 0 {
			return p.x
		}
		return p.y
	}
	at := func(along, off int) pt {
		if u.x == 0 {
			return pt{a.x + off, a.y + u.y*along}
		}
		return pt{a.x + u.x*along, a.y + off}
	}
	lo, hi, ok := d.edgeSpan(a, u)
	if !ok {
		return nil, false
	}
	riders := slices.Clone(w.rides)
	origin := map[int]string{}
	for j, k := range w.rides {
		origin[k] = w.leave[j]
	}
	// Each strand stands on the side its line heads to, so no two cross
	// before they meet.
	far := func(k int) int {
		rp := pointsOf(d.Routes[k].D)
		if len(rp) == 0 {
			return 0
		}
		return across(rp[len(rp)-1]) - across(a)
	}
	slices.SortFunc(riders, func(p, q int) int { return cmp.Or(cmp.Compare(far(p), far(q)), cmp.Compare(p, q)) })
	shown := min(len(riders), fanMost)
	keep := len(riders) > shown
	start := fanPitch
	if shown > 3 {
		start = fanPitch4
	}
	for pitch := start; pitch >= fanPitchMin; pitch -= 2 {
		spread := (shown - 1) * pitch
		if across(a)-spread/2 < lo || across(a)+spread/2 > hi {
			continue
		}
		var zone box
		if u.x == 0 {
			zone = box{a.x - spread/2 - 2, min(a.y+u.y*2, a.y+u.y*reach), spread + 4, reach - 2}
		} else {
			zone = box{min(a.x+u.x*2, a.x+u.x*reach), a.y - spread/2 - 2, reach - 2, spread + 4}
		}
		count := box{}
		lx, ly := 0, 0
		if keep {
			knot := at(reach, 0)
			lx, ly = knot.x+5, knot.y+map[bool]int{true: 11, false: -3}[u.y > 0]
			if u.x != 0 {
				tw := textWidth(fmt.Sprintf("×%d", w.Count), 11)
				lx, ly = knot.x+map[bool]int{true: 3, false: -tw - 3}[u.x > 0], knot.y-4
			}
			count = box{lx - 2, ly - 11, textWidth(fmt.Sprintf("×%d", w.Count), 11) + 4, 13}
		}
		if !d.fanClear(i, zone, [][2]pt{{a, b}}) || keep && w.Count > 0 && !d.fanClear(i, count, [][2]pt{{a, b}}) {
			continue
		}
		f := &Fan{Trunk: polyline(append([]pt{at(reach, 0)}, ps[1:]...)), Stem: fmt.Sprintf("%d %d", a.x, a.y)}
		var marks []Mark
		for s := range shown {
			off := (2*s - (shown - 1)) * pitch / 2
			p, c1, c2, m := at(0, off), at(8, off), at(5, 0), at(fanReach, 0)
			lit := w.Lit
			if !keep {
				lit = d.Routes[riders[s]].Lit
			}
			f.Strands = append(f.Strands, Strand{D: fmt.Sprintf("M%d %d C%d %d %d %d %d %d", p.x, p.y, c1.x, c1.y, c2.x, c2.y, m.x, m.y), Lit: lit})
		}
		for j, k := range riders {
			if og := origin[k]; og != "" && og != Badge(model.Extracted) {
				p := at(0, (2*min(j, shown-1)-(shown-1))*pitch/2)
				marks = append(marks, Mark{X: p.x, Y: p.y, Origin: og})
			}
		}
		d.Wires[i].Marks = marks
		if keep {
			if d.Wires[i].Count > 0 {
				d.Wires[i].LX, d.Wires[i].LY = lx, ly
			}
		} else {
			d.Wires[i].Count, d.Wires[i].LX, d.Wires[i].LY = 0, 0, 0
		}
		return f, true
	}
	return nil, false
}

// plies are the twist's two plies, from where the strands meet on the
// stretch a to b, crossing each other every 4px for plyReach.
func plies(a, b pt) []string {
	u := pt{sign(b.x - a.x), sign(b.y - a.y)}
	var out []string
	for _, s := range []int{1, -1} {
		var q strings.Builder
		m := pt{a.x + u.x*fanReach, a.y + u.y*fanReach}
		fmt.Fprintf(&q, "M%d %d", m.x, m.y)
		for k := range plyReach / 4 {
			side := s * (1 - 2*(k%2)) * 4
			if u.x == 0 {
				fmt.Fprintf(&q, " q%d %d 0 %d", side, u.y*2, u.y*4)
			} else {
				fmt.Fprintf(&q, " q%d %d %d 0", u.x*2, side, u.x*4)
			}
		}
		out = append(out, q.String())
	}
	return out
}

// edgeSpan is the stretch of the edge at a, which a line leaves heading
// u, that strands may start on: the box's or chip's edge, 4px in from its
// corners.
func (d *Drawing) edgeSpan(a, u pt) (lo, hi int, ok bool) {
	var rs []box
	for _, b := range d.Boxes {
		rs = append(rs, box{b.X, b.Y, b.W, b.H})
	}
	for _, p := range d.Ports {
		rs = append(rs, box{p.X, p.RectY(), p.W, p.H})
	}
	for _, r := range rs {
		switch {
		case u.x == 0 && a.x > r.x && a.x < r.x+r.w && (u.y > 0 && abs(a.y-(r.y+r.h)) <= 1 || u.y < 0 && abs(a.y-r.y) <= 1):
			return r.x + 4, r.x + r.w - 4, true
		case u.y == 0 && a.y > r.y && a.y < r.y+r.h && (u.x > 0 && abs(a.x-(r.x+r.w)) <= 1 || u.x < 0 && abs(a.x-r.x) <= 1):
			return r.y + 4, r.y + r.h - 4, true
		}
	}
	return 0, 0, false
}

// fanClear says area, where wire i's strands would stand, meets no other
// mark: no line but i's own first stretch own, arrowhead, dot, chip, word
// or count, other than i's own count, no box, band title, number, step
// word, key, origin mark of another line, +N pill or gap.
func (d *Drawing) fanClear(i int, area box, own [][2]pt) bool {
	w := d.Wires[i]
	d.Wires[i].Count = 0
	ok := d.clearOfInk(area, own) && !d.onBadge(area)
	d.Wires[i].Count = w.Count
	if !ok {
		return false
	}
	for _, b := range d.Boxes {
		if hit(area, box{b.X, b.Y, b.W, b.H}) {
			return false
		}
		if b.More > 0 && hit(area, box{b.MoreX(), b.MoreY(), b.MoreW(), 16}) {
			return false
		}
	}
	for _, z := range d.Zones {
		if hit(area, grow(z.titleArea(), inkGap)) {
			return false
		}
	}
	if c := d.Card; c != nil && hit(area, grow(box{c.X, c.Y, c.W, c.H}, cardRoom)) {
		return false
	}
	if l := d.Lead; l != nil {
		for _, h := range l.Hops {
			if h.Word != "" && hit(area, box{h.WX, h.WY - 10, textWidth(h.Word, wordPx), 12}) {
				return false
			}
		}
	}
	for j, o := range d.Wires {
		for _, g := range o.Gaps {
			if j == i && hit(area, box{g.Left(), g.Y - 3, g.Width(), 6}) {
				return false
			}
		}
		if j == i {
			continue
		}
		for _, m := range o.Marks {
			if hit(area, box{m.X - 4, m.Y - 4, 8, 8}) {
				return false
			}
		}
	}
	return true
}

// pointsOf are the points of a path written as polyline writes it, or nil
// for one of several subpaths.
func pointsOf(d string) []pt {
	if strings.Count(d, "M") != 1 {
		return nil
	}
	rs := runs(d)
	if len(rs) == 0 {
		return nil
	}
	ps := []pt{rs[0][0]}
	for _, r := range rs {
		ps = append(ps, r[1])
	}
	return ps
}

// fanCSS draws the strands of a page whose overview has them, in their
// line's colour and solid whatever its kind, fading with the ink while a
// reader points: the route lit then starts where the line does. A page
// with none carries none of it.
func (p page) fanCSS() string {
	if p.Template != "index" || !p.Overview.fanned {
		return ""
	}
	return "svg path.strand, svg path.ply { fill: none; stroke: var(--c); stroke-width: 1.5; stroke-linecap: round; transition: opacity .12s; }\n" +
		"svg path.ply { stroke-width: 1.1; }\n" +
		"svg.quiet path.strand { stroke-width: 1.2; }\n" +
		"@media (hover:hover){\n" + pointedCSS() + " :where(.strand,.ply){opacity:.1}\n}\n"
}

// Fanned says some path of the drawing starts as strands, so its page
// carries the strands' style and key.
func (d Drawing) Fanned() bool { return d.fanned }
