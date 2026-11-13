package model

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
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
	// Digest is the unit's code as it was when a script's or a reader's
	// sentence about a module or feature was first kept, so the sentence
	// reads STALE once that code moves on. A human's is never stamped.
	Digest string `yaml:"digest,omitempty"`
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
	// Exec is an execution context: a thread, a worker pool, an event loop,
	// an async runtime, or the tasks one spawn site starts.
	Exec Kind = "EXEC"
)

var kinds = map[Kind]string{
	Module: "architecture", Interface: "architecture", Event: "architecture",
	Data: "architecture", Config: "architecture", Feature: "features",
	Exec: "architecture",
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

// Closeness says how closely the anchor names the code a quote lies in, -1
// when it does not hold it. A symbol anchor holds only a quote that names
// its symbol in its file, and is closest; a file anchor holds its file; a
// directory anchor the files under it, the deeper the closer; and . every
// file, least closely.
func (a Anchor) Closeness(e Evidence) int {
	dir := strings.TrimSuffix(a.Path, "/")
	switch {
	case a.Symbol != "":
		if e.Path == dir && e.Symbol == a.Symbol {
			return len(dir) + 2
		}
	case e.Path == dir:
		return len(dir) + 1
	case dir == ".":
		return 0
	case strings.HasPrefix(e.Path, dir+"/"):
		return len(dir)
	}
	return -1
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
	// Artifact is the set of what the build makes of a module, such as a
	// shared library or a plugin module the host loads, as ArtifactSet keeps
	// it, with whoever said it: the script that reads the build, or the
	// supplement where no script does.
	Artifact *Fact `yaml:"artifact,omitempty"`
	// Run is how an execution context dispatches its work and how many of it
	// run, as ParseRun reads it, such as "tick 1". Only an EXEC carries it.
	Run *Fact `yaml:"run,omitempty"`
	// Relations is the shown claim on each edge. Hidden holds the other
	// writers' claims on a shown edge and the claims a mask hides.
	Relations []Relation `yaml:"relations,omitempty"`
	Hidden    []Relation `yaml:"hidden,omitempty"`
	// Phrases holds each writer's words for what crosses an edge from the
	// unit. They label lines and never make, keep or bring back one.
	Phrases []PhraseNote `yaml:"phrases,omitempty"`
	Masks   []Mask       `yaml:"masks,omitempty"`
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
	for i := range u.Phrases {
		for j := range u.Phrases[i].Evidence {
			f(u.Phrases[i].Writer, &u.Phrases[i].Evidence[j])
		}
	}
}

// EachShownEvidence visits the items of the claims that show: the unit's
// own, its winning parent claim's, each shown relation's, the claim that
// gives a shown edge its condition or, dead, gave it, and the note that names each shown
// edge. An outranked claim's items cannot change what anyone sees.
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
	for i := range u.Hidden {
		r := &u.Hidden[i]
		if c, ok := u.ConditionEvidence(r.Kind, r.To); ok && c == r && u.Shown(r.Kind, r.To) {
			for j := range r.Evidence {
				f(r.Writer, &r.Evidence[j])
			}
		}
	}
	for i := range u.Phrases {
		n := &u.Phrases[i]
		if s, ok := u.Note(n.Kind, n.To); ok && s.Writer == n.Writer && u.Shown(n.Kind, n.To) {
			for j := range n.Evidence {
				f(n.Writer, &n.Evidence[j])
			}
		}
	}
}

// Shown reports whether a live or drifted claim shows on the edge.
func (u Unit) Shown(kind RelationKind, to ID) bool {
	return slices.ContainsFunc(u.Relations, func(r Relation) bool { return r.To == to && r.Kind == kind && !r.Dead })
}

