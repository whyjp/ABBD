package screen

import (
	"fmt"
	"maps"
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
// relations all arise only in design, by a convention, an import, a trait,
// a macro or a build feature, with no call or data between the two at run
// time, is drawn dotted.

// planned are the ways a coupling arises without a call or data at run
// time.
var planned = map[model.Via]bool{"convention": true, "import": true, "trait": true, "macro": true, "build-feature": true}

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

// planOnly says every one of rels, and at least one, arises only in design.
func (t tree) planOnly(rels []string) bool {
	return len(rels) > 0 && !slices.ContainsFunc(rels, func(r string) bool { return !planned[t.vias[r]] })
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
	}
	if d.Lead == nil && d.hub() && 2*d.ownLines() >= len(d.Wires) {
		d.hubLines()
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

// ownLines counts the ink whose every route has the view's filled box at
// one end: the lines a hub's view keeps in colour. A hub's view rests the
// rest grey only when those are half its ink at least, so the view still
// reads at rest; otherwise every line keeps its own box's colour.
func (d *Drawing) ownLines() int {
	f := d.Filled()
	if f == nil {
		return 0
	}
	n := 0
	for _, w := range d.Wires {
		ks := classNumbers(w.Lit, "r")
		if len(ks) > 0 && !slices.ContainsFunc(ks, func(k int) bool { return k >= len(d.Routes) || !slices.Contains(d.Routes[k].ends, f.ID) }) {
			n++
		}
	}
	return n
}

// hubLines rests a hub's view the way a lead's does, except the hub's own
// lines: ink every route along which has the hub at one end keeps the
// hub's colour.
func (d *Drawing) hubLines() {
	f := d.Filled()
	if f == nil || f.Hue == "" || len(d.Routes) == 0 {
		return
	}
	hub, hue := f.ID, f.Hue
	d.quiet()
	used := map[string]bool{}
	for _, h := range d.Heads {
		used[h] = true
	}
	for i := range d.Wires {
		w := &d.Wires[i]
		ks := classNumbers(w.Lit, "r")
		if len(ks) == 0 || slices.ContainsFunc(ks, func(k int) bool { return !slices.Contains(d.Routes[k].ends, hub) }) {
			continue
		}
		w.Hue = hue
		if !w.Joins {
			w.Head = shapes[w.Kind] + "-" + hue
			used[w.Head] = true
		}
	}
	d.Heads = slices.Sorted(maps.Keys(used))
	d.unlabelKin()
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

// role is what a box is in its view beyond its kind.
func role(b Box) string {
	switch {
	case b.Role == "hub":
		return "허브"
	case b.Role == "entry":
		return "진입점"
	case !coloured(b):
		return "외부"
	}
	return ""
}

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
	s := kind
	switch {
	case r == "외부":
		s = r + " · " + kind
	case r != "":
		s += " · " + r
	}
	if s == "" || len(b.Lines) == 0 || subWidth(s) > b.W-16 {
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
	return slices.ContainsFunc(d.Wires, func(w Wire) bool { return w.Plan && !w.Back })
}

// runLength is how long the path d runs.
func runLength(d string) int {
	n := 0
	for _, r := range runs(d) {
		n += abs(r[1].x-r[0].x) + abs(r[1].y-r[0].y)
	}
	return n
}
