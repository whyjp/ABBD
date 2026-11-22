package model

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// A composition record is a stack widened to say how one view is composed:
// which kind of order its groups stand in, which view it is of, what each
// group is for, what the groups promise one another, and what the view
// offers outside. The reader writes it from the code and abbd recomputes
// every claim the relations can bear out, when it is stored and each time the
// state moves, so a claim the relations no longer bear out reads STALE.
//
// The kinds say what the order of the groups means.
const (
	// KindLayers: a group uses the groups below it. A stack with no kind
	// reads so.
	KindLayers = "layers"
	// KindStages: the groups are stages in the order they run.
	KindStages = "stages"
	// KindFamily: each group holds siblings that keep one promise, a
	// contract they provide or use.
	KindFamily = "family"
	// KindCore: the groups are what the relations make of the view: boxes
	// that use each other, or that reach outside it.
	KindCore = "core"
)

// What a group is, by its as field. A kind that names one meaning for its
// groups has it by default.
const (
	AsLayer    = "layer"
	AsStage    = "stage"
	AsMembers  = "members"
	AsBoundary = "boundary"
	AsCycle    = "cycle"
	AsClique   = "clique"
	AsHub      = "hub"
)

// OverviewView is the view field of a composition of the overview; any other
// view is a module's ID.
const OverviewView = "overview"

// groupAs lists what each kind's groups may be, the first being the default
// of a kind that has one; a core group must say which.
var groupAs = map[string][]string{
	"":         {AsLayer},
	KindLayers: {AsLayer},
	KindStages: {AsStage},
	KindFamily: {AsMembers},
	KindCore:   {AsBoundary, AsCycle, AsClique, AsHub},
}

// Composition is what a stack says beyond its groups.
type Composition struct {
	// View is OverviewView or the ID of the module whose inside the record
	// composes; empty for a stack that composes no view.
	View string
	Kind string
	// Faces are the contracts the view offers outside, which a unit inside
	// it provides.
	Faces []ID
	// Primary says the stack is the repository's primary flow, whose groups
	// are stages.
	Primary bool
}

// Said is whether the record says anything a plain stack does not.
func (c Composition) Said() bool { return c.View != "" || c.Kind != "" || len(c.Faces) > 0 }

// AsOf is what a group of the record is: the as it says, or its kind's
// default, "" for a core group that says none.
func (c Composition) AsOf(l Layer) string {
	if l.As != "" {
		return l.As
	}
	if as := groupAs[c.kind()]; c.kind() != KindCore && len(as) > 0 {
		return as[0]
	}
	return ""
}

// kind is the record's kind, stages for the stack marked the primary flow
// that names none.
func (c Composition) kind() string {
	if c.Kind == "" && c.Primary {
		return KindStages
	}
	return c.Kind
}

// Validate checks the record on its own, before the state: the kind and the
// groups' as are words it knows, and each group's promises and the faces are
// well formed contract IDs.
func (c Composition) Validate(layers []Layer) error {
	if _, ok := groupAs[c.Kind]; !ok {
		return fmt.Errorf("kind is layers, stages, family or core, not %q", c.Kind)
	}
	if c.Primary && c.Kind != "" && c.Kind != KindStages {
		return fmt.Errorf("a stack marked the primary flow is kind stages, not %s", c.Kind)
	}
	if c.View != "" && c.View != OverviewView {
		if k, _, err := ParseID(c.View); err != nil || k != Module {
			return fmt.Errorf("view is overview or the ID of a module, not %q", c.View)
		}
	}
	for i, l := range layers {
		if l.As != "" && !slices.Contains(groupAs[c.kind()], l.As) {
			return fmt.Errorf("layer %d is as %s, which a stack of kind %s does not hold; its groups are as %s", i+1, l.As, kindWord(c.kind()), strings.Join(groupAs[c.kind()], ", "))
		}
		as := c.AsOf(l)
		switch {
		case as == "":
			return fmt.Errorf("layer %d of a core stack says what it is: as %s", i+1, strings.Join(groupAs[KindCore], ", "))
		case as == AsHub && len(l.Units) != 1:
			return fmt.Errorf("layer %d is a hub, which is one box; it names %d units", i+1, len(l.Units))
		case (as == AsCycle || as == AsClique) && len(l.Units) < 2:
			return fmt.Errorf("layer %d is a %s, which takes two units or more; it names %d", i+1, as, len(l.Units))
		}
		if err := contractIDs(l.Promises, fmt.Sprintf("layer %d promises", i+1)); err != nil {
			return err
		}
		if c.Kind == KindFamily && len(l.Promises) == 0 {
			return fmt.Errorf("layer %d of a family names no promise; its siblings keep one contract, which promises names", i+1)
		}
	}
	if len(c.Faces) > 0 && c.View == "" {
		return fmt.Errorf("faces are what a view offers outside; say which view in the view field")
	}
	return contractIDs(c.Faces, "faces")
}

