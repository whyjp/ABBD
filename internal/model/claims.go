package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Writer is who set a fact, a relation or a parent claim. The origin a reader
// of the state sees follows from it.
type Writer string

const (
	Script     Writer = "script"
	Group      Writer = "group"
	Supplement Writer = "supplement"
	Reader     Writer = "reader"
	Human      Writer = "human"
)

var writerOrigin = map[Writer]Origin{
	Script: Extracted, Group: Extracted, Supplement: Inferred, Reader: Inferred, Human: UserStated,
}

// Origin is "" for a writer ABBD does not know.
func (w Writer) Origin() Origin { return writerOrigin[w] }

// Status is an evidence item's: live (""), drifted or dead.
type Status string

const (
	Live    Status = ""
	Drifted Status = "drifted"
	Dead    Status = "dead"
)

// StatusOf is a claim's status: live while any item is, else drifted while
// any is. A claim without evidence rests on its writer, so it is live.
func StatusOf(es []Evidence) Status {
	if len(es) == 0 {
		return Live
	}
	s := Dead
	for _, e := range es {
		switch e.Status {
		case Live:
			return Live
		case Drifted:
			s = Drifted
		}
	}
	return s
}

// Field names a field that more than one writer may set.
type Field string

const (
	NameField           Field = "name"
	PurposeField        Field = "purpose"
	ResponsibilityField Field = "responsibility"
	SignatureField      Field = "signature"
	RelationField       Field = "relation"
	ParentField         Field = "parent"
	// PhraseField is a phrase note on an edge. The script and the group
	// never write one.
	PhraseField Field = "phrase"
)

// Precedence lists, per field, the writers that may claim it, highest first.
// A one-value field is replaced only by its own writer or a higher one. For
// relations and parents, every writer's claim is stored and the first
// eligible one is shown.
var Precedence = map[Field][]Writer{
	NameField:           {Script, Reader, Supplement},
	PurposeField:        {Human, Reader, Supplement},
	ResponsibilityField: {Human, Reader, Supplement},
	SignatureField:      {Script, Reader, Supplement},
	RelationField:       {Script, Human, Reader, Supplement},
	ParentField:         {Human, Group, Script, Reader, Supplement},
	PhraseField:         {Human, Reader, Supplement},
}

func rank(f Field, w Writer) int {
	if i := slices.Index(Precedence[f], w); i >= 0 {
		return i
	}
	return len(Precedence[f])
}

// Outranks reports whether w's value of a field beats held's.
func Outranks(f Field, w, held Writer) bool { return rank(f, w) < rank(f, held) }

// order sorts the claims on one edge or one parent: live claims before
// drifted ones before dead ones, whoever wrote them, and by the table within
// each status. A claim without evidence is live.
func order(f Field, w Writer, s Status) int {
	return rank(f, w) + 2*len(Precedence[f])*map[Status]int{Live: 0, Drifted: 1, Dead: 2}[s]
}

// ParentClaim is one writer's claim on the unit that contains another.
type ParentClaim struct {
	To       ID         `yaml:"to"`
	Writer   Writer     `yaml:"writer"`
	Source   string     `yaml:"source,omitempty"`
	Evidence []Evidence `yaml:"evidence,omitempty"`
	// Over is, on a human's claim, the script's parent when the human stated
	// it, so a later change of the script's parent shows as a conflict.
	Over ID `yaml:"over,omitempty"`
}

func (c ParentClaim) validate(id ID) error {
	if k, _, err := ParseID(string(c.To)); err != nil || !k.Holds() || c.To == id {
		return fmt.Errorf("%s: parent %s is not another module or feature", id, c.To)
	}
	if err := sourced(c.Writer, c.Source); err != nil {
		return fmt.Errorf("%s parent: %w", id, err)
	}
	switch {
	case c.Writer.Origin() == Inferred && len(c.Evidence) == 0:
		return fmt.Errorf("%s: the %s's parent %s needs evidence", id, c.Writer, c.To)
	case c.Writer.Origin() == Extracted && len(c.Evidence) > 0:
		return fmt.Errorf("%s: the %s's parent cites no evidence", id, c.Writer)
	case c.Over != "" && c.Writer != Human:
		return fmt.Errorf("%s: only a human's parent records what it overrode", id)
	}
	if err := validateEvidence(c.Evidence); err != nil {
		return fmt.Errorf("%s parent evidence: %w", id, err)
	}
	return nil
}

