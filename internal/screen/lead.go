package screen

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

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
	Stay                   *Stay `json:",omitempty"`
}

// Stay numbers the steps a stored flow runs on one box of the overview
// after its last hop, or all of them where it never leaves that box, in a
// pill W wide set at X, Y beside the box, clear of every other mark. First
// and Last go on from the hops' numbers, one to a step, and Title names the
// flow and the steps' own numbers in it.
type Stay struct {
	Box         string
	First, Last int
	Title       string
	X, Y, W     int
}

// Text is the pill's words: the first and last number it carries.
func (s Stay) Text() string {
	if s.First == s.Last {
		return strconv.Itoa(s.First)
	}
	return strconv.Itoa(s.First) + "–" + strconv.Itoa(s.Last)
}

// stayH is how tall a stay's pill is.
const stayH = 16

// stayed is the steps of f that run on the box its last hop reached, from
// that step, or, with no hop, on the box of its first step the view shows,
// up to the first step on another box; nil where that is one step after no
// hop.
func (d *Drawing) stayed(t tree, f flowWalk, hops []use) *Stay {
	box := d.shown(t)
	i := 0
	if len(hops) > 0 {
		i = hops[len(hops)-1].step
	}
	for i < len(f.units) && box(f.units[i]) == "" {
		i++
	}
	if i == len(f.units) {
		return nil
	}
	at, k := box(f.units[i]), i+1
	for k < len(f.units) && box(f.units[k]) == at {
		k++
	}
	if len(hops) == 0 && k-i == 1 {
		return nil
	}
	name := at
	for _, b := range d.Boxes {
		if b.ID == at && b.Name != "" {
			name = b.Name
		}
	}
	n := len(hops) + 1
	steps := strconv.Itoa(i + 1)
	if k-i > 1 {
		steps += "–" + strconv.Itoa(k)
	}
	return &Stay{Box: at, First: n, Last: n + k - i - 1, Title: f.question + ": " + steps + "단계가 " + name + " 안에서 이어짐"}
}

// pin sets the stay's pill beside its box: right of its top corner, else
// over it at its right, then left of it, then under it, then in a corner
// inside it off its words, then the nearest spot round it, the first place
// that keeps off every box, line, label, number, word and band title.
func (d *Drawing) pin() {
	if d.Lead == nil || d.Lead.Stay == nil {
		return
	}
	s := d.Lead.Stay
	w := textWidth(s.Text(), 10) + 12
	for _, b := range d.Boxes {
		if b.ID != s.Box {
			continue
		}
		at := func(a box) {
			s.X, s.Y, s.W = a.x, a.y, w
		}
		for _, a := range []box{{b.X + b.W + 5, b.Y + 4, w, stayH}, {b.X + b.W - w - 4, b.Y - stayH - 5, w, stayH}, {b.X + 4, b.Y - stayH - 5, w, stayH}, {b.X - w - 5, b.Y + 4, w, stayH}, {b.X + b.W - w - 4, b.Y + b.H + 5, w, stayH}, {b.X + 4, b.Y + b.H + 5, w, stayH}} {
			if d.wordClear(grow(a, inkGap), nil) {
				at(a)
				return
			}
		}
		words := b.words()
		for _, a := range []box{{b.X + b.W - w - 5, b.Y + 5, w, stayH}, {b.X + 5, b.Y + 5, w, stayH}, {b.X + b.W - w - 5, b.Y + b.H - stayH - 5, w, stayH}, {b.X + 5, b.Y + b.H - stayH - 5, w, stayH}} {
			if d.wordClearOf(grow(a, inkGap), words, b.ID) {
				at(a)
				return
			}
		}
		for g := 5; g <= stayReach; g += 4 {
			for _, a := range ring(box{b.X, b.Y, b.W, b.H}, w, g) {
				if d.wordClear(grow(a, inkGap), nil) {
					at(a)
					return
				}
			}
		}
		at(box{b.X + b.W - w - 4, b.Y - stayH - 5, w, stayH})
	}
}

// stayReach is the furthest a stay's pill stands off its box.
const stayReach = 160

