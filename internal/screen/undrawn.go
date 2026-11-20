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
// hub that folds it (hubfold.go), else off the end that has more undrawn
// pairs, the first by ID when they tie, so the busiest boxes come first and
// no pair is listed twice.
func (o *overview) undrawn() []LeftOut {
	pairs := map[[2]string]bool{}
	deg := map[string]int{}
	for _, c := range o.counted {
		k := [2]string{min(c[0], c[1]), max(c[0], c[1])}
		if k[0] == k[1] || pairs[k] {
			continue
		}
		pairs[k] = true
		deg[k[0]]++
		deg[k[1]]++
	}
	folds := func(hub, user string) bool {
		return slices.Contains(o.fold[model.ID(hub)], model.ID(user))
	}
	own := map[string][]string{}
	for k := range pairs {
		a, b := k[0], k[1]
		switch {
		case folds(a, b) && !folds(b, a):
		case folds(b, a) && !folds(a, b):
			a, b = b, a
		case deg[b] > deg[a]:
			a, b = b, a
		}
		own[a] = append(own[a], b)
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
