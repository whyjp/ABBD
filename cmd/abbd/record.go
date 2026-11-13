package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/group"
	"github.com/whyjp/abbd/internal/model"
)

// listFlag collects every use of a repeatable flag, such as --evidence.
type listFlag []string

func (f *listFlag) String() string     { return strings.Join(*f, " ") }
func (f *listFlag) Set(s string) error { *f = append(*f, s); return nil }

// citations parses --evidence and refuses any quote the file it names does not
// hold now, so a finding enters the state only with evidence that is live.
func (e env) citations() ([]model.Evidence, error) {
	var es []model.Evidence
	for _, s := range e.evidence {
		ev, err := model.ParseEvidence(s)
		if err != nil {
			return nil, err
		}
		es = append(es, ev)
	}
	es, err := extract.NormalizeEvidence(es)
	if err != nil {
		return nil, err
	}
	c := evidence.New(e.root)
	for _, ev := range es {
		if !c.Check(ev).Live {
			return nil, fmt.Errorf("the code does not hold %s; quote it verbatim from the working tree", ev)
		}
		if at, off := c.Misplaced(ev); off {
			return nil, fmt.Errorf("%s holds the quote at line %s, more than %d lines from the lines %s you give; cite the lines the quote is on", ev.Path, at, evidence.Slack, ev.Lines)
		}
	}
	return es, nil
}

// source is what a USER_STATED fact rests on: where the human said it.
func (e env) sourceRef() string { return strings.TrimSpace(e.src) }

// writer is who a record speaks for: a human when a source is given, else a
// reader.
func (e env) writer() model.Writer {
	if e.sourceRef() != "" {
		return model.Human
	}
	return model.Reader
}

// save checks the unit's evidence, so its lines and digests are current, and
// writes it.
func (e env) save(u model.Unit) error {
	u = evidence.New(e.root).Apply([]model.Unit{u}, "")[0]
	return e.st.SaveUnits([]model.Unit{u})
}

// recordRelation writes a coupling. A reader's finding is INFERRED and rests
// on live evidence. With --source it is what a human stated. Each writer
// holds its own claim on an edge, and the precedence table picks the shown
// one. With --phrase on an edge that already shows, whoever's claim shows
// there, it stores only the writer's phrase note and leaves every claim's
// evidence, via and origin as they were. On an edge that does not show yet,
// it records the claim and the note. A phrase labels a line and never makes,
// keeps or changes one.
func (e env) recordRelation(pos []string) error {
	edge, err := model.ParseEdge(strings.Join(pos, "."))
	if err != nil {
		return err
	}
	w := e.writer()
	rel := model.Relation{To: edge.To, Kind: edge.Kind, Writer: w, Via: model.Via(e.via)}
	note := model.PhraseNote{To: edge.To, Kind: edge.Kind, Writer: w, Text: e.phrase}
	if w == model.Human {
		rel.Source, note.Source = e.sourceRef(), e.sourceRef()
	}
	switch {
	case edge.From == edge.To:
		return fmt.Errorf("%s couples a unit to itself", edge)
	case w == model.Reader && len(e.evidence) == 0:
		return fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE, or give the --source where a human stated it")
	}
	if err := rel.Via.Validate(); err != nil {
		return err
	}
	if err := model.ValidatePhrase(note.Text, rel.Via); err != nil {
		return err
	}
	if err := edge.Joins(); err != nil {
		return err
	}
	units, err := e.st.Writable()
	if err != nil {
		return err
	}
	u, ok := presentUnit(units, edge.From)
	if _, okTo := presentUnit(units, edge.To); !ok || !okTo {
		return fmt.Errorf("%s names a unit the state does not have", edge)
	}
	if rel.Evidence, err = e.citations(); err != nil {
		return err
	}
	note.Evidence = slices.Clone(rel.Evidence)
	held := ""
	for _, o := range u.Claims() {
		if o.To != edge.To || o.Kind != edge.Kind {
			continue
		}
		if err := model.ValidatePhrase(note.Text, o.Via); err != nil {
			return err
		}
		switch {
		case o.Writer == model.Script:
			held = fmt.Sprintf("%s is already EXTRACTED from the code", edge)
		case o.Writer == model.Human && w == model.Reader:
			held = fmt.Sprintf("%s is USER_STATED (%s); a reader's finding does not replace it", edge, o.Source)
		}
	}
	phrased := note.Text != ""
	labels := phrased && u.Shown(edge.Kind, edge.To)
	phraseKey := model.PhraseTarget(edge.Kind, edge.To)
	switch {
	case phrased && u.Masked(phraseKey, w, model.PhraseDigest(note)):
		return fmt.Errorf("%s: a human retracted the phrase %q (%s)", edge, note.Text, maskReason(u, phraseKey, model.PhraseDigest(note)))
	case labels:
	case held != "" && !phrased:
		return fmt.Errorf("%s; name what crosses it with --phrase", held)
	case held != "":
		return fmt.Errorf("%s", held)
	case w == model.Reader && rel.Via == "":
		return fmt.Errorf("say how the coupling arises with --via")
	case u.RelationMasked(rel):
		return fmt.Errorf("%s: a human retracted it (%s); a reader's finding does not bring it back, whatever its via", edge, maskReason(u, model.RelationTarget(edge.Kind, edge.To), model.RelationDigest(rel)))
	}
	if !labels {
		u.PutRelation(rel)
	}
	if phrased {
		u.PutNote(note)
	}
	if err := e.save(u); err != nil {
		return err
	}
	if !labels {
		fmt.Fprintf(e.stdout, "recorded %s, %s\n", edge, w.Origin())
	}
	if phrased {
		fmt.Fprintf(e.stdout, "recorded %s.phrase, %s: %q names what crosses it while its evidence is live and a claim shows on the edge\n", edge, w.Origin(), note.Text)
		if w.Origin() == model.Inferred {
			e.checkLanguage(edge.String()+".phrase", note.Text)
		}
	}
	if labels {
		fmt.Fprintf(e.stdout, "the edge already shows, so its claims stay as they were\n")
	}
	return nil
}

