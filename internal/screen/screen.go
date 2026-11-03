// Package screen renders the blueprint a human reads: the overview of the
// top-level units, one inside per module, and the unit catalogue. It reads the
// store and never writes the topology.
package screen

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// Input is everything the screen reads.
type Input struct {
	Units   []model.Unit
	Project store.Project
	// Synced is the watermark of the last sync; Tree is the working tree now.
	// Inputs is what the blueprint must be rendered from now, and Rendered
	// the hash of its files now, "" when there are none.
	Synced   store.State
	Tree     string
	Inputs   string
	Rendered string
	// Understandings are pstack how's texts by module; Digests are each
	// module's code as it is now, to tell a text written against older code.
	Understandings map[model.ID]store.Understanding
	Digests        map[model.ID]string
	// Notes are what readers wrote into each unit's slots. Strays are the
	// markdown files beside them that are no notes, from the project root.
	Notes  map[model.ID][]store.Note
	Strays []string
	// Figures are the questions readers and humans answered by kind, such as
	// the ordered walks, with the markdown files beside them that are no
	// figures, and why.
	Figures store.Figures
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
	if in.Figures, err = st.LoadFigures(); err != nil {
		return in, err
	}
	in.Digests = map[model.ID]string{}
	for id := range in.Understandings {
		in.Digests[id] = st.ModuleDigest(in.Units, id)
	}
	in.Rendered, err = st.BlueprintHash()
	return in, err
}

type Freshness string

const (
	Fresh   Freshness = "CURRENT"
	Stale   Freshness = "STALE"
	Unknown Freshness = "UNKNOWN"
)

// BlueprintFreshness compares the inputs the state says the blueprint was
// rendered from with the inputs now, and the files that render wrote with
// the files there now.
func (in Input) BlueprintFreshness() Freshness {
	switch {
	case in.Rendered == "" || in.Synced.Blueprint == "" || in.Synced.Rendered == "":
		return Unknown
	case in.Synced.Merged || in.Synced.Blueprint != in.Inputs || in.Synced.Rendered != in.Rendered:
		return Stale
	}
	return Fresh
}

// ActualFreshness says whether the state was synced from the working tree as it is now.
func (in Input) ActualFreshness() Freshness {
	switch {
	case in.Tree == "" || in.Synced.Fingerprint == "":
		return Unknown
	case in.Synced.Merged || in.Tree != in.Synced.Fingerprint:
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
	// Notes are the unit's slots and Entries its table when no inside shows
	// them.
	Notes   []Note   `json:",omitempty"`
	Entries *Entries `json:",omitempty"`
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
	// Flows are the stored flows, each a question answered in steps, and
	// Stacks the stored stacks, each a question answered in layers.
	Flows  []FlowView
	Stacks []StackView
	// Edges are every live relation between present units, for graph.json.
	Edges   []Line
	Extract []string
}

// Build derives the screen from the store. It is deterministic: the same
// input gives the same model, so the rendered files only change with the data.
func Build(in Input) Model {
	m := Model{Extract: in.Project.Extract}
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
	m.Flows, m.Stacks = o.flows(in), o.stacks(in)
	o.linkFigures(&m)
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