// ring is the places a pill w wide stands g off b: along its top and its
// bottom edge, then down its right and its left side, every 4px.
func ring(b box, w, g int) []box {
	var out []box
	for x := b.x - w - g; x <= b.x+b.w+g; x += 4 {
		out = append(out, box{x, b.y - stayH - g, w, stayH}, box{x, b.y + b.h + g, w, stayH})
	}
	for y := b.y - stayH - g; y <= b.y+b.h+g; y += 4 {
		out = append(out, box{b.x + b.w + g, y, w, stayH}, box{b.x - w - g, y, w, stayH})
	}
	return out
}

// words is the area each line of the box's name and its last line covers.
func (b Box) words() []box {
	var out []box
	ts := b.Lines
	if b.Sub != nil {
		ts = append(append([]Text(nil), ts...), *b.Sub)
	}
	for _, t := range ts {
		n := textWidth(t.S, 13)
		x := t.X
		if t.Mid {
			x -= n / 2
		}
		out = append(out, box{x, t.Y - 12, n, 15})
	}
	return out
}

// Hop is one numbered line of the lead: the route of a line the view draws,
// its arrowheads, where its number sits, and the classes that light it with
// its route while a reader points at either end.
type Hop struct {
	N       int
	D, Head string
	Tips    []string
	X, Y, R int
	Lit     string
	way     int
	// Word names the stored step the hop arrives at, set at WX, WY beside
	// its number; "" where no step names it or it has no room.
	Word   string
	WX, WY int
}

// leadMin is the fewest hops a lead has: one line is no path.
const leadMin = 2

// flowWalk is a stored flow as a lead reads it: the unit of each step, in
// order, and the word that names each step.
type flowWalk struct {
	key, question string
	units         []model.ID
	words         []string
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
			w.words = append(w.words, stepWord(p.Claim.Does))
		}
		out = append(out, w)
	}
	return out
}

// stepWord is the call a step's does line opens with: up to its first
// bracket, space or comma.
func stepWord(does string) string {
	if i := strings.IndexAny(does, "( ,"); i >= 0 {
		return does[:i]
	}
	return does
}

// use is a line the view draws from one box to another, by the route that
// draws it, weighed by how many stored relations it stands for. A line
// between two boxes that use each other is a use both ways.
type use struct {
	from, to string
	way      int
	weight   int
	// word names the stored step at to, on a lead that follows a flow, and
	// step is that step's place in the flow, from 0.
	word string
	step int
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
		out = append(out, use{from: r.ends[0], to: r.ends[1], way: k, weight: w})
		if r.Both {
			out = append(out, use{from: r.ends[1], to: r.ends[0], way: k, weight: w})
		}
	}
	return out
}

