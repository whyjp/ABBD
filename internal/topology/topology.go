// Package topology compares the intended topology a human approved with the
// actual topology extracted from code, and derives the project phase from it.
package topology

import (
	"fmt"
	"sort"

	"github.com/whyjp/abbd/internal/model"
)

// Intended is the human-approved topology. Only align and approve write it.
// A relation may end at a unit outside the intended set once a human has
// acknowledged it, so its later removal is still noticed.
type Intended struct {
	Units     []IntendedUnit `yaml:"units"`
	Relations []Edge         `yaml:"relations"`
}

type IntendedUnit struct {
	ID        model.ID   `yaml:"id"`
	Kind      model.Kind `yaml:"kind"`
	Signature string     `yaml:"signature,omitempty"`
}

type Edge struct {
	From model.ID           `yaml:"from"`
	To   model.ID           `yaml:"to"`
	Kind model.RelationKind `yaml:"kind"`
}

// String names the edge on the command line, so it holds no character a shell
// treats specially.
func (e Edge) String() string { return fmt.Sprintf("%s.%s.%s", e.From, e.Kind, e.To) }

type Phase string

const (
	Uninitialized Phase = "uninitialized"
	Bootstrapped  Phase = "bootstrapped"
	Aligned       Phase = "aligned"
	Drifted       Phase = "drifted"
)

// Effect says who must act on a difference.
type Effect string

const (
	// Blocked is a grade C change: it crosses the intended boundary and needs explicit approval.
	Blocked Effect = "BLOCKED"
	// Realign is a design-unit change a human must acknowledge with a short realignment.
	Realign Effect = "REALIGN"
	// Promote is below the line today and may become a design unit if a human adopts it.
	Promote Effect = "PROMOTE"
)

type Change string

const (
	NewRelation     Change = "new-relation"
	RemovedRelation Change = "removed-relation"
	ContractChanged Change = "contract-changed"
	UnitRemoved     Change = "unit-removed"
	UnitAdded       Change = "unit-added"
)

// Item is one difference between intended and actual.
type Item struct {
	Change Change
	Effect Effect
	Unit   model.ID
	Edge   Edge
	Old    string
	New    string
}

// Key is stable across runs, so a human can approve an item by name.
func (it Item) Key() string {
	switch it.Change {
	case NewRelation, RemovedRelation:
		return string(it.Change) + ":" + it.Edge.String()
	default:
		return string(it.Change) + ":" + string(it.Unit)
	}
}

func (it Item) String() string {
	s := fmt.Sprintf("%s %s", it.Effect, it.Key())
	if it.Change == ContractChanged {
		s += fmt.Sprintf(" (%q -> %q)", it.Old, it.New)
	}
	return s
}

func actualEdges(present map[model.ID]model.Unit) map[Edge]bool {
	edges := map[Edge]bool{}
	for id, u := range present {
		for _, r := range u.Relations {
			if _, ok := present[r.To]; ok && r.Counts() {
				edges[Edge{id, r.To, r.Kind}] = true
			}
		}
	}
	return edges
}

func presentUnits(units []model.Unit) map[model.ID]model.Unit {
	present := map[model.ID]model.Unit{}
	for _, u := range units {
		if !u.Absent {
			present[u.ID] = u
		}
	}
	return present
}

// Compare lists every difference, sorted by key.
//
// Grade C (Blocked) covers anything that changes what an intended contract
// promises or who depends on it, and any new coupling between two intended
// units. Other changes that touch an intended unit need a short realignment.
func Compare(in Intended, units []model.Unit) []Item {
	actual := presentUnits(units)
	intended := map[model.ID]IntendedUnit{}
	for _, u := range in.Units {
		intended[u.ID] = u
	}
	isContract := func(id model.ID) bool { u, ok := intended[id]; return ok && u.Kind.IsContract() }

	var items []Item
	for _, iu := range in.Units {
		au, ok := actual[iu.ID]
		switch {
		case !ok && iu.Kind.IsContract():
			items = append(items, Item{Change: UnitRemoved, Effect: Blocked, Unit: iu.ID})
		case !ok:
			items = append(items, Item{Change: UnitRemoved, Effect: Realign, Unit: iu.ID})
		case signature(au) != iu.Signature:
			items = append(items, Item{Change: ContractChanged, Effect: Blocked, Unit: iu.ID, Old: iu.Signature, New: signature(au)})
		}
	}
	for id := range actual {
		if _, ok := intended[id]; !ok {
			items = append(items, Item{Change: UnitAdded, Effect: Promote, Unit: id})
		}
	}

	edges := actualEdges(actual)
	want := map[Edge]bool{}
	for _, e := range in.Relations {
		want[e] = true
		_, from := actual[e.From]
		_, to := actual[e.To]
		if !from || !to || edges[e] {
			continue
		}
		effect := Realign
		if isContract(e.From) || isContract(e.To) {
			effect = Blocked
		}
		items = append(items, Item{Change: RemovedRelation, Effect: effect, Edge: e})
	}
	for e := range edges {
		if want[e] {
			continue
		}
		_, from := intended[e.From]
		_, to := intended[e.To]
		switch {
		case (from && to) || isContract(e.From) || isContract(e.To):
			items = append(items, Item{Change: NewRelation, Effect: Blocked, Edge: e})
		case from || to:
			items = append(items, Item{Change: NewRelation, Effect: Realign, Edge: e})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key() < items[j].Key() })
	return items
}

