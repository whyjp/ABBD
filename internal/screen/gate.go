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
// Each page the debt file names, one a reader looks at, may not raise what
// it records of the page's own totals.

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

// paged are the totals of one page the debt file may bound, by the names it
// gives them: what a reader sees at rest on that page, which a sum over
// every view would trade against views no one looks at. Each bound is a
// ceiling the total may not pass, or for a floor, one it may not fall under.
var paged = []struct {
	name  string
	total func(Totals) int
	floor bool
}{
	{"colour_switches", func(t Totals) int { return t.ColourSwitches }, false},
	{"parallel_ink", func(t Totals) int { return t.ParallelInk }, false},
	{"abreast", func(t Totals) int { return t.Abreast }, false},
	// combs are stubs a box's lines of one kind leave or reach it by side
	// by side, where a hand drawing draws one stem.
	{"combs", func(t Totals) int { return t.Combs }, false},
	{"doglegs", func(t Totals) int { return t.Doglegs }, false},
	// counts and dots are the ×N and junction dots a reader sees, which
	// the exemplars draw none of.
	{"counts", func(t Totals) int { return t.Counts }, false},
	{"dots", func(t Totals) int { return t.Dots }, false},
	// label repeats are words said again beside the same lines, where a
	// hand drawing says each once.
	{"label_repeats", func(t Totals) int { return t.LabelRepeats }, false},
	// labelled routes are the ones a reader finds a word on at rest, which
	// saying a word once must not take from the page.
	{"labelled", func(t Totals) int { return t.Labelled }, true},
	// density is the routes a box carries at rest, which the exemplars keep
	// to one or two.
	{"density", func(t Totals) int { return t.Density }, false},
}

// past says a page total got breaks its bound c.
func past(floor bool, got, c int) bool {
	if floor {
		return got < c
	}
	return got > c
}

// Gate marks each hard breach the view owes as debt, and lists every way
// views break the gate: a view with more hard breaches of a criterion than
// it owes, a page total past its bound, and, when views are every
// drawing, a page the debt file names that no view draws and a total past
// its ceiling.
func Gate(views []ViewLint, d store.LintDebt, every bool) []string {
	var fails []string
	drawn := map[string]bool{}
	for vi := range views {
		v := &views[vi]
		drawn[v.View] = true
		for _, g := range paged {
			if c, ok := d.Pages[v.View][g.name]; ok && past(g.floor, g.total(v.Totals), c) {
				fails = append(fails, fmt.Sprintf("%s: %s %d, past its page bound %d", v.View, g.name, g.total(v.Totals), c))
			}
		}
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
	for _, page := range slices.Sorted(maps.Keys(d.Pages)) {
		if !drawn[page] {
			fails = append(fails, fmt.Sprintf("%s: a page the debt file bounds, and no view draws it", page))
		}
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
// a view no longer owes in full, a page bound looser than the page's total, and,
// when views are every drawing, a ceiling above its total. Each may be
// lowered.
func Lowerable(views []ViewLint, d store.LintDebt, every bool) []string {
	var out []string
	measured := map[string]map[string]int{}
	for _, v := range views {
		measured[v.View] = v.Totals.Criteria
		for _, g := range paged {
			if c, ok := d.Pages[v.View][g.name]; ok && past(!g.floor, g.total(v.Totals), c) {
				out = append(out, fmt.Sprintf("%s: page bound %s %d, %d measured", v.View, g.name, c, g.total(v.Totals)))
			}
		}
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
// hard breaches by criterion, each page prior names with its totals, and
// each ceiling at its total or at what prior sets it to, whichever is
// lower. A ceiling never rises on its own, so the slack the gate allows a
// total cannot ratchet it up one record at a time.
func Record(views []ViewLint, prior store.LintDebt) store.LintDebt {
	d := store.LintDebt{Ceilings: map[string]int{}, Views: map[string]map[string]int{}}
	for _, v := range views {
		if was, ok := prior.Pages[v.View]; ok {
			if d.Pages == nil {
				d.Pages = map[string]map[string]int{}
			}
			d.Pages[v.View] = map[string]int{}
			for _, g := range paged {
				n := g.total(v.Totals)
				if c, ok := was[g.name]; ok && past(g.floor, n, c) {
					n = c
				}
				d.Pages[v.View][g.name] = n
			}
		}
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
