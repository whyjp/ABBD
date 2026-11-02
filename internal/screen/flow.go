package screen

import (
	"fmt"
	"slices"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A flow answers one question with the order things happen in: numbered
// steps, each on the unit whose code runs it, with what it does, a note, how
// it fails and the units it calls. The page draws it as a numbered list in
// HTML rather than a layered SVG: the browser wraps every line, so no label
// is cut, the steps read top to bottom in their order, and the only lines are
// the ones from each step to the next. A flow stores no unit and no
// relation. Each step names stored units, each call rides a stored relation,
// and a click on a step's unit follows the module inside that shows it.

// FlowView is one flow as the page draws it. Badge is the weakest origin of
// its steps and Source the one every step shares, if any. Stale counts its
// STALE steps.
type FlowView struct {
	ID, Key, Question, Badge, Source string
	Caption                          []Block `json:",omitempty"`
	Steps, Stale                     int
	Lanes                            []Lane
}

// Lane is a run of consecutive steps on one unit, drawn as one cell beside
// them. It groups by the stored unit only, so it adds no grouping.
type Lane struct {
	Unit, Name, Kind, Href string
	Steps                  []StepView
}

// StepView is one step. N is its place from 1 and ID its anchor. Badge and
// Source are set only where they differ from the flow's, so a mixed flow
// marks the steps a human stated. Reasons say why it is STALE: the cited
// code changed, or the state lost a unit or a relation the step names.
type StepView struct {
	N                                    int
	ID, Does, Note, Fails, Badge, Source string
	Calls                 []Call   `json:",omitempty"`
	Quotes                []string `json:",omitempty"`
	Reasons               []string `json:",omitempty"`
	Stale                 bool     `json:",omitempty"`
}

// Call is a unit a step reaches, named by the stored relation it rides.
type Call struct{ ID, Name, Verb, Href string }

var verbs = map[model.RelationKind]string{model.DependsOn: "호출", model.Consumes: "사용", model.Provides: "제공"}

// FlowRef is a flow as a module's inside lists it: the steps on units the
// module holds.
type FlowRef struct {
	ID, Question string
	Steps        []int
}

func flowID(k model.FlowKey) string { return "flow-" + string(k) }

// stepID is a step's anchor. A key holds no dot, so flow a's step 1 and flow
// a-1 never share one.
func stepID(k model.FlowKey, n int) string { return fmt.Sprintf("%s.%d", flowID(k), n) }

// flows derives each stored flow's view, in key order.
func (o *overview) flows(in Input) []FlowView {
	var out []FlowView
	for _, f := range in.Flows {
		if !f.Known {
			continue
		}
		v := FlowView{ID: flowID(f.Key), Key: string(f.Key), Question: f.Question, Badge: Badge(f.Origin()), Source: sharedSource(f), Caption: blocks(f.Caption), Steps: len(f.Steps)}
		for i, s := range f.Steps {
			reach := s.Against(in.Units)
			sv := StepView{N: i + 1, ID: stepID(f.Key, i+1), Does: s.Does, Note: s.Note, Fails: s.Fails}
			if b := Badge(s.Writer.Origin()); b != v.Badge {
				sv.Badge = b
			}
			if s.Source != v.Source {
				sv.Source = s.Source
			}
			sv.Reasons = append(slices.Clone(s.Changed), reach.Gaps...)
			sv.Stale = len(sv.Reasons) > 0
			for _, e := range s.Evidence {
				sv.Quotes = append(sv.Quotes, e.String())
			}
			for _, c := range s.Calls {
				if kind, ok := reach.Calls[c]; ok {
					sv.Calls = append(sv.Calls, Call{ID: string(c), Name: o.unitName(c), Verb: verbs[kind], Href: o.href(c)})
				}
			}
			if sv.Stale {
				v.Stale++
			}
			if n := len(v.Lanes); n > 0 && v.Lanes[n-1].Unit == string(s.Unit) {
				v.Lanes[n-1].Steps = append(v.Lanes[n-1].Steps, sv)
				continue
			}
			v.Lanes = append(v.Lanes, Lane{Unit: string(s.Unit), Name: o.unitName(s.Unit), Kind: kindWord(o.tree.units[s.Unit].Kind), Href: o.href(s.Unit), Steps: []StepView{sv}})
		}
		out = append(out, v)
	}
	return out
}

// sharedSource is the source every step of f was stated in, or "".
func sharedSource(f store.Flow) string {
	for _, s := range f.Steps {
		if s.Source == "" || s.Source != f.Steps[0].Source {
			return ""
		}
	}
	if len(f.Steps) == 0 {
		return ""
	}
	return f.Steps[0].Source
}

// unitName is a unit's name, or its ID when the state lost it.
func (o *overview) unitName(id model.ID) string {
	if u, ok := o.tree.units[id]; ok && u.Name.Value != "" {
		return u.Name.Value
	}
	return string(id)
}

// href is where a click on a unit goes: its own inside, else the inside of
// the nearest unit that holds it, else for a contract the inside of the
// module that shows its notes, else the overview.
func (o *overview) href(id model.ID) string {
	u, ok := o.tree.units[id]
	if !ok {
		return "#overview"
	}
	chain := o.tree.chain(id)
	for i := len(chain) - 1; i >= 0; i-- {
		if v := viewOf(o.tree, chain[i]); v != "" {
			return v
		}
	}
	if u.Kind.IsContract() {
		if v := viewOf(o.tree, o.noteHome(u)); v != "" {
			return v
		}
	}
	return "#overview"
}

// linkFlows lists each flow on every inside whose subtree holds the unit of
// one of its steps, with those steps' numbers.
func (o *overview) linkFlows(m *Model) {
	for i := range m.Insides {
		ins := &m.Insides[i]
		held := map[string]bool{}
		for _, id := range o.tree.subtree(model.ID(ins.ID)) {
			held[string(id)] = true
		}
		for _, f := range m.Flows {
			ref := FlowRef{ID: f.ID, Question: f.Question}
			for _, l := range f.Lanes {
				for _, s := range l.Steps {
					if held[l.Unit] {
						ref.Steps = append(ref.Steps, s.N)
					}
				}
			}
			if len(ref.Steps) > 0 {
				ins.Flows = append(ins.Flows, ref)
			}
		}
	}
}
