package screen

import (
	"fmt"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// primaryStack is a stack marked the primary flow, one stage to a layer.
func primaryStack(stages ...store.Part[model.Layer]) store.Figures {
	return store.Figures{Stacks: []store.Stack{{Key: "pipeline", Known: true, Primary: true, Question: "How does an event travel?", Parts: stages}}}
}

func deps(ids ...model.ID) []model.Relation {
	var out []model.Relation
	for _, id := range ids {
		out = append(out, rel(id, model.DependsOn))
	}
	return out
}

// pipeline is five stages that import each other every way, as vector's
// do, with an app on top that uses them and a library they all use.
func pipeline() []model.Unit {
	return []model.Unit{
		called("MOD-0001", "config", deps("MOD-0003", "MOD-0004", "MOD-0005", "MOD-0007")...),
		called("MOD-0002", "topology", deps("MOD-0001", "MOD-0004", "MOD-0007")...),
		called("MOD-0003", "sources", deps("MOD-0001", "MOD-0005", "MOD-0007")...),
		called("MOD-0004", "transforms", deps("MOD-0001", "MOD-0005", "MOD-0007")...),
		called("MOD-0005", "sinks", deps("MOD-0001", "MOD-0004", "MOD-0007")...),
		called("MOD-0006", "app", deps("MOD-0001", "MOD-0002")...),
		called("MOD-0007", "foundation"),
	}
}

func pipelineStages() store.Figures {
	return primaryStack(band("config", "MOD-0001"), band("topology", "MOD-0002"), band("sources", "MOD-0003"), band("transforms", "MOD-0004"), band("sinks", "MOD-0005"))
}

func flowMonotone(t *testing.T, m Model, view string) int {
	t.Helper()
	vs, err := Lint(m, view)
	if err != nil {
		t.Fatal(err)
	}
	return Sum(vs).FlowMonotone
}

// Five stages that import each other every way read left to right in their
// order, as one row that holds nothing else: the app that uses them stands
// above it and the library they use below.
func TestAPrimaryFlowReadsAcrossInStageOrder(t *testing.T) {
	m := Build(Input{Units: pipeline(), Figures: pipelineStages()})
	bs := boxes(m.Overview)
	stages := []string{"MOD-0001", "MOD-0002", "MOD-0003", "MOD-0004", "MOD-0005"}
	for i := 1; i < len(stages); i++ {
		a, b := bs[stages[i-1]], bs[stages[i]]
		if b.Y != a.Y || b.X < a.X+a.W {
			t.Errorf("stage %d %+v must stand right of stage %d %+v in its row", i+1, b, i, a)
		}
	}
	spine := bs["MOD-0001"]
	for _, b := range m.Overview.Boxes {
		if b.Y == spine.Y && !slices.Contains(stages, b.ID) {
			t.Errorf("%s stands in the spine's row", b.ID)
		}
	}
	if app, lib := bs["MOD-0006"], bs["MOD-0007"]; app.Y >= spine.Y || lib.Y <= spine.Y {
		t.Errorf("the app stands above the spine and the library below: app %+v, library %+v, spine %+v", app, lib, spine)
	}
	if n := flowMonotone(t, m, "overview"); n != 0 {
		t.Errorf("flow_monotone %d, want 0", n)
	}
}

// Stages too wide for one row fall back to one stage a row, top down.
func TestAPrimaryFlowTooWideForARowReadsDown(t *testing.T) {
	var units []model.Unit
	var parts []store.Part[model.Layer]
	id := func(i int) model.ID { return model.ID(fmt.Sprintf("MOD-%04d", i)) }
	for i := 1; i <= 8; i++ {
		var rels []model.Relation
		if i > 1 {
			rels = deps(id(i - 1))
		}
		units = append(units, called(id(i), fmt.Sprintf("a stage with a name long enough to fill its box %d", i), rels...))
		parts = append(parts, band(fmt.Sprint("stage ", i), id(i)))
	}
	m := Build(Input{Units: units, Figures: primaryStack(parts...)})
	bs := boxes(m.Overview)
	for i := 2; i <= 8; i++ {
		a, b := bs[string(id(i-1))], bs[string(id(i))]
		if b.Y < a.Y+a.H {
			t.Errorf("stage %d %+v must stand in a row below stage %d %+v", i, b, i-1, a)
		}
	}
	if m.Overview.axis.flow == nil || m.Overview.axis.flow.across {
		t.Errorf("the flow is laid down: %+v", m.Overview.axis.flow)
	}
	if n := flowMonotone(t, m, "overview"); n != 0 {
		t.Errorf("flow_monotone %d, want 0", n)
	}
}

// graph.json carries the primary flow and ranks each unit on it by its
// stage, and every other unit by its drawn row.
func TestGraphRanksTheUnitsOnThePrimaryFlowByStage(t *testing.T) {
	g := graphOf(t, Build(Input{Units: pipeline(), Figures: pipelineStages()}))
	if g.Primary == nil || g.Primary.Key != "pipeline" || len(g.Primary.Stages) != 5 || g.Primary.Stages[2].Name != "sources" || g.Primary.Stages[2].Units[0] != "MOD-0003" {
		t.Fatalf("graph.json carries the primary flow: %+v", g.Primary)
	}
	got := ranksOf(g)
	for id, st := range map[string]struct {
		n    int
		name string
	}{"MOD-0001": {1, "config"}, "MOD-0003": {3, "sources"}, "MOD-0005": {5, "sinks"}} {
		if r := got[id]; r.Basis != "flow" || r.N != st.n || r.Of != 5 || r.Stage != st.name {
			t.Errorf("%s ranks %+v, want stage %d of 5 (%s)", id, r, st.n, st.name)
		}
	}
	if r := got["MOD-0007"]; r.Basis != "dependency" || r.N != r.Of {
		t.Errorf("the library off the flow ranks by its row, the last: %+v", r)
	}
	if g := graphOf(t, build(pipeline())); g.Primary != nil || ranksOf(g)["MOD-0001"].Stage != "" {
		t.Errorf("with no stack marked, graph.json carries no flow")
	}
}

// A primary flow whose stages are units inside one module orders that
// module's inside, while the overview, which shows one stage, does not.
func TestAPrimaryFlowOrdersTheInsideItCrosses(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "core"),
		inside("MOD-0002", "MOD-0001"),
		inside("MOD-0003", "MOD-0001", rel("MOD-0002", model.DependsOn)),
		inside("MOD-0004", "MOD-0001", rel("MOD-0003", model.DependsOn)),
		called("MOD-0005", "cli", rel("MOD-0001", model.DependsOn)),
	}
	m := Build(Input{Units: units, Figures: primaryStack(band("bridge", "MOD-0002"), band("manager", "MOD-0003"), band("ipc", "MOD-0004"))})
	var ins Inside
	for _, i := range m.Insides {
		if i.ID == "MOD-0001" {
			ins = i
		}
	}
	bs := boxes(ins.Drawing)
	for _, p := range [][2]string{{"MOD-0002", "MOD-0003"}, {"MOD-0003", "MOD-0004"}} {
		a, b := bs[p[0]], bs[p[1]]
		if b.Y != a.Y || b.X < a.X+a.W {
			t.Errorf("%s %+v must stand right of %s %+v in its row", p[1], b, p[0], a)
		}
	}
	if n := flowMonotone(t, m, "MOD-0001"); n != 0 {
		t.Errorf("flow_monotone %d on the inside, want 0", n)
	}
	if m.Overview.axis.flow != nil {
		t.Errorf("an overview that shows one stage lays out no flow")
	}
}