// Claim returns the writer's parent claim, if it has one.
func (u Unit) Claim(w Writer) (ParentClaim, bool) {
	i := slices.IndexFunc(u.Parents, func(c ParentClaim) bool { return c.Writer == w })
	if i < 0 {
		return ParentClaim{}, false
	}
	return u.Parents[i], true
}

// PutParent replaces the writer's parent claim with c.
func (u *Unit) PutParent(c ParentClaim) {
	u.DropParents(c.Writer)
	u.Parents = append(u.Parents, c)
	sort.SliceStable(u.Parents, func(i, j int) bool {
		return rank(ParentField, u.Parents[i].Writer) < rank(ParentField, u.Parents[j].Writer)
	})
}

// DropParents removes the parent claims of the writers.
func (u *Unit) DropParents(ws ...Writer) {
	u.Parents = slices.DeleteFunc(slices.Clone(u.Parents), func(c ParentClaim) bool { return slices.Contains(ws, c.Writer) })
	if len(u.Parents) == 0 {
		u.Parents = nil
	}
}

// Mask hides lower writers' claims on one target of a unit, as long as the
// claim's content still matches Digest. A changed claim shows again, except
// that a human's mask on an edge holds whatever the via.
type Mask struct {
	// Target is "unit", "parent", "purpose", "responsibility", or KIND.TO
	// for a relation.
	Target string `yaml:"target"`
	Writer Writer `yaml:"writer"`
	Digest string `yaml:"digest"`
	Reason string `yaml:"reason"`
	Source string `yaml:"source,omitempty"`
}

func (m Mask) validate() error {
	switch {
	case m.Writer != Reader && m.Writer != Human:
		return fmt.Errorf("only a reader or a human retracts, not the %s", m.Writer)
	case strings.TrimSpace(m.Reason) == "":
		return fmt.Errorf("a retraction needs a reason")
	case m.Target == "" || m.Digest == "":
		return fmt.Errorf("a retraction needs a target and a digest")
	}
	return sourced(m.Writer, m.Source)
}

// Hides reports whether a mask by m may hide a claim by w: a reader's hides
// the supplement's, a human's hides the reader's and the supplement's.
func Hides(m, w Writer) bool {
	return w == Supplement && (m == Reader || m == Human) || w == Reader && m == Human
}

// Digest fingerprints the content a mask is bound to.
func Digest(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:8])
}

// RelationTarget names the relation claims on one edge for a mask.
func RelationTarget(kind RelationKind, to ID) string { return string(kind) + "." + string(to) }

// PhraseTarget names the phrase notes on one edge for a mask.
func PhraseTarget(kind RelationKind, to ID) string { return "phrase." + RelationTarget(kind, to) }

// MaskDigest is the content a mask on target binds to for each kind of claim.
func RelationDigest(r Relation) string  { return Digest(string(r.Kind), string(r.To), string(r.Via)) }
func PhraseDigest(n PhraseNote) string  { return Digest(n.Text) }
func ParentDigest(c ParentClaim) string { return Digest(string(c.To)) }
func FactDigest(f Fact) string          { return Digest(f.Value) }

// Masked reports whether a mask hides the writer's claim with this content.
func (u Unit) Masked(target string, w Writer, digest string) bool {
	return slices.ContainsFunc(u.Masks, func(m Mask) bool {
		return m.Target == target && m.Digest == digest && Hides(m.Writer, w)
	})
}