// maskReason is why the mask over the content was stored, or else why a
// human's mask on the whole target was.
func maskReason(u model.Unit, target, digest string) string {
	for _, m := range u.Masks {
		if m.Target == target && m.Digest == digest {
			return m.Reason
		}
	}
	for _, m := range u.Masks {
		if m.Target == target && m.Writer == model.Human {
			return m.Reason
		}
	}
	return ""
}

// recordUnit writes a unit the extraction script cannot see. Its evidence
// anchors its ID and keeps it present.
func (e env) recordUnit(pos []string) error {
	kind, name := model.Kind(pos[0]), strings.TrimSpace(pos[1])
	if err := kind.Validate(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("name the unit")
	}
	if len(e.evidence) == 0 {
		return fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE")
	}
	units, err := e.st.Writable()
	if err != nil {
		return err
	}
	parent := model.ID(e.parent)
	if p, ok := presentUnit(units, parent); parent != "" && (!ok || !p.Kind.Holds()) {
		return fmt.Errorf("--parent %s is not a module or feature of the state", parent)
	}
	es, err := e.citations()
	if err != nil {
		return err
	}
	anchors := evidenceAnchors(nil, es)
	for _, u := range model.Present(units) {
		// Many units can live in one file, so the same file alone does not
		// make the same unit; the same quoted code does.
		if u.Kind == kind && slices.Equal(u.Anchors, anchors) && slices.EqualFunc(u.Evidence, es, model.Evidence.Same) {
			return fmt.Errorf("%s %s already rests on the same code", u.ID, u.Name.Value)
		}
	}
	id, err := e.st.NextID(kind)
	if err != nil {
		return err
	}
	u := model.Unit{ID: id, Kind: kind, Anchors: anchors, Evidence: es, Name: model.Said(model.Reader, name)}
	if parent != "" {
		u.PutParent(model.ParentClaim{To: parent, Writer: model.Reader, Evidence: es})
	}
	if r := strings.TrimSpace(e.responsibility); r != "" {
		f := model.Said(model.Reader, r)
		u.Responsibility = &f
	}
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s %s, INFERRED; it stays while its evidence is in the code\n", u.ID, name)
	return nil
}

func evidenceAnchors(as []model.Anchor, es []model.Evidence) []model.Anchor {
	as = slices.Clone(as)
	for _, ev := range es {
		as = append(as, ev.Anchor())
	}
	return model.SortAnchors(as)
}