func kindWord(k string) string {
	if k == "" {
		return "layers"
	}
	return k
}

func contractIDs(ids []ID, what string) error {
	for i, id := range ids {
		k, _, err := ParseID(string(id))
		if err != nil {
			return err
		}
		if !k.IsContract() {
			return fmt.Errorf("%s names %s, which is no contract", what, id)
		}
		if slices.Contains(ids[:i], id) {
			return fmt.Errorf("%s names %s twice", what, id)
		}
	}
	return nil
}

// Gaps read the record against the state, group by group. Each group's units
// stand in the composed view, and what the relations must bear out they do:
// a cycle closes, a clique is complete, a hub is used with half the view's
// boxes, a boundary box reaches outside the view, a promise is provided by
// its group and used by another, a family's siblings are tied to a
// contract, and a face is provided from inside the view. A relation counts
// when it says how it arises (its via), since one that does not may be a
// static dependency and would make a tangle of it; a claim only such
// relations bear out says so. A layer index in a Gap is from 1.
func (c Composition) Gaps(layers []Layer, units []Unit) []Gap {
	if err := c.Validate(layers); err != nil {
		return []Gap{{Text: "the record is no composition: " + err.Error()}}
	}
	st := stateOf(units)
	var gaps []Gap
	add := func(part int, format string, args ...any) {
		g := Gap{Text: fmt.Sprintf(format, args...)}
		if part > 0 {
			g.Parts = []int{part}
		}
		gaps = append(gaps, g)
	}
	var boxes []ID
	if c.View != "" {
		boxes = st.boxesOf(c.View)
		if c.View != OverviewView {
			if m, ok := st[ID(c.View)]; !ok || !m.Kind.Holds() {
				add(0, "view %s is no module in the state", c.View)
				return gaps
			}
		}
		for i, l := range layers {
			for _, id := range l.Units {
				if _, ok := st[id]; ok && c.View != OverviewView && (id == ID(c.View) || !st.under(id, ID(c.View))) {
					add(i+1, "%s is not inside %s, the view this record composes", id, c.View)
				}
			}
		}
	}
	for i, l := range layers {
		n := i + 1
		switch c.AsOf(l) {
		case AsCycle:
			if !st.closes(l.Units, true) {
				add(n, "%s", st.notShown(l.Units, "close a cycle of use among themselves", st.closes(l.Units, false)))
			}
		case AsClique:
			if !st.complete(l.Units, true) {
				add(n, "%s", st.notShown(l.Units, "all use one another", st.complete(l.Units, false)))
			}
		case AsHub:
			switch {
			case c.View == "":
				add(n, "a hub is a box used with the view's boxes; say the view")
			case !st.hubbed(l.Units[0], boxes, true):
				add(n, "%s", st.notShown(l.Units, "is used both ways with half the view's other boxes", st.hubbed(l.Units[0], boxes, false)))
			}
		case AsBoundary:
			if c.View == "" || c.View == OverviewView {
				add(n, "a boundary is the boxes of a module that reach outside it; say the module in the view field")
				break
			}
			for _, id := range l.Units {
				if _, ok := st[id]; ok && !st.reachesOutside(id, ID(c.View)) {
					add(n, "%s has no relation that says how it arises with a unit outside %s", id, c.View)
				}
			}
		}
		for _, p := range l.Promises {
			if _, ok := st[p]; !ok {
				add(n, "%s is no longer in the state", p)
				continue
			}
			if !st.providedBy(p, l.Units) {
				add(n, "no unit of layer %d provides %s, which it promises", n, p)
			}
			if c.Kind != KindFamily && !st.usedByAnother(p, l.Units, layers) {
				add(n, "no unit of another layer uses %s, which layer %d promises", p, n)
			}
		}
		if c.Kind == KindFamily {
			for _, id := range l.Units {
				if _, ok := st[id]; ok && !st.tiedTo(id, l.Promises) {
					add(n, "%s neither provides nor uses a contract layer %d promises; a sibling is tied to the contract its family keeps", id, n)
				}
			}
		}
	}
	for _, f := range c.Faces {
		switch {
		case st[f].ID == "":
			add(0, "face %s is no longer in the state", f)
		case c.View != "" && !st.offeredBy(f, c.View):
			add(0, "no unit inside %s provides %s, which the view faces outside with", c.View, f)
		}
	}
	return gaps
}

