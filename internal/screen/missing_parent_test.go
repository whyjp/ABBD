package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A present unit whose parent is absent still appears, at the top level,
// with its relations.
func TestAUnitWhoseParentIsAbsentSitsAtTheTopLevel(t *testing.T) {
	units := []model.Unit{
		{ID: "MOD-0001", Kind: model.Module, Name: model.Fact{Value: "a", Origin: model.Extracted}, Parent: "MOD-0002",
			Relations: []model.Relation{{To: "MOD-0003", Kind: model.DependsOn, Origin: model.Extracted}}},
		{ID: "MOD-0002", Kind: model.Module, Name: model.Fact{Value: "gone", Origin: model.Extracted}, Absent: true},
		{ID: "MOD-0003", Kind: model.Module, Name: model.Fact{Value: "c", Origin: model.Extracted}},
	}
	tr := newTree(units)
	placed := false
	for _, kids := range tr.children {
		for _, k := range kids {
			placed = placed || k == "MOD-0001"
		}
	}
	_, rels, _, _ := top(tr)
	if !placed || len(rels) == 0 {
		t.Errorf("MOD-0001 placed in a view=%v; overview relations=%v (want MOD-0001 -> MOD-0003 drawn)", placed, rels)
	}
}