// Note is the phrase note that names what crosses an edge: the first, by
// status and then the table, that is neither dead nor masked. It says
// nothing about whether the edge still shows; Dangling does.
func (u Unit) Note(kind RelationKind, to ID) (PhraseNote, bool) {
	var best PhraseNote
	ok := false
	for _, n := range u.Phrases {
		s := n.Status()
		if n.To != to || n.Kind != kind || s == Dead || u.Masked(PhraseTarget(kind, to), n.Writer, PhraseDigest(n)) {
			continue
		}
		if !ok || order(PhraseField, n.Writer, s) < order(PhraseField, best.Writer, best.Status()) {
			best, ok = n, true
		}
	}
	return best, ok
}

// Condition is the condition an edge holds under, and the origin of the
// claim that gives it: the shown claim's own, or, when it gives none, that
// of the first other live or drifted claim on the edge that a mask does not
// hide. The script reads the edge, and a reader may know a condition the
// script cannot read, such as a CMake if(). A link scope says who sees the
// link, not when it holds, so any claim's condition shows over it.
func (u Unit) Condition(kind RelationKind, to ID) (string, Origin) {
	if r, ok := u.conditionClaim(kind, to, false); ok {
		return r.When, r.Writer.Origin()
	}
	return "", ""
}

// ConditionEvidence is the claim whose evidence backs the edge's condition:
// the one Condition reads, or, while no live claim gives a condition, a dead
// one, so its death stays counted and listed until a reader refreshes or
// retracts it.
func (u Unit) ConditionEvidence(kind RelationKind, to ID) (*Relation, bool) {
	return u.conditionClaim(kind, to, true)
}

func (u Unit) conditionClaim(kind RelationKind, to ID, dead bool) (*Relation, bool) {
	rank := func(r *Relation) int {
		n := 0
		if Scope(r.When) {
			n = 2
		}
		if r.Status() == Dead {
			n++
		}
		return n
	}
	var best *Relation
	for _, rs := range [][]Relation{u.Relations, u.Hidden} {
		for i := range rs {
			r := &rs[i]
			if r.To != to || r.Kind != kind || r.When == "" || !dead && r.Status() == Dead || u.RelationMasked(*r) {
				continue
			}
			if best == nil || rank(r) < rank(best) {
				best = r
			}
		}
	}
	return best, best != nil
}

// ConditionConflicts lists the shown edges where another live claim gives a
// condition, or a link scope, other than the shown claim's own. The shown
// claim's shows; the other stays stored until its writer drops it.
func ConditionConflicts(units []Unit) []string {
	var out []string
	for _, u := range Present(units) {
		for _, r := range u.Relations {
			if r.Dead || r.When == "" {
				continue
			}
			for _, h := range u.Hidden {
				if h.To != r.To || h.Kind != r.Kind || h.When == "" || h.When == r.When || Scope(h.When) != Scope(r.When) || h.Status() == Dead || u.RelationMasked(h) {
					continue
				}
				what := map[bool]string{false: "condition", true: "scope"}[Scope(r.When)]
				out = append(out, fmt.Sprintf("%s: the %s gives %s %q, the %s's %q shows", Edge{From: u.ID, To: r.To, Kind: r.Kind}, h.Writer, what, h.When, r.Writer, r.When))
			}
		}
	}
	return out
}

// DanglingNote is a phrase note that names no line, by its edge and writer.
type DanglingNote struct {
	Edge   Edge
	Writer Writer
}

