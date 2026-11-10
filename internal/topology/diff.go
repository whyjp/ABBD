package topology

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Ref names a unit as a human reads it. Why says why a removed unit is
// gone, when the state records it.
type Ref struct {
	ID   model.ID           `json:"id"`
	Kind model.Kind         `json:"kind"`
	Name string             `json:"name"`
	Why  model.AbsentReason `json:"why,omitempty"`
}

// Changed is one field of a unit that changed: its name, its parent, its
// purpose or its responsibility, with the origin of the value now.
type Changed struct {
	Ref
	Field  string       `json:"field"`
	Old    string       `json:"old"`
	New    string       `json:"new"`
	Origin model.Origin `json:"origin,omitempty"`
}

// Signature is a contract whose signature changed.
type Signature struct {
	Ref
	Old string `json:"old"`
	New string `json:"new"`
}

// Entry is a change to one entry of a contract: Change is "added", "removed"
// or "changed". A change names the Field that changed, "name", "shape" or
// "required", with its Old and New value; an added or removed entry carries
// its shape and need.
type Entry struct {
	Ref
	Entry  string `json:"entry"`
	Change string `json:"change"`
	Field  string `json:"field,omitempty"`
	Old    string `json:"old,omitempty"`
	New    string `json:"new,omitempty"`
}

// Listed is a contract that came to list its entries, or stopped, so its
// entries are not each a change.
type Listed struct {
	Ref
	Entries int  `json:"entries"`
	Now     bool `json:"now"`
}

