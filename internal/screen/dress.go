package screen

import (
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Dress is how a box and a line say their role at a glance, the way a
// diagram polished by hand does. A box is white with its colour in a bar
// down its left edge and a last line in small capitals: its kind, and its
// role where the view gives it one. One box may be filled with its colour:
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

func insides(ins []Inside) []*Drawing {
	var out []*Drawing
	for i := range ins {
		out = append(out, &ins[i].Drawing)
	}
	return out
}

// dress sets the dotted lines, the filled box and every box's last line.
func (d *Drawing) dress(t tree) {
	for i := range d.Wires {
		w := &d.Wires[i]
		w.Plan = len(w.Rels) > 0 && runLength(w.D) >= planMin && !slices.ContainsFunc(w.Rels, func(r string) bool { return !planned[t.vias[r]] })
	}
	if d.Lead == nil {
		d.hub()
	}
	for i := range d.Boxes {
		d.subtitle(&d.Boxes[i])
	}
}

// hub fills the box at least hubOver times as many lines touch as any
// other, when at least hubLines do. A focus view centres one box already.
func (d *Drawing) hub() {
	if slices.ContainsFunc(d.Boxes, func(b Box) bool { return strings.Contains(" "+b.Class+" ", " focus ") }) {
		return
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
		return
	}
	d.Boxes[top].Class += " lead hub"
}

// role is what a box is in its view beyond its kind.
func role(b Box) string {
	switch c := " " + b.Class + " "; {
	case strings.Contains(c, " hub "):
		return "허브"
	case strings.Contains(c, " lead "):
		return "진입점"
	case strings.Contains(c, " loose "):
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
		if strings.Contains(" "+b.Class+" ", " lead ") {
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
