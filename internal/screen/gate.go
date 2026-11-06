package screen

import (
	"fmt"
	"maps"
	"slices"

	"github.com/whyjp/abbd/internal/store"
)

// The gate holds the drawings of a repository to what they owe, as its
// .abbd/lint-debt.yaml records it. A view may keep the hard breaches it
// owes and no more. Over every view, the weighed sum, the tees and the
// excess turns may not pass their ceilings, and the placement debt may pass its ceiling by
// placementRise of it, so a change that saves turns may move a few boxes.

// placementRise is the share of its ceiling the placement debt may rise by,
// as a divisor: 2%.
const placementRise = 50

// gated are the totals with a ceiling, by the names the debt file gives
// them, and how far each may pass its ceiling.
var gated = []struct {
	name  string
	total func(Totals) int
	slack func(ceiling int) int
}{
	{"weighed", func(t Totals) int { return t.Weighed }, func(int) int { return 0 }},
	// tees count once in the weighed sum, as crossings, and a T reads as a
	// branch, a relation that is not there, so they may not rise whatever
	// else a change saves.
	{"tees", func(t Totals) int { return t.Tees }, func(int) int { return 0 }},
	// the weighed sum prices an excess turn at one crossing, so it would
	// buy a crossing saved with a bend the reader has to follow; the
	// excess turns may not rise whatever else a change saves.
	{"excess_turns", func(t Totals) int { return t.ExcessTurns }, func(int) int { return 0 }},
	{"placement", func(t Totals) int { return t.Placement }, func(c int) int { return c / placementRise }},
	{"detours", func(t Totals) int { return t.Detours }, func(int) int { return 0 }},
	// overflow is how many views pass one screen (S).
	{"overflow", func(t Totals) int { return t.Criteria["S"] }, func(int) int { return 0 }},
}

// Gate marks each hard breach the view owes as debt, and lists every way
// views break the gate: a view with more hard breaches of a criterion than
// it owes, and, when views are every drawing, a total past its ceiling.
func Gate(views []ViewLint, d store.LintDebt, every bool) []string {
	var fails []string
	for vi := range views {
		v := &views[vi]
		owed := d.Views[v.View]
		for _, c := range criteria {
			got := v.Totals.Criteria[c.ID]
			if !c.Hard || got == 0 {
				continue
			}
			within := got <= owed[c.ID]
			for bi := range v.Breaches {
				if v.Breaches[bi].Criterion == c.ID {
					v.Breaches[bi].Debt = within
				}
			}
			if !within {
				fails = append(fails, fmt.Sprintf("%s: %s of %s, %d recorded as debt", v.View, plural(got, "hard breach"), c.ID, owed[c.ID]))
			}
		}
	}
	if !every {
		return fails
	}
	t := Sum(views)
	for _, g := range gated {
		c, ok := d.Ceilings[g.name]
		if got := g.total(t); ok && got > c+g.slack(c) {
			fails = append(fails, fmt.Sprintf("%s %d, past its ceiling %d by more than %d", g.name, got, c, g.slack(c)))
		}
	}
	return fails
}

// Lowerable lists what the debt file records above what views measure: a debt
// a view no longer owes in full, and, when views are every drawing, a
// ceiling above its total. Each may be lowered.
func Lowerable(views []ViewLint, d store.LintDebt, every bool) []string {
	var out []string
	measured := map[string]map[string]int{}
	for _, v := range views {
		measured[v.View] = v.Totals.Criteria
	}
	for _, view := range slices.Sorted(maps.Keys(d.Views)) {
		got, ok := measured[view]
		if !ok && !every {
			continue
		}
		for _, id := range slices.Sorted(maps.Keys(d.Views[view])) {
			if owed := d.Views[view][id]; got[id] < owed {
				out = append(out, fmt.Sprintf("%s: %d recorded as debt of %s, %d measured", view, owed, id, got[id]))
			}
		}
	}
	if !every {
		return out
	}
	t := Sum(views)
	for _, g := range gated {
		if c, ok := d.Ceilings[g.name]; ok && g.total(t) < c {
			out = append(out, fmt.Sprintf("%s ceiling %d, %d measured", g.name, c, g.total(t)))
		}
	}
	return out
}

// Record is the debt of every drawing as views measure it: each view's
// hard breaches by criterion, and each ceiling at its total or at what prior
// sets it to, whichever is lower. A ceiling never rises on its own, so the
// slack the gate allows a total cannot ratchet it up one record at a time.
func Record(views []ViewLint, prior store.LintDebt) store.LintDebt {
	d := store.LintDebt{Ceilings: map[string]int{}, Views: map[string]map[string]int{}}
	for _, v := range views {
		for _, c := range criteria {
			if n := v.Totals.Criteria[c.ID]; c.Hard && n > 0 {
				if d.Views[v.View] == nil {
					d.Views[v.View] = map[string]int{}
				}
				d.Views[v.View][c.ID] = n
			}
		}
	}
	t := Sum(views)
	for _, g := range gated {
		d.Ceilings[g.name] = g.total(t)
		if c, ok := prior.Ceilings[g.name]; ok && c < d.Ceilings[g.name] {
			d.Ceilings[g.name] = c
		}
	}
	return d
}
