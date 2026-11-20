package screen

import (
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A drawing that leaves lines out says so under the picture, once, and lists
// them where a reader can open it: each undrawn pair stands under one box,
// with the box at its other end, and that box's focus view draws it.

// LeftOut is the pairs one box answers for: With names the boxes at their
// other ends, Href the focus view of the box that draws them all.
type LeftOut struct {
	Name string
	Href string
	With []string
}

// undrawn lists the pairs o counts instead of drawing. A pair hangs off the
// box that uses the other, which is the box whose focus view draws the line
// with the contracts it carries, and off the hub that folds it
// (hubfold.go) where one does. No pair is listed twice, and the boxes with
// most pairs come first.
func (o *overview) undrawn() []LeftOut {
	seen := map[[2]string]bool{}
	var pairs [][2]string
	for _, c := range o.counted {
		k := [2]string{min(c[0], c[1]), max(c[0], c[1])}
		if k[0] != k[1] && !seen[k] {
			seen[k] = true
			pairs = append(pairs, c)
		}
	}
	folds := func(hub, user string) bool {
		return slices.Contains(o.fold[model.ID(hub)], model.ID(user))
	}
	module := map[string]bool{}
	for _, m := range o.modules {
		module[string(m)] = true
	}
	own := map[string][]string{}
	for _, c := range pairs {
		owner, other := c[0], c[1]
		if folds(other, owner) && !folds(owner, other) || !module[owner] {
			owner, other = other, owner
		}
		if !module[owner] {
			continue
		}
		own[owner] = append(own[owner], other)
	}
	var out []LeftOut
	for id, with := range own {
		u := LeftOut{Name: o.name(model.ID(id)), Href: "#" + focusID("", model.ID(id))}
		for _, w := range with {
			u.With = append(u.With, o.name(model.ID(w)))
		}
		slices.Sort(u.With)
		out = append(out, u)
	}
	slices.SortFunc(out, func(x, y LeftOut) int {
		if d := len(y.With) - len(x.With); d != 0 {
			return d
		}
		return strings.Compare(x.Name+x.Href, y.Name+y.Href)
	})
	return out
}

// UndrawnPairs is how many pairs the list holds.
func (d Drawing) UndrawnPairs() int {
	n := 0
	for _, u := range d.Undrawn {
		n += len(u.With)
	}
	return n
}