// boxesOf are the boxes of a view: the units of the overview no present unit
// holds, or the units directly inside a module.
func (st state) boxesOf(view string) []ID {
	var out []ID
	for id, u := range st {
		if view == OverviewView {
			if _, ok := st[u.Parent]; !ok {
				out = append(out, id)
			}
		} else if u.Parent == ID(view) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// ViewBoxes are the boxes of a view in the state: the units of the overview
// no present unit holds, or the units directly inside the module.
func ViewBoxes(units []Unit, view string) []ID { return stateOf(units).boxesOf(view) }

// link reports whether a unit inside a uses, by a live relation, a unit
// inside b. A relation counts when viaOnly is false, or when it says its via.
func (st state) link(a, b ID, viaOnly bool) bool {
	for id, u := range st {
		if !st.under(id, a) {
			continue
		}
		for _, r := range u.Relations {
			if !r.Dead && r.Kind.Uses() && (!viaOnly || r.Via != "") && st.under(r.To, b) {
				return true
			}
		}
	}
	return false
}

// closes reports whether the boxes form one closed cycle: each reaches every
// other, by the uses among them, and is reached back.
func (st state) closes(ids []ID, viaOnly bool) bool {
	for _, from := range ids {
		seen := map[ID]bool{from: true}
		queue := []ID{from}
		for len(queue) > 0 {
			at := queue[0]
			queue = queue[1:]
			for _, to := range ids {
				if !seen[to] && st.link(at, to, viaOnly) {
					seen[to] = true
					queue = append(queue, to)
				}
			}
		}
		for _, to := range ids {
			if !seen[to] {
				return false
			}
		}
	}
	return true
}

// complete reports whether every pair of the boxes use each other.
func (st state) complete(ids []ID, viaOnly bool) bool {
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			if !st.link(a, b, viaOnly) || !st.link(b, a, viaOnly) {
				return false
			}
		}
	}
	return true
}

// hubbed reports whether the box uses and is used by at least half of the
// other boxes of its view.
func (st state) hubbed(hub ID, boxes []ID, viaOnly bool) bool {
	others, both := 0, 0
	for _, b := range boxes {
		if b == hub {
			continue
		}
		others++
		if st.link(hub, b, viaOnly) && st.link(b, hub, viaOnly) {
			both++
		}
	}
	return others > 0 && 2*both >= others
}

// reachesOutside reports whether a unit inside id has a relation that says
// its via with a unit outside the module, in either direction.
func (st state) reachesOutside(id, module ID) bool {
	for from, u := range st {
		for _, r := range u.Relations {
			if _, present := st[r.To]; !present || r.Dead || r.Via == "" || !r.Kind.Uses() {
				continue
			}
			if st.under(from, id) && !st.under(r.To, module) || st.under(r.To, id) && !st.under(from, module) {
				return true
			}
		}
	}
	return false
}

// providedBy reports whether a unit inside one of tops provides the contract.
func (st state) providedBy(contract ID, tops []ID) bool {
	for id, u := range st {
		if !st.within(id, tops) {
			continue
		}
		for _, r := range u.Relations {
			if !r.Dead && r.Kind == Provides && r.To == contract {
				return true
			}
		}
	}
	return false
}

