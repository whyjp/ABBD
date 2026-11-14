package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/screen"
)

// unphrased names the overview lines whose relations carry no live phrase,
// so the overview can say only a via word such as 데이터 there, and none
// for a call or an import. A line is the unordered pair of boxes its drawn
// relations join: a unit inside a module stands on the module's box, and a
// contract on the box of the module that provides it, the first by ID when
// several do. A phrase labels a line
// only while its quote is live, and only when half the line's relations
// carry it, so a line listed here may still say its via word after one
// phrase is added. When the overview record chose lines, only those lines
// need a phrase, since the record says which lines a reader follows. Each
// line names its relations, the edges a phrase goes on: a relation to a
// contract labels the line between the two modules too.
func unphrased(units []model.Unit, d screen.Drawing, chosen []model.Line) []string {
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
			case provider[id] != "":
				id = provider[id]
			default:
				return id
			}
		}
		return id
	}
	type pair struct{ from, to model.ID }
	// A wire is a drawn path, which a trunk and its drops share, so the
	// lines are the box pairs of the distinct relations drawn.
	edges, phrased := map[pair][]string{}, map[pair]bool{}
	seen := map[string]bool{}
	for _, w := range d.Wires {
		for _, r := range w.Rels {
			if seen[r] {
				continue
			}
			seen[r] = true
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
			edges[p] = append(edges[p], r)
			if note, ok := byID[e.From].Note(e.Kind, e.To); ok && note.Text != "" && note.Status() == model.Live {
				phrased[p] = true
			}
		}
	}
	for p := range phrased {
		delete(edges, p)
	}
	if len(chosen) > 0 {
		keep := map[pair]bool{}
		for _, l := range chosen {
			p := pair{box(l.From), box(l.To)}
			keep[p], keep[pair{p.to, p.from}] = true, true
		}
		for p := range edges {
			if !keep[p] {
				delete(edges, p)
			}
		}
	}
	var pairs []pair
	for p := range edges {
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
		es := edges[p]
		slices.Sort(es)
		list := strings.Join(es[:min(len(es), 3)], ", ")
		if len(es) > 3 {
			list += fmt.Sprintf(" and %d more", len(es)-3)
		}
		out = append(out, fmt.Sprintf("%s %s - %s %s (%s: %s)", p.from, byID[p.from].Name.Value, p.to, byID[p.to].Name.Value, plural(len(es), "relation"), list))
	}
	return out
}
