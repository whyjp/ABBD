package screen

import (
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

func unit(id model.ID, rels ...model.Relation) model.Unit {
	k, _, _ := model.ParseID(string(id))
	return model.Unit{ID: id, Kind: k, Name: model.Fact{Value: string(id), Origin: model.Extracted}, Relations: rels}
}

func positions(m Map) map[string][2]int {
	out := map[string][2]int{}
	for _, n := range m.Nodes {
		out[n.ID] = [2]int{n.X, n.Y}
	}
	return out
}

// A node keeps its place whatever else is added or removed, so two
// screenshots of the map line up and the eye finds what changed.
func TestMapLayoutDoesNotMoveBetweenRuns(t *testing.T) {
	provides := model.Relation{To: "IF-0001", Kind: model.Provides, Origin: model.Extracted}
	before := []model.Unit{unit("MOD-0001", provides), unit("MOD-0002"), unit("IF-0001")}
	in := topology.Select(before, map[model.ID]bool{"MOD-0001": true, "IF-0001": true})
	hw := map[model.Kind]int{model.Module: 2, model.Interface: 1}
	first := Draw(before, &in, topology.Compare(in, before), hw)
	if again := Draw(before, &in, topology.Compare(in, before), hw); !reflect.DeepEqual(first, again) {
		t.Fatalf("drawing the same data twice should give the same map")
	}

	gone := unit("MOD-0002")
	gone.Absent = true
	after := []model.Unit{unit("MOD-0001", provides, model.Relation{To: "MOD-0003", Kind: model.DependsOn, Origin: model.Extracted}), gone, unit("IF-0001"), unit("MOD-0003")}
	hw[model.Module] = 3
	second := Draw(after, &in, topology.Compare(in, after), hw)
	p1, p2 := positions(first), positions(second)
	for _, id := range []string{"MOD-0001", "IF-0001"} {
		if p1[id] != p2[id] {
			t.Errorf("%s moved from %v to %v", id, p1[id], p2[id])
		}
	}
	if _, ok := p2["MOD-0002"]; ok {
		t.Errorf("a removed unit below the line should leave its slot empty")
	}
	if p2["MOD-0003"] == p1["MOD-0002"] {
		t.Errorf("a new unit must not take the slot of a removed one")
	}
}