func signature(u model.Unit) string {
	if u.Signature == nil {
		return ""
	}
	return u.Signature.Value
}

func DerivePhase(units []model.Unit, in *Intended) Phase {
	if len(presentUnits(units)) == 0 {
		return Uninitialized
	}
	if in == nil {
		return Bootstrapped
	}
	for _, it := range Compare(*in, units) {
		if it.Effect != Promote {
			return Drifted
		}
	}
	return Aligned
}

// Present lists the units the latest extraction reported, sorted by ID.
func Present(units []model.Unit) []model.Unit {
	var out []model.Unit
	for _, u := range units {
		if !u.Absent {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Select is the intended topology a human adopts by confirming design units:
// the chosen units, their contract signatures, and every relation that touches
// one of them. A relation to an unchosen unit is acknowledged too, so its later
// removal is noticed and the alignment itself is not drift.
func Select(units []model.Unit, chosen map[model.ID]bool) Intended {
	actual := presentUnits(units)
	var in Intended
	for id, u := range actual {
		if chosen[id] {
			in.Units = append(in.Units, IntendedUnit{id, u.Kind, signature(u)})
		}
	}
	for e := range actualEdges(actual) {
		if chosen[e.From] || chosen[e.To] {
			in.Relations = append(in.Relations, e)
		}
	}
	return in.normalized()
}

// Approve folds one grade C item into the intended topology.
func Approve(in Intended, it Item) (Intended, error) {
	if it.Effect != Blocked {
		return in, fmt.Errorf("%s is %s, not a grade C change; realign instead", it.Key(), it.Effect)
	}
	return apply(in, it), nil
}

// RealignTo acknowledges every REALIGN item. It refuses while any grade C item
// is pending, because blocked changes come first.
func RealignTo(in Intended, units []model.Unit) (Intended, error) {
	items := Compare(in, units)
	for _, it := range items {
		if it.Effect == Blocked {
			return in, fmt.Errorf("%s is pending approval; approve it before realigning", it.Key())
		}
	}
	for _, it := range items {
		if it.Effect == Realign {
			in = apply(in, it)
		}
	}
	return in, nil
}

func apply(in Intended, it Item) Intended {
	switch it.Change {
	case NewRelation:
		in.Relations = append(in.Relations, it.Edge)
	case RemovedRelation:
		in.Relations = without(in.Relations, func(e Edge) bool { return e == it.Edge })
	case ContractChanged:
		units := append([]IntendedUnit(nil), in.Units...)
		for i := range units {
			if units[i].ID == it.Unit {
				units[i].Signature = it.New
			}
		}
		in.Units = units
	case UnitRemoved:
		var units []IntendedUnit
		for _, u := range in.Units {
			if u.ID != it.Unit {
				units = append(units, u)
			}
		}
		in.Units = units
		in.Relations = without(in.Relations, func(e Edge) bool { return e.From == it.Unit || e.To == it.Unit })
	}
	return in.normalized()
}

func without(es []Edge, drop func(Edge) bool) []Edge {
	var out []Edge
	for _, e := range es {
		if !drop(e) {
			out = append(out, e)
		}
	}
	return out
}

func (in Intended) normalized() Intended {
	sort.Slice(in.Units, func(i, j int) bool { return in.Units[i].ID < in.Units[j].ID })
	sort.Slice(in.Relations, func(i, j int) bool { return in.Relations[i].String() < in.Relations[j].String() })
	seen := map[Edge]bool{}
	var rels []Edge
	for _, e := range in.Relations {
		if !seen[e] {
			seen[e] = true
			rels = append(rels, e)
		}
	}
	in.Relations = rels
	if in.Units == nil {
		in.Units = []IntendedUnit{}
	}
	if in.Relations == nil {
		in.Relations = []Edge{}
	}
	return in
}
