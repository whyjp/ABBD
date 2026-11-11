package screen

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// rowsWithChips is a module whose top box uses a box that uses a third, and
// consumes n contracts with long names, so each chip's line passes two rows
// below its box.
func rowsWithChips(n int) []model.Unit {
	units := []model.Unit{called("MOD-0001", "app"),
		inside("MOD-0002", "MOD-0001", rel("MOD-0003", model.DependsOn)),
		inside("MOD-0003", "MOD-0001", rel("MOD-0004", model.DependsOn)),
		inside("MOD-0004", "MOD-0001")}
	for i := 1; i <= n; i++ {
		id := model.ID(fmt.Sprintf("IF-%04d", i))
		units = append(units, called(id, fmt.Sprintf("a contract with a long name %d", i)))
		units[1].Relations = append(units[1].Relations, rel(id, model.Consumes))
	}
	return units
}

// A view whose chips overflow one edge hands some to the other edge after
// the layout. A chip with a lane would leave its lane behind there, so such
// a view is not drawn laned, as it is not drawn seated.
func TestAViewWhoseChipsOverflowAnEdgeIsNotDrawnLaned(t *testing.T) {
	alone := []model.Unit{called("MOD-0001", "app"), inside("MOD-0002", "MOD-0001")}
	for i := 1; i <= 20; i++ {
		id := model.ID(fmt.Sprintf("IF-%04d", i))
		alone = append(alone, called(id, fmt.Sprintf("a contract with a long name %d", i)))
		alone[1].Relations = append(alone[1].Relations, rel(id, model.Consumes))
	}
	for name, units := range map[string][]model.Unit{"one box": alone, "three rows": rowsWithChips(20)} {
		o := planOverview(units, "", nil, tactic{}, nil, nil, nil)
		if _, ok := o.inside("MOD-0001", units, "", nil, tactic{}, nil); !ok {
			t.Fatalf("%s: the plain tactic draws the view", name)
		}
		for _, by := range []tactic{{seated: true}, {laned: true}, {laned: true, follow: true}} {
			if _, ok := o.inside("MOD-0001", units, "", nil, by, nil); ok {
				t.Errorf("%s: %+v draws a view whose bottom edge cannot hold its chips", name, by)
			}
		}
	}
}

// The three-row view with chips that fit is drawn laned, and its lanes
// change the drawing, so the overflow case above refuses a view that would
// otherwise take lanes.
func TestAViewWithRoomForItsChipsTakesLanes(t *testing.T) {
	units := rowsWithChips(2)
	o := planOverview(units, "", nil, tactic{}, nil, nil, nil)
	plain, ok := o.inside("MOD-0001", units, "", nil, tactic{}, nil)
	if !ok {
		t.Fatal("the plain tactic draws the view")
	}
	laned, ok := o.inside("MOD-0001", units, "", nil, tactic{laned: true}, nil)
	if !ok {
		t.Fatal("the laned tactic draws a view whose chips fit")
	}
	if reflect.DeepEqual(plain.Drawing, laned.Drawing) {
		t.Error("the laned drawing is the plain one: no lane formed")
	}
}
