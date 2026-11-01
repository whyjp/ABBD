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

// evidenceFlags collects every --evidence on the command line.
type evidenceFlags []string

func (f *evidenceFlags) String() string     { return strings.Join(*f, " ") }
func (f *evidenceFlags) Set(s string) error { *f = append(*f, s); return nil }

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
		if !c.Check(ev, nil).Live {
			return nil, fmt.Errorf("the code does not hold %s; quote it verbatim from the working tree", ev)
		}
	}
	return es, nil
}

// source is what a USER_STATED fact rests on: where the human said it.
func (e env) sourceRef() string { return strings.TrimSpace(e.src) }

// recordRelation writes a coupling. A reader's finding is INFERRED and rests
// on live evidence; with --source it is what a human stated.
func (e env) recordRelation(pos []string) error {
	edge, err := model.ParseEdge(strings.Join(pos, "."))
	if err != nil {
		return err
	}
	rel := model.Relation{To: edge.To, Kind: edge.Kind, Origin: model.Inferred, Via: model.Via(e.via)}
	if e.sourceRef() != "" {
		rel.Origin, rel.Source = model.UserStated, e.sourceRef()
	}
	switch {
	case edge.From == edge.To:
		return fmt.Errorf("%s couples a unit to itself", edge)
	case rel.Origin == model.Inferred && rel.Via == "":
		return fmt.Errorf("say how the coupling arises with --via")
	case rel.Origin == model.Inferred && len(e.evidence) == 0:
		return fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE, or give the --source where a human stated it")
	}
	if err := rel.Via.Validate(); err != nil {
		return err
	}
	if err := edge.Joins(); err != nil {
		return err
	}
	units, err := e.st.LoadUnits()
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
	rels := slices.Clone(u.Relations)
	switch i := slices.IndexFunc(rels, func(r model.Relation) bool { return r.To == edge.To && r.Kind == edge.Kind }); {
	case i < 0:
		rels = append(rels, rel)
	case rels[i].Origin == model.Extracted:
		return fmt.Errorf("%s is already EXTRACTED from the code", edge)
	case rels[i].Origin == model.UserStated && rel.Origin == model.Inferred:
		return fmt.Errorf("%s is USER_STATED (%s); a reader's finding does not replace it", edge, rels[i].Source)
	default:
		rels[i] = rel
	}
	u.Relations = model.SortRelations(rels)
	u = evidence.New(e.root).Apply([]model.Unit{u})[0]
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s, %s\n", edge, rel.Origin)
	return nil
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
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	parent := model.ID(e.parent)
	if p, ok := presentUnit(units, parent); parent != "" && (!ok || p.Kind != model.Module) {
		return fmt.Errorf("--parent %s is not a module of the state", parent)
	}
	es, err := e.citations()
	if err != nil {
		return err
	}
	var anchors []model.Anchor
	for _, ev := range es {
		anchors = append(anchors, ev.Anchor())
	}
	anchors = model.SortAnchors(anchors)
	for _, u := range model.Present(units) {
		// Many units can live in one file, so the same file alone does not
		// make the same unit; the same quoted code does.
		if u.Kind == kind && slices.Equal(u.Anchors, anchors) && slices.Equal(u.Evidence, es) {
			return fmt.Errorf("%s %s already rests on the same code", u.ID, u.Name.Value)
		}
	}
	id, err := e.st.NextID(kind)
	if err != nil {
		return err
	}
	u := model.Unit{
		ID: id, Kind: kind, Anchors: anchors, Parent: parent, Evidence: es,
		Name: model.Fact{Value: name, Origin: model.Inferred},
	}
	if r := strings.TrimSpace(e.responsibility); r != "" {
		u.Responsibility = &model.Fact{Value: r, Origin: model.Inferred}
	}
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s %s, INFERRED; it stays while its evidence is in the code\n", u.ID, name)
	return nil
}

// recordParent places a unit inside a module: a reader's grouping with
// --evidence, or what a human stated with --source. A sync keeps it unless
// the script reports a parent for the unit.
func (e env) recordParent(pos []string) error {
	id, parent := model.ID(pos[0]), model.ID(pos[1])
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	u, ok := presentUnit(units, id)
	if !ok {
		return fmt.Errorf("%s is not a unit of the state", id)
	}
	p, ok := presentUnit(units, parent)
	if !ok || p.Kind != model.Module || parent == id {
		return fmt.Errorf("%s is not another module of the state", parent)
	}
	if group.Made(p) {
		return fmt.Errorf("%s is a directory group sync makes to keep a view small and drops when it is not needed; record the module that owns %s", parent, id)
	}
	switch {
	case e.sourceRef() != "":
		u.ParentOrigin, u.ParentSource, u.ParentEvidence = model.UserStated, e.sourceRef(), nil
	case len(e.evidence) == 0:
		return fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE that shows %s belongs in %s, or give the --source where a human stated it", id, parent)
	default:
		if u.ParentEvidence, err = e.citations(); err != nil {
			return err
		}
		u.ParentOrigin, u.ParentSource = model.Inferred, ""
	}
	u.Parent = parent
	for i := range units {
		if units[i].ID == id {
			units[i] = u
		}
	}
	if c := model.ParentCycle(units); c != "" {
		return fmt.Errorf("placing %s in %s makes %s its own ancestor", id, parent, c)
	}
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s in %s, %s\n", id, parent, u.ParentOrigin)
	return nil
}

// statedFields are the why-fields a human states; code facts come from the script.
var statedFields = map[string]func(*model.Unit) **model.Fact{
	"purpose":        func(u *model.Unit) **model.Fact { return &u.Purpose },
	"responsibility": func(u *model.Unit) **model.Fact { return &u.Responsibility },
}

// recordFact writes what a human stated about a unit, with where they said it.
func (e env) recordFact(key string) error {
	id, field, _ := strings.Cut(key, ".")
	at, ok := statedFields[field]
	switch {
	case !ok:
		return fmt.Errorf("%q is not ID.purpose or ID.responsibility", key)
	case strings.TrimSpace(e.value) == "":
		return fmt.Errorf("give the stated text with --value")
	case e.sourceRef() == "":
		return fmt.Errorf("give where the human stated it with --source, such as a PR URL or \"conversation 2026-11-01\"")
	}
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	u, ok := presentUnit(units, model.ID(id))
	if !ok {
		return fmt.Errorf("%s is not a unit of the state", id)
	}
	*at(&u) = &model.Fact{Value: strings.TrimSpace(e.value), Origin: model.UserStated, Source: e.sourceRef()}
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "recorded %s, USER_STATED from %s\n", key, e.sourceRef())
	return nil
}
