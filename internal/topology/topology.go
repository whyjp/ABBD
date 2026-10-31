// Package topology compares the intended topology a human approved with the
// actual topology extracted from code, and derives the project phase from it.
package topology

import (
	"fmt"
	"slices"
	"sort"
	"strings"

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
	// ExternalConsumers is a human's statement that a contract has consumers
	// outside the code the extraction script reads.
	ExternalConsumers bool `yaml:"external_consumers,omitempty"`
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
func Compare(in Intended, units []model.Unit) []Item {
	actual := presentUnits(units)
	intended := in.byID()
	var items []Item
	add := func(it Item) {
		if effect, ok := in.Effect(it); ok {
			it.Effect = effect
			items = append(items, it)
		}
	}
	for _, iu := range in.Units {
		au, ok := actual[iu.ID]
		switch {
		case !ok:
			add(Item{Change: UnitRemoved, Unit: iu.ID})
		case signature(au) != iu.Signature:
			add(Item{Change: ContractChanged, Unit: iu.ID, Old: iu.Signature, New: signature(au)})
		}
	}
	for id := range actual {
		if _, ok := intended[id]; !ok {
			add(Item{Change: UnitAdded, Unit: id})
		}
	}
	edges := actualEdges(actual)
	for _, e := range in.Relations {
		if edges[e] {
			continue
		}
		it := Item{Change: RemovedRelation, Edge: e}
		if !in.droppedWithUnit(it, actual) {
			add(it)
		}
	}
	for e := range edges {
		add(Item{Change: NewRelation, Edge: e})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key() < items[j].Key() })
	return items
}

// Effect is the one rule for who must act on a change. ok is false when the
// change does not touch the intended topology at all.
//
// Grade C (Blocked) covers anything that changes what an intended contract
// promises or who depends on it, and any new coupling between two intended
// units. Other changes that touch an intended unit need a short realignment.
func (in Intended) Effect(it Item) (effect Effect, ok bool) {
	intended := in.byID()
	_, has := intended[it.Unit]
	isContract := func(id model.ID) bool { u, ok := intended[id]; return ok && u.Kind.IsContract() }
	switch it.Change {
	case UnitRemoved:
		switch {
		case !has:
			return "", false
		case isContract(it.Unit):
			return Blocked, true
		}
		return Realign, true
	case ContractChanged:
		return Blocked, isContract(it.Unit)
	case UnitAdded:
		return Promote, !has
	case RemovedRelation:
		if !slices.Contains(in.Relations, it.Edge) {
			return "", false
		}
		if isContract(it.Edge.From) || isContract(it.Edge.To) {
			return Blocked, true
		}
		return Realign, true
	case NewRelation:
		if slices.Contains(in.Relations, it.Edge) {
			return "", false
		}
		_, from := intended[it.Edge.From]
		_, to := intended[it.Edge.To]
		switch {
		case (from && to) || isContract(it.Edge.From) || isContract(it.Edge.To):
			return Blocked, true
		case from || to:
			return Realign, true
		}
	}
	return "", false
}

// droppedWithUnit reports whether a removed relation is already gated by the
// removal of an intended unit at one of its ends, since accepting that removal
// drops the unit's relations too. A relation that would gate harder than the
// removal, such as a deleted module's consumes edge into a contract, is reported
// on its own; otherwise deleting a consumer would pass as a realignment.
func (in Intended) droppedWithUnit(it Item, actual map[model.ID]model.Unit) bool {
	edge, _ := in.Effect(it)
	for _, id := range []model.ID{it.Edge.From, it.Edge.To} {
		if _, ok := actual[id]; ok {
			continue
		}
		if removal, ok := in.Effect(Item{Change: UnitRemoved, Unit: id}); ok && (removal == Blocked || edge != Blocked) {
			return true
		}
	}
	return false
}

func (in Intended) byID() map[model.ID]IntendedUnit {
	m := map[model.ID]IntendedUnit{}
	for _, u := range in.Units {
		m[u.ID] = u
	}
	return m
}

// Unit returns the intended unit with id.
func (in Intended) Unit(id model.ID) (IntendedUnit, bool) {
	u, ok := in.byID()[id]
	return u, ok
}

// HasConsumers reports whether anything depends on what contract id promises:
// a consumes or depends_on relation into it, intended or actual, or a human's
// external_consumers flag. It counts the same relations the turn impact follows.
func (in Intended) HasConsumers(units []model.Unit, id model.ID) bool {
	if u, ok := in.Unit(id); ok && u.ExternalConsumers {
		return true
	}
	consumes := func(e Edge) bool {
		return e.To == id && (e.Kind == model.Consumes || e.Kind == model.DependsOn)
	}
	if slices.ContainsFunc(in.Relations, consumes) {
		return true
	}
	for e := range actualEdges(presentUnits(units)) {
		if consumes(e) {
			return true
		}
	}
	return false
}

// ParseKey reads an item key as Item.Key prints it. The effect is left empty.
func ParseKey(key string) (Item, error) {
	change, rest, ok := strings.Cut(key, ":")
	if !ok {
		return Item{}, fmt.Errorf("%q is not a change key such as new-relation:MOD-0001.depends_on.MOD-0002", key)
	}
	it := Item{Change: Change(change)}
	switch it.Change {
	case NewRelation, RemovedRelation:
		e, err := ParseEdge(rest)
		if err != nil {
			return Item{}, fmt.Errorf("%s: %w", key, err)
		}
		it.Edge = e
	case ContractChanged, UnitRemoved, UnitAdded:
		if _, _, err := model.ParseID(rest); err != nil {
			return Item{}, fmt.Errorf("%s: %w", key, err)
		}
		it.Unit = model.ID(rest)
	default:
		return Item{}, fmt.Errorf("%s: unknown change %q", key, change)
	}
	return it, nil
}

// ParseEdge reads an edge as Edge.String prints it.
func ParseEdge(s string) (Edge, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Edge{}, fmt.Errorf("%q is not an edge such as MOD-0001.depends_on.MOD-0002", s)
	}
	e := Edge{From: model.ID(parts[0]), Kind: model.RelationKind(parts[1]), To: model.ID(parts[2])}
	for _, id := range []model.ID{e.From, e.To} {
		if _, _, err := model.ParseID(string(id)); err != nil {
			return Edge{}, err
		}
	}
	return e, e.Kind.Validate()
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
			in.Units = append(in.Units, IntendedUnit{ID: id, Kind: u.Kind, Signature: signature(u)})
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

// Accept folds an item into the intended topology whatever its effect. Only a
// turn that syncs calls it, for the changes the turn was allowed to make.
func Accept(in Intended, it Item) Intended { return apply(in, it) }

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
