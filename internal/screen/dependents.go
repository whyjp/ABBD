package screen

import (
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// A unit's direct dependents are the other present units that hold a live
// relation to it by which they depend on it or consume it, computed from the
// stored relations and written on the unit's row and in the module page's list of
// inside units, so a reader can answer what a change to the unit reaches
// first without leaving the page. It is the direct tier of `abbd impact`: a
// unit inside the one it depends on is no dependent of it, and a unit that
// only provides a contract is not one either.

// Dependent is one unit that directly depends on another.
type Dependent struct {
	ID, Name string
	// Paged says the unit has a page of its own, where the link goes;
	// otherwise it goes to the unit's row in the list of units.
	Paged bool `json:"-"`
}

// directDependents is, for each present unit, the units that depend on or
// consume it directly, in ID order.
func directDependents(units []model.Unit) map[model.ID][]model.ID {
	present := map[model.ID]model.Unit{}
	for _, u := range model.Present(units) {
		present[u.ID] = u
	}
	within := func(id, top model.ID) bool {
		for seen := map[model.ID]bool{}; id != "" && !seen[id]; id = present[id].Parent {
			if id == top {
				return true
			}
			seen[id] = true
		}
		return false
	}
	out := map[model.ID][]model.ID{}
	for _, u := range present {
		for _, r := range u.Relations {
			if r.Dead || r.Kind != model.DependsOn && r.Kind != model.Consumes {
				continue
			}
			if _, ok := present[r.To]; !ok || within(u.ID, r.To) {
				continue
			}
			if !slices.Contains(out[r.To], u.ID) {
				out[r.To] = append(out[r.To], u.ID)
			}
		}
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out
}

// dependentsOf names the units that depend directly on id, each with its page.
func (o *overview) dependentsOf(id model.ID) []Dependent {
	var out []Dependent
	for _, d := range o.dependents[id] {
		u := o.units[d]
		name := u.Name.Value
		if name == "" {
			name = string(d)
		}
		out = append(out, Dependent{ID: string(d), Name: name, Paged: viewOf(o.tree, d) != ""})
	}
	return out
}

// topUsed names the units inside the box m that the most other boxes use,
// each with how many, at most n, the most used first, so a reader of a
// foundation band sees which part of the box the count of boxes comes from.
func (o *overview) topUsed(m model.ID, n int) []string {
	box := func(id model.ID) model.ID {
		if ch := o.tree.chain(id); len(ch) > 0 {
			return ch[0]
		}
		return ""
	}
	type used struct {
		name  string
		boxes int
	}
	var all []used
	for id, deps := range o.dependents {
		if box(id) != m {
			continue
		}
		seen := map[model.ID]bool{}
		for _, d := range deps {
			if b := box(d); b != "" && b != m && isModuleOf(o, b) {
				seen[b] = true
			}
		}
		if len(seen) > 0 {
			name := o.units[id].Name.Value
			if name == "" {
				name = string(id)
			}
			all = append(all, used{name, len(seen)})
		}
	}
	slices.SortFunc(all, func(a, b used) int {
		if a.boxes != b.boxes {
			return b.boxes - a.boxes
		}
		return strings.Compare(a.name, b.name)
	})
	var out []string
	for _, u := range all[:min(n, len(all))] {
		out = append(out, u.name+" "+usedByText(u.boxes))
	}
	return out
}