// O4 holds when each stage stands after the one before along the view's
// axis, and counts each pair of consecutive stages that does not.
func TestO4CountsStagesOutOfOrder(t *testing.T) {
	staged := func(across bool, boxes []string, stage map[string]int) *look {
		l := lookAt(t, boxes, ``, nil)
		l.across = across
		for id, n := range stage {
			l.stage[l.at[id]] = n
		}
		return l
	}
	row := []string{"A 0 0 60 20", "B 100 0 60 20", "C 200 0 60 20"}
	if got := staged(true, row, map[string]int{"A": 1, "B": 2, "C": 3}).stagesInOrder(); len(got) != 0 {
		t.Errorf("%+v; a row in stage order holds", got)
	}
	if got := staged(true, row, map[string]int{"A": 1, "B": 3, "C": 2}).stagesInOrder(); len(got) != 1 || got[0].Items[len(got[0].Items)-1].ID != "B" {
		t.Errorf("%+v; want stage 3 (B) reported standing left of stage 2", got)
	}
	column := []string{"A 0 0 60 20", "B 0 80 60 20", "C 0 160 60 20"}
	if got := staged(false, column, map[string]int{"A": 1, "B": 2, "C": 3}).stagesInOrder(); len(got) != 0 {
		t.Errorf("%+v; rows top down in stage order hold", got)
	}
	if got := staged(false, column, map[string]int{"A": 2, "B": 1, "C": 3}).stagesInOrder(); len(got) != 1 {
		t.Errorf("%+v; want stage 2 above stage 1 reported", got)
	}
	if got := staged(true, row, nil).stagesInOrder(); len(got) != 0 {
		t.Errorf("%+v; a view with no flow has no O4 breach", got)
	}
}
