package identity

import (
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// A reader's parent goes with the module it names, or its unit would sit
// inside nothing.
func TestAKeptParentGoesWithItsModule(t *testing.T) {
	ev := []model.Evidence{{Path: "README.md", Lines: "1", Quote: "x"}}
	units := []model.Unit{
		{ID: "MOD-0001", Kind: model.Module, Parent: "MOD-0002", ParentOrigin: model.Inferred, ParentEvidence: ev},
		{ID: "MOD-0002", Kind: model.Module, Absent: true},
	}
	setParents(units, []extract.Unit{{Key: "a", Kind: model.Module}}, []model.ID{"MOD-0001"})
	if units[0].Parent != "" {
		t.Errorf("MOD-0001 still placed in absent MOD-0002 (origin %s)", units[0].ParentOrigin)
	}
}
