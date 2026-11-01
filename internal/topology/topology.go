// Package topology compares the intended topology a human approved with the
// actual topology extracted from code, and derives the project phase from it.
package topology

import (
	"errors"
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
	// Revision counts the changes a human or a synced turn made to it.
	Revision  int            `yaml:"revision"`
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
	// ContractTouched is a turn's finding, never a difference Compare reports:
	// a consumed contract's code changed while its signature stayed the same.
	ContractTouched Change = "contract-touched"
	// EvidenceTouched is a turn's finding: the lines that hold the evidence of
	// a confirmed semantic relation changed while the quote survived.
	EvidenceTouched Change = "evidence-touched"
	// Unscanned is a turn's finding: no one checked a direct design unit for
	// the couplings only a reader sees.
	Unscanned Change = "semantic-unscanned"
)

// Finding reports whether a change is a turn's grade B note rather than a
// difference between the intended and the actual topology.
func (c Change) Finding() bool {
	return c == ContractTouched || c == EvidenceTouched || c == Unscanned
}

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
	case NewRelation, RemovedRelation, EvidenceTouched:
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

// Coupling is a semantic relation of the actual topology that no human has
// decided and whose evidence is live. It never gates, but the turn impact
// follows it and the screen draws it.
type Coupling struct {
	Edge
	Via      model.Via
	Evidence []model.Evidence
}