// recordParent places a unit inside a module that holds it: a reader's finding with
// --evidence, or what a human stated with --source. The claim is stored
// beside the other writers'; a human's outranks the script's.
func (e env) recordParent(pos []string) error {
	id, parent := model.ID(pos[0]), model.ID(pos[1])
	units, err := e.st.Writable()
	if err != nil {
		return err
	}
	u, ok := presentUnit(units, id)
	if !ok {
		return fmt.Errorf("%s is not a unit of the state", id)
	}
	p, ok := presentUnit(units, parent)
	if !ok || !p.Kind.Holds() || parent == id {
		return fmt.Errorf("%s is not another module or feature of the state", parent)
	}
	if group.Made(p) {
		return fmt.Errorf("%s is a directory group sync makes to keep a view small and drops when it is not needed; record the module that owns %s", parent, id)
	}
	for k := parent; k != ""; {
		if k == id {
			return fmt.Errorf("placing %s in %s makes %s its own ancestor", id, parent, id)
		}
		q, _ := presentUnit(units, k)
		k = q.Parent
	}
	c := model.ParentClaim{To: parent, Writer: e.writer()}
	switch {
	case c.Writer == model.Human:
		s, _ := u.Claim(model.Script)
		c.Source, c.Over = e.sourceRef(), s.To
	case len(e.evidence) == 0:
		return fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE that shows %s belongs in %s, or give the --source where a human stated it", id, parent)
	default:
		if c.Evidence, err = e.citations(); err != nil {
			return err
		}
	}
	u.PutParent(c)
	if err := e.save(u); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s in %s, %s\n", id, parent, c.Writer.Origin())
	units, err = e.st.LoadUnits()
	if err != nil {
		return err
	}
	if now, _ := presentUnit(units, id); now.Parent != parent {
		fmt.Fprintf(e.stdout, "%s stays in %s for now: a claim above the %s's wins (abbd explain %s)\n", id, place(now.Parent), c.Writer, id)
	}
	return nil
}

func place(id model.ID) string {
	if id == "" {
		return "the top level"
	}
	return string(id)
}

// statedFields are the fields a human states: the why-fields, and how an
// execution context runs. Code facts come from the script.
var statedFields = map[string]func(*model.Unit) **model.Fact{
	"purpose":        func(u *model.Unit) **model.Fact { return &u.Purpose },
	"responsibility": func(u *model.Unit) **model.Fact { return &u.Responsibility },
	"run":            func(u *model.Unit) **model.Fact { return &u.Run },
}

// recordFact writes what a human stated about a unit, with where they said it.
func (e env) recordFact(key string) error {
	id, field, _ := strings.Cut(key, ".")
	at, ok := statedFields[field]
	slot, _, _ := strings.Cut(field, ".")
	_, isSlot := model.SlotNamed(slot)
	switch {
	case !ok && !isSlot:
		return fmt.Errorf("%q is not ID.purpose, ID.responsibility, EXEC-ID.run or ID.decisions.KEY", key)
	case strings.TrimSpace(e.value) == "":
		return fmt.Errorf("give the stated text with --value")
	case e.sourceRef() == "":
		return fmt.Errorf("give where the human stated it with --source, such as a PR URL or \"conversation 2026-11-01\"")
	case !ok:
		return e.recordWhy(key)
	}
	units, err := e.st.Writable()
	if err != nil {
		return err
	}
	u, ok := presentUnit(units, model.ID(id))
	if !ok {
		return fmt.Errorf("%s is not a unit of the state", id)
	}
	value := strings.TrimSpace(e.value)
	if field == "run" {
		r, err := model.ParseRun(value)
		if err != nil {
			return err
		}
		value = r.String()
	}
	f := model.Stated(value, e.sourceRef())
	*at(&u) = &f
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s, USER_STATED from %s\n", key, e.sourceRef())
	return nil
}

// recordWhy stores a human's reason on a decision a reader wrote. It
// leaves the note's stamp alone: the reason is what the human said, and
// whether the code still shows the decision is the note's to say.
func (e env) recordWhy(key string) error {
	r, err := model.ParseNoteRef(key)
	if err != nil {
		return err
	}
	if r.Slot.Name != "decisions" {
		return fmt.Errorf("only a decision takes a human's why, as ID.decisions.KEY; %s is a %s note", key, r.Slot.Name)
	}
	n, ok, err := e.st.LoadNote(r)
	switch {
	case err != nil:
		return err
	case !ok:
		return fmt.Errorf("%s holds no decision; store it first with abbd understand %s FILE --evidence PATH:LINES:QUOTE", r, r)
	}
	f := model.Stated(strings.TrimSpace(e.value), e.sourceRef())
	n.Why = &f
	if err := e.st.SaveNote(n); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s's why, USER_STATED from %s\n", key, e.sourceRef())
	return nil
}