// usedByAnother reports whether a unit inside another layer of the record
// uses the contract.
func (st state) usedByAnother(contract ID, mine []ID, layers []Layer) bool {
	for id, u := range st {
		if st.within(id, mine) {
			continue
		}
		if !slices.ContainsFunc(layers, func(l Layer) bool { return st.within(id, l.Units) }) {
			continue
		}
		for _, r := range u.Relations {
			if !r.Dead && r.Kind.Uses() && r.Via != "" && r.To == contract {
				return true
			}
		}
	}
	return false
}

// tiedTo reports whether a unit inside id provides or uses one of the contracts.
func (st state) tiedTo(id ID, contracts []ID) bool {
	for from, u := range st {
		if !st.under(from, id) {
			continue
		}
		for _, r := range u.Relations {
			if !r.Dead && (r.Kind == Provides || r.Kind.Uses() && r.Via != "") && slices.Contains(contracts, r.To) {
				return true
			}
		}
	}
	return false
}

// offeredBy reports whether a unit inside the view provides the contract.
func (st state) offeredBy(contract ID, view string) bool {
	for id, u := range st {
		if view != OverviewView && !st.under(id, ID(view)) {
			continue
		}
		for _, r := range u.Relations {
			if !r.Dead && r.Kind == Provides && r.To == contract {
				return true
			}
		}
	}
	return false
}

// notShown words a claim the relations do not bear out, and says when it is
// relations with no via that would.
func (st state) notShown(ids []ID, claim string, withoutVia bool) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = string(id)
	}
	text := fmt.Sprintf("the relations that say how they arise do not show that %s %s", strings.Join(names, ", "), claim)
	if withoutVia {
		text += "; only relations with no via say so, and they may be static dependencies, so fill each via first or leave the boxes ungrouped"
	}
	return text
}

// GroupAddress is the address of one group of a stack: stack-KEY.N, N from
// 1, as the stack's page anchors it. The group's inside view stands at it.
func GroupAddress(k FigureKey, n int) string { return fmt.Sprintf("stack-%s.%d", k, n) }

// ParseGroupAddress reads stack-KEY.N back. It checks the shape only; the
// stored stack says whether the group is there.
func ParseGroupAddress(s string) (FigureKey, int, error) {
	rest, ok := strings.CutPrefix(s, "stack-")
	if !ok {
		return "", 0, fmt.Errorf("%q is no group address; it reads stack-KEY.N", s)
	}
	key, num, ok := strings.Cut(rest, ".")
	n, err := strconv.Atoi(num)
	if !ok || err != nil || n < 1 || strconv.Itoa(n) != num {
		return "", 0, fmt.Errorf("%q is no group address; it reads stack-KEY.N, N counted from 1", s)
	}
	k, err := ParseFigureKey[Layer](key)
	if err != nil {
		return "", 0, err
	}
	return k, n, nil
}

// GroupFacts say what a group of boxes holds inside: the most boxes that use
// each other round one cycle (0 for none), and how many of them have a
// relation with a unit outside the module that says its via. Uses with no via
// count for the cycle, as the drawing draws them, since this only describes
// the inside and claims nothing.
func GroupFacts(units []Unit, members []ID, module ID) (cycle, boundary int) {
	st := stateOf(units)
	left := slices.Clone(members)
	for len(left) > 0 {
		at := left[0]
		comp := []ID{at}
		for _, o := range left[1:] {
			if st.leadsTo(at, o, members) && st.leadsTo(o, at, members) {
				comp = append(comp, o)
			}
		}
		if len(comp) > 1 {
			cycle = max(cycle, len(comp))
		}
		left = slices.DeleteFunc(left, func(id ID) bool { return slices.Contains(comp, id) })
	}
	for _, id := range members {
		if st.reachesOutside(id, module) {
			boundary++
		}
	}
	return cycle, boundary
}

// leadsTo reports whether a reaches b along the uses among boxes.
func (st state) leadsTo(a, b ID, boxes []ID) bool {
	seen := map[ID]bool{a: true}
	queue := []ID{a}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, to := range boxes {
			if !seen[to] && st.link(at, to, false) {
				if to == b {
					return true
				}
				seen[to] = true
				queue = append(queue, to)
			}
		}
	}
	return false
}
