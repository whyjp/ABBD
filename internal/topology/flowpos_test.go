package topology

import (
	"reflect"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func via(r model.Relation, v model.Via) model.Relation { r.Via = v; return r }

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
		{Key: "other", Units: []model.ID{"MOD-0001", "MOD-0003"}},
	}
	for _, c := range []struct {
		id   model.ID
		want []FlowPosition
	}{
		{"MOD-0003", []FlowPosition{
			{Flow: "run", Of: 4, Steps: []FlowStep{{1, "MOD-0003"}, {3, "MOD-0003"}}, Upstream: []model.ID{"MOD-0005"}, Downstream: []model.ID{"MOD-0005", "MOD-0001"}},
			{Flow: "other", Of: 2, Steps: []FlowStep{{2, "MOD-0003"}}, Upstream: []model.ID{"MOD-0001"}},
		}},
		{"MOD-0004", []FlowPosition{{Flow: "run", Of: 4, Steps: []FlowStep{{2, "MOD-0005"}}, Upstream: []model.ID{"MOD-0003"}, Downstream: []model.ID{"MOD-0003"}}}},
		{"DATA-0001", []FlowPosition{{Upstream: []model.ID{"MOD-0001"}, Downstream: []model.ID{"MOD-0002"}}}},
		{"EXEC-0001", []FlowPosition{{Upstream: []model.ID{"MOD-0001"}}}},
		{"MOD-0002", []FlowPosition{{Upstream: []model.ID{"DATA-0001"}}}},
	} {
		if got := FlowPositions(units, flows, c.id); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.id, got, c.want)
		}
	}
	if got := FlowPositions(units, nil, "MOD-0003"); got != nil {
		t.Errorf("a call is no flow, so a unit in no flow and no data relation has no position: %+v", got)
	}
}
