package screen

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// tree is the parent tree the blueprint decomposes along: the top level is
// the overview, and every unit with children has a view of its own that
// shows them. A unit with children and no relations of its own, such as a
// group a reader recorded or sync made, is abstract: the view above it
// draws its members' relations as its own.
type tree struct {
	units    map[model.ID]model.Unit
	children map[model.ID][]model.ID
	// vias say how each stored relation arises, by its key.
	vias map[string]model.Via
}

func newTree(units []model.Unit) tree {
	t := tree{units: map[model.ID]model.Unit{}, children: map[model.ID][]model.ID{}, vias: map[string]model.Via{}}
	for _, u := range model.Present(units) {
		t.units[u.ID] = u
		for _, r := range u.Relations {
			t.vias[key(u.ID, r.Kind, r.To)] = r.Via
		}
	}
	// A unit whose parent is not present sits at the top level, so nothing
	// present ever drops out of the blueprint.
	for _, u := range model.Present(units) {
		if _, ok := t.units[u.Parent]; !ok {
			u.Parent = ""
			t.units[u.ID] = u
		}
		t.children[u.Parent] = append(t.children[u.Parent], u.ID)
	}
	for k := range t.children {
		slices.Sort(t.children[k])
	}
	return t
}

// chain is id's ancestors from the top down, ending with id.
func (t tree) chain(id model.ID) []model.ID {
	var out []model.ID
	for k := id; k != ""; k = t.units[k].Parent {
		if slices.Contains(out, k) {
			break
		}
		out = append(out, k)
	}
	slices.Reverse(out)
	return out
}

func (t tree) abstract(id model.ID) bool {
	if len(t.children[id]) == 0 {
		return false
	}
	for _, r := range t.units[id].Relations {
		if !r.Dead {
			return false
		}
	}
	return true
}

// sources are the units whose relations a box for id draws: id, and through
// an abstract unit, its members.
func (t tree) sources(id model.ID) []model.ID {
	out := []model.ID{id}
	if t.abstract(id) {
		for _, c := range t.children[id] {
			out = append(out, t.sources(c)...)
		}
	}
	return out
}

// subtree is id and every unit it holds, at any depth.
func (t tree) subtree(id model.ID) []model.ID {
	out := []model.ID{id}
	for _, c := range t.children[id] {
		out = append(out, t.subtree(c)...)
	}
	return out
}

// within is the child of container that holds id, or id itself.
func (t tree) within(id, container model.ID) (model.ID, bool) {
	c := t.chain(id)
	if container == "" {
		return c[0], len(c) > 0
	}
	for i, k := range c {
		if k == container && i+1 < len(c) {
			return c[i+1], true
		}
	}
	return "", false
}

// outside is the largest unit holding id that does not hold container: what
// a view of container shows of id, as a port on its frame.
func (t tree) outside(id, container model.ID) model.ID {
	a, c := t.chain(container), t.chain(id)
	for i, k := range c {
		if i >= len(a) || a[i] != k {
			return k
		}
	}
	return ""
}

func key(from model.ID, kind model.RelationKind, to model.ID) string {
	return model.Edge{From: from, To: to, Kind: kind}.String()
}