// lead finds and draws the view's lead: a stored flow's where the state
// holds any, else the one the lines show plainly. On the overview (whole)
// a stored flow shows even where it leaves its first box once or never:
// the steps it then runs on its last box are numbered on that box (Stay).
func (d *Drawing) lead(t tree, flows []flowWalk, whole bool) {
	uses := d.uses()
	var hops []use
	var key, title string
	var stay *Stay
	if len(flows) > 0 {
		best := -1
		for k, f := range flows {
			if hs := d.followed(t, f, uses); best < 0 || len(hs) > len(hops) {
				hops, key, title, best = hs, f.key, f.question, k
			}
		}
		if whole && len(hops) < leadMin {
			stay = d.stayed(t, flows[best], hops)
		}
	} else {
		hops = inferred(d.Boxes, uses)
		title = "주 흐름 (추정): 아무도 쓰지 않는 하나뿐인 상자에서, 관계를 가장 많이 실은 선을 따라감"
	}
	if len(hops) < leadMin && stay == nil {
		return
	}
	entry := ""
	if len(hops) > 0 {
		entry = hops[0].from
	} else {
		entry = stay.Box
	}
	l := &Lead{Key: key, Title: title, Entry: entry, Stay: stay}
	for _, b := range d.Boxes {
		if b.ID == l.Entry {
			l.Hue = b.Hue
		}
	}
	if l.Hue == "" {
		l.Hue = hueKey(-1)
	}
	for i, h := range hops {
		r := d.ways[h.way]
		hp := Hop{N: i + 1, D: r.D, Head: r.Head, Tips: r.Tips, way: h.way, Word: h.word}
		if h.way < len(d.Routes) {
			hp.Lit = d.Routes[h.way].Lit
		}
		l.Hops = append(l.Hops, hp)
	}
	d.Lead = l
	// A view past hover's caps has no routes to light, so resting its lines
	// grey would leave no way to see their colours. A lead with no line
	// leaves every line its colour.
	if len(d.Routes) > 0 && len(hops) > 0 {
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
	said := map[string]string{}
	for i, u := range f.units {
		b := box(u)
		if b != "" && i < len(f.words) {
			said[b] = f.words[i]
		}
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
		h := uses[j]
		h.word, h.step = said[h.to], i
		hops = append(hops, h)
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

// number sets each hop's number once the view's boxes carry their last
// lines (dress), so a number keeps off the words they end with. A number
// first takes a place on ink no other hop runs along, so a trunk two hops
// share does not carry one hop's number for both.
func (d *Drawing) number() {
	if d.Lead == nil {
		return
	}
	var taken []pt
	for i := range d.Lead.Hops {
		h := &d.Lead.Hops[i]
		var others [][2]pt
		for j, o := range d.Lead.Hops {
			if j != i {
				others = append(others, runs(o.D)...)
			}
		}
		at, rad := d.badge(runs(h.D), others, taken, d.Lead.Key != "")
		taken = append(taken, at)
		h.X, h.Y, h.R = at.x, at.y, rad
	}
}

// name sets each hop's step word beside its number, once the bands carry
// their titles, so a word keeps off them; a word with no room is dropped.
func (d *Drawing) name() {
	if d.Lead == nil {
		return
	}
	var words []box
	for i := range d.Lead.Hops {
		h := &d.Lead.Hops[i]
		if h.Word == "" {
			continue
		}
		w := textWidth(h.Word, wordPx)
		spots := []box{{h.X + h.R + 3, h.Y - 7, w, 13}, {h.X - h.R - 3 - w, h.Y - 7, w, 13}, {h.X + 2, h.Y - h.R - 15, w, 13}, {h.X + 2, h.Y + h.R + 2, w, 13}, {h.X - 2 - w, h.Y - h.R - 15, w, 13}, {h.X - 2 - w, h.Y + h.R + 2, w, 13}}
		// Failing those, the word sits right over or under its number,
		// slid along from centred until it is clear.
		for s := 0; s <= w/2; s += 2 {
			for _, x := range []int{h.X - w/2 - s, h.X - w/2 + s} {
				spots = append(spots, box{x, h.Y - h.R - 15, w, 13}, box{x, h.Y + h.R + 2, w, 13})
			}
		}
		for _, a := range spots {
			if d.wordClear(a, words) {
				h.WX, h.WY = a.x, a.y+11
				words = append(words, a)
				break
			}
		}
		if h.WX == 0 {
			h.Word = ""
		}
	}
}

// wordPx is the size a step's word sets in beside its number.
const wordPx = 10

// wordClear says a step's word set over area meets no box, line, label,
// number or other step's word, and stands inkGap off every band's title:
// ink, labels and counts by clearOfInk.
func (d *Drawing) wordClear(area box, words []box) bool {
	return d.wordClearOf(area, words, "")
}

// wordClearOf is wordClear inside the box own: it meets no other box.
func (d *Drawing) wordClearOf(area box, words []box, own string) bool {
	for _, b := range d.Boxes {
		if b.ID != own && hit(area, box{b.X - 2, b.Y - 2, b.W + 4, b.H + 4}) {
			return false
		}
	}
	for _, z := range d.Zones {
		if hit(area, grow(z.titleArea(), inkGap)) {
			return false
		}
	}
	for _, w := range words {
		if hit(area, w) {
			return false
		}
	}
	return d.clearOfInk(area, nil) && !d.onBadge(area)
}

// badge is where a hop's number sits: halfway along its way, or on a lead
// that follows a stored flow (arrive) just short of where its way ends, or
// failing that the nearest point to it, by fortieths, off every box, label
// and count, badgeApart from the numbers already set and inkGap from every
// other line, arrowhead, dot and chip; failing that, set up to its radius to
// one side; failing that, the same at the small radius.
func (d *Drawing) badge(way, others [][2]pt, taken []pt, arrive bool) (pt, int) {
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
	free := func(p pt, r int) bool {
		for _, b := range d.Boxes {
			if hit(box{p.x - r, p.y - r, 2 * r, 2 * r}, box{b.X - 2, b.Y - 2, b.W + 4, b.H + 4}) {
				return false
			}
		}
		here := box{p.x - r - 2, p.y - r - 2, 2*r + 4, 2*r + 4}
		for _, w := range d.Wires {
			if w.Label != nil && hit(here, w.Label.area()) || w.Count > 0 && hit(here, box{w.LX - 2, w.LY - 11, 24, 13}) {
				return false
			}
		}
		if crowded(p, taken) {
			return false
		}
		return d.badgeClear(p, r, way)
	}
	shared := func(p pt) bool {
		return slices.ContainsFunc(others, func(o [2]pt) bool { return on(p, o[0], o[1]) || stretchDist(p, o[0], o[1]) < 1 })
	}
	for _, alone := range []bool{true, false} {
		for _, r := range []int{badgeR, badgeSmall} {
			for _, side := range []int{0, -r / 2, r / 2, -r, r, -r - badgeBeside, r + badgeBeside} {
				for k := 0; k <= 36; k++ {
					f := .5 + float64((k+1)/2)*.025*float64(1-2*(k%2))
					if arrive {
						f = 1 - float64(k+1)*.025
					}
					if p := along(f, side); free(p, r) && (!alone || !shared(along(f, 0))) {
						return p, r
					}
				}
			}
		}
	}
	// Where the line has no room at all, the number sits on the edge of a
	// box the line meets, beside where it leaves or arrives, off that
	// box's words and chips and clear of every other mark.
	for _, r := range []int{badgeR, badgeSmall} {
		for _, f := range []float64{0, 1} {
			for _, side := range []int{-r - badgeBeside, r + badgeBeside, -2*r - badgeBeside, 2*r + badgeBeside} {
				// along sets the point off the run at that end, across
				// that run, whichever way it runs.
				p := along(f, side)
				if d.badgeClear(p, r, way) && !d.onWords(p, r) && !crowded(p, taken) {
					return p, r
				}
			}
		}
	}
	return along(.5, 0), badgeR
}

// crowded says a number at p stands within badgeApart of one already set.
func crowded(p pt, taken []pt) bool {
	return slices.ContainsFunc(taken, func(q pt) bool {
		return math.Hypot(float64(p.x-q.x), float64(p.y-q.y)) < badgeApart
	})
}

// badgeApart is the least distance between two numbers' centres, so each
// reads as its own hop's and a lead that doubles back does not stack its
// numbers into one cluster.
const badgeApart = 48

// badgeR is the radius of a hop's number, and badgeSmall the radius it
// takes where its line has no room for it. A badge may stand off its line
// by up to its radius, still touching it, and last of all badgeBeside
// further, beside it the way a label stands beside its line. inkGap is the least room a badge
// or a band's title keeps from any line, arrowhead, dot or chip that is
// not its own.
const badgeR, badgeSmall, badgeBeside, inkGap = 8, 6, 4, 3

// clearOfInk says no ink of the view meets area: no stretch of a line
// except one along own, no arrowhead, dot or chip, and no line's label or
// count. A stroke reaches
// 2px off its centre, an arrowhead 9px back from its tip and 5px to each
// side.
func (d *Drawing) clearOfInk(area box, own [][2]pt) bool {
	for _, w := range d.Wires {
		rs := runs(w.D)
		for _, r := range rs {
			mine := false
			for _, o := range own {
				if overlap(r, o) > 0 {
					mine = true
				}
			}
			if !mine && hit(area, grow(seg{a: r[0], b: r[1]}.bounds(), 2)) {
				return false
			}
		}
		if w.Head != "" && !w.Joins && len(rs) > 0 {
			last := rs[len(rs)-1]
			if hit(area, tipBox(last[1], last[0])) {
				return false
			}
		}
	}
	for _, j := range d.Dots {
		if hit(area, box{j.X - 3, j.Y - 3, 6, 6}) {
			return false
		}
	}
	for _, p := range d.Ports {
		if hit(area, box{p.X, p.RectY(), p.W, p.H}) {
			return false
		}
	}
	for _, w := range d.Wires {
		if w.Label != nil && hit(area, w.Label.area()) || w.Count > 0 && hit(area, countBox(w)) {
			return false
		}
	}
	return true
}

// countBox is the area a line's ×N count covers.
func countBox(w Wire) box {
	return box{w.LX - 2, w.LY - 11, textWidth(fmt.Sprintf("×%d", w.Count), 11) + 4, 13}
}

// badgeClear says a number of radius r at c keeps inkGap from every line
// of the view that does not run along own, every arrowhead, dot and chip,
// measured from its rim. A stroke reaches 1.5px off its centre.
func (d *Drawing) badgeClear(c pt, r int, own [][2]pt) bool {
	need := float64(r + inkGap)
	for _, w := range d.Wires {
		rs := runs(w.D)
		for _, x := range rs {
			mine := false
			for _, o := range own {
				mine = mine || overlap(x, o) > 0
			}
			if !mine && stretchDist(c, x[0], x[1]) < need+1.5 {
				return false
			}
		}
		if w.Head != "" && !w.Joins && len(rs) > 0 && boxDist(c, tipBox(rs[len(rs)-1][1], rs[len(rs)-1][0])) < need {
			return false
		}
	}
	for _, j := range d.Dots {
		if math.Hypot(float64(c.x-j.X), float64(c.y-j.Y)) < need+3 {
			return false
		}
	}
	for _, p := range d.Ports {
		if boxDist(c, box{p.X, p.RectY(), p.W, p.H}) < need {
			return false
		}
	}
	return true
}

// onWords says a number of radius r at c meets a box's name or last
// line, or a line's label or count.
func (d *Drawing) onWords(c pt, r int) bool {
	area := box{c.x - r - 2, c.y - r - 2, 2*r + 4, 2*r + 4}
	for _, b := range d.Boxes {
		for _, l := range append(slices.Clone(b.Lines), func() []Text {
			if b.Sub != nil {
				return []Text{*b.Sub}
			}
			return nil
		}()...) {
			w := textWidth(l.S, 13)
			x := l.X
			if l.Mid {
				x -= w / 2
			}
			if hit(area, box{x, l.Y - 12, w, 15}) {
				return true
			}
		}
	}
	for _, w := range d.Wires {
		if w.Label != nil && hit(area, w.Label.area()) || w.Count > 0 && hit(area, box{w.LX - 2, w.LY - 11, 24, 13}) {
			return true
		}
	}
	return false
}

// stretchDist is how far p lies from the stretch a-b.
func stretchDist(p, a, b pt) float64 {
	ax, ay, bx, by, px, py := float64(a.x), float64(a.y), float64(b.x), float64(b.y), float64(p.x), float64(p.y)
	dx, dy := bx-ax, by-ay
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l))
	}
	return math.Hypot(px-ax-t*dx, py-ay-t*dy)
}

