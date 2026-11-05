package screen

import (
	"fmt"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A drawing of exactly one screen fits, and one pixel more does not. The
// overview has less room under a list of questions, so the same drawing
// that fits a bare overview runs past the screen under two flows.
func TestADrawingFitsOneScreenUpToItsEdge(t *testing.T) {
	fits := func(v view) []Breach { return (&look{v: v}).fitsOneScreen() }
	edge := Drawing{Width: screenW, Height: screenH}
	if got := fits(views(Model{Overview: edge})[0]); len(got) != 0 {
		t.Errorf("a %dx%d overview: %+v, want it to fit", screenW, screenH, got)
	}
	tall := Drawing{Width: screenW, Height: screenH + 1}
	if got := fits(views(Model{Overview: tall})[0]); len(got) != 1 || got[0].Got != 1 {
		t.Errorf("a %dx%d overview: %+v, want one breach 1px past", screenW, screenH+1, got)
	}
	asked := views(Model{Overview: edge, Flows: make([]FlowView, 2)})[0]
	if got := fits(asked); len(got) != 1 {
		t.Errorf("a %dx%d overview under two flows: %+v, want one breach", screenW, screenH, got)
	}
}

// Two boxes joined by one two-headed line stand well in rows next to each
// other. Further apart, they are one breach of P1, with the rows past that.
func TestAMutualPairStandsCloseOrBreaksP1(t *testing.T) {
	mutual := []Wire{{Kind: "mutual", ends: []string{"A", "B"}}}
	rels := [][2]string{{"A", "B"}, {"B", "A"}}
	near := newLook(view{name: "sketch", d: Drawing{Relations: rels, Wires: mutual}}, sketch(t, []string{"A 40 0 60 20", "B 40 80 60 20"}, ""))
	if got := near.usersAbove(); len(got) != 0 {
		t.Errorf("a mutual pair in rows next to each other: %+v, want none", got)
	}
	far := newLook(view{name: "sketch", d: Drawing{Relations: rels, Wires: mutual}}, sketch(t, []string{"A 40 0 60 20", "C 200 80 60 20", "B 40 160 60 20"}, ""))
	if got := far.usersAbove(); len(got) != 1 || got[0].Got != 1 {
		t.Errorf("a mutual pair two rows apart: %+v, want one breach 1 row past", got)
	}
}

// Past the screen comes before every other measure a reader pays after
// the hard breaches, and how far past only after what lint weighs, so a
// drawing a pixel shorter that still scrolls wins nothing by crossing more.
func TestFittingTheScreenWeighsBeforeHowFarPast(t *testing.T) {
	fits, scrolls := cost{seen: 40}, cost{over: 1, seen: 4}
	if !fits.beats(scrolls) {
		t.Error("a drawing that scrolls beats one that fits")
	}
	shorter, clearer := cost{over: 99, seen: 40}, cost{over: 100, seen: 4}
	if shorter.beats(clearer) {
		t.Error("a drawing 1px shorter that still scrolls beats one that crosses less")
	}
}

// The overview takes a new ranking only where it fits the screen or has
// fewer hard breaches, and never where it stands more boxes off what they
// join; otherwise it keeps the plain one, however few turns the other saves.
func TestTheOverviewKeepsThePlainRankingUnlessAnotherFixesIt(t *testing.T) {
	pick := func(costs ...cost) int {
		ts := overviewTactics[:len(costs)]
		return choose(ts, func(t tactic, read *cost) (int, bool) {
			i := slices.Index(ts, t)
			if t.counted {
				i = slices.Index(ts, tactic{fold: t.fold, entry: t.entry})
			}
			*read = costs[i]
			return i, true
		}, func(int, *cost) {})
	}
	plain := cost{over: 50, seen: 40}
	if got := pick(plain, cost{seen: 80}); got != 1 {
		t.Errorf("a fold that fits where plain scrolls: picked %d, want the fold", got)
	}
	if got := pick(plain, cost{seen: 80, moved: 1}); got != 0 {
		t.Errorf("a fold that fits but moves a box: picked %d, want plain", got)
	}
	if got := pick(cost{seen: 40}, cost{seen: 4}); got != 0 {
		t.Errorf("a fold that only saves turns: picked %d, want plain", got)
	}
	if got := pick(cost{hard: 2, seen: 40}, cost{seen: 80}); got != 1 {
		t.Errorf("a fold with fewer hard breaches: picked %d, want the fold", got)
	}
	// The fold drawn again with room for its counts reads better but moves
	// a box, so the gate passes it over and the fold stands uncounted.
	ts := overviewTactics[:2]
	got := choose(ts, func(t tactic, read *cost) (int, bool) {
		switch {
		case t.counted:
			*read = cost{seen: 4, moved: 1}
			return -1, true
		case t.fold:
			*read = cost{seen: 80, roomy: true}
			return 1, true
		}
		*read = plain
		return 0, true
	}, func(int, *cost) {})
	if got != 1 {
		t.Errorf("a counted fold that moves a box: picked %d, want the uncounted fold", got)
	}
}

// A chain of modules that use each other ranks a row each when drawn
// plainly. Short, it fits the screen and keeps that ranking; long enough
// to run past it, Build folds the pairs into shared rows.
func TestBuildFoldsAChainOnlyWhenItRunsPastTheScreen(t *testing.T) {
	chain := func(n int) []model.Unit {
		var us []model.Unit
		for i := 1; i <= n; i++ {
			var rs []model.Relation
			for _, j := range []int{i - 1, i + 1} {
				if j >= 1 && j <= n {
					rs = append(rs, rel(model.ID(fmt.Sprintf("MOD-%04d", j)), model.DependsOn))
				}
			}
			us = append(us, called(model.ID(fmt.Sprintf("MOD-%04d", i)), fmt.Sprintf("m%d", i), rs...))
		}
		return us
	}
	plainly := func(us []model.Unit) Drawing {
		return planOverview(us, "", nil, tactic{}).draw(func(model.ID) string { return "" })
	}
	short := chain(8)
	if p, got := plainly(short), build(short).Overview; p.Height > screenH || got.Height != p.Height {
		t.Errorf("8 modules: plain %dpx tall, drawn %dpx; want the plain drawing, which fits", p.Height, got.Height)
	}
	long := chain(9)
	if p, got := plainly(long), build(long).Overview; p.Height <= screenH || got.Height > screenH {
		t.Errorf("9 modules: plain %dpx tall, drawn %dpx; want plain past %d and the drawing within it", p.Height, got.Height, screenH)
	}
}

// Where the places ranked first come into a cycle, an entering tactic
// breaks it there: the box they use stands first of the cycle, not the
// one the plain pick takes by its edges alone.
func TestEntryBreaksACycleWhereItIsEntered(t *testing.T) {
	nodes := []lnode{{"a", 100, 60}, {"b", 100, 60}, {"c", 100, 60}, {"s", 100, 60}}
	edges := []ledge{{"s", "b"}, {"a", "b"}, {"b", "c"}, {"c", "a"}}
	plain := arrange(slices.Clone(nodes), edges, overviewBudget, nil, tactic{})
	entry := arrange(slices.Clone(nodes), edges, overviewBudget, nil, tactic{entry: true})
	if entry.idle {
		t.Fatal("an entry that changed the ranking says it is idle")
	}
	if !(entry.row["b"] < entry.row["c"] && entry.row["c"] < entry.row["a"]) {
		t.Errorf("entry ranks rows %v; want b, then c, then a", entry.row)
	}
	if plain.row["b"] < plain.row["a"] {
		t.Errorf("plain ranks rows %v; want a above b, so entry changes something", plain.row)
	}
}
