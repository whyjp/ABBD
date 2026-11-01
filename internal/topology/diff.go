package topology

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Ref names a unit as a human reads it.
type Ref struct {
	ID   model.ID   `json:"id"`
	Kind model.Kind `json:"kind"`
	Name string     `json:"name"`
}

// Signature is a contract whose signature changed.
type Signature struct {
	Ref
	Old string `json:"old"`
	New string `json:"new"`
}

// Relation is a relation between two units, with their names.
type Relation struct {
	From     model.ID           `json:"from"`
	Kind     model.RelationKind `json:"kind"`
	To       model.ID           `json:"to"`
	FromName string             `json:"from_name"`
	ToName   string             `json:"to_name"`
}

// Coupling is a reader's relation, with how it arises and the code it rests on.
type Coupling struct {
	Relation
	Via   model.Via `json:"via,omitempty"`
	Quote string    `json:"quote,omitempty"`
}

// Diff is the topology difference between two states of the code, for a PR
// description and a human's review.
type Diff struct {
	UnitsAdded   []Ref `json:"units_added"`
	UnitsRemoved []Ref `json:"units_removed"`
	// Signatures are the contracts whose observable shape changed.
	Signatures []Signature `json:"contract_signatures_changed"`
	// Code are the contracts whose code changed under the same signature.
	Code []Ref `json:"contract_code_changed"`
	// RelationsAdded and RelationsRemoved are drawn relations between
	// top-level units; one from an inside unit counts as its module's.
	RelationsAdded   []Relation `json:"relations_added"`
	RelationsRemoved []Relation `json:"relations_removed"`
	InferredAdded    []Coupling `json:"inferred_added"`
}

// Compare lists what changed from base to now, each category sorted.
func Compare(base, now []model.Unit) Diff {
	b, n := byID(base), byID(now)
	var d Diff
	for _, id := range ids(n) {
		u := n[id]
		old, had := b[id]
		switch {
		case !had:
			d.UnitsAdded = append(d.UnitsAdded, ref(u))
		case !u.Kind.IsContract():
		case sig(old) != sig(u):
			d.Signatures = append(d.Signatures, Signature{Ref: ref(u), Old: sig(old), New: sig(u)})
		case old.Digest != "" && u.Digest != "" && old.Digest != u.Digest:
			d.Code = append(d.Code, ref(u))
		}
	}
	for _, id := range ids(b) {
		if _, ok := n[id]; !ok {
			d.UnitsRemoved = append(d.UnitsRemoved, ref(b[id]))
		}
	}
	was, is := between(b), between(n)
	for _, k := range keys(is) {
		if _, ok := was[k]; !ok {
			d.RelationsAdded = append(d.RelationsAdded, is[k])
		}
	}
	for _, k := range keys(was) {
		if _, ok := is[k]; !ok {
			d.RelationsRemoved = append(d.RelationsRemoved, was[k])
		}
	}
	old := inferred(b)
	cur := inferred(n)
	for _, k := range keys(cur) {
		if _, ok := old[k]; !ok {
			d.InferredAdded = append(d.InferredAdded, cur[k])
		}
	}
	return d
}

// Boundary reports whether the change reaches the boundary between modules:
// a contract's signature changed, a contract was added or removed, or a drawn
// relation between top-level units appeared or disappeared. A contract whose
// code changed under the same signature does not count; every edit to an
// interface file would. It informs a label; it decides nothing.
func (d Diff) Boundary() bool {
	contract := func(r Ref) bool { return r.Kind.IsContract() }
	return len(d.Signatures) > 0 || len(d.RelationsAdded) > 0 || len(d.RelationsRemoved) > 0 ||
		slices.ContainsFunc(d.UnitsAdded, contract) || slices.ContainsFunc(d.UnitsRemoved, contract)
}

// Lines says the difference in one line per change.
func (d Diff) Lines() []string {
	var out []string
	for _, r := range d.UnitsAdded {
		out = append(out, fmt.Sprintf("+ unit %s %s", r.ID, r.Name))
	}
	for _, r := range d.UnitsRemoved {
		out = append(out, fmt.Sprintf("- unit %s %s", r.ID, r.Name))
	}
	for _, s := range d.Signatures {
		out = append(out, fmt.Sprintf("~ contract %s %s signature: %q -> %q", s.ID, s.Name, s.Old, s.New))
	}
	for _, r := range d.Code {
		out = append(out, fmt.Sprintf("~ contract %s %s: its code changed under the same signature", r.ID, r.Name))
	}
	for _, r := range d.RelationsAdded {
		out = append(out, "+ relation "+r.line())
	}
	for _, r := range d.RelationsRemoved {
		out = append(out, "- relation "+r.line())
	}
	for _, c := range d.InferredAdded {
		s := "+ inferred " + c.line()
		if c.Via != "" {
			s += " via " + string(c.Via)
		}
		if c.Quote != "" {
			s += ": " + c.Quote
		}
		out = append(out, s)
	}
	return out
}

func (r Relation) line() string {
	return fmt.Sprintf("%s %s %s (%s %s %s)", r.From, r.Kind, r.To, r.FromName, r.Kind, r.ToName)
}

func byID(units []model.Unit) map[model.ID]model.Unit {
	m := map[model.ID]model.Unit{}
	for _, u := range model.Present(units) {
		m[u.ID] = u
	}
	return m
}

func ids(m map[model.ID]model.Unit) []model.ID {
	var out []model.ID
	for id := range m {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func ref(u model.Unit) Ref { return Ref{ID: u.ID, Kind: u.Kind, Name: u.Name.Value} }

func sig(u model.Unit) string {
	if u.Signature == nil {
		return ""
	}
	return u.Signature.Value
}

// top is the top-level unit that holds id: itself, or the module it sits in.
func top(units map[model.ID]model.Unit, id model.ID) model.ID {
	seen := map[model.ID]bool{}
	for {
		p := units[id].Parent
		if p == "" || seen[id] {
			return id
		}
		seen[id] = true
		id = p
	}
}

// between lists the drawn relations between top-level units, keyed by edge.
func between(units map[model.ID]model.Unit) map[string]Relation {
	out := map[string]Relation{}
	for _, u := range units {
		for _, r := range u.Relations {
			if _, ok := units[r.To]; !ok || !r.Counts() {
				continue
			}
			e := model.Edge{From: top(units, u.ID), To: top(units, r.To), Kind: r.Kind}
			if e.From != e.To {
				out[e.String()] = Relation{From: e.From, Kind: e.Kind, To: e.To, FromName: units[e.From].Name.Value, ToName: units[e.To].Name.Value}
			}
		}
	}
	return out
}

// inferred lists the live reader's couplings, keyed by edge.
func inferred(units map[model.ID]model.Unit) map[string]Coupling {
	out := map[string]Coupling{}
	for _, u := range units {
		for _, r := range u.Relations {
			if _, ok := units[r.To]; !ok || !r.Guess() || r.Dead {
				continue
			}
			c := Coupling{Relation: Relation{From: u.ID, Kind: r.Kind, To: r.To, FromName: u.Name.Value, ToName: units[r.To].Name.Value}, Via: r.Via}
			if len(r.Evidence) > 0 {
				c.Quote = r.Evidence[0].String()
			}
			out[model.Edge{From: u.ID, To: r.To, Kind: r.Kind}.String()] = c
		}
	}
	return out
}

// String is the whole difference as text, or a line saying there is none.
func (d Diff) String() string {
	ls := d.Lines()
	if len(ls) == 0 {
		return "no topology change\n"
	}
	return strings.Join(ls, "\n") + "\n"
}
