package screen

import (
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// pstack's restate and catch up has a drawing that saves 3 excess turns
// with 7 staircase steps and 2 turn-backs. The chooser keeps the drawing a
// reader sees fewer staircases in, and the gate weighs every view by what
// the chooser ranks it by, so it weighs no other drawing of the view with
// as few hard breaches and as few chips moved below the one kept.
func TestTheGateWeighsAViewAsTheChooserRanksIt(t *testing.T) {
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	o := planOverview(units, "", nil, tactic{})
	type drawn struct {
		weighed, hard, moved int
	}
	var all []drawn
	kept := choose(tactics, func(by tactic, read *cost) (Inside, bool) {
		return o.inside("MOD-0006", units, "", nil, by, read)
	}, func(ins Inside, read *cost) {
		read.judge(ins.Drawing)
		vl, err := lintView(view{d: ins.Drawing, budget: insideWidth, room: screenH})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, drawn{vl.Totals.Weighed, read.hard, read.moved})
	})
	got, err := lintView(view{d: kept.Drawing, budget: insideWidth, room: screenH})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Fatalf("%d drawings judged, want the view drawn more than one way", len(all))
	}
	for _, d := range all {
		if d.hard <= got.Totals.Hard && d.moved <= got.Totals.Placement && d.weighed < got.Totals.Weighed {
			t.Errorf("the gate weighs the kept drawing %d, and a drawing the chooser passed over %d", got.Totals.Weighed, d.weighed)
		}
	}
}
