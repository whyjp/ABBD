package screen

import (
	"fmt"
	"maps"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// A line whose relations all rest on a static dependency, an import, a
// trait, a macro, a build setting or a convention with no call or data on
// record, is drawn dotted (dress.go). Most of a large repository's lines
// are such imports, and drawn at rest they bury the lines that carry calls
// and data. So by default the overview and each module's inside leave a
// static line out, list it under the picture as 정적 의존, and draw it in
// the focus views of the boxes at its ends, which draw every line they
// have. A static pair stays drawn where it reaches a contract, joins two
// stages of the primary flow or two steps of a stored flow, carries the
// view's lead or the stored overview record, rests on a relation a reader
// or a human stated or marked, or, where the lead is inferred from the
// lines, is a line that lead needs (keepLead):
// those are how the code is meant to be read. A line left out still takes
// part in the layering, so the boxes keep their rows where the layout can.
// store.Project.StaticLines draws every static line again.

// staticWord names a line left out for resting on a static dependency only.
const staticWord = "정적 의존"

// meant says a reader or a human stated one of rels rather than an
// extractor finding it: who states a coupling means it to be read, so such
// a static line stays drawn. Only what an extractor found alone, such as a
// bare import, rests in the list.
func (t tree) meant(rels []string) bool {
	return slices.ContainsFunc(rels, func(r string) bool { return t.stated[r] })
}

// quietCut takes the static pairs out of the overview's cut c, a nil c
// being one that draws every pair. marked are the stored relations that
// must be drawn. It is c itself when no pair is static.
func (o *overview) quietCut(c *cut, marked map[string]bool) *cut {
	ls, rels, contract, uses := o.links()
	lead := o.leadPairs(uses, o.walks)
	stage := o.flow.stagesOn(func(u model.ID) string {
		if ch := o.tree.chain(u); len(ch) > 0 && isModuleOf(o, ch[0]) {
			return string(ch[0])
		}
		return ""
	})
	var said map[pair]string
	if c != nil {
		said = c.said
	}
	static := map[pair]bool{}
	for _, l := range ls {
		if c != nil && !c.keep[l] {
			continue
		}
		a, b := stage[string(l[0])], stage[string(l[1])]
		switch {
		case contract[l] || lead[l] || said[l] != "",
			a > 0 && b > 0 && a != b,
			!o.tree.planOnly(rels[l]), o.tree.meant(rels[l]),
			slices.ContainsFunc(rels[l], func(r string) bool { return marked[r] }):
			continue
		}
		static[l] = true
	}
	if o.infers() {
		keepLead(uses, func(u use) bool { return c == nil || c.keep[pairOf(model.ID(u.from), model.ID(u.to))] },
			func(u use) pair { return pairOf(model.ID(u.from), model.ID(u.to)) }, static)
	}
	if len(static) == 0 {
		return c
	}
	out := &cut{keep: map[pair]bool{}, left: true, static: static}
	if c != nil {
		out.keep = maps.Clone(c.keep)
		out.said, out.fold, out.band, out.bandUses, out.aside = c.said, c.fold, c.band, c.bandUses, c.aside
		for l := range c.static {
			static[l] = true
		}
	} else {
		for _, l := range ls {
			out.keep[l] = true
		}
	}
	for l := range static {
		out.keep[l] = false
	}
	return out
}

// quietLines takes out of an inside's lines at rest those whose pair, two
// inside boxes or a box and a chip, rests on a static dependency only, as
// the overview decides by pair. A pair stays drawn where a line of it
// reaches a contract, joins two stages of the primary flow or two boxes
// that follow each other on a stored flow, carries a relation a reader
// marked or stated, or is one an inferred lead needs (keepLead). box gives
// the inside box a unit stands in.
func (o *overview) quietLines(lines []line, box func(model.ID) string) (kept, left []line) {
	contract := func(id model.ID) bool { return o.tree.units[id].Kind.IsContract() }
	stage, walked := o.flow.stagesOn(box), o.walkPairs(box)
	static := map[string]bool{}
	for _, ln := range lines {
		static[lineKey(ln)] = true
	}
	for _, ln := range lines {
		a, b := stage[string(ln.from)], stage[string(ln.to)]
		f, t := string(ln.from), string(ln.to)
		if contract(ln.from) || contract(ln.to) || !o.tree.planOnly(ln.rels) || o.tree.meant(ln.rels) ||
			ln.inner && (a > 0 && b > 0 && a != b || walked[[2]string{min(f, t), max(f, t)}]) ||
			slices.ContainsFunc(ln.rels, func(r string) bool { return o.marked[r] }) {
			static[lineKey(ln)] = false
		}
	}
	var uses []use
	key := map[use]string{}
	for _, ln := range lines {
		if ln.inner {
			u := use{from: string(ln.from), to: string(ln.to)}
			uses, key[u] = append(uses, u), lineKey(ln)
		}
	}
	if o.infers() {
		keepLead(uses, func(use) bool { return true }, func(u use) string { return key[u] }, static)
	}
	for _, ln := range lines {
		if static[lineKey(ln)] {
			left = append(left, ln)
		} else {
			kept = append(kept, ln)
		}
	}
	return kept, left
}

// leftStatic lists an inside's static lines left out, each pair once
// under the box inside the module, whose focus view in scope draws it.
func (o *overview) leftStatic(scope model.ID, left []line) []LeftOut {
	seen := map[[2]model.ID]bool{}
	own := map[model.ID][]string{}
	var order []model.ID
	for _, ln := range left {
		k := [2]model.ID{min(ln.from, ln.to), max(ln.from, ln.to)}
		if !ln.inner {
			k = [2]model.ID{ln.from, ln.to}
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, ok := own[ln.from]; !ok {
			order = append(order, ln.from)
		}
		own[ln.from] = append(own[ln.from], o.name(ln.to))
	}
	var out []LeftOut
	for _, id := range order {
		names := own[id]
		slices.Sort(names)
		out = append(out, LeftOut{Name: o.name(id), Href: "#" + focusID(scope, id), Static: names})
	}
	sortLeftOut(out)
	return out
}

// infers says a view's lead is the one its lines show (lead.go): no flow is
// stored and the project draws an inferred lead. Only then does hiding a
// line change where the lead starts.
func (o *overview) infers() bool { return !o.stored && len(o.walks) == 0 }

// keepLead puts back, of the static pairs, those the view's lead needs.
// The lead is found again from the lines a view draws (lead.go), from the
// one box nothing uses, so hiding leaves a line into every box something
// used and a line out of every box nothing used. uses are the view's lines
// as uses, drawn says whether a use's line is drawn at all, and pairOf
// names the pair a use's line belongs to.
func keepLead[P comparable](uses []use, drawn func(use) bool, pairOf func(use) P, static map[P]bool) {
	in, out := map[string]bool{}, map[string]bool{}
	for _, u := range uses {
		if drawn(u) {
			in[u.to], out[u.from] = true, true
		}
	}
	has := func(f func(use) bool) bool {
		return slices.ContainsFunc(uses, func(v use) bool { return drawn(v) && !static[pairOf(v)] && f(v) })
	}
	for _, u := range uses {
		if drawn(u) && static[pairOf(u)] && in[u.to] && !has(func(v use) bool { return v.to == u.to }) {
			delete(static, pairOf(u))
		}
	}
	for _, u := range uses {
		if drawn(u) && static[pairOf(u)] && !in[u.from] && !has(func(v use) bool { return v.from == u.from }) {
			delete(static, pairOf(u))
		}
	}
}

// walkPairs are the pairs of boxes, by box, that follow each other on a
// stored flow: a line between them is one a reader follows the flow along.
func (o *overview) walkPairs(box func(model.ID) string) map[[2]string]bool {
	out := map[[2]string]bool{}
	for _, w := range o.walks {
		prev := ""
		for _, u := range w.units {
			b := box(u)
			if b == "" {
				// A step outside the view breaks the run: the boxes on
				// either side of it do not follow each other here.
				prev = ""
				continue
			}
			if b == prev {
				continue
			}
			if prev != "" {
				out[[2]string{min(prev, b), max(prev, b)}] = true
			}
			prev = b
		}
	}
	return out
}

// lineKey is the pair a line of an inside belongs to: two inside boxes,
// either way round, or a box and a chip on the frame.
func lineKey(ln line) string {
	if ln.inner {
		return fmt.Sprint(min(ln.from, ln.to), " ", max(ln.from, ln.to))
	}
	return fmt.Sprint(ln.from, " ", ln.to, " ", ln.exit)
}
