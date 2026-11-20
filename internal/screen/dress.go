package screen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Dress is how a box and a line say their role at a glance, the way a
// diagram polished by hand does. A box wears its colour as its border over
// a faint tint of it, and ends in a line in small capitals: its kind, and
// its role where the view gives it one. One box may be filled with its colour:
// the entry the lead starts from, or, in a view with no lead, a hub, the
// box at least half again as many lines touch as any other. A line whose
// relations all rest on a static dependency, a convention, an import, a
// trait, a macro, a build feature or a link, with no call or data between
// the two on record, is drawn dotted, and its tooltip says 정적 의존.

// planned are the ways a static dependency arises: with no call or data on
// record.
var planned = map[model.Via]bool{"convention": true, "import": true, "trait": true, "macro": true, "build-feature": true, "link": true}

const (
	// hubLines is the fewest lines that make a box a hub, and hubOver how
	// many times the next box's lines it needs.
	hubLines = 4
	hubOver  = 1.5
	subPx    = 10
	// planMin is the shortest ink drawn dotted: a dotted stub leaves the
	// pointer only its gaps where other lines cover its dots.
	planMin = 30
)

// planOnly says every one of rels, and at least one, rests on a static
// dependency only.
func (t tree) planOnly(rels []string) bool {
	return len(rels) > 0 && !slices.ContainsFunc(rels, func(r string) bool { return !planned[t.vias[r]] || t.toLibrary[r] })
}

func insides(ins []Inside) []*Drawing {
	var out []*Drawing
	for i := range ins {
		out = append(out, &ins[i].Drawing)
	}
	return out
}

// dress sets the dotted lines, the filled box and every box's last line.
func (d *Drawing) dress(t tree) {
	// A road cuts a dotted line's ink into paths at every split and
	// merge, so the paths of one kind whose lines all arise in design and
	// that meet end to end are weighed together against planMin.
	root := make([]int, len(d.Wires))
	for i := range root {
		root[i] = i
	}
	find := func(i int) int {
		for root[i] != i {
			i = root[i]
		}
		return i
	}
	ends := map[string][]int{}
	for i, w := range d.Wires {
		if !t.planOnly(w.Rels) {
			continue
		}
		for _, r := range runs(w.D) {
			for _, p := range r {
				k := fmt.Sprint(w.Kind, p)
				for _, j := range ends[k] {
					root[find(j)] = find(i)
				}
				ends[k] = append(ends[k], i)
			}
		}
	}
	long := map[int]int{}
	for i, w := range d.Wires {
		if t.planOnly(w.Rels) {
			long[find(i)] += runLength(w.D)
		}
	}
	for i := range d.Wires {
		w := &d.Wires[i]
		w.Plan = t.planOnly(w.Rels) && long[find(i)] >= planMin
		if w.Plan && w.Title != "" {
			w.Title = staticWord + "(호출 근거 없음): " + w.Title
		}
	}
	// A hub's view fills its hub and keeps its lines' weight: its lines
	// rest grey like every other (paint), the hub's own included, and
	// pointing at the hub lights them in its colour.
	if d.Lead == nil {
		d.hub()
	}
	for i := range d.Boxes {
		d.subtitle(&d.Boxes[i])
	}
	for i := range d.Ports {
		p := &d.Ports[i]
		x, y := p.X+p.W/2, p.RectY()+p.H/2
		for _, b := range d.Boxes {
			if b.X <= x && x <= b.X+b.W && b.Y <= y && y <= b.Y+b.H {
				p.OnFill, p.Hue = b.Role != "", b.Hue
			}
		}
	}
}

// hub fills the box at least hubOver times as many lines touch as any
// other, when at least hubLines do, and says whether it found one. A focus
// view centres one box already.
func (d *Drawing) hub() bool {
	if slices.ContainsFunc(d.Boxes, func(b Box) bool { return strings.Contains(" "+b.Class+" ", " focus ") }) {
		return false
	}
	n := map[string]int{}
	for _, r := range d.Relations {
		n[r[0]]++
		n[r[1]]++
	}
	top, next := -1, 0
	for i, b := range d.Boxes {
		if b.Hue == "" {
			continue
		}
		switch c := n[b.ID]; {
		case top < 0 || c > n[d.Boxes[top].ID]:
			if top >= 0 {
				next = n[d.Boxes[top].ID]
			}
			top = i
		case c > next:
			next = c
		}
	}
	if top < 0 || n[d.Boxes[top].ID] < hubLines || float64(n[d.Boxes[top].ID]) < hubOver*float64(next) {
		return false
	}
	d.Boxes[top].Role = "hub"
	return true
}