// target is what refresh and retract act on: a unit, one of its fields, or
// an edge from it.
type target struct {
	id    model.ID
	field string // "", "parent", "purpose" or "responsibility"
	edge  *model.Edge
	// phrase says the target is the phrase note on edge, not its claims.
	phrase bool
	// when says the target is the claims on edge that give it a condition.
	when bool
}

func parseTarget(s string) (target, error) {
	if e, ok := strings.CutSuffix(s, ".phrase"); ok && strings.Count(e, ".") == 2 {
		edge, err := model.ParseEdge(e)
		return target{id: edge.From, edge: &edge, phrase: true}, err
	}
	if e, ok := strings.CutSuffix(s, ".when"); ok && strings.Count(e, ".") == 2 {
		edge, err := model.ParseEdge(e)
		return target{id: edge.From, edge: &edge, when: true}, err
	}
	if strings.Count(s, ".") == 2 {
		edge, err := model.ParseEdge(s)
		return target{id: edge.From, edge: &edge}, err
	}
	id, field, _ := strings.Cut(s, ".")
	if _, _, err := model.ParseID(id); err != nil {
		return target{}, err
	}
	switch field {
	case "", "parent", "purpose", "responsibility":
	default:
		return target{}, fmt.Errorf("%q is not ID, ID.parent, ID.purpose, ID.responsibility, an edge such as MOD-0001.depends_on.MOD-0002, its phrase, EDGE.phrase, or its condition, EDGE.when", s)
	}
	return target{id: model.ID(id), field: field}, nil
}

func unitByID(units []model.Unit, id model.ID) (model.Unit, bool) {
	i := slices.IndexFunc(units, func(u model.Unit) bool { return u.ID == id })
	if i < 0 {
		return model.Unit{}, false
	}
	return units[i], true
}

// refresh replaces the evidence of a reader's claim. When the shown INFERRED
// claim is the supplement's, the reader's claim with the new evidence goes
// above it, and a supplement unit becomes the reader's.
func (e env) refresh(s string) error {
	t, err := parseTarget(s)
	if err != nil {
		return err
	}
	if t.field == "purpose" || t.field == "responsibility" {
		return fmt.Errorf("%s rests on no evidence; a human states it with abbd record fact", s)
	}
	if len(e.evidence) == 0 {
		return fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE")
	}
	units, err := e.st.Writable()
	if err != nil {
		return err
	}
	u, ok := unitByID(units, t.id)
	if !ok {
		return fmt.Errorf("%s is not a unit of the state", t.id)
	}
	es, err := e.citations()
	if err != nil {
		return err
	}
	switch {
	case t.phrase:
		var base *model.PhraseNote
		for _, n := range u.Phrases {
			if n.To == t.edge.To && n.Kind == t.edge.Kind && (n.Writer == model.Reader || n.Writer == model.Supplement && base == nil) {
				base = &n
			}
		}
		if base == nil {
			return fmt.Errorf("%s has no INFERRED phrase to refresh", s)
		}
		n := *base
		n.Writer, n.Evidence = model.Reader, es
		u.PutNote(n)
	case t.edge != nil:
		var base *model.Relation
		for _, r := range u.Claims() {
			if r.To == t.edge.To && r.Kind == t.edge.Kind && (!t.when || r.When != "") && (r.Writer == model.Reader || r.Writer == model.Supplement && base == nil) {
				base = &r
			}
		}
		if base == nil {
			return fmt.Errorf("%s has no INFERRED claim to refresh", s)
		}
		r := *base
		r.Writer, r.Evidence, r.Digest = model.Reader, es, ""
		u.PutRelation(r)
	case t.field == "parent":
		c, ok := u.Claim(model.Reader)
		if !ok {
			if c, ok = u.Claim(model.Supplement); !ok {
				return fmt.Errorf("%s has no INFERRED parent claim to refresh", t.id)
			}
		}
		c.Writer, c.Evidence = model.Reader, es
		u.PutParent(c)
	default:
		if !u.Semantic() {
			return fmt.Errorf("%s is the %s's unit; only a unit a reader or the supplement found rests on evidence", t.id, u.Name.Writer)
		}
		u.Name.Writer, u.Evidence = model.Reader, es
		u.Anchors = evidenceAnchors(u.Anchors, es)
		if u.Absent && u.AbsentReason != model.Retracted {
			u.SetAbsent("")
		}
	}
	if err := e.save(u); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "refreshed %s, INFERRED by a reader\n", s)
	return nil
}

