package topology

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// FlowSteps is a stored flow as a position reads it: its key and the unit
// of each step, in step order.
type FlowSteps struct {
	Key   string
	Units []model.ID
}

// FlowStep is one step of a flow a unit holds: its number from 1, and the
// unit that runs it, the unit itself or a unit inside it.
type FlowStep struct {
	N    int
	Unit model.ID
}

// FlowPosition is where a unit sits in a primary flow, so a reader takes
// it in the order the flow runs. Flow names a stored flow, whose Steps of
// Of the unit or a unit inside it runs; Upstream and Downstream then run
// the steps just before and just after those, in flow order. An empty Flow
// is a position read from the posts_to relations and the relations via
// data, with no steps: Upstream hands work or data to the unit, Downstream
// takes them from it, in ID order.
type FlowPosition struct {
	Flow                 string
	Steps                []FlowStep
	Of                   int
	Upstream, Downstream []model.ID
}

// FlowPositions are the unit's positions: one per stored flow whose steps
// it or a unit inside it runs, else at most one read from posts_to and data
// relations, else none.
func FlowPositions(units []model.Unit, flows []FlowSteps, id model.ID) []FlowPosition {
	own := owner(units, id)
	var out []FlowPosition
	for _, f := range flows {
		p := FlowPosition{Flow: f.Key, Of: len(f.Units)}
		for i, u := range f.Units {
			if own(u) {
				p.Steps = append(p.Steps, FlowStep{N: i + 1, Unit: u})
			}
		}
		if len(p.Steps) == 0 {
			continue
		}
		for _, s := range p.Steps {
			if s.N > 1 && !own(f.Units[s.N-2]) {
				p.Upstream = appendNew(p.Upstream, f.Units[s.N-2])
			}
			if s.N < p.Of && !own(f.Units[s.N]) {
				p.Downstream = appendNew(p.Downstream, f.Units[s.N])
			}
		}
		out = append(out, p)
	}
	if len(out) > 0 {
		return out
	}
	var p FlowPosition
	for _, l := range Links(units) {
		from, to, ok := carries(l)
		switch {
		case !ok, own(from) == own(to):
		case own(to):
			p.Upstream = appendNew(p.Upstream, from)
		default:
			p.Downstream = appendNew(p.Downstream, to)
		}
	}
	if len(p.Upstream)+len(p.Downstream) == 0 {
		return nil
	}
	slices.Sort(p.Upstream)
	slices.Sort(p.Downstream)
	return []FlowPosition{p}
}

// carries is the way work or data moves along a link: a posts_to hands it
// to the context it names, a provides via data writes the contract, and a
// consumes or depends_on via data reads what it names. A link that moves
// neither is no part of a flow.
func carries(l Link) (from, to model.ID, ok bool) {
	switch {
	case l.Kind == model.PostsTo, l.Via == "data" && l.Kind == model.Provides:
		return l.From, l.To, true
	case l.Via == "data" && l.Kind.Uses():
		return l.To, l.From, true
	}
	return "", "", false
}

// owner reports whether a unit is id or sits inside it.
func owner(units []model.Unit, id model.ID) func(model.ID) bool {
	parent := map[model.ID]model.ID{}
	for _, u := range model.Present(units) {
		parent[u.ID] = u.Parent
	}
	return func(x model.ID) bool {
		for seen := map[model.ID]bool{}; x != "" && !seen[x]; x = parent[x] {
			if x == id {
				return true
			}
			seen[x] = true
		}
		return false
	}
}

func appendNew(ids []model.ID, id model.ID) []model.ID {
	if slices.Contains(ids, id) {
		return ids
	}
	return append(ids, id)
}
