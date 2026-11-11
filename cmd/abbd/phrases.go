package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// unphrased names the overview lines whose relations carry no live phrase,
// so the overview can say only a via word such as 호출 or 임포트 there. A
// line is a drawn wire, named by the boxes at its ends: a unit inside a
// module stands on the module's box, and a contract on the box of the
// module that provides it, as the overview draws it. A phrase labels a line
// only while its quote is live, and only when half the line's relations
// carry it, so a line listed here may still say its via word after one
// phrase is added.
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
			u := byID[id]
			_, parent := byID[u.Parent]
			switch {
			case u.Parent != "" && parent:
				id = u.Parent
			case u.Parent == "" && provider[id] != "":
				id = provider[id]
			default:
				return id
			}
		}
		return id
	}
	type pair struct{ from, to model.ID }
	count := map[pair]int{}
	for _, w := range d.Wires {
		var line *pair
		n, live := 0, false
		for _, r := range w.Rels {
			e, err := model.ParseEdge(r)
			if err != nil {
				continue
			}
			p := pair{box(e.From), box(e.To)}
			if p.from == p.to {
				continue
			}
			if p.from > p.to {
				p = pair{p.to, p.from}
			}
			if line == nil {
				line = &p
			}
			n++
			if note, ok := byID[e.From].Note(e.Kind, e.To); ok && note.Text != "" && note.Status() == model.Live {
				live = true
			}
		}
		if line != nil && !live {
			count[*line] += n
		}
	}
	var pairs []pair
	for p := range count {
		pairs = append(pairs, p)
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		if a.from != b.from {
			return cmp.Compare(a.from, b.from)
		}
		return cmp.Compare(a.to, b.to)
	})
	var out []string
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("%s %s - %s %s (%s)", p.from, byID[p.from].Name.Value, p.to, byID[p.to].Name.Value, plural(count[p], "relation")))
	}
	return out
}
