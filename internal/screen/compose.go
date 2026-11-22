package screen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A composition record of kind core names groups of a view's boxes that use
// each other, or reach outside the module. The view draws each such group
// as one folded box, with the group's role and what its inside holds, and the
// box opens the group's own view at stack-KEY.N. The record is read, never
// guessed: a view with none draws as it always did, byte for byte.
//
// The folded box is a unit of its own for the drawing only. It stands where
// its boxes stood, holds them, and is a module that has an inside, so the
// view, the lines lifted onto it, its page and the lint all treat it as they
// treat a module that holds others. It is never stored.

// foldedGroup is one folded group: its unit, and the boxes it holds.
type foldedGroup struct {
	unit    model.Unit
	members []model.ID
}

// composeGroups returns the units the screen draws: in's units with a folded
// group for each group of each record the state bears out, and the boxes of a
// group re-parented under it. A record is drawn when it is known, composes a
// view, is of kind core, and every claim it makes holds against the state and
// the code it cites; a group is folded when it is a cycle, a clique or a
// boundary of boxes the view draws directly. Without such a record it returns
// the units as they are.
func composeGroups(in Input) []model.Unit {
	groups, _ := planGroups(in.Units, in.Figures.Stacks)
	if len(groups) == 0 {
		return in.Units
	}
	units := slices.Clone(in.Units)
	for _, g := range groups {
		for i := range units {
			if slices.Contains(g.members, units[i].ID) {
				units[i].Parent = g.unit.ID
			}
		}
		units = append(units, g.unit)
	}
	return units
}

// planGroups are the groups of the stacks the screen folds, and why each
// other group of a stack that composes a view is left as lines, by its
// address.
func planGroups(units []model.Unit, stacks []store.Stack) ([]foldedGroup, map[string]string) {
	var groups []foldedGroup
	left := map[string]string{}
	taken := map[model.ID]string{}
	for _, f := range stacks {
		if !f.Known || f.Primary || f.View == "" {
			continue
		}
		reason := ""
		gaps := f.Gaps(units)
		switch {
		case f.View == model.OverviewView:
			// A box folded in the overview would lose the page of each box it holds.
			reason = "the overview's own record is not drawn yet"
		case f.Kind != model.KindCore:
			reason = "only a record of kind core is drawn"
		case len(gaps) > 0:
			reason = "the record is STALE: " + gaps[0].Text
		}
		c := f.Composition()
		boxes := model.ViewBoxes(units, f.View)
		for i, p := range f.Parts {
			l, address := p.Claim, model.GroupAddress(f.Key, i+1)
			why := reason
			if as := c.AsOf(l); why == "" && as != model.AsCycle && as != model.AsClique && as != model.AsBoundary {
				why = "only a cycle, a clique or a boundary is folded, not " + as
			}
			if why == "" {
				if stale := p.Why(units); len(stale) > 0 {
					why = "the group is STALE: " + stale[0]
				}
			}
			if why == "" && slices.ContainsFunc(l.Units, func(id model.ID) bool { return !slices.Contains(boxes, id) }) {
				why = "a box of the group is not one the view draws directly"
			}
			if why == "" {
				for _, id := range l.Units {
					if other, ok := taken[id]; ok {
						why = "box " + string(id) + " is held by " + other + ", which is folded first"
						break
					}
				}
			}
			if why != "" {
				left[address] = why
				continue
			}
			for _, id := range l.Units {
				taken[id] = address
			}
			groups = append(groups, foldedGroup{foldUnit(units, f.View, address, l), l.Units})
		}
	}
	return groups, left
}

// GroupDrawn says whether the group a stack address names is drawn as a folded
// box with a page of its own, and if not, why not.
func GroupDrawn(units []model.Unit, stacks []store.Stack, address string) (bool, string) {
	groups, left := planGroups(units, stacks)
	for _, g := range groups {
		if addressOf(g.unit.ID) == address {
			return true, ""
		}
	}
	return false, left[address]
}

// foldUnit is the folded unit of group l of the view: a module of its own, named
// and described as the record says, holding the anchors of its boxes.
func foldUnit(units []model.Unit, view, address string, l model.Layer) model.Unit {
	module := model.ID(view)
	g := model.Unit{ID: groupID(l.Units, address), Kind: model.Module, Parent: module, Name: model.Said(model.Reader, l.Name)}
	role := model.Said(model.Reader, l.Role)
	g.Responsibility = &role
	for _, u := range units {
		if slices.Contains(l.Units, u.ID) {
			for _, a := range u.Anchors {
				if !slices.Contains(g.Anchors, a) {
					g.Anchors = append(g.Anchors, a)
				}
			}
		}
	}
	cycle, boundary := model.GroupFacts(units, l.Units, module)
	g.Mark = groupMark(len(l.Units), cycle, boundary)
	return g
}

// groupID is the unit ID a folded group is drawn under: its address after the
// lowest ID of its boxes and a tilde, so the group stands in the order its
// first box stood in wherever the drawing breaks a tie by ID, and the address
// is read back after the tilde.
func groupID(members []model.ID, address string) model.ID {
	return slices.Min(members) + "~" + model.ID(address)
}

// groupMark is what a group's box says of its inside, in words a reader
// takes at rest: the boxes that use each other round a cycle, and how many of
// the group's boxes reach outside the module.
func groupMark(n, cycle, boundary int) string {
	var parts []string
	if cycle > 1 {
		parts = append(parts, fmt.Sprintf("↻ 안에 순환 %d상자", cycle))
	}
	if boundary > 0 {
		parts = append(parts, fmt.Sprintf("바깥과 이어진 %d상자", boundary))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d상자", n)
	}
	return strings.Join(parts, " · ")
}

// fitMark is the most of a group's mark that fits width on the box's last
// line, which says it in place of the kind: all of it, else its first part,
// else its last; "" when none fits, and the tooltip then holds it whole.
func fitMark(mark string, width int) string {
	parts := strings.Split(mark, " · ")
	for _, s := range []string{mark, parts[0], parts[len(parts)-1]} {
		if s != "" && subWidth(s) <= width {
			return s
		}
	}
	return ""
}

// addressOf is the address a unit's view is known by: a folded group's
// address after the tilde of its drawing ID (groupID), else the ID itself.
func addressOf(id model.ID) string {
	if _, address, ok := strings.Cut(string(id), "~"); ok {
		return address
	}
	return string(id)
}

// nameGroups gives each folded group's view, and the group's line in the view
// of the module it stands in, the group's address for an ID, as the page, its
// anchor and graph.json name it. The drawings keep their drawing IDs.
func (m *Model) nameGroups() {
	for i := range m.Insides {
		ins := &m.Insides[i]
		ins.ID = addressOf(model.ID(ins.ID))
		for j := range ins.Units {
			ins.Units[j].ID = addressOf(model.ID(ins.Units[j].ID))
		}
	}
}