// Use is a unit that came to use one entry of a contract it already used,
// or stopped: a consumer of a route, a writer or a reader of a field.
type Use struct {
	Relation
	Entry string `json:"entry"`
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
	// Entries are the entries of a contract that were added, removed or
	// changed shape, and Listed the contracts that came to list entries or
	// stopped.
	Entries []Entry  `json:"contract_entries_changed"`
	Listed  []Listed `json:"contract_entries_listed"`
	// UsesAdded and UsesRemoved are entries a unit came to use, or stopped,
	// on a relation that was there before and is still there.
	UsesAdded   []Use `json:"entry_uses_added"`
	UsesRemoved []Use `json:"entry_uses_removed"`
	// UsesWidened are uses that came to take the whole contract, and
	// UsesNarrowed the reverse; a whole use takes every entry, so neither
	// is a line per entry.
	UsesWidened  []Relation `json:"entry_uses_widened"`
	UsesNarrowed []Relation `json:"entry_uses_narrowed"`
	// RelationsAdded and RelationsRemoved are drawn relations between
	// top-level units; one from an inside unit counts as its module's.
	RelationsAdded   []Relation `json:"relations_added"`
	RelationsRemoved []Relation `json:"relations_removed"`
	InferredAdded    []Coupling `json:"inferred_added"`
	// InferredDead are reader's couplings that were live and whose
	// evidence is now dead, so the blueprint no longer draws them.
	InferredDead []Coupling `json:"inferred_dead"`
	// Changed are the units that kept their ID and changed a name, a
	// parent, a purpose or a responsibility.
	Changed []Changed `json:"units_changed"`
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
		default:
			d.fields(old, u)
			if u.Kind.IsContract() {
				d.contract(old, u)
			}
		}
	}
	gone := map[model.ID]model.AbsentReason{}
	for _, u := range now {
		if u.Absent {
			gone[u.ID] = u.AbsentReason
		}
	}
	for _, id := range ids(b) {
		if _, ok := n[id]; !ok {
			r := ref(b[id])
			r.Why = gone[id]
			d.UnitsRemoved = append(d.UnitsRemoved, r)
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
	d.UsesAdded, d.UsesRemoved = newUses(b, n, was, is), newUses(n, b, is, was)
	d.UsesWidened, d.UsesNarrowed = widened(b, n, was, is), widened(n, b, is, was)
	old := inferred(b)
	cur := inferred(n)
	for _, k := range keys(cur) {
		if _, ok := old[k]; !ok {
			d.InferredAdded = append(d.InferredAdded, cur[k])
		}
	}
	died := deadInferred(n)
	for _, k := range keys(died) {
		// Another reader's coupling on the same edge may still be live.
		if _, live := cur[k]; live {
			continue
		}
		if _, ok := old[k]; ok {
			d.InferredDead = append(d.InferredDead, died[k])
		}
	}
	return d
}

// fields notes each field a unit kept its ID through and changed.
func (d *Diff) fields(old, u model.Unit) {
	val := func(f *model.Fact) (string, model.Origin) {
		if f == nil {
			return "", ""
		}
		return f.Value, f.Origin
	}
	op, _ := val(old.Purpose)
	np, no := val(u.Purpose)
	or, _ := val(old.Responsibility)
	nr, ro := val(u.Responsibility)
	for _, c := range []Changed{
		{Field: "name", Old: old.Name.Value, New: u.Name.Value, Origin: u.Name.Origin},
		{Field: "parent", Old: string(old.Parent), New: string(u.Parent), Origin: u.ParentOrigin},
		{Field: "purpose", Old: op, New: np, Origin: no},
		{Field: "responsibility", Old: or, New: nr, Origin: ro},
	} {
		if c.Old != c.New {
			c.Ref = ref(u)
			d.Changed = append(d.Changed, c)
		}
	}
}

// contract notes how one contract changed: its signature, its entries one
// by one, else its code under the same shape.
func (d *Diff) contract(old, u model.Unit) {
	shape := false
	if sig(old) != sig(u) {
		d.Signatures = append(d.Signatures, Signature{Ref: ref(u), Old: sig(old), New: sig(u)})
		shape = true
	}
	switch {
	case len(old.Entries) == 0 && len(u.Entries) > 0:
		d.Listed = append(d.Listed, Listed{Ref: ref(u), Entries: len(u.Entries), Now: true})
		shape = true
	case len(old.Entries) > 0 && len(u.Entries) == 0:
		d.Listed = append(d.Listed, Listed{Ref: ref(u), Entries: len(old.Entries)})
		shape = true
	default:
		for _, e := range u.Entries {
			was, had := old.EntryNamed(e.Key)
			switch {
			case !had:
				d.Entries = append(d.Entries, Entry{Ref: ref(u), Entry: e.Name, Change: "added", New: e.Look()})
			default:
				n := len(d.Entries)
				for _, f := range []struct{ field, old, now string }{{"name", was.Name, e.Name}, {"shape", was.Shape, e.Shape}, {"required", was.Need(), e.Need()}} {
					if f.old != f.now {
						d.Entries = append(d.Entries, Entry{Ref: ref(u), Entry: e.Name, Change: "changed", Field: f.field, Old: f.old, New: f.now})
					}
				}
				if len(d.Entries) == n {
					continue
				}
			}
			shape = true
		}
		for _, e := range old.Entries {
			if _, ok := u.EntryNamed(e.Key); !ok {
				d.Entries = append(d.Entries, Entry{Ref: ref(u), Entry: e.Name, Change: "removed", Old: e.Look()})
				shape = true
			}
		}
	}
	if !shape && old.Digest != "" && u.Digest != "" && old.Digest != u.Digest {
		d.Code = append(d.Code, ref(u))
	}
}

// newUses lists the entries a unit's drawn relations use in now and did not
// in base, where the relation between the top-level units was there in both
// and the contract listed entries in both: a new relation is already a
// relation added, and a contract that came to list entries is one line.
func newUses(base, now map[model.ID]model.Unit, was, is map[string]Relation) []Use {
	had, whole := entryUses(base)
	uses, _ := entryUses(now)
	var out []Use
	for k, u := range uses {
		_, before := was[k.edge]
		_, after := is[k.edge]
		if _, used := had[k]; before && after && !used && !whole[u.Relation] && len(base[u.To].Entries) > 0 {
			out = append(out, u)
		}
	}
	slices.SortFunc(out, func(a, b Use) int {
		return strings.Compare(fmt.Sprint(a.From, a.Kind, a.To, a.Entry), fmt.Sprint(b.From, b.Kind, b.To, b.Entry))
	})
	return out
}

// widened lists the uses that take the whole contract in now and only some
// of its entries in base, under a relation there in both.
func widened(base, now map[model.ID]model.Unit, was, is map[string]Relation) []Relation {
	uses, wasWhole := entryUses(base)
	narrow := map[Relation]bool{}
	for k, u := range uses {
		_, before := was[k.edge]
		_, after := is[k.edge]
		narrow[u.Relation] = before && after
	}
	_, whole := entryUses(now)
	var out []Relation
	for r := range whole {
		if narrow[r] && !wasWhole[r] {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Relation) int { return strings.Compare(a.line(), b.line()) })
	return out
}

// entryUse is one entry a unit uses, under the relation between the
// top-level units that holds it.
type entryUse struct {
	edge, use string
}

// entryUses lists each entry a drawn relation uses: the module that holds
// the user, the contract, the entry. Whole are the relations to a contract
// that lists entries and that name none of them, so use all of them.
func entryUses(units map[model.ID]model.Unit) (out map[entryUse]Use, whole map[Relation]bool) {
	out, whole = map[entryUse]Use{}, map[Relation]bool{}
	for _, u := range units {
		for _, r := range u.Relations {
			to, ok := units[r.To]
			if !ok || !r.Counts() {
				continue
			}
			from := top(units, u.ID)
			e := model.Edge{From: from, To: top(units, r.To), Kind: r.Kind}
			if len(r.Entries) == 0 && len(to.Entries) > 0 {
				whole[Relation{From: from, Kind: r.Kind, To: r.To, FromName: units[from].Name.Value, ToName: to.Name.Value}] = true
			}
			for _, k := range r.Entries {
				name := k
				if en, ok := to.EntryNamed(k); ok {
					name = en.Name
				}
				use := Use{Relation: Relation{From: from, Kind: r.Kind, To: r.To, FromName: units[from].Name.Value, ToName: to.Name.Value}, Entry: name}
				out[entryUse{e.String(), fmt.Sprint(from, r.Kind, r.To, k)}] = use
			}
		}
	}
	return out, whole
}

// Boundary reports whether the change reaches the boundary between modules:
// a contract's signature changed, a contract was added or removed, or a drawn
// relation between top-level units appeared or disappeared. A contract whose
// code changed under the same signature does not count; every edit to an
// interface file would. It informs a label; it decides nothing.
func (d Diff) Boundary() bool {
	contract := func(r Ref) bool { return r.Kind.IsContract() }
	return len(d.Signatures) > 0 || len(d.Entries) > 0 || len(d.Listed) > 0 || len(d.RelationsAdded) > 0 || len(d.RelationsRemoved) > 0 ||
		slices.ContainsFunc(d.UnitsAdded, contract) || slices.ContainsFunc(d.UnitsRemoved, contract)
}

// Lines says the difference in one line per change.
func (d Diff) Lines() []string {
	var out []string
	for _, r := range d.UnitsAdded {
		out = append(out, fmt.Sprintf("+ unit %s %s", r.ID, r.Name))
	}
	for _, r := range d.UnitsRemoved {
		s := fmt.Sprintf("- unit %s %s", r.ID, r.Name)
		if r.Why != "" {
			s += " (" + string(r.Why) + ")"
		}
		out = append(out, s)
	}
	for _, s := range d.Signatures {
		out = append(out, fmt.Sprintf("~ contract %s %s signature: %q -> %q", s.ID, s.Name, s.Old, s.New))
	}
	for _, l := range d.Listed {
		n := fmt.Sprintf("%d entries", l.Entries)
		if l.Entries == 1 {
			n = "1 entry"
		}
		if l.Now {
			out = append(out, fmt.Sprintf("~ contract %s %s now lists its %s", l.ID, l.Name, n))
		} else {
			out = append(out, fmt.Sprintf("~ contract %s %s no longer lists its %s", l.ID, l.Name, n))
		}
	}
	for _, e := range d.Entries {
		switch e.Change {
		case "added":
			out = append(out, strings.TrimSpace(fmt.Sprintf("+ entry %s %s %s %s", e.ID, e.Name, e.Entry, e.New)))
		case "removed":
			out = append(out, strings.TrimSpace(fmt.Sprintf("- entry %s %s %s %s", e.ID, e.Name, e.Entry, e.Old)))
		default:
			field := e.Field
			if field == "shape" {
				field = "signature"
				if e.Kind == model.Data || e.Kind == model.Config {
					field = "type"
				}
			}
			out = append(out, fmt.Sprintf("~ entry %s %s %s %s: %q -> %q", e.ID, e.Name, e.Entry, field, e.Old, e.New))
		}
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
	for _, u := range d.UsesAdded {
		out = append(out, fmt.Sprintf("+ entry use %s %s %s %s (%s %s %s)", u.From, u.Kind, u.To, u.Entry, u.FromName, u.Kind, u.ToName))
	}
	for _, u := range d.UsesRemoved {
		out = append(out, fmt.Sprintf("- entry use %s %s %s %s (%s %s %s)", u.From, u.Kind, u.To, u.Entry, u.FromName, u.Kind, u.ToName))
	}
	for _, r := range d.UsesWidened {
		out = append(out, fmt.Sprintf("~ entry use %s %s %s now uses the whole contract (%s %s %s)", r.From, r.Kind, r.To, r.FromName, r.Kind, r.ToName))
	}
	for _, r := range d.UsesNarrowed {
		out = append(out, fmt.Sprintf("~ entry use %s %s %s now uses only some of its entries (%s %s %s)", r.From, r.Kind, r.To, r.FromName, r.Kind, r.ToName))
	}
	for _, c := range d.Changed {
		switch c.Field {
		case "name":
			out = append(out, fmt.Sprintf("~ renamed %s %q -> %q%s", c.ID, c.Old, c.New, by(c.Origin)))
		case "parent":
			out = append(out, fmt.Sprintf("~ moved %s %s: %s -> %s%s", c.ID, c.Name, orTop(c.Old), orTop(c.New), by(c.Origin)))
		default:
			out = append(out, fmt.Sprintf("~ %s %s %s: %q -> %q%s", c.Field, c.ID, c.Name, c.Old, c.New, by(c.Origin)))
		}
	}
	for _, c := range d.InferredDead {
		out = append(out, "- inferred dead "+c.line())
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

func orTop(p string) string {
	if p == "" {
		return "top level"
	}
	return p
}

func by(o model.Origin) string {
	if o == "" {
		return ""
	}
	return ", " + string(o)
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

// deadInferred lists the reader's couplings whose evidence is dead, keyed
// by edge.
func deadInferred(units map[model.ID]model.Unit) map[string]Coupling {
	out := map[string]Coupling{}
	for _, u := range units {
		for _, r := range u.Relations {
			if _, ok := units[r.To]; !ok || !r.Guess() || !r.Dead {
				continue
			}
			out[model.Edge{From: u.ID, To: r.To, Kind: r.Kind}.String()] = Coupling{Relation: Relation{From: u.ID, Kind: r.Kind, To: r.To, FromName: u.Name.Value, ToName: units[r.To].Name.Value}, Via: r.Via}
		}
	}
	return out
}

// Within keeps the changes that touch a unit in, by either end of a
// relation or use, so a reviewer can read one module's or feature's share.
func (d Diff) Within(in func(model.ID) bool) Diff {
	refs := func(rs []Ref) []Ref { return keep(rs, func(r Ref) bool { return in(r.ID) }) }
	rels := func(rs []Relation) []Relation {
		return keep(rs, func(r Relation) bool { return in(r.From) || in(r.To) })
	}
	return Diff{
		UnitsAdded:       refs(d.UnitsAdded),
		UnitsRemoved:     refs(d.UnitsRemoved),
		Signatures:       keep(d.Signatures, func(s Signature) bool { return in(s.ID) }),
		Code:             refs(d.Code),
		Entries:          keep(d.Entries, func(e Entry) bool { return in(e.ID) }),
		Listed:           keep(d.Listed, func(l Listed) bool { return in(l.ID) }),
		UsesAdded:        keep(d.UsesAdded, func(u Use) bool { return in(u.From) || in(u.To) }),
		UsesRemoved:      keep(d.UsesRemoved, func(u Use) bool { return in(u.From) || in(u.To) }),
		UsesWidened:      rels(d.UsesWidened),
		UsesNarrowed:     rels(d.UsesNarrowed),
		RelationsAdded:   rels(d.RelationsAdded),
		RelationsRemoved: rels(d.RelationsRemoved),
		InferredAdded:    keep(d.InferredAdded, func(c Coupling) bool { return in(c.From) || in(c.To) }),
		InferredDead:     keep(d.InferredDead, func(c Coupling) bool { return in(c.From) || in(c.To) }),
		Changed:          keep(d.Changed, func(c Changed) bool { return in(c.ID) }),
	}
}

func keep[T any](xs []T, ok func(T) bool) []T {
	var out []T
	for _, x := range xs {
		if ok(x) {
			out = append(out, x)
		}
	}
	return out
}

// RelationEnds lists the units a relation of rs really joins in units:
// the ends of each counted relation that lifts onto one of rs.
func RelationEnds(units []model.Unit, rs []Relation) []model.ID {
	want := map[string]bool{}
	for _, r := range rs {
		want[model.Edge{From: r.From, To: r.To, Kind: r.Kind}.String()] = true
	}
	m := byID(units)
	var out []model.ID
	for _, u := range m {
		for _, r := range u.Relations {
			if _, ok := m[r.To]; !ok || !r.Counts() {
				continue
			}
			if want[model.Edge{From: top(m, u.ID), To: top(m, r.To), Kind: r.Kind}.String()] {
				out = append(out, u.ID, r.To)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Subtree says whether a unit is id or sits under it, in either state, so
// a unit that moved in or out still counts.
func Subtree(id model.ID, states ...[]model.Unit) func(model.ID) bool {
	in := map[model.ID]bool{id: true}
	for _, units := range states {
		m := map[model.ID]model.Unit{}
		for _, u := range units {
			m[u.ID] = u
		}
		for _, u := range units {
			seen := map[model.ID]bool{}
			for p := u.ID; p != "" && !seen[p]; p = m[p].Parent {
				seen[p] = true
				if p == id {
					in[u.ID] = true
				}
			}
		}
	}
	return func(x model.ID) bool { return in[x] }
}

// String is the whole difference as text, or a line saying there is none.
func (d Diff) String() string {
	ls := d.Lines()
	if len(ls) == 0 {
		return "no topology change\n"
	}
	return strings.Join(ls, "\n") + "\n"
}
