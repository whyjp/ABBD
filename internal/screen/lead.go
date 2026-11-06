package screen

import (
	"maps"
	"math"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// The lead is the one way through a view a reader follows first: drawn bold
// in the colour of the box it starts from, its lines numbered in order, that
// box filled, and every other line of the view thin and grey until a shape
// is pointed at. It adds no line. Each hop is the whole way of a line the
// view already draws.
//
// It comes from a stored flow when the state holds one: the flow's steps in
// order, each on the box that shows its unit, for as long as each next box
// is joined to the one before by a line the view draws. A state with no
// flow gets a lead only where the view shows one plainly. The entry is the
// one box that no other box uses and that uses others. From it the lead
// follows the line that stands for strictly the most relations, box by box,
// to a box not yet on it, and stops where two lines tie. A view where
// either reading is unclear, or that gives fewer than two hops, draws none.

// Lead is the way a view draws bold. Key names the stored flow it follows,
// "" when the renderer read it from the lines. Entry is the box it starts
// from, and Hue that box's colour, which the lead is drawn in.
type Lead struct {
	Key, Title, Entry, Hue string
	Hops                   []Hop
}

// Hop is one numbered line of the lead: the route of a line the view draws,
// its arrowheads, where its number sits, and the classes that light it with
// its route while a reader points at either end.
type Hop struct {
	N       int
	D, Head string
	Tips    []string
	X, Y    int
	Lit     string
	way     int
}

// leadMin is the fewest hops a lead has: one line is no path.
const leadMin = 2

// flowWalk is a stored flow as a lead reads it: the unit of each step, in order.
type flowWalk struct {
	key, question string
	units         []model.ID
}

func walks(in Input) []flowWalk {
	var out []flowWalk
	for _, f := range in.Figures.Flows {
		if !f.Known {
			continue
		}
		w := flowWalk{key: string(f.Key), question: f.Question}
		for _, p := range f.Parts {
			w.units = append(w.units, p.Claim.Unit)
		}
		out = append(out, w)
	}
	return out
}

// use is a line the view draws from one box to another, by the route that
// draws it, weighed by how many stored relations it stands for. A line
// between two boxes that use each other is a use both ways.
type use struct {
	from, to string
	way      int
	weight   int
}

func (d *Drawing) uses() []use {
	boxes := map[string]bool{}
	for _, b := range d.Boxes {
		boxes[b.ID] = true
	}
	var out []use
	for k, r := range d.ways {
		if r.Kind == "provides" || len(r.ends) != 2 || !boxes[r.ends[0]] || !boxes[r.ends[1]] {
			continue
		}
		w := len(slices.Compact(slices.Sorted(slices.Values(r.rels))))
		out = append(out, use{r.ends[0], r.ends[1], k, w})
		if r.Both {
			out = append(out, use{r.ends[1], r.ends[0], k, w})
		}
	}
	return out
}

// lead finds and draws the view's lead: a stored flow's where the state
// holds any, else the one the lines show plainly.
func (d *Drawing) lead(t tree, flows []flowWalk) {
	uses := d.uses()
	var hops []use
	var key, title string
	if len(flows) > 0 {
		for _, f := range flows {
			if hs := d.followed(t, f, uses); len(hs) > len(hops) {
				hops, key, title = hs, f.key, f.question
			}
		}
	} else {
		hops = inferred(d.Boxes, uses)
		title = "주 흐름 (추정): 아무도 쓰지 않는 하나뿐인 상자에서, 관계를 가장 많이 실은 선을 따라감"
	}
	if len(hops) < leadMin {
		return
	}
	l := &Lead{Key: key, Title: title, Entry: hops[0].from}
	for _, b := range d.Boxes {
		if b.ID == l.Entry {
			l.Hue = b.Hue
		}
	}
	if l.Hue == "" {
		l.Hue = hueKey(-1)
	}
	var taken []pt
	for i, h := range hops {
		r := d.ways[h.way]
		hp := Hop{N: i + 1, D: r.D, Head: r.Head, Tips: r.Tips, way: h.way}
		if h.way < len(d.Routes) {
			hp.Lit = d.Routes[h.way].Lit
		}
		at := d.badge(runs(r.D), taken)
		taken = append(taken, at)
		hp.X, hp.Y = at.x, at.y
		l.Hops = append(l.Hops, hp)
	}
	d.Lead = l
	// A view past hover's caps has no routes to light, so resting its lines
	// grey would leave no way to see their colours.
	if len(d.Routes) > 0 {
		d.quiet()
		d.unlabelKin()
	}
	for i := range d.Boxes {
		if d.Boxes[i].ID == l.Entry && d.Boxes[i].Hue != "" {
			d.Boxes[i].Role = "entry"
		}
	}
}

// followed is how far the flow f runs on the view, from its first step
// whose unit the view shows. A step on a box already on the lead adds no
// hop: the run returns there. A step on a new box joins it by the line from
// the box on the lead that ran a step most recently and uses it, or, when
// no box on the lead uses it, by its own line to the box of the step
// before, as an entry that calls the first step's box does. With neither,
// or at a step the view does not show, the lead stops.
func (d *Drawing) followed(t tree, f flowWalk, uses []use) []use {
	boxes := map[string]bool{}
	for _, b := range d.Boxes {
		boxes[b.ID] = true
	}
	box := func(u model.ID) string {
		c := t.chain(u)
		for i := len(c) - 1; i >= 0; i-- {
			if boxes[string(c[i])] {
				return string(c[i])
			}
		}
		return ""
	}
	var hops []use
	var on []string
	for _, u := range f.units {
		b := box(u)
		if b == "" {
			if len(on) == 0 {
				continue
			}
			break
		}
		if k := slices.Index(on, b); k >= 0 {
			on = append(slices.Delete(on, k, k+1), b)
			continue
		}
		if len(on) == 0 {
			on = []string{b}
			continue
		}
		j := -1
		for k := len(on) - 1; k >= 0 && j < 0; k-- {
			j = slices.IndexFunc(uses, func(x use) bool { return x.from == on[k] && x.to == b })
		}
		if j < 0 {
			j = slices.IndexFunc(uses, func(x use) bool { return x.from == b && x.to == on[len(on)-1] })
		}
		if j < 0 {
			break
		}
		hops = append(hops, uses[j])
		on = append(on, b)
	}
	return hops
}

// inferred is the lead the lines show plainly: from the one box no other
// box uses and that uses others, along the line with strictly the most
// relations to a box not yet on the lead, until none or two tie.
func inferred(boxes []Box, uses []use) []use {
	in, out := map[string]int{}, map[string]int{}
	for _, u := range uses {
		in[u.to]++
		out[u.from]++
	}
	var entries []string
	for _, b := range boxes {
		if in[b.ID] == 0 && out[b.ID] > 0 {
			entries = append(entries, b.ID)
		}
	}
	if len(entries) != 1 {
		return nil
	}
	at, on := entries[0], map[string]bool{entries[0]: true}
	var hops []use
	for {
		best, tie := -1, false
		for j, u := range uses {
			if u.from != at || on[u.to] {
				continue
			}
			switch {
			case best < 0 || u.weight > uses[best].weight:
				best, tie = j, false
			case u.weight == uses[best].weight:
				tie = true
			}
		}
		if best < 0 || tie {
			return hops
		}
		hops = append(hops, uses[best])
		at = uses[best].to
		on[at] = true
	}
}

// badge is where a hop's number sits: halfway along its way, or failing
// that the nearest point to it, by twentieths, off every box, label and
// count and clear of the numbers already set; failing that, set a little
// to one side.
func (d *Drawing) badge(way [][2]pt, taken []pt) pt {
	total := 0
	for _, r := range way {
		total += abs(r[1].x-r[0].x) + abs(r[1].y-r[0].y)
	}
	// along is the point a share f of the way along, set off it by
	// side pixels across the stretch it lies on.
	along := func(f float64, side int) pt {
		left := int(math.Round(f * float64(total)))
		for _, r := range way {
			n := abs(r[1].x-r[0].x) + abs(r[1].y-r[0].y)
			if left <= n && n > 0 {
				p := pt{r[0].x + (r[1].x-r[0].x)*left/n, r[0].y + (r[1].y-r[0].y)*left/n}
				if r[0].x == r[1].x {
					p.x += side
				} else {
					p.y += side
				}
				return p
			}
			left -= n
		}
		if len(way) == 0 {
			return pt{}
		}
		return way[len(way)-1][1]
	}
	free := func(p pt) bool {
		for _, b := range d.Boxes {
			if hit(box{p.x - badgeR, p.y - badgeR, 2 * badgeR, 2 * badgeR}, box{b.X - 2, b.Y - 2, b.W + 4, b.H + 4}) {
				return false
			}
		}
		here := box{p.x - badgeR - 2, p.y - badgeR - 2, 2*badgeR + 4, 2*badgeR + 4}
		for _, w := range d.Wires {
			if w.Label != nil && hit(here, w.Label.area()) || w.Count > 0 && hit(here, box{w.LX - 2, w.LY - 11, 24, 13}) {
				return false
			}
		}
		for _, q := range taken {
			if abs(p.x-q.x) < 3*badgeR && abs(p.y-q.y) < 3*badgeR {
				return false
			}
		}
		return true
	}
	for _, side := range []int{0, -badgeShift, badgeShift} {
		for k := 0; k <= 16; k++ {
			f := .5 + float64((k+1)/2)*.05*float64(1-2*(k%2))
			if p := along(f, side); free(p) {
				return p
			}
		}
	}
	return along(.5, 0)
}

// badgeR is the radius of a hop's number, and badgeShift how far it may
// stand off its line, still on it, where its line has no room.
const badgeR, badgeShift = 8, 6

// quiet draws every line of a view that has a lead in grey, its heads,
// labels and dots too, a bypass's included, so the lead is the one
// coloured way: three blind judges preferred that 3 to 0 over keeping the
// bypass orange. The bypass keeps its open head, and pointing at its ink
// lights its route in orange, since each route keeps the colour paint gave
// it. The caller drops the
// labels the grey leaves over another line's track (unlabelKin).
func (d *Drawing) quiet() {
	d.Quiet = true
	n := hueKey(-1)
	used := map[string]bool{}
	for i := range d.Wires {
		w := &d.Wires[i]
		w.Hue = n
		if !w.Joins {
			w.Head = shapes[w.Kind] + "-" + w.Hue
			used[w.Head] = true
		}
	}
	for i := range d.Dots {
		d.Dots[i].Hue = n
	}
	d.Heads = slices.Sorted(maps.Keys(used))
}

// unlabelKin drops a label that, once the view is quiet, stands right over
// a flat track of another line of its colour, where it would read as that
// line's. Labels were set while the lines wore their boxes' colours. The
// words stay in the line's tooltip.
func (d *Drawing) unlabelKin() {
	for i := range d.Wires {
		l := d.Wires[i].Label
		if l == nil {
			continue
		}
		lb := l.area()
		for j, o := range d.Wires {
			if j == i || o.Hue != d.Wires[i].Hue || o.Net != 0 && o.Net == d.Wires[i].Net {
				continue
			}
			for _, r := range runs(o.D) {
				if r[0].y != r[1].y || max(r[0].x, r[1].x) <= lb.x || lb.x+lb.w <= min(r[0].x, r[1].x) {
					continue
				}
				if abs(lb.y+lb.h-r[0].y) <= labelOff+2 || abs(lb.y-r[0].y) <= labelOff+2 {
					d.Wires[i].Label = nil
				}
			}
		}
	}
}
