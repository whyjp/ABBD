package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// unphrased names each pair of overview boxes whose drawn lines carry no
// phrase on any relation, so the overview can say only a via word such as
// 호출 or 임포트 there. A contract stands on the box of the module that
// provides it, as the overview draws it.
func unphrased(units []model.Unit, d screen.Drawing) []string {
	byID := map[model.ID]model.Unit{}
	provider := map[model.ID]model.ID{}
	for _, u := range model.Present(units) {
		byID[u.ID] = u
	}
	for _, u := range model.Present(units) {
		for _, r := range u.Relations {
			if r.Kind == model.Provides {
				if p, ok := provider[r.To]; !ok || u.ID < p {
					provider[r.To] = u.ID
				}
			}
		}
	}
	box := func(id model.ID) model.ID {
		for range len(byID) {
			u, ok := byID[id]
			switch {
			case !ok:
				return id
			case u.Parent != "":
				id = u.Parent
			case provider[id] != "":
				id = provider[id]
			default:
				return id
			}
		}
		return id
	}
	type pair struct{ from, to model.ID }
	count, phrased := map[pair]int{}, map[pair]bool{}
	for _, w := range d.Wires {
		for _, r := range w.Rels {
			e, err := model.ParseEdge(r)
			if err != nil {
				continue
			}
			p := pair{box(e.From), box(e.To)}
			if p.from == p.to {
				continue
			}
			count[p]++
			if n, ok := byID[e.From].Note(e.Kind, e.To); ok && n.Text != "" {
				phrased[p] = true
			}
		}
	}
	var pairs []pair
	for p := range count {
		if !phrased[p] {
			pairs = append(pairs, p)
		}
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		if a.from != b.from {
			return cmp.Compare(a.from, b.from)
		}
		return cmp.Compare(a.to, b.to)
	})
	var out []string
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("%s %s -> %s %s (%s)", p.from, byID[p.from].Name.Value, p.to, byID[p.to].Name.Value, plural(count[p], "relation")))
	}
	return out
}
