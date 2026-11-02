package model

import (
	"fmt"
	"regexp"
	"slices"
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

// Fact is one field value and the writer that set it.
type Fact struct {
	Value  string `yaml:"value"`
	Writer Writer `yaml:"writer"`
	// Source is where a human stated the value, such as a PR URL.
	Source string `yaml:"source,omitempty"`
	// Origin is what the reader of the state sees. Resolve derives it from Writer.
	Origin Origin `yaml:"-"`
}

// Said is a value a writer other than a human set.
func Said(w Writer, value string) Fact { return Fact{Value: value, Writer: w, Origin: w.Origin()} }

// Stated is a value a human stated, with where.
func Stated(value, source string) Fact {
	return Fact{Value: value, Writer: Human, Source: source, Origin: UserStated}
}

// Guess reports whether only a reader vouches for the value.
func (f *Fact) Guess() bool { return f != nil && f.Origin == Inferred }

// A why-field states intent, which code can never prove.
func (f Fact) validate(why bool) error {
	if err := sourced(f.Writer, f.Source); err != nil {
		return err
	}
	if why && f.Writer.Origin() == Extracted {
		return fmt.Errorf("a why-field is a human's or a reader's, not the %s's", f.Writer)
	}
	return nil
}

// sourced checks that what a human stated says where, and only that.
func sourced(w Writer, source string) error {
	switch {
	case w.Origin() == "":
		return fmt.Errorf("unknown writer %q", w)
	case w == Human && strings.TrimSpace(source) == "":
		return fmt.Errorf("what a human stated needs a source, such as a PR URL or \"conversation 2026-11-01\"")
	case w != Human && source != "":
		return fmt.Errorf("only what a human stated carries a source, not the %s's", w)
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

// Holds reports whether path is the anchor's code: the anchor's own file, or
// a file under it when it is a directory.
func (a Anchor) Holds(path string) bool {
	dir := strings.TrimSuffix(a.Path, "/")
	return path == dir || dir == "." || strings.HasPrefix(path, dir+"/")
}

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

// AbsentReason says why a unit is no longer there.
type AbsentReason string

const (
	// NotReported is a unit the script named and no longer reports.
	NotReported AbsentReason = "not-reported"
	// Withdrawn is a unit the setup supplement named and no longer reports.
	Withdrawn AbsentReason = "withdrawn"
	// EvidenceDead is a semantic unit whose evidence is all dead.
	EvidenceDead AbsentReason = "evidence-dead"
	// Retracted is a unit a reader or a human retracted.
	Retracted AbsentReason = "retracted"
	// Unneeded is a directory group no view needs.
	Unneeded AbsentReason = "unneeded"
)

var absentReasons = map[AbsentReason]bool{NotReported: true, Withdrawn: true, EvidenceDead: true, Retracted: true, Unneeded: true}

// Unit is one design unit: a module, an external contract, or a feature.
type Unit struct {
	ID   ID   `yaml:"id"`
	Kind Kind `yaml:"kind"`
	// SupplementRef is the setup supplement's own ref for the unit, so a
	// re-run of setup finds it without comparing evidence that sync rewrites.
	// A store from before the ref spells it supplement_key; the store reads that.
	SupplementRef string   `yaml:"supplement_ref,omitempty"`
	Name          Fact     `yaml:"name"`
	Anchors       []Anchor `yaml:"anchors"`
	// Parents holds every writer's claim on the module or feature that
	// contains the unit. Resolve picks one into Parent.
	Parents []ParentClaim `yaml:"parents,omitempty"`
	// Evidence is what a semantic unit, one the script did not report, rests on.
	Evidence       []Evidence `yaml:"evidence,omitempty"`
	Purpose        *Fact      `yaml:"purpose,omitempty"`
	Responsibility *Fact      `yaml:"responsibility,omitempty"`
	// Signature is the observable shape of an external contract.
	Signature *Fact `yaml:"signature,omitempty"`
	// Entries are the contract's parts the script reads one by one, such as
	// a route, a method, a flag or a field, so each has its own row, its own
	// consumers and its own diff line. Signature then says what they do not.
	Entries []Entry `yaml:"entries,omitempty"`
	// Digest fingerprints the code behind a contract's anchors, so a diff sees
	// a change of meaning the signature cannot show.
	Digest string `yaml:"digest,omitempty"`
	// Relations is the shown claim on each edge. Hidden holds the other
	// writers' claims on a shown edge and the claims a mask hides.
	Relations []Relation `yaml:"relations,omitempty"`
	Hidden    []Relation `yaml:"hidden,omitempty"`
	Masks     []Mask     `yaml:"masks,omitempty"`
	// Absent marks a unit that is no longer there, and AbsentReason says why.
	Absent       bool         `yaml:"absent,omitempty"`
	AbsentReason AbsentReason `yaml:"absent_reason,omitempty"`
	// Parent, ParentOrigin and ParentWriter are the winning parent claim,
	// derived by Resolve.
	Parent       ID     `yaml:"-"`
	ParentOrigin Origin `yaml:"-"`
	ParentWriter Writer `yaml:"-"`
	// Notes is the user-owned "## Notes" section, carried through untouched.
	Notes string `yaml:"-"`
}

// Semantic reports whether only a reader vouches for the unit: the script
// never reported it, so its evidence decides whether it is still there.
func (u Unit) Semantic() bool { return u.Name.Writer.Origin() == Inferred }

// SetAbsent marks the unit gone for reason, or present when reason is "".
func (u *Unit) SetAbsent(reason AbsentReason) { u.Absent, u.AbsentReason = reason != "", reason }

// Claims lists every relation claim the unit stores, shown or hidden.
func (u Unit) Claims() []Relation { return append(slices.Clone(u.Relations), u.Hidden...) }

// EachEvidence visits every evidence item the unit stores, with the writer
// of the claim it belongs to.
func (u *Unit) EachEvidence(f func(w Writer, e *Evidence)) {
	for i := range u.Evidence {
		f(u.Name.Writer, &u.Evidence[i])
	}
	for i := range u.Parents {
		for j := range u.Parents[i].Evidence {
			f(u.Parents[i].Writer, &u.Parents[i].Evidence[j])
		}
	}
	for _, rs := range [][]Relation{u.Relations, u.Hidden} {
		for i := range rs {
			for j := range rs[i].Evidence {
				f(rs[i].Writer, &rs[i].Evidence[j])
			}
		}
	}
}

// EachShownEvidence visits the items of the claims that show: the unit's
// own, its winning parent claim's and each shown relation's. An outranked
// claim's items cannot change what anyone sees.
func (u *Unit) EachShownEvidence(f func(w Writer, e *Evidence)) {
	for i := range u.Evidence {
		f(u.Name.Writer, &u.Evidence[i])
	}
	if c := u.ShownParent(); c != nil {
		for j := range c.Evidence {
			f(c.Writer, &c.Evidence[j])
		}
	}
	for i := range u.Relations {
		for j := range u.Relations[i].Evidence {
			f(u.Relations[i].Writer, &u.Relations[i].Evidence[j])
		}
	}
}

// ShownParent is the parent claim Resolve picked, or nil.
func (u *Unit) ShownParent() *ParentClaim {
	for i := range u.Parents {
		if u.Parents[i].Writer == u.ParentWriter && u.Parents[i].To == u.Parent {
			return &u.Parents[i]
		}
	}
	return nil
}

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
	if len(u.Entries) > 0 && !u.Kind.IsContract() {
		return fmt.Errorf("%s: only contracts carry entries", u.ID)
	}
	if err := ValidateEntries(u.Entries); err != nil {
		return fmt.Errorf("%s: %w", u.ID, err)
	}
	seen := map[Writer]bool{}
	for _, c := range u.Parents {
		if err := c.validate(u.ID); err != nil {
			return err
		}
		if seen[c.Writer] {
			return fmt.Errorf("%s: two parent claims by the %s", u.ID, c.Writer)
		}
		seen[c.Writer] = true
	}
	if u.Semantic() && len(u.Evidence) == 0 {
		return fmt.Errorf("%s: a unit the script did not report needs evidence", u.ID)
	}
	if err := validateEvidence(u.Evidence); err != nil {
		return fmt.Errorf("%s evidence: %w", u.ID, err)
	}
	claims := map[string]bool{}
	for _, r := range u.Claims() {
		if err := r.validate(); err != nil {
			return fmt.Errorf("%s relation to %s: %w", u.ID, r.To, err)
		}
		k := fmt.Sprintf("%s %s %s", r.Kind, r.To, r.Writer)
		if claims[k] {
			return fmt.Errorf("%s: two %s claims by the %s on %s", u.ID, r.Kind, r.Writer, r.To)
		}
		claims[k] = true
	}
	for _, m := range u.Masks {
		if err := m.validate(); err != nil {
			return fmt.Errorf("%s mask on %s: %w", u.ID, m.Target, err)
		}
	}
	switch {
	case u.Absent && !absentReasons[u.AbsentReason]:
		return fmt.Errorf("%s: an absent unit needs an absent_reason, got %q", u.ID, u.AbsentReason)
	case !u.Absent && u.AbsentReason != "":
		return fmt.Errorf("%s: absent_reason on a present unit", u.ID)
	}
	return nil
}

func (r Relation) validate() error {
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if err := r.Via.Validate(); err != nil {
		return err
	}
	if err := validateEvidence(r.Evidence); err != nil {
		return err
	}
	if err := sourced(r.Writer, r.Source); err != nil {
		return err
	}
	if r.Writer.Origin() == Inferred && len(r.Evidence) == 0 {
		return fmt.Errorf("an INFERRED coupling needs evidence")
	}
	return nil
}

func validateEvidence(es []Evidence) error {
	for _, e := range es {
		if err := e.Validate(); err != nil {
			return err
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

// Holds reports whether a unit of kind k may contain others: a module holds
// its units, and a feature holds its parts, such as a skill's steps.
func (k Kind) Holds() bool { return k == Module || k == Feature }

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
// Lines, when given, is where the quote starts and ends, such as "120" or
// "120-134". Its status is stored by sync and never read from an extraction.
type Evidence struct {
	Path   string `yaml:"path" json:"path"`
	Lines  string `yaml:"lines,omitempty" json:"lines,omitempty"`
	Symbol string `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Quote  string `yaml:"quote" json:"quote"`
	Status Status `yaml:"status,omitempty" json:"-"`
	// Since is the revision of the sync that first found the item drifted.
	Since string `yaml:"since,omitempty" json:"-"`
	// Reason says why a dead item died.
	Reason string `yaml:"reason,omitempty" json:"-"`
}

// Same reports whether two items cite the same code, wherever sync last
// found it. The symbol only names where in the file to look, so an item that
// gains or changes one still cites the same code.
func (e Evidence) Same(o Evidence) bool {
	return e.Path == o.Path && e.Quote == o.Quote
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
	case e.Status != Live && e.Status != Drifted && e.Status != Dead:
		return fmt.Errorf("evidence %s: unknown status %q", e.Path, e.Status)
	case e.Status == Dead && e.Reason == "":
		return fmt.Errorf("evidence %s: a dead item needs a reason", e.Path)
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

// Relation is one writer's claim on an edge from the unit that holds it.
type Relation struct {
	To       ID           `yaml:"to"`
	Kind     RelationKind `yaml:"kind"`
	Writer   Writer       `yaml:"writer"`
	Source   string       `yaml:"source,omitempty"`
	Via      Via          `yaml:"via,omitempty"`
	Evidence []Evidence   `yaml:"evidence,omitempty"`
	// Entries narrows the claim to these entries of the contract it points
	// at, by key: the routes a page calls, the fields a module writes.
	Entries []string `yaml:"entries,omitempty"`
	// Digest fingerprints the lines that hold the live evidence.
	Digest string `yaml:"digest,omitempty"`
	// Origin and Dead are derived by Resolve. Dead marks a claim whose
	// evidence is all dead.
	Origin Origin `yaml:"-"`
	Dead   bool   `yaml:"-"`
}

// Status is the claim's: live while any item is, else drifted while any is.
func (r Relation) Status() Status { return StatusOf(r.Evidence) }

// Drifted reports whether the claim counts on evidence that drifted only.
func (r Relation) Drifted() bool { return r.Status() == Drifted }

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

// Present lists the units that are there, sorted by ID.
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
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
