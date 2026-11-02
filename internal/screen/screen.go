// Package screen renders the blueprint a human reads: the overview of the
// top-level units, one inside per module, and the unit catalogue. It reads the
// store and never writes the topology.
package screen

import (
	"encoding/json"
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// Input is everything the screen reads.
type Input struct {
	Units   []model.Unit
	Project store.Project
	// Synced is the watermark of the last sync; Tree is the working tree now.
	Synced store.State
	Tree   string
	Inputs string
	// Rendered is the watermark the blueprint carries, nil when missing.
	Rendered *Watermark
	// Understandings are pstack how's texts by module; Digests are each
	// module's code as it is now, to tell a text written against older code.
	Understandings map[model.ID]store.Understanding
	Digests        map[model.ID]string
	// Notes are what readers wrote into each unit's slots. Strays are the
	// markdown files beside them that are no notes, from the project root.
	Notes  map[model.ID][]store.Note
	Strays []string
	// Flows are the ordered walks readers and humans stored, each a
	// question; FlowStrays the markdown files beside them that are no flows, and why.
	Flows      []store.Flow
	FlowStrays []store.Stray
}

// Watermark ties a rendered file to the .abbd inputs it came from.
type Watermark struct {
	Inputs string `json:"inputs_hash"`
}

// Load reads the store. It leaves Tree empty: snapshotting the working tree
// costs a git call, and the caller decides when to pay it.
func Load(st store.Store) (Input, error) {
	var in Input
	var err error
	if in.Units, err = st.LoadUnits(); err != nil {
		return in, err
	}
	if in.Project, err = st.LoadProject(); err != nil {
		return in, err
	}
	if in.Synced, err = st.LoadState(); err != nil {
		return in, err
	}
	if in.Inputs, err = st.InputsHash(); err != nil {
		return in, err
	}
	if in.Understandings, err = st.LoadUnderstandings(); err != nil {
		return in, err
	}
	if in.Notes, in.Strays, err = st.LoadNotes(); err != nil {
		return in, err
	}
	if in.Flows, in.FlowStrays, err = st.LoadFlows(); err != nil {
		return in, err
	}
	in.Digests = map[model.ID]string{}
	for id := range in.Understandings {
		in.Digests[id] = st.ModuleDigest(in.Units, id)
	}
	b, err := st.ReadBlueprint(GraphFile)
	if err != nil {
		return in, err
	}
	if b != nil {
		var g struct{ Watermark Watermark }
		if json.Unmarshal(b, &g) == nil {
			in.Rendered = &g.Watermark
		}
	}
	return in, nil
}

// Current is the watermark the rendered files must carry right now.
func (in Input) Current() Watermark { return Watermark{Inputs: in.Inputs} }

type Freshness string

const (
	Fresh   Freshness = "CURRENT"
	Stale   Freshness = "STALE"
	Unknown Freshness = "UNKNOWN"
)

// BlueprintFreshness compares the blueprint with the state it renders.
func (in Input) BlueprintFreshness() Freshness {
	switch {
	case in.Rendered == nil:
		return Unknown
	case *in.Rendered != in.Current():
		return Stale
	}
	return Fresh
}

// ActualFreshness says whether the state was synced from the working tree as it is now.
func (in Input) ActualFreshness() Freshness {
	switch {
	case in.Tree == "" || in.Synced.Fingerprint == "":
		return Unknown
	case in.Tree != in.Synced.Fingerprint:
		return Stale
	}
	return Fresh
}

type Fact struct {
	Value, Badge string
}

type UnitRow struct {
	ID, Kind, Name, Signature string
	Purpose, Responsibility   *Fact
	Anchors                   []string
	// Parent is the module that contains the unit.
	Parent string `json:",omitempty"`
	// Evidence is the code a unit only a reader found rests on.
	Evidence []string `json:",omitempty"`
	// Notes are the unit's slots when no inside shows them.
	Notes []Note `json:",omitempty"`
}

type Model struct {
	Units []UnitRow
	// Overview is the first level: modules, their contracts as ports, and
	// the lines between them. Insides are the second, one per module.
	Overview Drawing
	// Links are relations between two contracts, which the overview draws
	// as chips and so lists here instead.
	Links []Coupling
	// Focus are the views derived from the overview when it is dense.
	Focus   []Focus
	Insides []Inside
	// Flows are the stored flows, each a question answered in steps.
	Flows []FlowView
	// Edges are every live relation between present units, for graph.json.
	Edges     []Line
	Actual    Freshness
	Blueprint Freshness
	Extract   []string
	Watermark Watermark
	Synced    store.State
	Tree      string
}

// Build derives the screen from the store. It is deterministic: the same
// input gives the same model, so the rendered files only change with the data.
func Build(in Input) Model {
	m := Model{
		Actual: in.ActualFreshness(), Blueprint: in.BlueprintFreshness(),
		Extract: in.Project.Extract, Watermark: in.Current(), Synced: in.Synced, Tree: in.Tree,
	}
	for _, u := range model.Present(in.Units) {
		row := UnitRow{
			ID: string(u.ID), Kind: string(u.Kind), Name: u.Name.Value, Signature: value(u.Signature),
			Purpose: fact(u.Purpose), Responsibility: fact(u.Responsibility), Anchors: anchors(u),
			Parent: string(u.Parent),
		}
		for _, e := range u.Evidence {
			row.Evidence = append(row.Evidence, e.String())
		}
		m.Units = append(m.Units, row)
	}
	m.Edges = lines(in.Units)
	o := planOverview(in.Units, "", nil)
	m.Overview = o.draw(func(id model.ID) string { return viewOf(o.tree, id) })
	m.Focus = o.overviewFocus(in.Units, m.Overview)
	for _, c := range o.contracts {
		for _, r := range model.SortRelations(o.units[c].Relations) {
			if _, ok := o.tree.units[r.To]; ok && !r.Dead {
				l := Coupling{From: o.name(c), To: o.name(r.To), Kind: string(r.Kind), Via: string(r.Via)}
				for _, e := range r.Evidence {
					l.Quotes = append(l.Quotes, e.String())
				}
				m.Links = append(m.Links, l)
			}
		}
	}
	// Every module of the top level, and every unit that holds others, has
	// a view of its own.
	var views []model.ID
	for id := range o.tree.units {
		if viewOf(o.tree, id) != "" {
			views = append(views, id)
		}
	}
	slices.Sort(views)
	text := func(id model.ID) *Understanding {
		if u, ok := in.Understandings[id]; ok {
			return &Understanding{Stale: u.Digest != in.Digests[id], Blocks: blocks(u.Text)}
		}
		return nil
	}
	for _, id := range views {
		ins := o.inside(id, in.Units, "", nil)
		ins.Focus = o.insideFocus(id, in.Units, ins)
		ins.Understanding = text(id)
		for i, iu := range ins.Units {
			if viewOf(o.tree, model.ID(iu.ID)) == "" {
				ins.Units[i].Understanding = text(model.ID(iu.ID))
			}
		}
		m.Insides = append(m.Insides, ins)
	}
	o.placeNotes(in, &m)
	m.Flows = o.flows(in)
	o.linkFlows(&m)
	return m
}

// Badge says where a value came from: read from code, stated by a human, or
// inferred by a reader.
func Badge(o model.Origin) string {
	switch o {
	case model.UserStated:
		return "stated"
	case model.Inferred:
		return "inferred"
	}
	return "extracted"
}

func fact(f *model.Fact) *Fact {
	if f == nil {
		return nil
	}
	return &Fact{Value: f.Value, Badge: Badge(f.Origin)}
}

func value(f *model.Fact) string {
	if f == nil {
		return ""
	}
	return f.Value
}

func anchors(u model.Unit) []string {
	var out []string
	for _, a := range u.Anchors {
		s := a.Path
		if a.Symbol != "" {
			s += " " + a.Symbol
		}
		out = append(out, s)
	}
	return out
}
