package topology

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A contains hop is as certain as whoever placed the unit inside.
func TestAContainsHopCarriesWhoPlacedTheUnit(t *testing.T) {
	units := []model.Unit{
		{ID: "MOD-0001", Kind: model.Module, Name: model.Fact{Value: "inner", Origin: model.Extracted},
			Parent: "MOD-0002", ParentOrigin: model.Inferred},
		{ID: "MOD-0002", Kind: model.Module, Name: model.Fact{Value: "outer", Origin: model.Extracted}},
	}
	hops, ok := Path(units, "MOD-0002", "MOD-0001")
	if !ok {
		t.Fatal("no path")
	}
	if w := Weakest(hops); w != model.Inferred {
		t.Errorf("path through a reader's parent has weakest origin %s, want INFERRED (hops %+v)", w, hops)
	}
}
