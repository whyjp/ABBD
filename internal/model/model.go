package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Origin string

const (
	// Extracted facts come from the project's deterministic script.
	Extracted Origin = "EXTRACTED"
	// Inferred facts come from a pstack reader and rest on evidence.
	Inferred Origin = "INFERRED"
	// UserStated facts are what a human said.
	UserStated Origin = "USER_STATED"
)

var origins = map[Origin]bool{UserStated: true, Extracted: true, Inferred: true}

// Fact is one field value and where it came from.
type Fact struct {
	Value  string `yaml:"value"`
	Origin Origin `yaml:"origin"`
	// Source is where a human stated a USER_STATED value, such as a PR URL.
	Source string `yaml:"source,omitempty"`
}

// Guess reports whether only a reader vouches for the value.
func (f *Fact) Guess() bool { return f != nil && f.Origin == Inferred }

// A why-field states intent, which code can never prove.
func (f Fact) validate(why bool) error {
	if err := sourced(f.Origin, f.Source); err != nil {
		return err
	}
	if why && f.Origin == Extracted {
		return fmt.Errorf("why-field origin must be USER_STATED or INFERRED, got %s", f.Origin)
	}
	return nil
}

// sourced checks that a fact a human stated says where, and only such a fact.
func sourced(o Origin, source string) error {
	switch {
	case !origins[o]:
		return fmt.Errorf("unknown origin %q", o)
	case o == UserStated && strings.TrimSpace(source) == "":
		return fmt.Errorf("a USER_STATED fact needs a source, such as a PR URL or \"conversation 2026-11-01\"")
	case o != UserStated && source != "":
		return fmt.Errorf("only a USER_STATED fact carries a source, not %s", o)
	}
	return nil
}

type Kind string

const (
	Module    Kind = "MOD"
	Interface Kind = "IF"
	Event     Kind = "EVT"
	Data      Kind = "DATA"
	Config    Kind = "CFG"
	Feature   Kind = "FEAT"
)

var kinds = map[Kind]string{
	Module: "architecture", Interface: "architecture", Event: "architecture",
	Data: "architecture", Config: "architecture", Feature: "features",
}

// Domain-layer prefixes are reserved and not part of this restart.
var reserved = map[Kind]bool{"DOM": true, "STATE": true, "TRN": true, "INV": true}

func (k Kind) Dir() string { return kinds[k] }

func (k Kind) Validate() error {
	if reserved[k] {
		return fmt.Errorf("kind %s is reserved for the domain layer", k)
	}
	if _, ok := kinds[k]; !ok {
		return fmt.Errorf("unknown kind %q", k)
	}
	return nil
}

type Anchor struct {
	Path   string `yaml:"path" json:"path"`
	Symbol string `yaml:"symbol,omitempty" json:"symbol,omitempty"`
}

func (a Anchor) Key() string { return a.Path + "#" + a.Symbol }

