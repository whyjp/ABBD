package screen

import (
	"maps"

	"github.com/whyjp/abbd/internal/model"
)

// A box the overview record marks as a sample, an artifact or a tool shows
// how to use, build or test the product and is no part of it. The overview
// leaves it out of the picture, so it takes no place among the product's
// boxes and no count of them, and names it in a line under the picture,
// 주변, each name linking the box's focus view. Its lines are counted and
// listed under the picture like any left out, and its focus view draws them
// all. A stage of the primary flow, and a box a line of the record or the
// lead runs through, stays a box, since a reader reads through those lines.

// Aside is one box left out of the picture.
type Aside struct {
	Name  string
	Scope string
	Href  string
}

// asideBoxes leaves the boxes of scope out of the cut c, nil for every link
// drawn. It is c itself when no box is left to set aside.
func (o *overview) asideBoxes(c *cut, scope map[model.ID]string) *cut {
	if len(scope) == 0 {
		return c
	}
	box := func(u model.ID) string {
		if ch := o.tree.chain(u); len(ch) > 0 && isModuleOf(o, ch[0]) {
			return string(ch[0])
		}
		return ""
	}
	stage := o.flow.stagesOn(box)
	ls, _, _, uses := o.links()
	lead := o.leadPairs(uses, o.walks)
	var said map[pair]string
	if c != nil {
		said = c.said
	}
	read := map[model.ID]bool{}
	for _, l := range ls {
		if lead[l] || said[l] != "" {
			read[l[0]], read[l[1]] = true, true
		}
	}
	aside := map[model.ID]bool{}
	for _, m := range o.modules {
		if scope[m] != "" && stage[string(m)] == 0 && !read[m] {
			aside[m] = true
		}
	}
	if len(aside) == 0 {
		return c
	}
	keep := map[pair]bool{}
	folded := map[pair]bool{}
	for _, l := range ls {
		if c == nil || c.keep[l] {
			keep[l] = !aside[l[0]] && !aside[l[1]]
		}
		folded[l] = aside[l[0]] || aside[l[1]]
	}
	out := counting(ls, func() map[pair]bool {
		// Counting counts what is neither kept nor folded as left out.
		k := maps.Clone(keep)
		for l, f := range folded {
			if f {
				k[l] = true
			}
		}
		return k
	}())
	out.keep, out.aside = keep, aside
	if c != nil {
		out.said, out.static, out.fold, out.band, out.bandUses = c.said, c.static, c.fold, c.band, c.bandUses
		out.left = c.left
	}
	return out
}

// asideList names the boxes the overview leaves out of the picture, in the
// order of the modules.
func (o *overview) asideList(href func(model.ID) string) []Aside {
	var out []Aside
	for _, m := range o.modules {
		if o.aside[m] {
			out = append(out, Aside{Name: o.name(m), Scope: o.scope[m], Href: href(m)})
		}
	}
	return out
}
