// Package screen renders the blueprint a human reads: the overview of the
// top-level units, one inside per module, and the unit catalogue. It reads the
// store and never writes the topology.
package screen

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/owners"
	"github.com/whyjp/abbd/internal/store"
)

// Input is everything the screen reads.
type Input struct {
	Units []model.Unit
	// Owners names who owns each unit's code, by the repository's
	// CODEOWNERS; a unit no rule covers is left out.
	Owners  map[model.ID][]string
	Project store.Project
	// Synced is the watermark of the last sync; Tree is the working tree now.
	// Inputs is what the blueprint must be rendered from now, and Rendered
	// the hash of its files now, "" when there are none.
	Synced   store.State
	Tree     string
	Inputs   string
	Rendered string
	// Understandings are readers' texts by module; Digests are each
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
	// Overview is the overview's own record a reader chose: its zones and
	// its lines. The primary flow is the stack marked primary, in Figures.
	Overview store.Overview
	// Primary are the stored relations, by relation key (model.Edge's
	// String), the overview must draw whatever else it counts instead
	// (primary.go) when Project.Cut() is on. Nil leaves the choice to
	// the stored flows and stacks.
	Primary map[string]bool
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
	// A name taken from the checkout's folder would differ per clone with
	// no input changing, so with none written the blueprint is ABBD's.
	if in.Project.Name == "" {
		in.Project.Name = "ABBD"
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
	in.Owners = OwnersOf(owners.Read(st.ProjectDir()), in.Units)
	if in.Figures, err = st.LoadFigures(); err != nil {
		return in, err
	}
	if in.Overview, err = st.LoadOverview(); err != nil {
		return in, err
	}
	in.Digests = map[model.ID]string{}
	for id := range in.Understandings {
		in.Digests[id] = st.ModuleDigest(in.Units, id)
	}
	// A stamped sentence reads STALE against its unit's code as it is now.
	for _, u := range model.Present(in.Units) {
		if _, ok := in.Digests[u.ID]; !ok && (stamped(u.Purpose) || stamped(u.Responsibility)) {
			in.Digests[u.ID] = st.ModuleDigest(in.Units, u.ID)
		}
	}
	in.Rendered, err = st.BlueprintHash()
	return in, err
}

type Freshness string

const (
	Fresh   Freshness = "CURRENT"
	Stale   Freshness = "STALE"
	Unknown Freshness = "UNKNOWN"
	// Undrawn says no pages are drawn in this checkout while the state they
	// are drawn from is current, as in a checkout that lacks them.
	Undrawn Freshness = "NOT DRAWN"
)

// BlueprintFreshness compares the inputs the state says the blueprint was
// rendered from with the inputs now, and the files that render wrote with
// the files there now.
func (in Input) BlueprintFreshness() Freshness {
	switch {
	case in.Synced.Blueprint == "" || in.Synced.Rendered == "":
		return Unknown
	case in.Synced.Merged || in.Synced.Blueprint != in.Inputs:
		return Stale
	case in.Rendered == "":
		return Undrawn
	case in.Synced.Rendered != in.Rendered:
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
	// Stale says the unit's code changed since the sentence was stamped.
	Stale bool `json:",omitempty"`
}

type UnitRow struct {
	ID, Kind, Name, Signature string
	Purpose, Responsibility   *Fact
	Anchors                   []string
	// Parent is the module that contains the unit.
	Parent string `json:",omitempty"`
	// Artifact is the set of build artifacts a module is, comma-joined.
	Artifact string `json:",omitempty"`
	// Zone is the name of the overview's zone that holds the unit, itself
	// or through a unit that holds it, when a zone does.
	Zone string `json:",omitempty"`
	// Evidence is the code a unit only a reader found rests on.
	Evidence []string `json:",omitempty"`
	// Notes are the unit's slots and Entries its table when no inside shows
	// them.
	Notes   []Note   `json:",omitempty"`
	Entries *Entries `json:",omitempty"`
	// Rank is where the unit's box stands on the overview, for a unit the
	// overview draws.
	Rank *Rank `json:",omitempty"`
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
	// Flows are the stored flows, each a question answered in steps,
	// Stacks the stored stacks, each a question answered in layers, and
	// Tables the stored tables, each a question answered in a grid.
	Flows  []FlowView
	Stacks []StackView
	Tables []TableView
	// Zones and Lines are the overview's zones and the lines a reader chose
	// for it, with their words.
	Zones []ZoneView
	Lines []ChosenLine
	// Name and Purpose are the project's, and Parts its top-level modules
	// and every feature with the one line each says of itself, which the
	// overview opens with.
	Name, Purpose string `json:"-"`
	Parts         []Part `json:"-"`
	// Decisions are every decision note, the unit it sits on first, for the
	// decisions index.
	Decisions []Decision `json:"-"`
	// Primary is the repository's primary flow, which orders the views it
	// crosses, nil when no stack is marked.
	Primary *PrimaryFlow `json:",omitempty"`
	// Edges are every live relation between present units, for graph.json.
	Edges   []Line
	Extract []string
}

// Build derives the screen from the store. It is deterministic: the same
// input gives the same model, so the rendered files only change with the data.
func Build(in Input) Model {
	m := Model{Extract: in.Project.Extract}
	ov := readOverview(in)
	m.Zones, m.Lines = ov.zones, ov.lines
	for _, u := range model.Present(in.Units) {
		row := UnitRow{
			ID: string(u.ID), Kind: string(u.Kind), Name: u.Name.Value, Signature: value(u.Signature),
			Purpose: sentenceOf(u.Purpose, in.Digests[u.ID]), Responsibility: sentenceOf(u.Responsibility, in.Digests[u.ID]), Anchors: anchors(u), Artifact: model.ArtifactSet(u.Built()),
			Parent: string(u.Parent), Zone: ov.zone[u.ID],
		}
		for _, e := range u.Evidence {
			row.Evidence = append(row.Evidence, cite(e))
		}
		m.Units = append(m.Units, row)
	}
	m.Edges = lines(in.Units)
	m.Decisions = decisions(in)
	m.Name, m.Purpose = in.Project.Name, in.Project.Purpose
	m.Parts = parts(in.Units)
	// The overview is drawn in the plain order of its rows only. Every
	// module's page holds a focus view seated by it, so an overview that took
	// another order after a small change would redraw every page. It may
	// still take another ranking, the way a person lays out a cycle, where
	// that fits the screen or has fewer hard breaches and moves no box off
	// what it joins (choose). The screen it fits is what the page leaves
	// under the list of questions drawn above it.
	ls, fs := layerings(in), walks(in)
	o := planOverview(in.Units, "", nil, tactic{}, ls, fs, nil)
	var c *cut
	if in.Project.Cut() {
		c = o.primary(in.Primary, ls, fs)
	}
	flows, stacks, tables := o.flows(in), o.stacks(in), o.tables(in)
	room := overviewRoom(len(flows), len(stacks), len(tables))
	href := func(id model.ID) string { return viewOf(o.tree, id) }
	draw := func(t tactic, read *cost) (Drawing, bool) {
		p := planOverview(in.Units, "", nil, t, ls, fs, c)
		if p.layout.idle {
			return Drawing{}, false
		}
		d := p.draw(href)
		*read = p.read
		return d, true
	}
	judge := func(d Drawing, read *cost) {
		read.judge(d)
		read.over = pastScreen(d, room)
	}
	m.Overview = choose(overviewTactics, draw, judge)
	// An overview that lays the primary flow out and still breaks a hard
	// rule is drawn again with its chains free to come back among their
	// ends as well (tactic.reach), and keeps whichever reads best.
	if _, ok := in.Figures.Primary(); ok {
		var read cost
		read.judge(m.Overview)
		if read.hard > 0 {
			ts := slices.Clone(overviewTactics)
			for _, t := range overviewTactics {
				t.reach = true
				ts = append(ts, t)
			}
			m.Overview = choose(ts, draw, judge)
		}
	}
	m.Focus = o.overviewFocus(in.Units, m.Overview, c.hides())
	for _, c := range o.contracts {
		for _, r := range model.SortRelations(o.units[c].Relations) {
			if _, ok := o.tree.units[r.To]; ok && !r.Dead {
				l := Coupling{From: o.name(c), To: o.name(r.To), Kind: string(r.Kind), Via: string(r.Via)}
				for _, e := range r.Evidence {
					l.Quotes = append(l.Quotes, cite(e))
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
			bs := blocks(u.Text)
			lead, rest := split(bs)
			return &Understanding{Reader: u.Reader, Stale: u.Digest != in.Digests[id], Blocks: bs, Lead: lead, Rest: rest}
		}
		return nil
	}
	for _, id := range views {
		ins := choose(tactics, func(t tactic, read *cost) (Inside, bool) {
			return o.inside(id, in.Units, "", nil, t, read)
		}, func(ins Inside, read *cost) { read.judge(ins.Drawing) })
		ins.Focus = o.insideFocus(id, in.Units, ins)
		ins.Understanding = text(id)
		ins.Standing = o.standing(id, in)
		for i, c := range ins.Inferred {
			ins.Inferred[i].Note = relationNote(in, o.units[model.ID(c.FromID)], model.RelationKind(c.Kind), model.ID(c.ToID))
		}
		ins.Owners = in.Owners[id]
		u := o.units[id]
		ins.Purpose, ins.Responsibility = sentenceOf(u.Purpose, in.Digests[id]), sentenceOf(u.Responsibility, in.Digests[id])
		var tied map[string][]Block
		if ins.Understanding != nil {
			tied = concepts(ins.Understanding.Blocks, ins.Units)
		}
		for i, iu := range ins.Units {
			if viewOf(o.tree, model.ID(iu.ID)) == "" {
				ins.Units[i].Understanding = text(model.ID(iu.ID))
			}
			ins.Units[i].Concepts = tied[iu.ID]
		}
		m.Insides = append(m.Insides, ins)
	}
	hues := colours(m.drawings(), o.tree)
	for _, d := range m.drawings() {
		d.paint(hues)
	}
	leads := append([]*Drawing{&m.Overview}, insides(m.Insides)...)
	for i, d := range leads {
		d.lead(o.tree, fs, i == 0)
	}
	for _, d := range m.drawings() {
		d.dress(o.tree)
	}
	// Numbers and band titles keep off every word, so they are set once
	// dress has given each box its last line; step words keep off both.
	for _, d := range leads {
		d.number()
		d.zone(o.tree, ls)
		d.pin()
		d.name()
		d.card()
	}
	o.placeNotes(in, &m)
	m.Flows, m.Stacks, m.Tables = flows, stacks, tables
	o.linkFigures(&m)
	m.Primary = o.flow.record()
	rank(&m)
	return m
}

// rank gives each unit the overview draws its Rank there, and each unit a
// module's inside draws its Rank on that inside.
func rank(m *Model) {
	on := ranks(m.Overview, m.Primary)
	for i, u := range m.Units {
		if r, ok := on[u.ID]; ok {
			m.Units[i].Rank = &r
		}
	}
	for i := range m.Insides {
		ins := &m.Insides[i]
		on := ranks(ins.Drawing, m.Primary)
		for j, u := range ins.Units {
			if r, ok := on[u.ID]; ok {
				ins.Units[j].Rank = &r
			}
		}
	}
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

func stamped(f *model.Fact) bool { return f != nil && f.Digest != "" }

// sentence is a unit's purpose or responsibility as the page shows it,
// STALE when the code it was stamped against has moved on.
func sentenceOf(f *model.Fact, now string) *Fact {
	v := fact(f)
	if v != nil && stamped(f) && f.Digest != now {
		v.Stale = true
	}
	return v
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

// OwnersOf names the owners of each present unit's anchors, in the order
// CODEOWNERS gives them, each once.
func OwnersOf(o owners.Owners, units []model.Unit) map[model.ID][]string {
	out := map[model.ID][]string{}
	for _, u := range model.Present(units) {
		var names []string
		for _, a := range u.Anchors {
			for _, n := range o.For(a.Path) {
				if !slices.Contains(names, n) {
					names = append(names, n)
				}
			}
		}
		if len(names) > 0 {
			out[u.ID] = names
		}
	}
	return out
}

// relationNote is a reader's note on why one relation of from exists.
func relationNote(in Input, from model.Unit, kind model.RelationKind, to model.ID) *Note {
	key := model.RelationKey(kind, to)
	for _, n := range notes(in, from) {
		if n.slot == "relations" && n.Key == key {
			return &n
		}
	}
	return nil
}

// Decision is one decision note as the index lists it, with its unit.
type Decision struct {
	Unit, Name string
	Note       Note
}

// decisions lists every decision note of a present unit, in unit order.
func decisions(in Input) []Decision {
	var out []Decision
	for _, u := range model.Present(in.Units) {
		for _, n := range notes(in, u) {
			if n.slot == "decisions" {
				out = append(out, Decision{Unit: string(u.ID), Name: u.Name.Value, Note: n})
			}
		}
	}
	return out
}

// Part is a top-level module or a feature as the overview's opening lists
// it: its name and the one line it says of itself.
type Part struct {
	ID, Name, Kind string
	Sentence       string
}

// parts lists the top-level modules, then every feature, each in ID order,
// so a cold reader meets the main parts of the project before the drawing.
func parts(units []model.Unit) []Part {
	var mods, feats []Part
	for _, u := range model.Present(units) {
		p := Part{ID: string(u.ID), Name: u.Name.Value, Kind: string(u.Kind)}
		if f := sentence(u); f != nil {
			p.Sentence = f.Value
		}
		switch {
		case u.Kind == model.Feature:
			feats = append(feats, p)
		case u.Kind == model.Module && u.Parent == "":
			mods = append(mods, p)
		}
	}
	return append(mods, feats...)
}
