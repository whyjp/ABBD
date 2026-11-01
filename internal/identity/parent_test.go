package identity

import (
	"testing"

	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
)

// The script reports MOD-0003 inside MOD-0002, while a reader earlier put
// MOD-0002 inside MOD-0003, a cycle, and MOD-0001 inside MOD-0004. Only the
// reader's parent on the cycle goes.
func TestBreakingAParentCycleKeepsParentsOffIt(t *testing.T) {
	ev := []model.Evidence{{Path: "README.md", Lines: "1", Quote: "x"}}
	mod := func(id model.ID, parent model.ID) model.Unit {
		u := model.Unit{ID: id, Kind: model.Module}
		if parent != "" {
			u.Parent, u.ParentOrigin, u.ParentEvidence = parent, model.Inferred, ev
		}
		return u
	}
	units := []model.Unit{
		mod("MOD-0001", "MOD-0004"),
		mod("MOD-0002", "MOD-0003"),
		mod("MOD-0003", ""),
		mod("MOD-0004", ""),
	}
	input := []extract.Unit{
		{Key: "a", Kind: model.Module},
		{Key: "b", Kind: model.Module},
		{Key: "c", Kind: model.Module, Parent: "b"},
		{Key: "d", Kind: model.Module},
	}
	ids := []model.ID{"MOD-0001", "MOD-0002", "MOD-0003", "MOD-0004"}
	setParents(units, input, ids)
	if c := model.ParentCycle(units); c != "" {
		t.Fatalf("cycle remains at %s", c)
	}
	if units[0].Parent != "MOD-0004" {
		t.Errorf("MOD-0001's reader parent (no cycle) was dropped: parent=%q", units[0].Parent)
	}
	if units[2].Parent != "MOD-0002" {
		t.Errorf("script parent of MOD-0003 lost: %q", units[2].Parent)
	}
}

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
