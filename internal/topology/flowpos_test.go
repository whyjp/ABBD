package topology

import (
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func via(r model.Relation, v model.Via) model.Relation { r.Via = v; return r }

func side(r Role, xs ...model.ID) Side { return Side{Role: r, Units: xs} }

func TestFlowPositions(t *testing.T) {
	units := []model.Unit{
		unit("MOD-0001", "", via(rel("DATA-0001", model.Provides, model.Extracted), "data"), rel("EXEC-0001", model.PostsTo, model.Extracted)),
		unit("MOD-0002", "", via(rel("DATA-0001", model.Consumes, model.Extracted), "data"), via(rel("MOD-0003", model.DependsOn, model.Extracted), "call")),
		unit("MOD-0003", ""),
		unit("MOD-0004", ""),
		unit("MOD-0005", "MOD-0004"),
		unit("DATA-0001", ""),
		unit("EXEC-0001", ""),
	}
	flows := []FlowSteps{
		{Key: "run", Units: []model.ID{"MOD-0003", "MOD-0005", "MOD-0003", "MOD-0001"}},
		{Key: "other", Units: []model.ID{"MOD-0001", "MOD-0003"}, Lead: true},
	}
	for _, c := range []struct {
		id   model.ID
		want []FlowPosition
	}{
		{"MOD-0003", []FlowPosition{
			{Flow: "other", Lead: true, Of: 2, Steps: []FlowStep{{2, "MOD-0003"}}, Sides: []Side{side(Before, "MOD-0001")}},
			{Flow: "run", Of: 4, Steps: []FlowStep{{1, "MOD-0003"}, {3, "MOD-0003"}}, Sides: []Side{side(Before, "MOD-0005"), side(After, "MOD-0005", "MOD-0001")}},
		}},
		{"MOD-0004", []FlowPosition{{Flow: "run", Of: 4, Steps: []FlowStep{{2, "MOD-0005"}}, Sides: []Side{side(Before, "MOD-0003"), side(After, "MOD-0003")}}}},
		{"DATA-0001", []FlowPosition{{Sides: []Side{side(ReadBy, "MOD-0002"), side(ProvidedBy, "MOD-0001")}}}},
		{"EXEC-0001", []FlowPosition{{Sides: []Side{side(WorkFrom, "MOD-0001")}}}},
		{"MOD-0002", []FlowPosition{{Sides: []Side{side(Reads, "DATA-0001")}}}},
	} {
		if got := FlowPositions(units, flows, c.id); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.id, got, c.want)
		}
	}
	if got := FlowPositions(units, nil, "MOD-0003"); got != nil {
		t.Errorf("a call is no flow, so a unit in no flow and no data relation has no position: %+v", got)
	}
}

// A contract its reader provides, as a decoder defines the JSON it decodes:
// providing says who stands behind the contract, not that it comes first.
// A reader inside another listed reader is left to that one, and a module
// is never said to be provided by what provides a contract inside it.
func TestAProviderIsNamedByItsRoleAndNeverUpstream(t *testing.T) {
	units := []model.Unit{
		unit("MOD-0001", "", via(rel("DATA-0001", model.Consumes, model.Extracted), "data"), via(rel("DATA-0002", model.Consumes, model.Extracted), "data")),
		unit("MOD-0004", "MOD-0001", via(rel("DATA-0001", model.Consumes, model.Extracted), "data")),
		unit("MOD-0017", "", via(rel("DATA-0001", model.Provides, model.Extracted), "data"), via(rel("DATA-0001", model.Consumes, model.Extracted), "data")),
		unit("MOD-0013", "", via(rel("DATA-0002", model.Provides, model.Extracted), "data")),
		unit("MOD-0030", ""),
		unit("DATA-0001", ""),
		unit("DATA-0002", "MOD-0030"),
		unit("DATA-0003", ""),
		unit("MOD-0050", "", via(rel("DATA-0003", model.Provides, model.Extracted), "data")),
	}
	for _, c := range []struct {
		id   model.ID
		want []FlowPosition
	}{
		{"DATA-0001", []FlowPosition{{Sides: []Side{side(ReadBy, "MOD-0001", "MOD-0017"), side(ProvidedBy, "MOD-0017")}}}},
		{"DATA-0002", []FlowPosition{{Sides: []Side{side(ReadBy, "MOD-0001"), side(ProvidedBy, "MOD-0013")}}}},
		{"MOD-0030", []FlowPosition{{Sides: []Side{side(ReadBy, "MOD-0001")}}}},
		{"MOD-0017", []FlowPosition{{Sides: []Side{side(Reads, "DATA-0001")}}}},
		{"MOD-0013", nil},
		{"DATA-0003", nil},
	} {
		if got := FlowPositions(units, nil, c.id); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.id, got, c.want)
		}
	}
}