// partial is a refusal after the command saved part of what it was asked,
// so the blueprint is still rendered for what it saved.
type partial struct{ error }

// retract removes the writer's own claim on the target and masks each lower
// INFERRED claim on it, bound to that claim's content. Run again, it finds
// nothing left to do.
func (e env) retract(s string) error {
	t, err := parseTarget(s)
	if err != nil {
		return err
	}
	reason := strings.TrimSpace(e.reason)
	if reason == "" {
		return fmt.Errorf("say why with --reason")
	}
	units, err := e.st.Writable()
	if err != nil {
		return err
	}
	u, ok := unitByID(units, t.id)
	if !ok {
		return fmt.Errorf("%s is not a unit of the state", t.id)
	}
	w := e.writer()
	mask := func(target, digest string) bool {
		return u.PutMask(model.Mask{Target: target, Writer: w, Digest: digest, Reason: reason, Source: e.sourceRef()})
	}
	changed, held := false, ""
	// what is what an edge retraction took, said when the edge still shows.
	what := ""
	switch {
	case t.edge != nil:
		// Retracting a line retracts the words on it too.
		key := model.PhraseTarget(t.edge.Kind, t.edge.To)
		for _, n := range slices.Clone(u.Phrases) {
			switch {
			case t.when || n.To != t.edge.To || n.Kind != t.edge.Kind:
			case n.Writer == w:
				u.DropNotes(func(o model.PhraseNote) bool { return o.To == n.To && o.Kind == n.Kind && o.Writer == w })
				changed = true
			case model.Hides(w, n.Writer):
				changed = mask(key, model.PhraseDigest(n)) || changed
			case t.phrase && !u.Masked(key, n.Writer, model.PhraseDigest(n)):
				held = string(n.Writer)
			}
		}
		if t.phrase {
			break
		}
		if changed {
			what = "the phrase"
		}
		key = model.RelationTarget(t.edge.Kind, t.edge.To)
		claims := slices.DeleteFunc(u.Claims(), func(r model.Relation) bool {
			return r.To != t.edge.To || r.Kind != t.edge.Kind || t.when && r.When == ""
		})
		for _, r := range claims {
			switch {
			case r.Writer == w:
				u.DropRelations(func(o model.Relation) bool { return o.To == r.To && o.Kind == r.Kind && o.Writer == w })
				changed, what = true, fmt.Sprintf("the %s's claim", w)
			case model.Hides(w, r.Writer):
				if mask(key, model.RelationDigest(r)) {
					changed, what = true, "the lower claims"
				}
			case !u.RelationMasked(r):
				held = string(r.Writer)
			}
		}
	case t.field == "parent":
		for _, c := range slices.Clone(u.Parents) {
			switch {
			case c.Writer == w:
				u.DropParents(w)
				changed = true
			case model.Hides(w, c.Writer):
				changed = mask("parent", model.ParentDigest(c)) || changed
			case !u.Masked("parent", c.Writer, model.ParentDigest(c)):
				held = string(c.Writer)
			}
		}
	case t.field != "":
		f := *statedFields[t.field](&u)
		switch {
		case f == nil:
		case f.Writer == w:
			*statedFields[t.field](&u), changed = nil, true
		case model.Hides(w, f.Writer):
			mask(t.field, model.FactDigest(*f))
			*statedFields[t.field](&u), changed = nil, true
		default:
			held = string(f.Writer)
		}
	default:
		switch {
		case u.Name.Writer == w:
			if u.AbsentReason != model.Retracted {
				u.SetAbsent(model.Retracted)
				changed = true
			}
		case model.Hides(w, u.Name.Writer):
			changed = mask("unit", model.FactDigest(u.Name))
		default:
			held = string(u.Name.Writer)
		}
	}
	fixed := map[bool]string{true: ", and a wrong code fact is fixed in the extractor"}[held == string(model.Script) || held == string(model.Group)]
	if held != "" && !changed {
		return fmt.Errorf("%s is the %s's; a %s does not retract it%s", s, held, w, fixed)
	}
	if held != "" && what != "" {
		// The edge still shows, so the command did not do what it was asked
		// and says what it did instead.
		if err := e.save(u); err != nil {
			return err
		}
		return partial{fmt.Errorf("retracted %s on %s. The edge is the %s's and stays%s", what, s, held, fixed)}
	}
	if !changed {
		fmt.Fprintf(e.stdout, "%s is already retracted\n", s)
		return nil
	}
	if err := e.save(u); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "retracted %s: %s\n", s, reason)
	return nil
}
