package topology

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func unit(id model.ID, parent model.ID, rels ...model.Relation) model.Unit {
	k, _, _ := model.ParseID(string(id))
	return model.Unit{ID: id, Kind: k, Parent: parent, Name: model.Fact{Value: string(id), Origin: model.Extracted}, Relations: rels}
}

func rel(to model.ID, kind model.RelationKind) model.Relation {
	return model.Relation{To: to, Kind: kind, Origin: model.Extracted}
}

// A module's inside explains it to a human; only what crosses its boundary
// asks one.
func TestAModulesInsideNeverGates(t *testing.T) {
	before := []model.Unit{unit("MOD-0001", ""), unit("MOD-0002", ""), unit("IF-0001", "")}
	in := Select(before, map[model.ID]bool{"MOD-0001": true, "MOD-0002": true, "IF-0001": true})

	after := []model.Unit{
		unit("MOD-0001", "", rel("DATA-0001", model.Provides), rel("MOD-0003", model.DependsOn)),
		unit("MOD-0002", "", rel("DATA-0001", model.Consumes)),
		unit("IF-0001", ""),
		unit("DATA-0001", "MOD-0001"),
		unit("MOD-0003", "MOD-0001", rel("IF-0001", model.Consumes), rel("DATA-0001", model.Consumes)),
	}
	got := map[string]Effect{}
	for _, it := range Compare(in, after) {
		got[it.Key()] = it.Effect
	}
	for _, key := range []string{
		"unit-added:DATA-0001", "unit-added:MOD-0003",
		"new-relation:MOD-0001.provides.DATA-0001", "new-relation:MOD-0001.depends_on.MOD-0003",
		"new-relation:MOD-0003.consumes.IF-0001", "new-relation:MOD-0003.consumes.DATA-0001",
	} {
		if e, ok := got[key]; ok {
			t.Errorf("%s stays inside MOD-0001, but Compare reports it as %s", key, e)
		}
	}
	if got["new-relation:MOD-0002.consumes.DATA-0001"] != Blocked {
		t.Errorf("another module reaching into MOD-0001's inside crosses the boundary and must gate: %v", got)
	}
	for _, c := range Candidates(in, after, func(string) bool { return false }) {
		if c.Unit == "DATA-0001" || c.Unit == "MOD-0003" {
			t.Errorf("%s lives inside a module and is never offered as a design unit", c.Unit)
		}
	}
}