// Dangling lists the edges of present units whose phrase note names no
// line: the edge has no live or drifted claim, or its far end is gone. A
// note never becomes the edge, so it waits until a writer who may retract it
// does: a reader for a reader's or the supplement's, a human for a human's.
func Dangling(units []Unit) []DanglingNote {
	present := map[ID]bool{}
	for _, u := range units {
		present[u.ID] = !u.Absent
	}
	var out []DanglingNote
	for _, u := range Present(units) {
		for _, n := range u.Phrases {
			if s, ok := u.Note(n.Kind, n.To); !ok || s.Writer != n.Writer {
				continue
			}
			if !present[n.To] || !u.Shown(n.Kind, n.To) {
				out = append(out, DanglingNote{Edge{From: u.ID, To: n.To, Kind: n.Kind}, n.Writer})
			}
		}
	}
	return out
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
	if u.Run != nil {
		if u.Kind != Exec {
			return fmt.Errorf("%s: only an execution context carries a run", u.ID)
		}
		if _, err := ParseRun(u.Run.Value); err != nil {
			return fmt.Errorf("%s: %w", u.ID, err)
		}
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
	notes := map[string]bool{}
	for _, n := range u.Phrases {
		if err := n.validate(); err != nil {
			return fmt.Errorf("%s phrase on %s %s: %w", u.ID, n.Kind, n.To, err)
		}
		for _, r := range u.Claims() {
			if r.To != n.To || r.Kind != n.Kind {
				continue
			}
			if err := ValidatePhrase(n.Text, r.Via); err != nil {
				return fmt.Errorf("%s phrase on %s %s: %w", u.ID, n.Kind, n.To, err)
			}
		}
		k := fmt.Sprintf("%s %s %s", n.Kind, n.To, n.Writer)
		if notes[k] {
			return fmt.Errorf("%s: two phrases by the %s on %s %s", u.ID, n.Writer, n.Kind, n.To)
		}
		notes[k] = true
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
	if err := r.Kind.Quoted(len(r.Evidence)); err != nil {
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

// PhraseNote is one writer's words for what crosses an edge from the unit
// that holds it, such as a route, a message or a query. It is not a claim on
// the edge: it labels the line while one shows, and names nothing once none
// does.
type PhraseNote struct {
	To       ID           `yaml:"to"`
	Kind     RelationKind `yaml:"kind"`
	Writer   Writer       `yaml:"writer"`
	Source   string       `yaml:"source,omitempty"`
	Text     string       `yaml:"text"`
	Evidence []Evidence   `yaml:"evidence,omitempty"`
}

// Status is the note's: live while any item is, else drifted while any is.
func (n PhraseNote) Status() Status { return StatusOf(n.Evidence) }

func (n PhraseNote) validate() error {
	if err := n.Kind.Validate(); err != nil {
		return err
	}
	if !slices.Contains(Precedence[PhraseField], n.Writer) {
		return fmt.Errorf("a phrase is a reader's or a human's words, not the %s's", n.Writer)
	}
	if err := sourced(n.Writer, n.Source); err != nil {
		return err
	}
	if err := validateEvidence(n.Evidence); err != nil {
		return err
	}
	switch {
	case n.Writer.Origin() == Inferred && len(n.Evidence) == 0:
		return fmt.Errorf("an INFERRED phrase needs evidence")
	case n.Text == "":
		return fmt.Errorf("a phrase needs its text")
	}
	return ValidatePhrase(n.Text, "")
}

const (
	// PhraseMax and PhraseWords bound a phrase, so it fits beside a line.
	PhraseMax, PhraseWords = 24, 4
)

// ValidatePhrase checks that a phrase is a few words on one line that say
// more than its via does.
func ValidatePhrase(p string, via Via) error {
	switch {
	case p == "":
		return nil
	case strings.TrimSpace(p) != p || strings.ContainsAny(p, "\n\r\t"):
		return fmt.Errorf("phrase %q: one line, with no space at either end", p)
	case utf8.RuneCountInString(p) > PhraseMax:
		return fmt.Errorf("phrase %q: at most %d characters", p, PhraseMax)
	case len(strings.Fields(p)) > PhraseWords:
		return fmt.Errorf("phrase %q: at most %d words", p, PhraseWords)
	case via != "" && via.restates(p):
		return fmt.Errorf("phrase %q only repeats the via %s; name what crosses, such as a route, a message or a query", p, via)
	}
	return nil
}

// viaSynonyms are phrases that say no more than their via, in lower case.
var viaSynonyms = map[Via][]string{
	"import":        {"imports", "types", "model types", "type import", "package import", "uses"},
	"call":          {"calls", "function call", "method call", "invokes", "invocation", "함수 호출"},
	"data":          {"reads data", "writes data", "data flow"},
	"config":        {"configuration", "settings", "options"},
	"trait":         {"interface", "implements"},
	"callback":      {"callbacks", "hook"},
	"macro":         {"macros"},
	"build-feature": {"build feature", "feature flag"},
	"link":          {"links", "linked", "linking", "link time"},
	"convention":    {"conventions", "naming"},
	"load":          {"loads", "dynamic load", "plugin load", "dlopen"},
	"spawn":         {"spawns", "process", "subprocess", "exec"},
	"queue":         {"queues", "enqueue", "work queue"},
	"channel":       {"channels", "sends", "message", "messages"},
	"signal":        {"signals", "notify", "wakes", "wake up"},
	"proxy":         {"proxies", "forwards", "delegates"},
}

// restates reports whether a phrase only says the via in other words.
func (v Via) restates(p string) bool {
	l := strings.ToLower(p)
	return l == string(v) || p == v.Word() || slices.Contains(viaSynonyms[v], l)
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
	// RunsOn says the unit's code runs on an execution context. From one
	// execution context to another, its tasks run on that runtime.
	RunsOn RelationKind = "runs_on"
	// PostsTo says the unit hands work or data to an execution context.
	PostsTo RelationKind = "posts_to"
)

// ends says which kinds of unit each relation kind may join: a module or a
// feature provides a contract, anything may consume a contract or depend on
// any unit, a module, a feature or an execution context runs on an execution
// context, and a module or an execution context posts to one. Quotes is the
// fewest evidence items a claim of the kind needs: a posts_to quotes the
// producer and the consumer.
var ends = map[RelationKind]struct {
	from, to func(Kind) bool
	quotes   int
}{
	Provides:  {from: implementer, to: Kind.IsContract},
	Consumes:  {from: anyKind, to: Kind.IsContract},
	DependsOn: {from: anyKind, to: anyKind},
	RunsOn:    {from: func(k Kind) bool { return implementer(k) || k == Exec }, to: isExec},
	PostsTo:   {from: func(k Kind) bool { return k == Module || k == Exec }, to: isExec, quotes: 2},
}

func implementer(k Kind) bool { return k == Module || k == Feature }
func anyKind(Kind) bool       { return true }
func isExec(k Kind) bool      { return k == Exec }

// Uses reports whether a relation of kind k runs from a unit that uses the
// other: it consumes it or depends on it. A provides runs from the unit that
// implements a contract to the contract it serves, and uses nothing. A
// runs_on or a posts_to is a run-time fact, not a dependency of the code:
// the context calls the code that runs on it, and the code a producer calls
// to post is its own depends_on or consumes. Neither is a use, so neither
// breaches a layer.
func (k RelationKind) Uses() bool { return k == Consumes || k == DependsOn }

// RunTime reports whether a relation of kind k says what runs on an
// execution context or what is handed to one.
func (k RelationKind) RunTime() bool { return k == RunsOn || k == PostsTo }

// Quoted checks that a claim of kind k cites enough evidence items.
func (k RelationKind) Quoted(n int) error {
	if q := ends[k].quotes; n < q {
		return fmt.Errorf("a %s quotes both the producer and the consumer, so it needs %d or more evidence items, got %d", k, q, n)
	}
	return nil
}

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
		return fmt.Errorf("%s %s %s is not a relation ABBD knows: a module or feature provides a contract, anything consumes a contract or depends_on any unit, a module, feature or EXEC runs_on an EXEC, and a module or EXEC posts_to an EXEC", from, k, to)
	}
	return nil
}

// MaxWhen bounds a relation's condition, which names a feature, a cfg or a
// scope rather than restating the build file.
const MaxWhen = 60

// Scope says a relation's when is a link scope, which says who sees the
// link rather than when it holds.
func Scope(when string) bool {
	return when == "PRIVATE" || when == "PUBLIC" || when == "INTERFACE"
}

// ValidateWhen refuses a condition that runs over MaxWhen characters or
// over one line.
func ValidateWhen(w string) error {
	if strings.ContainsAny(w, "\r\n") || len([]rune(w)) > MaxWhen {
		return fmt.Errorf("condition %q: one line of at most %d characters, such as feature = \"sinks-kafka\", cfg(unix) or dev", w, MaxWhen)
	}
	return nil
}

// Artifact is what a build makes of a module: an executable, a static or
// shared library, a module the host loads at run time, or a Rust cdylib.
type Artifact string

var artifactWords = map[Artifact]string{
	"exe": "실행 파일", "static": "정적 라이브러리", "shared": "공유 라이브러리", "module": "로드 모듈", "cdylib": "cdylib",
}

// Built is the set of build artifacts the unit is, sorted, or none. A crate
// may build as a cdylib and a staticlib at once.
func (u Unit) Built() []Artifact {
	if u.Artifact == nil || u.Artifact.Value == "" {
		return nil
	}
	var out []Artifact
	for _, a := range strings.Split(u.Artifact.Value, ",") {
		out = append(out, Artifact(a))
	}
	return out
}

// ArtifactSet is a set of artifacts as a unit stores it: sorted, without
// repeats, joined by commas.
func ArtifactSet(as []Artifact) string {
	var ss []string
	for _, a := range as {
		ss = append(ss, string(a))
	}
	slices.Sort(ss)
	return strings.Join(slices.Compact(ss), ",")
}

// Words says a set of artifacts in the blueprint's words.
func Words(as []Artifact) string {
	var ws []string
	for _, a := range as {
		ws = append(ws, a.Word())
	}
	return strings.Join(ws, " · ")
}

// Word is the artifact in the blueprint's words, or "" for none.
func (a Artifact) Word() string { return artifactWords[a] }

func (a Artifact) Validate() error {
	if _, ok := artifactWords[a]; a != "" && !ok {
		return fmt.Errorf("unknown artifact %q: use exe, static, shared, module or cdylib", a)
	}
	return nil
}

// Via says how a coupling arises, so a human sees why two units are tied.
type Via string

var vias = map[Via]bool{
	"import": true, "trait": true, "callback": true, "macro": true, "config": true,
	"build-feature": true, "data": true, "convention": true, "call": true,
	"load": true, "spawn": true, "link": true,
	"queue": true, "channel": true, "signal": true, "proxy": true,
}

var viaWords = map[Via]string{
	"call": "호출", "data": "데이터", "convention": "관례", "config": "설정",
	"import": "임포트", "trait": "트레이트", "callback": "콜백", "macro": "매크로",
	"build-feature": "빌드 기능", "load": "로드", "spawn": "실행", "link": "링크",
	"queue": "큐", "channel": "채널", "signal": "신호", "proxy": "프록시",
}

// plain are the vias whose word a line's style already says: a solid line
// is a call and a dotted one a plan such as an import. The blueprint keeps
// their word off a line at rest and in its tooltip only.
var plain = map[Via]bool{"call": true, "import": true}

// Word is the via in the blueprint's words, or "" for none.
func (v Via) Word() string { return viaWords[v] }

// Plain says the via's word tells a reader nothing the line's style does not.
func (v Via) Plain() bool { return plain[v] }

func (v Via) Validate() error {
	if v != "" && !vias[v] {
		return fmt.Errorf("unknown via %q: use import, trait, callback, macro, config, build-feature, data, convention, call, load, spawn, link, queue, channel, signal or proxy", v)
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
	// When is the condition the relation holds under, as the build says it:
	// a feature, a cfg, a dev or build dependency, or a link scope such as
	// PRIVATE. Empty means it always holds.
	When string `yaml:"when,omitempty"`
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
