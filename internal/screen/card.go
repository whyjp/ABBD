package screen

import "slices"

// A card is the small key a hand-drawn diagram keeps in a corner of its
// own canvas: a few samples, each of a mark the view draws as the sample
// shows it and few readers would guess, so the drawing reads on its own,
// without the key below the page. A dotted line is listed only where one
// is drawn dotted, not dashed for climbing. It names only what this view draws, and it sits only in room
// nothing else uses, so it never covers a box, chip, line, word or band.
// A view with no such room keeps only the key below it.

// Card is the key as the drawing sets it, its samples top down.
type Card struct {
	X, Y, W, H int
	Items      []CardItem
}

// CardItem is one sample: the kind of mark it shows, its words, and the
// baseline it sits on. Hue is the colour a coloured sample takes.
type CardItem struct {
	Kind, Text, Hue string
	// SX is where the sample starts, TX where the words do, and Y their
	// baseline.
	SX, TX, Y int
}

const (
	cardW, cardRow, cardHead, cardPad = 188, 17, 22, 8
	// cardRoom is how far the card keeps from every other mark.
	cardRoom = 6
)

// card sets the view's key where it has room for it.
func (d *Drawing) card() {
	var items []CardItem
	if l := d.Lead; l != nil {
		items = append(items, CardItem{Kind: "lead", Text: "주 흐름, 번호는 순서", Hue: l.Hue})
	}
	if f := d.Filled(); f != nil {
		items = append(items, CardItem{Kind: "fill", Text: map[string]string{"entry": "채운 상자: 진입점", "hub": "채운 상자: 허브"}[f.Role], Hue: f.Hue})
	}
	if slices.ContainsFunc(d.Wires, func(w Wire) bool { return warns(w.Kind) }) {
		items = append(items, CardItem{Kind: "bypass", Text: "계약 없이 직접 의존 (열린 머리)"})
	}
	if slices.ContainsFunc(d.Wires, func(w Wire) bool { return w.Plan && !w.Back }) {
		items = append(items, CardItem{Kind: "plan", Text: "설계로만 이어짐"})
	}
	if slices.ContainsFunc(d.Wires, func(w Wire) bool { return w.Back }) {
		items = append(items, CardItem{Kind: "back", Text: "거슬러 오르는 선"})
	}
	if len(items) < 2 {
		return
	}
	h := cardHead + len(items)*cardRow + cardPad
	at, ok := d.room(cardW, h)
	if !ok {
		return
	}
	c := &Card{X: at.x, Y: at.y, W: cardW, H: h}
	for i, it := range items {
		it.Y = c.Y + cardHead + i*cardRow + 11
		it.SX, it.TX = c.X+10, c.X+46
		c.Items = append(c.Items, it)
	}
	d.Card = c
}

// room is the first free w by h place in the view: its four corners
// first, then places along an edge within a third of a corner, clear by
// cardRoom of every box,
// chip, line, arrowhead, dot, label, count, number and band, inside the
// frame when the view has one.
func (d *Drawing) room(w, h int) (pt, bool) {
	edge := box{8, 8, d.Width - 16, d.Height - 16}
	if f := d.Frame; f != nil {
		edge = box{f.X + 8, f.Y + 8, f.W - 16, f.H - 16}
	}
	if edge.w < w || edge.h < h {
		return pt{}, false
	}
	free := func(p pt) bool {
		a := grow(box{p.x, p.y, w, h}, cardRoom)
		for _, b := range d.Boxes {
			if hit(a, box{b.X, b.Y, b.W, b.H}) {
				return false
			}
		}
		for _, z := range d.Zones {
			if hit(a, box{z.X, z.Y, z.W, z.H}) {
				return false
			}
		}
		return d.clearOfInk(a, nil) && !d.onBadge(a)
	}
	right, bottom := edge.x+edge.w-w, edge.y+edge.h-h
	for _, p := range []pt{{edge.x, bottom}, {right, bottom}, {right, edge.y}, {edge.x, edge.y}} {
		if free(p) {
			return p, true
		}
	}
	// Then along each edge from its corners inward, up to a third of the
	// way, so the card stays a corner's key and never floats mid-edge.
	reach := func(n int) int { return n / 3 }
	for s := 16; s <= max(reach(right-edge.x), reach(bottom-edge.y)); s += 16 {
		for _, p := range []pt{{edge.x + s, bottom}, {right - s, bottom}, {edge.x, bottom - s}, {right, bottom - s}, {right - s, edge.y}, {edge.x + s, edge.y}, {right, edge.y + s}, {edge.x, edge.y + s}} {
			along := p.y == bottom || p.y == edge.y
			if along && s > reach(right-edge.x) || !along && s > reach(bottom-edge.y) {
				continue
			}
			if free(p) {
				return p, true
			}
		}
	}
	return pt{}, false
}
