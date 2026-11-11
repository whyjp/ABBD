package screen

import (
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A zone is a tinted band behind the boxes of one grouping the project
// itself shows, titled in small capitals, so a reader sees which boxes
// answer for one part before reading a line. It is a background and never
// a box: it holds no line, takes no pointer and moves nothing. The
// groupings come from the state alone, never from the drawing. A stored
// stack's layers come first, each around the most of its boxes that sit
// together when that is two boxes or more, numbered from the top among the
// bands drawn, so the numbers run without a gap. Without such a
// stack, boxes whose code sits in one directory share that directory's
// band. Contracts
// with systems outside, drawn as boxes because nothing in the repository
// provides them, share one more band, even when there is one, so a lone
// outside contract reads as outside rather than as a stray box. A band is drawn only where its boxes
// sit together: its area, with room for its title, holds no other box or
// chip and meets no other band. Where they are apart the band is left out,
// and the boxes read as they would without it.

// Zone is one band as the page draws it. N is a stored layer's place from
// the top among the layers drawn, 0 for any other grouping. Hue is the
// palette key its tint and title take: the colour most of its boxes wear.
type Zone struct {
	N          int
	Title, Hue string
	X, Y, W, H int
	TX, TY     int
	// Lines are the title set on two lines, where one line has no room
	// clear of ink; empty when it fits on one.
	Lines []string `json:",omitempty"`
}

// titleArea is the room the band's title takes as set.
func (z Zone) titleArea() box {
	lines := z.Lines
	if len(lines) == 0 {
		lines = []string{z.Label()}
	}
	return box{z.TX - 2, z.TY - 10, linesWidth(lines) + 4, 13 + (len(lines)-1)*zoneLine}
}

// Label is the band's title as set: its layer's number, then its name.
func (z Zone) Label() string {
	if z.N > 0 {
		return strconv.Itoa(z.N) + " · " + z.Title
	}
	return z.Title
}

const (
	// zoneSide and zoneFoot are how far a band reaches past its boxes,
	// and zoneHead how far above them, where its title sits.
	zoneSide, zoneFoot, zoneHead = 8, 8, 20
	// zoneMore is how much further a band reaches above or below its
	// boxes when no place in its usual room keeps its title clear of ink.
	zoneMore = 12
	// zoneLine is how far a title's second line sits below its first.
	zoneLine = 12
	zonePx   = 10
	// zoneMono is how wide a Latin letter of a title sets in the 10px
	// monospace the page uses, and zoneTrack the 0.14em it spaces them by,
	// so a title is measured as the page draws it.
	zoneMono, zoneTrack = 6.0, 1.4
)

// layering is a stored stack as zones read it: each layer's units, top down.
type layering struct {
	names []string
	units [][]model.ID
}

func layerings(in Input) []layering {
	var out []layering
	for _, s := range in.Figures.Stacks {
		if !s.Known {
			continue
		}
		var l layering
		for _, p := range s.Parts {
			l.names = append(l.names, p.Claim.Name)
			l.units = append(l.units, p.Claim.Units)
		}
		out = append(out, l)
	}
	return out
}

// group is one grouping's boxes before it is drawn.
type group struct {
	n     int
	title string
	ids   []string
}

// zone draws the bands of d.
func (d *Drawing) zone(t tree, stacks []layering) {
	var groups []group
	if g := d.layered(t, stacks); len(g) > 0 {
		groups = g
	} else {
		groups = d.folders(t)
	}
	var outside []string
	for _, b := range d.Boxes {
		if !coloured(b) {
			outside = append(outside, b.ID)
		}
	}
	if len(outside) >= 1 && len(outside) < len(d.Boxes) {
		groups = append(groups, group{title: "외부 계약", ids: outside})
	}
	n := 0
	for _, g := range groups {
		if z, ok := d.band(g); ok {
			if z.N > 0 {
				n++
				z.N = n
			}
			d.Zones = append(d.Zones, z)
		}
	}
}

// layered is the tiers of the stored stack that covers the most of d's
// boxes, each the most of its boxes that sit together when that is two
// boxes or more (stackOf).
func (d *Drawing) layered(t tree, stacks []layering) []group {
	var out []group
	for _, g := range stackOf(stacks, d.shown(t), d.together) {
		if len(g.ids) >= 2 {
			out = append(out, g)
		}
	}
	return out
}

// tiers are s's layers as the boxes that show them, top down, leaving out
// a layer no box shows. A box shown in two layers stays in the first.
func (s layering) tiers(box func(model.ID) string) []group {
	var out []group
	seen := map[string]bool{}
	for i, us := range s.units {
		g := group{n: i + 1, title: s.names[i]}
		for _, u := range us {
			if b := box(u); b != "" && !seen[b] {
				seen[b] = true
				g.ids = append(g.ids, b)
			}
		}
		if len(g.ids) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// stackOf is the tiers of the stored stack that covers the most boxes in
// tiers of two boxes or more, when two of its tiers at least hold two, each
// tier's boxes as fit keeps them. The layout keeps them all, and a band
// only those that sit together.
func stackOf(stacks []layering, box func(model.ID) string, fit func([]string) []string) []group {
	var best []group
	most := 0
	for _, s := range stacks {
		ts := s.tiers(box)
		k, n := 0, 0
		for i := range ts {
			if ts[i].ids = fit(ts[i].ids); len(ts[i].ids) >= 2 {
				k++
				n += len(ts[i].ids)
			}
		}
		if k >= 2 && n > most {
			best, most = ts, n
		}
	}
	return best
}

// together is the largest run of ids whose band, at its narrowest, holds
// no other box: taken in reading order, each box joins the run before it
// when the band around both stays clear, and starts a run when it does not.
func (d *Drawing) together(ids []string) []string {
	var bs []Box
	for _, b := range d.Boxes {
		if slices.Contains(ids, b.ID) {
			bs = append(bs, b)
		}
	}
	slices.SortStableFunc(bs, func(a, b Box) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})
	var best, run []Box
	for _, b := range bs {
		if run = append(run, b); !d.sitTogether(run) {
			run = []Box{b}
		}
		if len(run) > len(best) {
			best = slices.Clone(run)
		}
	}
	var out []string
	for _, b := range best {
		out = append(out, b.ID)
	}
	return out
}

// sitTogether says a band around bs, with no room for a title yet, stands
// clear of every other box.
func (d *Drawing) sitTogether(bs []Box) bool {
	in := map[string]bool{}
	lo, hi := pt{1 << 30, 1 << 30}, pt{-1 << 30, -1 << 30}
	for _, b := range bs {
		in[b.ID] = true
		lo, hi = pt{min(lo.x, b.X), min(lo.y, b.Y)}, pt{max(hi.x, b.X+b.W), max(hi.y, b.Y+b.H)}
	}
	return d.clear(Zone{X: lo.x - zoneSide, Y: lo.y - zoneFoot, W: hi.x - lo.x + 2*zoneSide, H: hi.y - lo.y + 2*zoneFoot}, in)
}

// folders groups d's boxes by the directory that holds each one's own
// directory, the folder its code sits in. A folder that holds every box,
// or one box only, says nothing the view does not.
func (d *Drawing) folders(t tree) []group {
	at := map[string][]string{}
	var keys []string
	for _, b := range d.Boxes {
		if !coloured(b) {
			continue
		}
		u, ok := t.units[base(model.ID(b.ID))]
		if !ok {
			continue
		}
		dir := path.Dir(home(u))
		if dir == "." || dir == "/" || dir == "" {
			continue
		}
		if _, seen := at[dir]; !seen {
			keys = append(keys, dir)
		}
		at[dir] = append(at[dir], b.ID)
	}
	var out []group
	for _, k := range keys {
		if n := len(at[k]); n >= 2 && n < len(d.Boxes) {
			out = append(out, group{title: k + "/", ids: at[k]})
		}
	}
	return out
}

// home is the directory a unit's code sits in: the deepest directory that
// holds every file it is anchored to, "." when they share none.
func home(u model.Unit) string {
	var dirs []string
	for _, a := range u.Anchors {
		p := strings.TrimSuffix(a.Path, "/")
		if path.Ext(p) != "" || strings.Contains(path.Base(p), ".") {
			p = path.Dir(p)
		}
		dirs = append(dirs, p)
	}
	if len(dirs) == 0 {
		return "."
	}
	common := strings.Split(dirs[0], "/")
	for _, d := range dirs[1:] {
		parts := strings.Split(d, "/")
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	if len(common) == 0 {
		return "."
	}
	return strings.Join(common, "/")
}

// shown maps a unit to the box that shows it in d: its own, or its
// nearest ancestor's.
func (d *Drawing) shown(t tree) func(model.ID) string {
	boxes := map[string]bool{}
	for _, b := range d.Boxes {
		boxes[b.ID] = true
	}
	return func(u model.ID) string {
		c := t.chain(u)
		for i := len(c) - 1; i >= 0; i-- {
			if boxes[string(c[i])] {
				return string(c[i])
			}
		}
		return ""
	}
}

// band is g's zone, when its boxes sit together: around them, with room
// above or below for the title, on the canvas and inside the frame, clear
// of every other box and chip and of the bands already set. The title
// takes the first place that keeps inkGap from every line, arrowhead,
// dot, chip, label, count and number. It tries the usual room above first,
// its left corner, its right, then along its top, so titles read along
// the tops of their bands. Then the usual room's corner the fewest of
// those cross, then the rest of that room, then the other usual room,
// then a room zoneMore taller, on two lines. With none, it keeps the
// corner the fewest cross.
func (d *Drawing) band(g group) (Zone, bool) {
	in := map[string]bool{}
	for _, id := range g.ids {
		in[id] = true
	}
	lo, hi := pt{1 << 30, 1 << 30}, pt{-1 << 30, -1 << 30}
	hues := map[string]int{}
	for _, b := range d.Boxes {
		if in[b.ID] {
			lo, hi = pt{min(lo.x, b.X), min(lo.y, b.Y)}, pt{max(hi.x, b.X+b.W), max(hi.y, b.Y+b.H)}
			hues[b.Hue]++
		}
	}
	type spot struct {
		z      Zone
		lines  []string
		x, y   int
		corner bool
		// top says the title sits along the top of the usual room above.
		top bool
	}
	// The spots in the order they are tried: the four corners of the
	// usual room, then every place along its top and its bottom, then the
	// same in a taller room, where the title takes two lines.
	var spots []spot
	for _, more := range []int{0, zoneMore} {
		var rooms []spot
		for _, below := range []bool{false, true} {
			head, foot := zoneHead+more, zoneFoot
			if below {
				head, foot = zoneFoot, zoneHead+more
			}
			z := Zone{N: g.n, Title: g.title, X: lo.x - zoneSide, Y: lo.y - head, W: hi.x - lo.x + 2*zoneSide, H: hi.y - lo.y + head + foot}
			if !d.clear(z, in) {
				continue
			}
			lines := []string{z.Label()}
			if more > 0 {
				lines = halves(z.Label())
			}
			y := z.Y + 14
			if below {
				y = z.Y + z.H - 6 - (len(lines)-1)*zoneLine
			}
			rooms = append(rooms, spot{z: z, lines: lines, y: y})
		}
		for _, r := range rooms {
			w := linesWidth(r.lines)
			for _, x := range []int{r.z.X + zoneSide + 2, r.z.X + r.z.W - zoneSide - 2 - w} {
				if x >= r.z.X+zoneSide {
					spots = append(spots, spot{r.z, r.lines, x, r.y, more == 0, more == 0 && r.y == r.z.Y+14})
				}
			}
		}
		for _, r := range rooms {
			w := linesWidth(r.lines)
			for x := r.z.X + zoneSide + 2; x <= r.z.X+r.z.W-zoneSide-2-w; x += 4 {
				spots = append(spots, spot{r.z, r.lines, x, r.y, false, more == 0 && r.y == r.z.Y+14})
			}
		}
	}
	if len(spots) == 0 {
		return Zone{}, false
	}
	set := func(s spot) Zone {
		z := s.z
		z.TX, z.TY = s.x, s.y
		if len(s.lines) > 1 {
			z.Lines = s.lines
		}
		return z
	}
	area := func(s spot) box { return set(s).titleArea() }
	// main is, of the usual room's corners, the one the fewest lines,
	// labels and counts cross. Past the top of the room above it is tried
	// first, then the rest of its room, so a band keeps the shape its
	// least crossed corner gives it.
	main, cost := -1, -1
	for k, sp := range spots {
		if c := d.crossed(area(sp)); sp.corner && (cost < 0 || c < cost) {
			main, cost = k, c
		}
	}
	if main < 0 {
		main = 0
	}
	order := []int{main}
	for k, sp := range spots {
		if k != main && sameRoom(sp.z, spots[main].z) {
			order = append(order, k)
		}
	}
	for k, sp := range spots {
		if !sameRoom(sp.z, spots[main].z) {
			order = append(order, k)
		}
	}
	var top []int
	for _, corner := range []bool{true, false} {
		for k, sp := range spots {
			if sp.top && sp.corner == corner {
				top = append(top, k)
			}
		}
	}
	best := set(spots[main])
	for _, k := range append(top, order...) {
		if a := grow(area(spots[k]), inkGap); d.clearOfInk(a, nil) && !d.onBadge(a) && d.crossed(a) == 0 {
			best = set(spots[k])
			break
		}
	}
	n := -1
	for h, k := range hues {
		if k > n || k == n && h < best.Hue {
			best.Hue, n = h, k
		}
	}
	if best.Hue == "" {
		best.Hue = hueKey(-1)
	}
	return best, true
}

// clear says the band z stands on the canvas, inside the frame, and off
// every box but its own, every frame chip and every band already set.
func (d *Drawing) clear(z Zone, in map[string]bool) bool {
	area := box{z.X, z.Y, z.W, z.H}
	edge := box{0, 0, d.Width, d.Height}
	if f := d.Frame; f != nil {
		edge = box{f.X + 2, f.Y + 2, f.W - 4, f.H - 4}
		for _, p := range d.Ports {
			if hit(area, box{p.X, p.RectY(), p.W, p.H}) {
				return false
			}
		}
	}
	if area.x < edge.x || area.y < edge.y || area.x+area.w > edge.x+edge.w || area.y+area.h > edge.y+edge.h {
		return false
	}
	for _, b := range d.Boxes {
		if !in[b.ID] && hit(area, box{b.X - 2, b.Y - 2, b.W + 4, b.H + 4}) {
			return false
		}
	}
	for _, o := range d.Zones {
		if hit(area, box{o.X - 2, o.Y - 2, o.W + 4, o.H + 4}) {
			return false
		}
	}
	return true
}

// crossed is how many stretches of line cross the area, with each label
// and count on it weighing three.
func (d *Drawing) crossed(area box) int {
	n := 0
	for _, w := range d.Wires {
		for _, r := range runs(w.D) {
			if hit(area, seg{a: r[0], b: r[1]}.bounds()) {
				n++
			}
		}
		if w.Label != nil && hit(area, w.Label.area()) {
			n += 3
		}
		if w.Count > 0 && hit(area, box{w.LX - 2, w.LY - 11, 24, 13}) {
			n += 3
		}
	}
	return n
}

// sameRoom says two bands are one band, the same rectangle.
func sameRoom(a, b Zone) bool { return a.X == b.X && a.Y == b.Y && a.W == b.W && a.H == b.H }

// linesWidth is how wide a title set on lines is.
func linesWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, zoneWidth(l))
	}
	return w
}

// halves sets s on two lines, broken at the space nearest its middle, or
// leaves it whole when it has no space.
func halves(s string) []string {
	r := []rune(s)
	best := -1
	for i, c := range r {
		if c == ' ' && (best < 0 || abs(i-len(r)/2) < abs(best-len(r)/2)) {
			best = i
		}
	}
	if best <= 0 {
		return []string{s}
	}
	return []string{strings.TrimSpace(string(r[:best])), strings.TrimSpace(string(r[best:]))}
}

// zoneWidth is how wide a title is set: monospace capitals spaced apart,
// a wide letter such as Hangul taking a full em.
func zoneWidth(s string) int {
	w := 0.0
	for _, r := range s {
		if r < 0x80 {
			w += zoneMono
		} else {
			w += zonePx
		}
		w += zoneTrack
	}
	return int(math.Ceil(w))
}

// onBadge says area meets a number of the view's lead.
func (d *Drawing) onBadge(area box) bool {
	if d.Lead == nil {
		return false
	}
	for _, h := range d.Lead.Hops {
		if hit(area, box{h.X - h.R, h.Y - h.R, 2 * h.R, 2 * h.R}) {
			return true
		}
	}
	return false
}
