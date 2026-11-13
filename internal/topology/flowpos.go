package topology

import (
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// FlowSteps is a stored flow as a position reads it: its key, the unit of
// each step in step order, and whether it is the lead, the one flow the
// overview draws bold.
type FlowSteps struct {
	Key   string
	Units []model.ID
	Lead  bool
}

// FlowStep is one step of a flow a unit holds: its number from 1, and the
// unit that runs it, the unit itself or a unit inside it.
type FlowStep struct {
	N    int
	Unit model.ID
}

// Role is what the units of one side of a position are to the unit.
type Role int

const (
	// Before and After run the steps just before and just after the unit's
	// steps of a stored flow.
	Before Role = iota
	After
	// WorkFrom posts work to the unit, and WorkTo is posted work by it.
	WorkFrom
	WorkTo
	// Reads is what the unit reads via data, and ReadBy reads the unit.
	Reads
	ReadBy
	// ProvidedBy implements or defines the unit itself, a contract, and not
	// a contract inside it. Providing says nothing of which way data moves,
	// so it orders nothing.
	ProvidedBy
)

// Side is the units that stand in one role to the unit, in flow order on a
// stored flow and in ID order otherwise. A unit inside another the side
// names is left to that one.
type Side struct {
	Role  Role
	Units []model.ID
}

// FlowPosition is where a unit sits in a primary flow, so a reader takes
// it in the order the flow runs. Flow names a stored flow, of Of steps,
// whose Steps the unit or a unit inside it runs, and Lead marks the one the
// overview draws bold. An empty Flow is a position read from the posts_to
// relations and the relations via data, with no steps. Sides are in Role
// order, each with at least one unit.
type FlowPosition struct {
	Flow  string
	Lead  bool
	Steps []FlowStep
	Of    int
	Sides []Side
}

// FlowPositions are the unit's positions: one per stored flow whose steps
// it or a unit inside it runs, the lead first, else at most one read from
// posts_to and data relations that move work or data, else none.
func FlowPositions(units []model.Unit, flows []FlowSteps, id model.ID) []FlowPosition {
	t := newParents(units)
	own := func(x model.ID) bool { return t.within(x, id) }
	var out []FlowPosition
	for _, f := range flows {
		p := FlowPosition{Flow: f.Key, Lead: f.Lead, Of: len(f.Units)}
		by := map[Role][]model.ID{}
		for i, u := range f.Units {
			if !own(u) {
				continue
			}
			p.Steps = append(p.Steps, FlowStep{N: i + 1, Unit: u})
			if i > 0 && !own(f.Units[i-1]) {
				by[Before] = appendNew(by[Before], f.Units[i-1])
			}
			if i+1 < len(f.Units) && !own(f.Units[i+1]) {
				by[After] = appendNew(by[After], f.Units[i+1])
			}
		}
		if len(p.Steps) > 0 {
			p.Sides = t.sides(by)
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		slices.SortStableFunc(out, func(a, b FlowPosition) int {
			if a.Lead == b.Lead {
				return 0
			}
			if a.Lead {
				return -1
			}
			return 1
		})
		return out
	}
	by := map[Role][]model.ID{}
	for _, l := range Links(units) {
		c := carries(l)
		r, ok := carried[c]
		switch {
		case !ok, own(l.From) == own(l.To), c == provides && l.To != id:
		case own(l.From):
			by[r[0]] = appendNew(by[r[0]], l.To)
		default:
			by[r[1]] = appendNew(by[r[1]], l.From)
		}
	}
	if len(by[WorkFrom])+len(by[WorkTo])+len(by[Reads])+len(by[ReadBy]) == 0 {
		return nil
	}
	for _, ids := range by {
		slices.Sort(ids)
	}
	return []FlowPosition{{Sides: t.sides(by)}}
}

// carry is what a link says of the way work or data moves.
type carry int

const (
	noCarry carry = iota
	posts
	reads
	provides
)

// carried is, per carry, the role a link's To takes to its From and the
// role its From takes to its To.
var carried = map[carry][2]Role{
	posts:    {WorkTo, WorkFrom},
	reads:    {Reads, ReadBy},
	provides: {ProvidedBy, ProvidedBy},
}

// carries is what a link moves: a posts_to hands work to the context it
// names, and a consumes or depends_on via data reads the contract it names.
// A provides via data implements or defines the contract, which says who
// stands behind it but not which way its data moves.
func carries(l Link) carry {
	switch {
	case l.Kind == model.PostsTo:
		return posts
	case l.Via == "data" && l.Kind.Uses():
		return reads
	case l.Via == "data" && l.Kind == model.Provides:
		return provides
	}
	return noCarry
}

// parents maps each present unit to the unit it sits inside.
type parents map[model.ID]model.ID

func newParents(units []model.Unit) parents {
	p := parents{}
	for _, u := range model.Present(units) {
		p[u.ID] = u.Parent
	}
	return p
}

// within reports whether x is id or sits inside it.
func (p parents) within(x, id model.ID) bool {
	for seen := map[model.ID]bool{}; x != "" && !seen[x]; x = p[x] {
		if x == id {
			return true
		}
		seen[x] = true
	}
	return false
}

// sides are the roles that name a unit, in Role order, each without a unit
// that sits inside another it names.
func (p parents) sides(by map[Role][]model.ID) []Side {
	var out []Side
	for r := Before; r <= ProvidedBy; r++ {
		var ids []model.ID
		for _, x := range by[r] {
			if !slices.ContainsFunc(by[r], func(y model.ID) bool { return y != x && p.within(x, y) }) {
				ids = append(ids, x)
			}
		}
		if len(ids) > 0 {
			out = append(out, Side{Role: r, Units: ids})
		}
	}
	return out
}

func appendNew(ids []model.ID, id model.ID) []model.ID {
	if slices.Contains(ids, id) {
		return ids
	}
	return append(ids, id)
}