// classNumbers are the numbers of the classes in cls that are p and digits.
func classNumbers(cls, p string) []int {
	var out []int
	for _, c := range strings.Fields(cls) {
		if n, err := strconv.Atoi(strings.TrimPrefix(c, p)); err == nil && strings.HasPrefix(c, p) {
			out = append(out, n)
		}
	}
	return out
}

// role is what a box is in its view beyond its kind: the build artifact a
// module is, then its place in the view.
func role(b Box) string {
	if p := standing(b); b.Artifact != "" && p != "" {
		return b.Artifact + " · " + p
	} else if b.Artifact != "" {
		return b.Artifact
	}
	return standing(b)
}

// standing is a box's part in its view: its hub, its entry, or outside.
func standing(b Box) string {
	switch {
	case b.Role == "hub":
		return "허브"
	case b.Role == "entry":
		return "진입점"
	case outside(b):
		return "외부"
	}
	return ""
}

// outside says a box stands outside the project: no hub, no entry, and
// drawn in no module's colour.
func outside(b Box) bool { return b.Role != "hub" && b.Role != "entry" && !coloured(b) }

// subtitle sets a box's last line: its kind, and its role. An inside unit
// has its kind already. A module on the overview takes the line only where
// its chips leave room for it below its name.
func (d *Drawing) subtitle(b *Box) {
	kind := kindWord(model.Kind(b.Kind))
	r := role(*b)
	if b.Sub != nil {
		if r != "" && subWidth(b.Sub.S+" · "+r) <= b.W-16 {
			b.Sub.S += " · " + r
		}
		return
	}
	if b.Gist != "" {
		kind = b.Gist
	}
	s := kind
	switch {
	case b.Ambient:
		// A foundation band says it is one, and how many boxes stand on it.
		s = foundationText(b.UsedBy)
	case outside(*b):
		s = r + " · " + kind
	case r != "":
		s += " · " + r
	}
	width := subWidth
	if b.Gist != "" && !b.Ambient {
		width = monoWidth
		if width(s) > b.W-16 {
			s = kind
		}
	}
	if s == "" || len(b.Lines) == 0 || width(s) > b.W-16 {
		return
	}
	top, bottom := b.Y, b.Y+b.H
	for _, p := range d.Ports {
		x := p.X + p.W/2
		if x < b.X || x > b.X+b.W || p.RectY() < b.Y || p.RectY() > b.Y+b.H {
			continue
		}
		if p.Below {
			bottom = min(bottom, p.RectY())
		} else {
			top = max(top, p.RectY()+p.H)
		}
	}
	n := len(b.Lines)
	block := (n-1)*nameLine + 26
	if bottom-top < block+6 {
		return
	}
	y := top + (bottom-top-block)/2 + 10
	for i := range b.Lines {
		b.Lines[i].Y = y + i*nameLine
	}
	b.Sub = &Text{S: s, X: b.Lines[0].X, Y: y + (n-1)*nameLine + 15, Mid: b.Lines[0].Mid}
}

// subWidth is how wide a box's last line is set: small capitals, spaced.
func subWidth(s string) int {
	return textWidth(strings.ToUpper(s), subPx) + len([]rune(s))
}

// Filled is the box the view fills, if any, for the key.
func (d Drawing) Filled() *Box {
	for i, b := range d.Boxes {
		if b.Role != "" {
			return &d.Boxes[i]
		}
	}
	return nil
}

// Planned says the view draws a dotted line, for the key.
func (d Drawing) Planned() bool {
	return slices.ContainsFunc(d.Wires, func(w Wire) bool { return w.Plan })
}

// runLength is how long the path d runs.
func runLength(d string) int {
	n := 0
	for _, r := range runs(d) {
		n += abs(r[1].x-r[0].x) + abs(r[1].y-r[0].y)
	}
	return n
}
