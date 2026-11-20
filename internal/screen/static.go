package screen

import (
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
// have. A static line stays drawn where it reaches a contract, where it
// joins two stages of the primary flow, where the view's lead or the
// stored overview record rides it, and where a reader or a human stated
// one of its relations: those are how the code is meant to be read. A line
// left out still sets which box stands above the other, so no box moves.
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
	if len(static) == 0 {
		return c
	}
	out := &cut{keep: map[pair]bool{}, left: true, static: static}
	if c != nil {
		out.keep = maps.Clone(c.keep)
		out.said, out.fold, out.band, out.bandUses = c.said, c.fold, c.band, c.bandUses
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

// quietLines takes out of an inside's lines at rest those that rest on a
// static dependency only, unless the line reaches a contract or joins two
// stages of the primary flow. stage gives each inside box its stage.
func (o *overview) quietLines(lines []line, stage map[string]int) (kept, left []line) {
	contract := func(id model.ID) bool { return o.tree.units[id].Kind.IsContract() }
	for _, ln := range lines {
		a, b := stage[string(ln.from)], stage[string(ln.to)]
		if contract(ln.from) || contract(ln.to) || ln.inner && a > 0 && b > 0 && a != b || !o.tree.planOnly(ln.rels) || o.tree.meant(ln.rels) {
			kept = append(kept, ln)
			continue
		}
		left = append(left, ln)
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