func SortAnchors(as []Anchor) []Anchor {
	seen := map[string]bool{}
	out := make([]Anchor, 0, len(as))
	for _, a := range as {
		if !seen[a.Key()] {
			seen[a.Key()] = true
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

type ID string

var idPattern = regexp.MustCompile(`^([A-Z]+)-(\d{4,})$`)

func ParseID(s string) (Kind, int, error) {
	m := idPattern.FindStringSubmatch(s)
	if m == nil {
		return "", 0, fmt.Errorf("malformed id %q", s)
	}
	var n int
	fmt.Sscanf(m[2], "%d", &n)
	return Kind(m[1]), n, nil
}

func FormatID(k Kind, n int) ID { return ID(fmt.Sprintf("%s-%04d", k, n)) }

// Unit is one design unit: a module, an external contract, or a feature.
type Unit struct {
	ID      ID       `yaml:"id"`
	Kind    Kind     `yaml:"kind"`
	Name    Fact     `yaml:"name"`
	Anchors []Anchor `yaml:"anchors"`
	// Parent is the module that contains this unit. ParentOrigin says who
	// placed it there, the way a relation's origin does: the script
	// (EXTRACTED, also when empty), a reader citing ParentEvidence
	// (INFERRED), or a human naming ParentSource (USER_STATED). A sync keeps
	// a reader's or a human's parent unless the script reports one.
	Parent         ID         `yaml:"parent,omitempty"`
	ParentOrigin   Origin     `yaml:"parent_origin,omitempty"`
	ParentSource   string     `yaml:"parent_source,omitempty"`
	ParentEvidence []Evidence `yaml:"parent_evidence,omitempty"`
	// Evidence is what a semantic unit, one the script did not report, rests on.
	Evidence       []Evidence `yaml:"evidence,omitempty"`
	Purpose        *Fact      `yaml:"purpose,omitempty"`
	Responsibility *Fact      `yaml:"responsibility,omitempty"`
	// Signature is the observable shape of an external contract.
	Signature *Fact `yaml:"signature,omitempty"`
	// Digest fingerprints the code behind a contract's anchors, so a diff sees
	// a change of meaning the signature cannot show.
	Digest    string     `yaml:"digest,omitempty"`
	Relations []Relation `yaml:"relations,omitempty"`
	// Absent marks a unit the latest extraction did not report, or a semantic
	// unit whose evidence is all gone.
	Absent bool `yaml:"absent,omitempty"`
	// Notes is the user-owned "## Notes" section, carried through untouched.
	Notes string `yaml:"-"`
}

// Semantic reports whether only a reader vouches for the unit: the script
// never reported it, so its evidence decides whether it is still there.
func (u Unit) Semantic() bool { return u.Name.Origin == Inferred }

func (u Unit) Validate() error {
	if err := u.Kind.Validate(); err != nil {
		return err
	}
	k, _, err := ParseID(string(u.ID))
	if err != nil {
		return err
	}
	if k != u.Kind {
		return fmt.Errorf("%s: id prefix does not match kind %s", u.ID, u.Kind)
	}
	if len(u.Anchors) == 0 {
		return fmt.Errorf("%s: a unit needs at least one anchor", u.ID)
	}
	if err := u.Name.validate(false); err != nil {
		return fmt.Errorf("%s name: %w", u.ID, err)
	}
	for label, f := range map[string]*Fact{"purpose": u.Purpose, "responsibility": u.Responsibility} {
		if f != nil {
			if err := f.validate(true); err != nil {
				return fmt.Errorf("%s %s: %w", u.ID, label, err)
			}
		}
	}
	if u.Signature != nil {
		if !u.Kind.IsContract() {
			return fmt.Errorf("%s: only contracts carry a signature", u.ID)
		}
		if err := u.Signature.validate(false); err != nil {
			return fmt.Errorf("%s signature: %w", u.ID, err)
		}
	}
	if u.Parent != "" {
		if k, _, err := ParseID(string(u.Parent)); err != nil || k != Module || u.Parent == u.ID {
			return fmt.Errorf("%s: parent %s is not another module", u.ID, u.Parent)
		}
	}
	switch {
	case u.Parent == "" && (u.ParentOrigin != "" || u.ParentSource != "" || len(u.ParentEvidence) > 0):
		return fmt.Errorf("%s: a parent origin without a parent", u.ID)
	case u.ParentOrigin != "":
		if err := sourced(u.ParentOrigin, u.ParentSource); err != nil {
			return fmt.Errorf("%s parent: %w", u.ID, err)
		}
		if u.ParentOrigin == Inferred && len(u.ParentEvidence) == 0 {
			return fmt.Errorf("%s: a reader's parent %s needs evidence", u.ID, u.Parent)
		}
	}
	for _, e := range u.ParentEvidence {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("%s parent evidence: %w", u.ID, err)
		}
	}
	if u.Semantic() && len(u.Evidence) == 0 {
		return fmt.Errorf("%s: a unit the script did not report needs evidence", u.ID)
	}
	for _, e := range u.Evidence {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("%s evidence: %w", u.ID, err)
		}
	}
	for _, r := range u.Relations {
		if err := r.Kind.Validate(); err != nil {
			return fmt.Errorf("%s relation: %w", u.ID, err)
		}
		if err := r.Via.Validate(); err != nil {
			return fmt.Errorf("%s relation: %w", u.ID, err)
		}
		for _, e := range r.Evidence {
			if err := e.Validate(); err != nil {
				return fmt.Errorf("%s relation evidence: %w", u.ID, err)
			}
		}
		if err := sourced(r.Origin, r.Source); err != nil {
			return fmt.Errorf("%s relation: %w", u.ID, err)
		}
		if r.Origin == Inferred && len(r.Evidence) == 0 {
			return fmt.Errorf("%s relation to %s: an INFERRED coupling needs evidence", u.ID, r.To)
		}
	}
	return nil
}

// ParentCycle names a present unit whose chain of parents comes back to
// itself, or "" when the parents form a tree.
func ParentCycle(units []Unit) ID {
	parent := map[ID]ID{}
	for _, u := range Present(units) {
		parent[u.ID] = u.Parent
	}
	ids := make([]ID, 0, len(parent))
	for id := range parent {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		seen := map[ID]bool{}
		for k := id; k != ""; k = parent[k] {
			if seen[k] {
				return id
			}
			seen[k] = true
		}
	}
	return ""
}

func (k Kind) IsContract() bool {
	return k == Interface || k == Event || k == Data || k == Config
}

type RelationKind string

const (
	DependsOn RelationKind = "depends_on"
	Consumes  RelationKind = "consumes"
	Provides  RelationKind = "provides"
)

// ends says which kinds of unit each relation kind may join: a module or a
// feature provides a contract, and anything may consume a contract or depend
// on any unit.
var ends = map[RelationKind]struct{ from, to func(Kind) bool }{
	Provides:  {from: implementer, to: Kind.IsContract},
	Consumes:  {from: anyKind, to: Kind.IsContract},
	DependsOn: {from: anyKind, to: anyKind},
}

func implementer(k Kind) bool { return k == Module || k == Feature }
func anyKind(Kind) bool       { return true }

func (k RelationKind) Validate() error {
	if _, ok := ends[k]; !ok {
		return fmt.Errorf("unknown relation kind %q", k)
	}
	return nil
}

// Joins checks that a relation of kind k may run from a unit of kind from to
// one of kind to.
func (k RelationKind) Joins(from, to Kind) error {
	if err := k.Validate(); err != nil {
		return err
	}
	if e := ends[k]; !e.from(from) || !e.to(to) {
		return fmt.Errorf("%s %s %s is not a relation ABBD knows: a module or feature provides a contract, and anything consumes a contract or depends_on any unit", from, k, to)
	}
	return nil
}

// Via says how a coupling arises, so a human sees why two units are tied.
type Via string

var vias = map[Via]bool{
	"import": true, "trait": true, "callback": true, "macro": true, "config": true,
	"build-feature": true, "data": true, "convention": true, "call": true,
}

func (v Via) Validate() error {
	if v != "" && !vias[v] {
		return fmt.Errorf("unknown via %q: use import, trait, callback, macro, config, build-feature, data, convention or call", v)
	}
	return nil
}

// Evidence is a short verbatim excerpt of the code a semantic fact rests on.
// Lines, when given, is where the quote starts and ends, such as "120" or "120-134".
type Evidence struct {
	Path   string `yaml:"path" json:"path"`
	Lines  string `yaml:"lines,omitempty" json:"lines,omitempty"`
	Symbol string `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Quote  string `yaml:"quote" json:"quote"`
}

func (e Evidence) Anchor() Anchor { return Anchor{Path: e.Path, Symbol: e.Symbol} }

func (e Evidence) String() string {
	loc := e.Path
	if e.Lines != "" {
		loc += ":" + e.Lines
	}
	return fmt.Sprintf("%s %q", loc, e.Quote)
}

var linesPattern = regexp.MustCompile(`^(\d+)(?:-(\d+))?$`)

// Span reads Lines. ok is false when no lines are given.
func (e Evidence) Span() (first, last int, ok bool) {
	m := linesPattern.FindStringSubmatch(e.Lines)
	if m == nil {
		return 0, 0, false
	}
	fmt.Sscanf(m[1], "%d", &first)
	last = first
	if m[2] != "" {
		fmt.Sscanf(m[2], "%d", &last)
	}
	return first, last, true
}

func (e Evidence) Validate() error {
	switch {
	case e.Path == "":
		return fmt.Errorf("evidence needs a path")
	case e.Path == ".abbd" || strings.HasPrefix(e.Path, ".abbd/") || e.Path == ".git" || strings.HasPrefix(e.Path, ".git/"):
		return fmt.Errorf("evidence %s must cite the code, not ABBD's or git's own files, which repeat the quote", e.Path)
	case strings.TrimSpace(e.Quote) == "":
		return fmt.Errorf("evidence %s needs a quote", e.Path)
	case e.Lines != "":
		first, last, ok := e.Span()
		if !ok || first < 1 || last < first {
			return fmt.Errorf("evidence %s: lines %q is not a range such as 120 or 120-134", e.Path, e.Lines)
		}
	}
	return nil
}

// ParseEvidence reads PATH:LINES:QUOTE as the command line gives it. LINES may
// be empty, and the quote may itself hold colons.
func ParseEvidence(s string) (Evidence, error) {
	path, rest, ok := strings.Cut(s, ":")
	lines, quote, ok2 := strings.Cut(rest, ":")
	if !ok || !ok2 {
		return Evidence{}, fmt.Errorf("%q is not PATH:LINES:QUOTE, such as src/a.rs:120-122:if x != y {", s)
	}
	e := Evidence{Path: strings.TrimSpace(path), Lines: strings.TrimSpace(lines), Quote: quote}
	return e, e.Validate()
}

// Relation is an edge from the unit that holds it.
type Relation struct {
	To       ID           `yaml:"to"`
	Kind     RelationKind `yaml:"kind"`
	Origin   Origin       `yaml:"origin"`
	Source   string       `yaml:"source,omitempty"`
	Via      Via          `yaml:"via,omitempty"`
	Evidence []Evidence   `yaml:"evidence,omitempty"`
	// Dead marks a semantic relation whose evidence the code no longer holds.
	Dead bool `yaml:"dead,omitempty"`
	// Digest fingerprints the lines that hold the live evidence.
	Digest string `yaml:"digest,omitempty"`
}

// Counts reports whether the edge is drawn as the topology: the script read
// it or a human stated it. A reader's coupling is listed beside it instead.
func (r Relation) Counts() bool {
	return !r.Dead && (r.Origin == Extracted || r.Origin == UserStated)
}

// Semantic reports whether a reader found the relation and left the evidence
// that keeps it alive.
func (r Relation) Semantic() bool { return len(r.Evidence) > 0 }

func (r Relation) Guess() bool { return r.Origin == Inferred }

// Edge names a relation by its ends and kind.
type Edge struct {
	From, To ID
	Kind     RelationKind
}

// String names the edge on the command line, so it holds no character a shell
// treats specially.
func (e Edge) String() string { return fmt.Sprintf("%s.%s.%s", e.From, e.Kind, e.To) }

// Joins checks the edge's kind against the kinds its IDs name.
func (e Edge) Joins() error {
	from, _, _ := ParseID(string(e.From))
	to, _, _ := ParseID(string(e.To))
	return e.Kind.Joins(from, to)
}

// ParseEdge reads an edge as Edge.String prints it.
func ParseEdge(s string) (Edge, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Edge{}, fmt.Errorf("%q is not an edge such as MOD-0001.depends_on.MOD-0002", s)
	}
	e := Edge{From: ID(parts[0]), Kind: RelationKind(parts[1]), To: ID(parts[2])}
	for _, id := range []ID{e.From, e.To} {
		if _, _, err := ParseID(string(id)); err != nil {
			return Edge{}, err
		}
	}
	return e, e.Kind.Validate()
}

// Present lists the units the latest sync found, sorted by ID.
func Present(units []Unit) []Unit {
	var out []Unit
	for _, u := range units {
		if !u.Absent {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func SortRelations(rs []Relation) []Relation {
	out := append([]Relation(nil), rs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
