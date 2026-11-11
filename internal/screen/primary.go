package screen

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// An overview whose boxes use each other many times over reads as a tangle
// however it is routed: the exemplars draw one to two lines per box. So the
// overview draws its primary links and counts the rest on their boxes, and
// each box's focus view (focus.go) still draws every line it has.
//
// What is primary is decided once, by primary, from what the store already
// says: the relations a stored flow's steps ride, the relations between a
// stored stack's neighbouring layers, and the relations a reader or a human
// marked (Input.Primary), and the lines of the lead the overview would
// draw (lead.go). Past those, links rank by how many stored relations they
// carry, then by whether a hub is at one end, then by whether a contract
// carries them, and the best ones that join the drawing into one piece are
// kept, with a line into every box something uses. That draws a little
// over one line a box.

// sparse is the most links per box, in tenths, an overview draws whole:
// the exemplars draw 10 to 17.
const sparse = 17

// pair is two places the overview may draw lines between, in sorted order:
// what it keeps or counts, so the two lines of a pair that use each other
// stay together.
type pair [2]model.ID

func pairOf(a, b model.ID) pair {
	if b < a {
		a, b = b, a
	}
	return pair{a, b}
}

// cut is what the overview draws of its links: the kept ones, and the
// number each box counts instead of drawing.
type cut struct {
	keep map[pair]bool
	more map[model.ID]int
}

// hides says the cut leaves any pair undrawn.
func (c *cut) hides() bool { return c != nil && len(c.more) > 0 }

// primary decides which of the planned overview's links it draws. marked
// are the stored relations, by relation key, that must be drawn.
func (o *overview) primary(marked map[string]bool, stacks []layering, flows []flowWalk) *cut {
	isModule := func(id model.ID) bool { return slices.Contains(o.modules, id) }
	rels := map[pair][]string{}
	contract := map[pair]bool{}
	var uses []use
	var ls []pair
	add := func(l pair, rs []string) {
		if _, ok := rels[l]; !ok {
			ls = append(ls, l)
		}
		rels[l] = append(rels[l], rs...)
	}
	// The lines a drawing of every pair would draw, as uses, so the lead it
	// would find is found here and kept (lead.go).
	covered := func(a, b model.ID) bool { return len(o.carry[[2]model.ID{a, b}]) > 0 }
	direct := func(e model.Edge) bool { return e.Kind == model.DependsOn && isModule(e.From) && isModule(e.To) }
	for _, k := range o.pairs {
		var rs []string
		for _, c := range o.carry[k] {
			rs = append(rs, o.between(k[0], c)...)
		}
		rs = append(rs, o.rels[model.Edge{From: k[0], To: k[1], Kind: model.DependsOn}.String()]...)
		l := pairOf(k[0], k[1])
		add(l, rs)
		contract[l] = true
		if isModule(k[1]) {
			uses = append(uses, use{from: string(k[0]), to: string(k[1]), weight: distinct(rs)})
		}
	}
	for _, e := range o.relations {
		if !direct(e) || covered(e.From, e.To) {
			continue
		}
		rs := o.rels[e.String()]
		add(pairOf(e.From, e.To), rs)
		back := model.Edge{From: e.To, To: e.From, Kind: model.DependsOn}
		if slices.Contains(o.relations, back) && !covered(e.To, e.From) {
			rs = append(slices.Clone(rs), o.rels[back.String()]...)
		}
		uses = append(uses, use{from: string(e.From), to: string(e.To), weight: distinct(rs)})
	}
	for _, e := range o.relations {
		if direct(e) && covered(e.From, e.To) {
			add(pairOf(e.From, e.To), o.rels[e.String()])
		}
	}
	if 10*len(ls) <= sparse*len(o.modules) {
		return nil
	}

	keep := map[pair]bool{}
	for _, l := range ls {
		if slices.ContainsFunc(rels[l], func(r string) bool { return marked[r] }) {
			keep[l] = true
		}
	}
	var boxes []Box
	for _, m := range o.modules {
		boxes = append(boxes, Box{ID: string(m)})
	}
	lead := inferred(boxes, uses)
	d := Drawing{Boxes: boxes}
	for _, f := range flows {
		lead = append(lead, d.followed(o.tree, f, uses)...)
	}
	for _, h := range lead {
		keep[pairOf(model.ID(h.from), model.ID(h.to))] = true
	}
	box := func(u model.ID) string {
		if c := o.tree.chain(u); len(c) > 0 && isModule(c[0]) {
			return string(c[0])
		}
		return ""
	}
	tier := map[model.ID]int{}
	for i, g := range stackOf(stacks, box, func(ids []string) []string { return ids }) {
		for _, id := range g.ids {
			tier[model.ID(id)] = i + 1
		}
	}
	for _, l := range ls {
		if a, b := tier[l[0]], tier[l[1]]; a > 0 && b > 0 && (a-b == 1 || b-a == 1) {
			keep[l] = true
		}
	}

	users := map[model.ID]int{}
	for _, u := range uses {
		users[model.ID(u.to)]++
	}
	hub := func(l pair) bool { return users[l[0]] >= hubUsers || users[l[1]] >= hubUsers }
	slices.SortStableFunc(ls, func(a, b pair) int {
		switch {
		case distinct(rels[a]) != distinct(rels[b]):
			return distinct(rels[b]) - distinct(rels[a])
		case hub(a) != hub(b):
			return map[bool]int{true: -1, false: 1}[hub(a)]
		case contract[a] != contract[b]:
			return map[bool]int{true: -1, false: 1}[contract[a]]
		}
		return 0
	})
	// Whatever the cut, the drawing stays one piece where its links join
	// it, and a box something uses still shows a line into it, so the lead
	// finds the same entry.
	root := map[model.ID]model.ID{}
	var find func(model.ID) model.ID
	find = func(id model.ID) model.ID {
		if r, ok := root[id]; ok && r != id {
			root[id] = find(r)
			return root[id]
		}
		return id
	}
	join := func(l pair) bool {
		a, b := find(l[0]), find(l[1])
		root[a] = b
		return a != b
	}
	for _, l := range ls {
		if keep[l] {
			join(l)
		}
	}
	for _, l := range ls {
		if join(l) {
			keep[l] = true
		}
	}
	into := map[string]bool{}
	for _, u := range uses {
		into[u.to] = into[u.to] || keep[pairOf(model.ID(u.from), model.ID(u.to))]
	}
	for _, l := range ls {
		for _, u := range uses {
			if !into[u.to] && pairOf(model.ID(u.from), model.ID(u.to)) == l {
				keep[l], into[u.to] = true, true
			}
		}
	}
	c := &cut{keep: keep, more: map[model.ID]int{}}
	for _, l := range ls {
		if !keep[l] {
			c.more[l[0]]++
			c.more[l[1]]++
		}
	}
	return c
}

func distinct(rs []string) int { return len(slices.Compact(slices.Sorted(slices.Values(rs)))) }