// Couplings lists the live semantic guesses between present units, sorted by edge.
func Couplings(units []model.Unit) []Coupling {
	present := presentUnits(units)
	var out []Coupling
	for id, u := range present {
		for _, r := range u.Relations {
			if _, ok := present[r.To]; ok && r.Semantic() && !r.Dead && r.Guess() {
				out = append(out, Coupling{Edge: Edge{id, r.To, r.Kind}, Via: r.Via, Evidence: r.Evidence})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
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
// promises or who depends on it, any new coupling between two intended units,
// and any relation between an intended unit and a contract, intended or not:
// a contract's consumers and providers are its boundary even before a human
// adopts it. Other changes that touch an intended unit need a short realignment.
func (in Intended) Effect(it Item) (effect Effect, ok bool) {
	intended := in.byID()
	_, has := intended[it.Unit]
	isContract := func(id model.ID) bool { u, ok := intended[id]; return ok && u.Kind.IsContract() }
	anyContract := func(id model.ID) bool { k, _, err := model.ParseID(string(id)); return err == nil && k.IsContract() }
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
		if anyContract(it.Edge.From) || anyContract(it.Edge.To) {
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
		case !from && !to:
		case (from && to) || anyContract(it.Edge.From) || anyContract(it.Edge.To):
			return Blocked, true
		default:
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
	case NewRelation, RemovedRelation, EvidenceTouched:
		e, err := ParseEdge(rest)
		if err != nil {
			return Item{}, fmt.Errorf("%s: %w", key, err)
		}
		it.Edge = e
	case ContractChanged, UnitRemoved, UnitAdded, ContractTouched, Unscanned:
		if _, _, err := model.ParseID(rest); err != nil {
			return Item{}, fmt.Errorf("%s: %w", key, err)
		}
		it.Unit = model.ID(rest)
	default:
		return Item{}, fmt.Errorf("%s: unknown change %q", key, change)
	}
	return it, nil
}

// Joins checks the edge's kind against the kinds its IDs name.
func (e Edge) Joins() error {
	from, _, _ := model.ParseID(string(e.From))
	to, _, _ := model.ParseID(string(e.To))
	return e.Kind.Joins(from, to)
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

// ErrPending means a grade C change waits for a human, so realignment must wait too.
var ErrPending = errors.New("pending approval")

// RealignTo acknowledges the REALIGN items a human chose by key. It refuses
// while any grade C item is pending, because blocked changes come first, and
// when keys names no row, so an empty choice never acknowledges them all.
func RealignTo(in Intended, units []model.Unit, keys []string) (Intended, error) {
	items := Compare(in, units)
	realign := map[string]Item{}
	for _, it := range items {
		switch it.Effect {
		case Blocked:
			return in, fmt.Errorf("%s is %w; approve it before realigning", it.Key(), ErrPending)
		case Realign:
			realign[it.Key()] = it
		}
	}
	if len(keys) == 0 {
		return in, errors.New("name at least one REALIGN row to acknowledge")
	}
	for _, k := range keys {
		it, ok := realign[k]
		if !ok {
			return in, fmt.Errorf("%s is not a REALIGN row of this project", k)
		}
		in = apply(in, it)
	}
	return in, nil
}

// Candidate is a below-the-line unit proposed to a human as a design unit.
type Candidate struct {
	Unit model.ID
	Kind model.Kind
	// Design lists the design units it has a relation with.
	Design []model.ID
	// Question names the decision. It lists the unit's relations, so a
	// rejection holds only until they change.
	Question string
}

// Candidates lists the below-the-line units worth a human's decision: every
// module and contract, and any other unit that two or more design units
// relate to. A unit whose current question a human rejected is left out.
func Candidates(in Intended, units []model.Unit, rejected func(question string) bool) []Candidate {
	actual := presentUnits(units)
	edges := actualEdges(actual)
	intended := in.byID()
	var out []Candidate
	for _, u := range Present(units) {
		if _, ok := intended[u.ID]; ok {
			continue
		}
		c := Candidate{Unit: u.ID, Kind: u.Kind, Question: promotionQuestion(u.ID, edges)}
		for e := range edges {
			for _, id := range []model.ID{e.From, e.To} {
				if _, ok := intended[id]; ok && (e.From == u.ID || e.To == u.ID) && !slices.Contains(c.Design, id) {
					c.Design = append(c.Design, id)
				}
			}
		}
		slices.Sort(c.Design)
		if (u.Kind == model.Module || u.Kind.IsContract() || len(c.Design) >= 2) && !rejected(c.Question) {
			out = append(out, c)
		}
	}
	return out
}

// PromotionQuestion names the decision on a below-the-line unit as it stands now.
func PromotionQuestion(units []model.Unit, id model.ID) string {
	return promotionQuestion(id, actualEdges(presentUnits(units)))
}

func promotionQuestion(id model.ID, edges map[Edge]bool) string {
	var touching []string
	for e := range edges {
		if e.From == id || e.To == id {
			touching = append(touching, e.String())
		}
	}
	slices.Sort(touching)
	return fmt.Sprintf("promote %s with relations [%s]", id, strings.Join(touching, " "))
}

// Adopt makes a below-the-line unit a design unit, with its relations to other
// below-the-line units. A relation to a design unit enters only if a human
// already acknowledged it at the grade it has once the unit is a design unit;
// one acknowledged at a lower grade, such as a realigned coupling between two
// modules, leaves the intended topology and is asked again. Adoption never
// approves or realigns a relation.
func Adopt(in Intended, units []model.Unit, id model.ID) (Intended, error) {
	actual := presentUnits(units)
	u, ok := actual[id]
	design := in.byID()
	switch _, isDesign := design[id]; {
	case isDesign:
		return in, fmt.Errorf("%s is already a design unit", id)
	case !ok:
		return in, fmt.Errorf("%s is not in the actual topology", id)
	}
	before := in
	in.Units = append(slices.Clone(in.Units), IntendedUnit{ID: id, Kind: u.Kind, Signature: signature(u)})
	in.Relations = slices.DeleteFunc(slices.Clone(in.Relations), func(e Edge) bool {
		if e.From != id && e.To != id {
			return false
		}
		it := Item{Change: NewRelation, Edge: e}
		was, _ := Intended{Units: before.Units}.Effect(it)
		now, _ := Intended{Units: in.Units}.Effect(it)
		return now == Blocked && was != Blocked
	})
	for e := range actualEdges(actual) {
		other := e.To
		if e.To == id {
			other = e.From
		}
		if _, toDesign := design[other]; (e.From == id || e.To == id) && !toDesign {
			in.Relations = append(in.Relations, e)
		}
	}
	return in.normalized(), nil
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