// RelationMasked reports whether a mask hides the claim. A human's mask holds
// the whole edge, whatever the via, so a lower writer cannot bring the edge
// back by naming another via. A reader's mask stays bound to the content.
func (u Unit) RelationMasked(r Relation) bool {
	target := RelationTarget(r.Kind, r.To)
	return u.Masked(target, r.Writer, RelationDigest(r)) || slices.ContainsFunc(u.Masks, func(m Mask) bool {
		return m.Target == target && m.Writer == Human && Hides(Human, r.Writer)
	})
}

// PutMask adds m unless the same mask is already there, and reports whether
// it added it.
func (u *Unit) PutMask(m Mask) bool {
	if slices.ContainsFunc(u.Masks, func(o Mask) bool {
		return o.Target == m.Target && o.Digest == m.Digest && o.Writer == m.Writer
	}) {
		return false
	}
	u.Masks = append(u.Masks, m)
	sort.SliceStable(u.Masks, func(i, j int) bool { return u.Masks[i].Target < u.Masks[j].Target })
	return true
}

// PutRelation replaces the writer's claim on r's edge with r and resolves.
func (u *Unit) PutRelation(r Relation) {
	u.DropRelations(func(o Relation) bool { return o.To == r.To && o.Kind == r.Kind && o.Writer == r.Writer })
	u.Hidden = append(u.Hidden, r)
	u.resolve()
}

// PutNote replaces the writer's phrase note on n's edge with n.
func (u *Unit) PutNote(n PhraseNote) {
	u.DropNotes(func(o PhraseNote) bool { return o.To == n.To && o.Kind == n.Kind && o.Writer == n.Writer })
	u.Phrases = append(u.Phrases, n)
	sort.SliceStable(u.Phrases, func(i, j int) bool {
		a, b := u.Phrases[i], u.Phrases[j]
		return a.To < b.To || a.To == b.To && a.Kind < b.Kind
	})
}

// DropNotes removes every phrase note that drop matches.
func (u *Unit) DropNotes(drop func(PhraseNote) bool) {
	u.Phrases = slices.DeleteFunc(slices.Clone(u.Phrases), drop)
	if len(u.Phrases) == 0 {
		u.Phrases = nil
	}
}

// DropRelations removes every claim, shown or hidden, that drop matches.
func (u *Unit) DropRelations(drop func(Relation) bool) {
	all := slices.DeleteFunc(u.Claims(), drop)
	u.Relations, u.Hidden = nil, all
	u.resolve()
}

// Resolve derives the read model in place: each fact's and claim's origin,
// each edge's shown claim, the masks, and every unit's parent.
func Resolve(units []Unit) {
	for i := range units {
		units[i].resolve()
	}
	resolveParents(units)
}

func (u *Unit) resolve() {
	for _, f := range []*Fact{&u.Name, u.Purpose, u.Responsibility, u.Signature} {
		if f != nil {
			f.Origin = f.Writer.Origin()
		}
	}
	for target, f := range map[string]**Fact{"purpose": &u.Purpose, "responsibility": &u.Responsibility, "signature": &u.Signature} {
		if *f != nil && u.Masked(target, (*f).Writer, FactDigest(**f)) {
			*f = nil
		}
	}
	if !u.Absent && u.Masked("unit", u.Name.Writer, FactDigest(u.Name)) {
		u.SetAbsent(Retracted)
	}
	all := u.Claims()
	for i := range all {
		all[i].Origin = all[i].Writer.Origin()
		all[i].Dead = len(all[i].Evidence) > 0 && all[i].Status() == Dead
	}
	key := func(r Relation) int {
		k := order(RelationField, r.Writer, r.Status())
		if u.RelationMasked(r) {
			k += 1 << 20
		}
		return k
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.To != b.To || a.Kind != b.Kind {
			return a.To < b.To || a.To == b.To && a.Kind < b.Kind
		}
		return key(a) < key(b)
	})
	u.Relations, u.Hidden = nil, nil
	for i, r := range all {
		first := i == 0 || all[i-1].To != r.To || all[i-1].Kind != r.Kind
		if first && !u.RelationMasked(r) {
			u.Relations = append(u.Relations, r)
		} else {
			u.Hidden = append(u.Hidden, r)
		}
	}
}