// boxDist is how far p lies from the rectangle r.
func boxDist(p pt, r box) float64 {
	dx := math.Max(0, math.Max(float64(r.x-p.x), float64(p.x-(r.x+r.w))))
	dy := math.Max(0, math.Max(float64(r.y-p.y), float64(p.y-(r.y+r.h))))
	return math.Hypot(dx, dy)
}

// tipBox is the area an arrowhead at tip, pointing away from from, covers.
func tipBox(tip, from pt) box {
	dx, dy := sign(tip.x-from.x), sign(tip.y-from.y)
	back := pt{tip.x - 9*dx, tip.y - 9*dy}
	b := box{min(tip.x, back.x), min(tip.y, back.y), abs(tip.x - back.x), abs(tip.y - back.y)}
	if dx == 0 {
		b.x, b.w = b.x-5, 10
	} else {
		b.y, b.h = b.y-5, 10
	}
	return b
}

// quiet draws every line of a view that has a lead in grey, its heads,
// labels and dots too, a bypass's included, so the lead is the one
// coloured way. The bypass keeps its open head, and pointing at its ink
// lights its route in its box's colour, since each route keeps the colour
// paint gave it. The caller drops the
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
		if d.Wires[i].Label == nil {
			d.payBack(i)
		}
	}
}

// payBack puts back the counts of the buses whose word only wire i's
// label said, now that it is dropped.
func (d *Drawing) payBack(i int) {
	for k := range d.Wires {
		if o := d.Wires[k].owed; o != nil && o.by == i {
			d.Wires[k].Count, d.Wires[k].LX, d.Wires[k].LY, d.Wires[k].owed = o.count, o.lx, o.ly, nil
		}
	}
}
