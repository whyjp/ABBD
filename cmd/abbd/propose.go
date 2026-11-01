package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

// evidenceFlags collects every --evidence on the command line.
type evidenceFlags []string

func (f *evidenceFlags) String() string     { return strings.Join(*f, " ") }
func (f *evidenceFlags) Set(s string) error { *f = append(*f, s); return nil }

// citations parses --evidence and refuses any quote the tree does not hold
// now, so a guess enters the store only with evidence that is live.
func (e env) citations() ([]model.Evidence, error) {
	if len(e.evidence) == 0 {
		return nil, fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE")
	}
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
	}
	return es, nil
}

func presentUnit(v view, id model.ID) (model.Unit, bool) {
	i := slices.IndexFunc(v.units, func(u model.Unit) bool { return u.ID == id && !u.Absent })
	if i < 0 {
		return model.Unit{}, false
	}
	return v.units[i], true
}

// proposeRelation records an agent's guess at a coupling. It stays INFERRED
// and UNCONFIRMED: the impact follows it while its evidence is live, and only
// a human's confirm lets it gate.
func (e env) proposeRelation(pos []string) error {
	edge, err := topology.ParseEdge(strings.Join(pos, "."))
	if err != nil {
		return err
	}
	via := model.Via(e.via)
	switch {
	case edge.From == edge.To:
		return fmt.Errorf("%s couples a unit to itself", edge)
	case via == "":
		return fmt.Errorf("say how the coupling arises with --via")
	}
	if err := via.Validate(); err != nil {
		return err
	}
	if err := edge.Joins(); err != nil {
		return err
	}
	v, err := e.load()
	if err != nil {
		return err
	}
	u, ok := presentUnit(v, edge.From)
	if _, okTo := presentUnit(v, edge.To); !ok || !okTo {
		return fmt.Errorf("%s names a unit the actual topology does not have", edge)
	}
	es, err := e.citations()
	if err != nil {
		return err
	}
	rel := model.Relation{To: edge.To, Kind: edge.Kind, Origin: model.Inferred, Confirmation: model.Unconfirmed, Via: via, Evidence: es}
	rels := slices.Clone(u.Relations)
	switch i := slices.IndexFunc(rels, func(r model.Relation) bool { return r.To == edge.To && r.Kind == edge.Kind }); {
	case i < 0:
		rels = append(rels, rel)
	case rels[i].Counts():
		return fmt.Errorf("%s is already part of the actual topology", edge)
	case rels[i].Confirmation == model.Rejected:
		return fmt.Errorf("a human rejected %s; it stays rejected", edge)
	case rels[i].Confirmation == model.Confirmed:
		return fmt.Errorf("a human confirmed %s; its evidence is gone, so it waits for that human as a removed relation", edge)
	default:
		rels[i] = rel
	}
	u.Relations = model.SortRelations(rels)
	u = evidence.New(e.root).Apply([]model.Unit{u})[0]
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "proposed %s via %s, INFERRED and UNCONFIRMED; a human confirms it with abbd confirm %s\n", edge, via, edge)
	return nil
}

// proposeUnit records an agent's guess at a unit the extraction script cannot
// see. Its evidence anchors its ID and keeps it present.
func (e env) proposeUnit(pos []string) error {
	kind, name := model.Kind(pos[0]), strings.TrimSpace(pos[1])
	if err := kind.Validate(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("name the unit")
	}
	v, err := e.load()
	if err != nil {
		return err
	}
	parent := model.ID(e.parent)
	if p, ok := presentUnit(v, parent); parent != "" && (!ok || p.Kind != model.Module) {
		return fmt.Errorf("--parent %s is not a module of the actual topology", parent)
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
	for _, u := range topology.Present(v.units) {
		if u.Kind == kind && slices.Equal(u.Anchors, anchors) {
			return fmt.Errorf("%s %s already rests on the same code", u.ID, u.Name.Value)
		}
	}
	state, err := e.st.LoadState()
	if err != nil {
		return err
	}
	if state.HighWater == nil {
		state.HighWater = map[model.Kind]int{}
	}
	for _, u := range v.units {
		if k, n, err := model.ParseID(string(u.ID)); err == nil && k == kind && n > state.HighWater[kind] {
			state.HighWater[kind] = n
		}
	}
	state.HighWater[kind]++
	u := model.Unit{
		ID: model.FormatID(kind, state.HighWater[kind]), Kind: kind, Anchors: anchors, Parent: parent, Evidence: es,
		Name: model.Fact{Value: name, Origin: model.Inferred, Confirmation: model.Unconfirmed},
	}
	if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
		return err
	}
	if err := e.st.SaveState(state); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "proposed %s %s, INFERRED; it stays while its evidence is in the code\n", u.ID, name)
	return nil
}

// checkEvidence prints, for every semantic unit and relation, whether each
// quote is in the working tree now and where. It reads and never writes, so
// the same tree prints the same lines.
func (e env) checkEvidence() error {
	v, err := e.load()
	if err != nil {
		return err
	}
	c := evidence.New(e.root)
	live, dead := 0, 0
	line := func(subject string, via model.Via, ev model.Evidence) {
		r := c.Check(ev)
		state := "dead"
		if r.Live {
			state, live = "live", live+1
		} else {
			dead++
		}
		if via != "" {
			subject += " via " + string(via)
		}
		if r.Live {
			fmt.Fprintf(e.stdout, "%s %s: %s found at %s\n", state, subject, ev, r.At)
		} else {
			fmt.Fprintf(e.stdout, "%s %s: %s\n", state, subject, ev)
		}
	}
	units := slices.Clone(v.units)
	slices.SortFunc(units, func(a, b model.Unit) int { return strings.Compare(string(a.ID), string(b.ID)) })
	for _, u := range units {
		if u.Absent && !u.Semantic() {
			continue
		}
		for _, ev := range u.Evidence {
			line(string(u.ID)+" "+u.Name.Value, "", ev)
		}
		for _, r := range u.Relations {
			for _, ev := range r.Evidence {
				line(topology.Edge{From: u.ID, To: r.To, Kind: r.Kind}.String(), r.Via, ev)
			}
		}
	}
	fmt.Fprintf(e.stdout, "%d live, %d dead\n", live, dead)
	return nil
}
