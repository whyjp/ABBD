// Package screen renders the blueprint a human reads: the overview of the
// top-level units, one inside per module, and the unit catalogue. It reads the
// store and never writes the topology.
package screen

import (
	"encoding/json"

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
	// Generated is the watermark the rendered files carry, nil when missing.
	Generated *Watermark
	// Understandings are pstack how's texts by module; Digests are each
	// module's code as it is now, to tell a text written against older code.
	Understandings map[model.ID]store.Understanding
	Digests        map[model.ID]string
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
	in.Digests = map[model.ID]string{}
	for id := range in.Understandings {
		in.Digests[id] = st.ModuleDigest(in.Units, id)
	}
	b, err := st.ReadGenerated(GraphFile)
	if err != nil {
		return in, err
	}
	if b != nil {
		var g struct{ Watermark Watermark }
		if json.Unmarshal(b, &g) == nil {
			in.Generated = &g.Watermark
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

// GeneratedFreshness compares the rendered files with the inputs they derive from.
func (in Input) GeneratedFreshness() Freshness {
	switch {
	case in.Generated == nil:
		return Unknown
	case *in.Generated != in.Current():
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
}

type Model struct {
	Units []UnitRow
	// Overview is the first level: modules, their contracts as ports, and
	// the lines between them. Insides are the second, one per module.
	Overview Drawing
	Insides  []Inside
	// Edges are every live relation between present units, for graph.json.
	Edges     []Line
	Actual    Freshness
	Generated Freshness
	Extract   []string
	Watermark Watermark
	Synced    store.State
	Tree      string
}

// Build derives the screen from the store. It is deterministic: the same
// input gives the same model, so the rendered files only change with the data.
func Build(in Input) Model {
	m := Model{
		Actual: in.ActualFreshness(), Generated: in.GeneratedFreshness(),
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
	o := planOverview(in.Units)
	m.Overview = o.draw(func(id model.ID) string { return "#m-" + string(id) })
	for _, id := range o.modules {
		ins := o.inside(id, in.Units)
		if u, ok := in.Understandings[id]; ok {
			ins.Understanding = &Understanding{Stale: u.Digest != in.Digests[id], Blocks: blocks(u.Text)}
		}
		m.Insides = append(m.Insides, ins)
	}
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
