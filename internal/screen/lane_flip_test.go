package screen

import (
	"fmt"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A view whose chips overflow one edge hands some to the other edge after
// the layout. A chip with a lane would leave its lane behind there, so such
// a view is not drawn laned, as it is not drawn seated.
func TestAViewWhoseChipsOverflowAnEdgeIsNotDrawnLaned(t *testing.T) {
	units := []model.Unit{called("MOD-0001", "app"), inside("MOD-0002", "MOD-0001")}
	for i := 1; i <= 20; i++ {
		id := model.ID(fmt.Sprintf("IF-%04d", i))
		units = append(units, called(id, fmt.Sprintf("a contract with a long name %d", i)))
		units[1].Relations = append(units[1].Relations, rel(id, model.Consumes))
	}
	o := planOverview(units, "", nil)
	if _, ok := o.inside("MOD-0001", units, "", nil, tactic{}, nil); !ok {
		t.Fatal("the plain tactic draws the view")
	}
	for _, by := range []tactic{{seated: true}, {laned: true}, {laned: true, follow: true}} {
		if _, ok := o.inside("MOD-0001", units, "", nil, by, nil); ok {
			t.Errorf("%+v draws a view whose bottom edge cannot hold its chips", by)
		}
	}
}
