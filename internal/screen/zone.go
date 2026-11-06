package screen

import (
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
// stack's layers come first, numbered from the top. Without one, boxes
// whose code sits in one directory share that directory's band. Contracts
// with systems outside, drawn as boxes because nothing in the repository
// provides them, share one more band. A band is drawn only where its boxes
// sit together: its area, with room for its title, holds no other box or
// chip and meets no other band. Where they are apart the band is left out,
// and the boxes read as they would without it.

// Zone is one band as the page draws it. N is a stored layer's place from
// the top, 0 for any other grouping. Hue is the palette key its tint and
// title take: the colour most of its boxes wear.
type Zone struct {
	N          int
	Title, Hue string
	X, Y, W, H int
	TX, TY     int
	// Lines are the title set on two lines, where one line has no room
	// clear of ink; empty when it fits on one.
	Lines []string `json:",omitempty"`
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
	if len(outside) >= 2 && len(outside) < len(d.Boxes) {
		groups = append(groups, group{title: "외부 계약", ids: outside})
	}
	for _, g := range groups {
		if z, ok := d.band(g); ok {
			d.Zones = append(d.Zones, z)
		}
	}
}

// layered reads the stored stack that covers the most of d's boxes, when
// it covers two of its layers at least.
func (d *Drawing) layered(t tree, stacks []layering) []group {
	box := d.shown(t)
	var best []group
	most := 0
	for _, s := range stacks {
		var gs []group
		n := 0
		for i, us := range s.units {
			g := group{n: i + 1, title: s.names[i]}
			for _, u := range us {
				if b := box(u); b != "" && !slices.Contains(g.ids, b) {
					g.ids = append(g.ids, b)
				}
			}
			if len(g.ids) > 0 {
				gs = append(gs, g)
				n += len(g.ids)
			}
		}
		if len(gs) >= 2 && n > most {
			best, most = gs, n
		}
	}
	return best
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
// takes the first place along the top, then the bottom, that keeps inkGap
// from every line, arrowhead, dot, chip and number; with none, the band
// reaches zoneMore further and sets its title on two lines, and failing
// that the title
// takes the place fewest lines, labels and counts cross.
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
	best, cost := Zone{}, -1
	plain, plainCost := Zone{}, -1
	for _, try := range []struct {
		below bool
		more  int
	}{{false, 0}, {true, 0}, {false, zoneMore}, {true, zoneMore}} {
		if cost == 0 {
			break
		}
		below := try.below
		head, foot := zoneHead+try.more, zoneFoot
		if below {
			head, foot = zoneFoot, zoneHead+try.more
		}
		z := Zone{N: g.n, Title: g.title, X: lo.x - zoneSide, Y: lo.y - head, W: hi.x - lo.x + 2*zoneSide, H: hi.y - lo.y + head + foot}
		if !d.clear(z, in) {
			continue
		}
		lines := []string{z.Label()}
		if try.more > 0 {
			lines = halves(z.Label())
		}
		w := 0
		for _, l := range lines {
			w = max(w, zoneWidth(l))
		}
		tall := 13 + (len(lines)-1)*zoneLine
		y := z.Y + 14
		if below {
			y = z.Y + z.H - 6 - (len(lines)-1)*zoneLine
		}
		for x := z.X + zoneSide + 2; x == z.X+zoneSide+2 || x <= z.X+z.W-zoneSide-2-w; x += 4 {
			area := box{x - 2, y - 10, w + 4, tall}
			c := 1000 + d.crossed(area)
			if d.clearOfInk(grow(area, inkGap), nil) && !d.onBadge(grow(area, inkGap)) {
				c = 0
			}
			if cost < 0 || c < cost {
				best, cost = z, c
				best.TX, best.TY = x, y
				if len(lines) > 1 {
					best.Lines = lines
				}
			}
			if try.more == 0 && (plainCost < 0 || c < plainCost) {
				plain, plainCost = z, c
				plain.TX, plain.TY = x, y
			}
		}
	}
	if cost < 0 {
		return Zone{}, false
	}
	// A band that reaching further does not clear keeps its usual room.
	if cost > 0 && plainCost >= 0 {
		best = plain
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

// zoneWidth is how wide a title is set: small capitals spaced apart.
func zoneWidth(s string) int {
	return textWidth(strings.ToUpper(s), zonePx) + len([]rune(s))*2
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