// tier is the order in which parent claims are accepted when they could
// close a cycle: the script's tree first, then the human's, the reader's and
// the supplement's claims.
func tier(w Writer) int {
	switch w {
	case Script, Group:
		return 0
	case Human:
		return 1
	case Reader:
		return 2
	}
	return 3
}

// eligible lists a unit's parent claims in the order they win, leaving out
// those that are dead, masked, or point at a unit that is not a present
// module or feature.
func eligible(u Unit, holds map[ID]bool) []ParentClaim {
	var out []ParentClaim
	for _, c := range u.Parents {
		s := StatusOf(c.Evidence)
		if s == Dead || u.Masked("parent", c.Writer, ParentDigest(c)) || !holds[c.To] || c.To == u.ID {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return order(ParentField, out[i].Writer, StatusOf(out[i].Evidence)) < order(ParentField, out[j].Writer, StatusOf(out[j].Evidence))
	})
	return out
}

// resolveParents gives each present unit the first eligible parent claim
// that closes no cycle. Claims are accepted tier by tier, each tier in unit
// ID order. A unit whose best claim would close a cycle falls to its next
// claim, and acceptance starts again from the first tier, so that claim is
// weighed before any lower tier. A skipped claim stays stored.
func resolveParents(units []Unit) {
	holds := map[ID]bool{}
	for _, u := range units {
		if !u.Absent && u.Kind.Holds() {
			holds[u.ID] = true
		}
	}
	ids := make([]ID, 0, len(units))
	cands := map[ID][]ParentClaim{}
	for _, u := range units {
		if !u.Absent {
			ids = append(ids, u.ID)
			cands[u.ID] = eligible(u, holds)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	chosen := map[ID]ParentClaim{}
	decided := map[ID]bool{}
	closes := func(id, to ID) bool {
		for k := to; k != ""; k = chosen[k].To {
			if k == id {
				return true
			}
		}
		return false
	}
	for changed := true; changed; {
		changed = false
	tiers:
		for t := 0; t < 4; t++ {
			for _, id := range ids {
				if decided[id] {
					continue
				}
				if len(cands[id]) == 0 {
					decided[id], changed = true, true
					continue
				}
				c := cands[id][0]
				if tier(c.Writer) != t {
					continue
				}
				if closes(id, c.To) {
					cands[id], changed = cands[id][1:], true
					break tiers
				}
				chosen[id], decided[id], changed = c, true, true
			}
		}
	}
	for i := range units {
		u := &units[i]
		c, ok := chosen[u.ID]
		if u.Absent && len(u.Parents) > 0 {
			c, ok = u.Parents[0], true
		}
		u.Parent, u.ParentOrigin, u.ParentWriter = "", "", ""
		if ok {
			u.Parent, u.ParentOrigin, u.ParentWriter = c.To, c.Writer.Origin(), c.Writer
		}
	}
}

// ParentConflicts lists the units where a human's parent overrides the
// script's and the script's parent changed since the human stated it to one
// the human did not choose.
func ParentConflicts(units []Unit) []string {
	var out []string
	for _, u := range Present(units) {
		h, ok := u.Claim(Human)
		if !ok {
			continue
		}
		s, _ := u.Claim(Script)
		if s.To != h.Over && s.To != h.To {
			was := place(h.Over)
			out = append(out, fmt.Sprintf("%s: a human placed it in %s (%s) over the script's %s, and the script now places it in %s", u.ID, h.To, h.Source, was, place(s.To)))
		}
	}
	return out
}

func place(id ID) string {
	if id == "" {
		return "top level"
	}
	return string(id)
}
