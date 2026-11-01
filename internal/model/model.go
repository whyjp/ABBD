package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Origin string

const (
	UserStated Origin = "USER_STATED"
	Adopted    Origin = "ADOPTED"
	Extracted  Origin = "EXTRACTED"
	Observed   Origin = "OBSERVED"
	Inferred   Origin = "INFERRED"
)

type Verification string

const (
	NotRun       Verification = "NOT_RUN"
	Passed       Verification = "PASSED"
	Failed       Verification = "FAILED"
	Unverifiable Verification = "UNVERIFIABLE"
)

type Confirmation string

const (
	Unconfirmed Confirmation = "UNCONFIRMED"
	Confirmed   Confirmation = "CONFIRMED"
	Rejected    Confirmation = "REJECTED"
)

var origins = map[Origin]bool{UserStated: true, Adopted: true, Extracted: true, Observed: true, Inferred: true}
var verifications = map[Verification]bool{"": true, NotRun: true, Passed: true, Failed: true, Unverifiable: true}
var confirmations = map[Confirmation]bool{"": true, Unconfirmed: true, Confirmed: true, Rejected: true}

// Fact is one field value with its evidence recorded on three independent axes.
type Fact struct {
	Value        string       `yaml:"value"`
	Origin       Origin       `yaml:"origin"`
	Verification Verification `yaml:"verification,omitempty"`
	Confirmation Confirmation `yaml:"confirmation,omitempty"`
	// Rejected is the guess a human rejected, kept beside the value that replaced it.
	Rejected string `yaml:"rejected,omitempty"`
}

// guess reports whether only an agent vouches for a value: INFERRED, and no
// human has confirmed or rejected it.
func guess(o Origin, c Confirmation) bool {
	return o == Inferred && c != Confirmed && c != Rejected
}

func (f *Fact) Guess() bool { return f != nil && guess(f.Origin, f.Confirmation) }

// A why-field states intent, which code can never prove.
func (f Fact) validate(why bool) error {
	if !origins[f.Origin] {
		return fmt.Errorf("unknown origin %q", f.Origin)
	}
	if !verifications[f.Verification] {
		return fmt.Errorf("unknown verification %q", f.Verification)
	}
	if !confirmations[f.Confirmation] {
		return fmt.Errorf("unknown confirmation %q", f.Confirmation)
	}
	if why && f.Origin != UserStated && f.Origin != Adopted && f.Origin != Inferred {
		return fmt.Errorf("why-field origin must be USER_STATED, ADOPTED or INFERRED, got %s", f.Origin)
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
	// Parent is the module that contains this unit.
	Parent ID `yaml:"parent,omitempty"`
	// Evidence is what a semantic unit, one the script did not report, rests on.
	Evidence       []Evidence `yaml:"evidence,omitempty"`
	Purpose        *Fact      `yaml:"purpose,omitempty"`
	Responsibility *Fact      `yaml:"responsibility,omitempty"`
	// Signature is the observable shape of an external contract.
	Signature *Fact `yaml:"signature,omitempty"`
	// Digest fingerprints the code behind a contract's anchors, so a turn sees
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
		if !origins[r.Origin] {
			return fmt.Errorf("%s relation: unknown origin %q", u.ID, r.Origin)
		}
		if !confirmations[r.Confirmation] {
			return fmt.Errorf("%s relation: unknown confirmation %q", u.ID, r.Confirmation)
		}
	}
	return nil
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
	To           ID           `yaml:"to"`
	Kind         RelationKind `yaml:"kind"`
	Origin       Origin       `yaml:"origin"`
	Confirmation Confirmation `yaml:"confirmation,omitempty"`
	Via          Via          `yaml:"via,omitempty"`
	Evidence     []Evidence   `yaml:"evidence,omitempty"`
	// Dead marks a semantic relation whose evidence the code no longer holds.
	Dead bool `yaml:"dead,omitempty"`
	// Digest fingerprints the lines that hold the live evidence, so a turn
	// sees an edit to them while the quote survives.
	Digest string `yaml:"digest,omitempty"`
}

// Counts reports whether the edge is part of the actual topology. A coupling
// the script cannot see enters it only once a human states or confirms it,
// and leaves it when its evidence is gone.
func (r Relation) Counts() bool {
	return !r.Dead && (r.Origin == Extracted || r.Origin == UserStated || r.Confirmation == Confirmed)
}

// Semantic reports whether a reader found the relation and left the evidence
// that keeps it alive.
func (r Relation) Semantic() bool { return len(r.Evidence) > 0 }

func (r Relation) Guess() bool { return guess(r.Origin, r.Confirmation) }

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
