package screen

import (
	"fmt"
	"slices"
	"strings"
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

// P1 and O3 leave to O4 only a pair whose boxes both stand by their stage:
// a line between a staged box and one the lines placed is still judged by
// the rows.
func TestAPairWithOneStagedBoxIsStillJudgedByItsRows(t *testing.T) {
	climb := func(stage map[string]int) *look {
		l := lookAt(t, []string{"A 40 0 60 20", "B 40 80 60 20"}, ``, [][2]string{{"B", "A"}})
		for id, n := range stage {
			l.stage[l.at[id]] = n
		}
		return l
	}
	for _, c := range []struct {
		stage map[string]int
		want  int
	}{{nil, 1}, {map[string]int{"A": 1}, 1}, {map[string]int{"B": 2}, 1}, {map[string]int{"A": 2, "B": 1}, 0}} {
		l := climb(c.stage)
		if got := len(l.usersAbove()); got != c.want {
			t.Errorf("stages %v: %d P1 breaches, want %d", c.stage, got, c.want)
		}
		if got := len(l.climbsCloseCycles()); got != c.want {
			t.Errorf("stages %v: %d O3 breaches, want %d", c.stage, got, c.want)
		}
	}
}

// A stage whose unit is drawn inside another box ranks that unit by its
// stage on the inside that draws it, as the box that holds it ranks by the
// stage on the view that lays the flow out.
func TestAUnitAStageNamesRanksByItInsideItsHolder(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "core"),
		inside("MOD-0002", "MOD-0001"),
		inside("MOD-0003", "MOD-0001", rel("MOD-0002", model.DependsOn)),
		inside("MOD-0004", "MOD-0001", rel("MOD-0003", model.DependsOn)),
		inside("MOD-0006", "MOD-0004"),
		inside("MOD-0007", "MOD-0004", rel("MOD-0006", model.DependsOn)),
		called("MOD-0005", "cli", rel("MOD-0001", model.DependsOn)),
	}
	g := graphOf(t, Build(Input{Units: units, Figures: primaryStack(band("bridge", "MOD-0002"), band("manager", "MOD-0003"), band("ipc", "MOD-0006"))}))
	on := map[string]map[string]Rank{}
	for _, ins := range g.Modules {
		on[ins.ID] = map[string]Rank{}
		for _, u := range ins.Units {
			if u.Rank != nil {
				on[ins.ID][u.ID] = *u.Rank
			}
		}
	}
	if r := on["MOD-0001"]["MOD-0004"]; r.Basis != "flow" || r.N != 3 || r.Stage != "ipc" {
		t.Errorf("the box that holds the ipc stage ranks by it on the inside that lays the flow out: %+v", r)
	}
	if r := on["MOD-0004"]["MOD-0006"]; r.Basis != "flow" || r.N != 3 || r.Of != 3 || r.Stage != "ipc" {
		t.Errorf("the unit the ipc stage names ranks by it on its holder's inside: %+v", r)
	}
	if r := on["MOD-0004"]["MOD-0007"]; r.Basis == "flow" {
		t.Errorf("a unit no stage names keeps its row rank: %+v", r)
	}
}

// On a view that lays the primary flow out, the lead starts at the first
// stage, and every numbered hop has one head and runs from an earlier stage
// to a later one; the primary stack counts no breach though every stage
// uses the config.
func TestTheNumberedLeadReadsInStageOrder(t *testing.T) {
	m := Build(Input{Units: pipeline(), Figures: pipelineStages()})
	d := m.Overview
	if d.Lead == nil || d.Lead.Key != "pipeline" || len(d.Lead.Hops) < leadMin {
		t.Fatalf("the overview numbers the primary flow: %+v", d.Lead)
	}
	if d.Lead.Entry != "MOD-0001" {
		t.Errorf("the lead starts at the first stage, config, not %s", d.Lead.Entry)
	}
	stage := d.axis.flow.stage
	for _, h := range d.Lead.Hops {
		r := d.Routes[h.way]
		if r.Both || len(r.ends) != 2 || stage[r.ends[0]] >= stage[r.ends[1]] || r.src != r.ends[0] {
			t.Errorf("hop %d runs %v, both %v: want one head from an earlier stage to a later one", h.N, r.ends, r.Both)
		}
	}
	for _, s := range m.Stacks {
		if s.Breaches != 0 || !s.Primary {
			t.Errorf("the primary stack counts %d breaches, want none", s.Breaches)
		}
	}
}

// Down a flow too wide for one row, each stage's band keeps its stage's
// number, as graph.json and the stack's page give it, though a stage
// between draws no band.
func TestAStageBandKeepsItsStageNumber(t *testing.T) {
	id := func(i int) model.ID { return model.ID(fmt.Sprintf("MOD-%04d", i)) }
	var units []model.Unit
	for i := 1; i <= 8; i++ {
		var rels []model.Relation
		if i > 1 {
			rels = deps(id(i - 1))
		}
		units = append(units, called(id(i), fmt.Sprintf("a stage box with a name long enough to fill it %d", i), rels...))
	}
	m := Build(Input{Units: units, Figures: primaryStack(band("read", id(1), id(2), id(3)), band("parse", id(4)), band("write", id(5), id(6), id(7), id(8)))})
	if f := m.Overview.axis.flow; f == nil || f.across {
		t.Fatalf("the flow is laid down: %+v", f)
	}
	var got []string
	for _, z := range m.Overview.Zones {
		got = append(got, z.Label())
	}
	if !slices.Contains(got, "1 · read") || !slices.Contains(got, "3 · write") {
		t.Errorf("bands %q, want 1 · read and 3 · write", got)
	}
}

// The key under a drawing whose lead follows the primary stack names the
// stack, never a stored flow.
func TestTheKeyNamesThePrimaryStack(t *testing.T) {
	files, err := Files(Build(Input{Units: pipeline(), Figures: pipelineStages()}))
	if err != nil {
		t.Fatal(err)
	}
	page := string(files["index.html"])
	if !strings.Contains(page, "주 흐름: stack pipeline의 단계 순서") || strings.Contains(page, "저장된 흐름 pipeline") {
		t.Errorf("the overview's key must name stack pipeline as the primary flow")
	}
}

// climbs counts the O3 breaches of a view: lines that climb to a box that
// does not reach them back.
func climbs(t *testing.T, m Model, view string) int {
	t.Helper()
	vs, err := Lint(m, view)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, v := range vs {
		for _, b := range v.Breaches {
			if b.Criterion == "O3" {
				n++
			}
		}
	}
	return n
}

// Down the stages, boxes on a chain from one stage to the next stand
// between the two stages' rows, so no line climbs.
func TestABoxStandsBetweenTheStagesItJoins(t *testing.T) {
	id := func(i int) model.ID { return model.ID(fmt.Sprintf("MOD-%04d", i)) }
	var units []model.Unit
	var parts []store.Part[model.Layer]
	for i := 1; i <= 6; i++ {
		var rels []model.Relation
		if i < 6 {
			rels = deps(id(i + 1))
		}
		if i == 1 {
			rels = append(rels, deps("MOD-0011")...)
		}
		units = append(units, called(id(i), fmt.Sprintf("a stage with a name long enough to fill its box %d", i), rels...))
		parts = append(parts, band(fmt.Sprint("stage ", i), id(i)))
	}
	units = append(units, called("MOD-0011", "zbuff", deps("MOD-0012")...), called("MOD-0012", "decompressor", deps(id(2))...))
	m := Build(Input{Units: units, Figures: primaryStack(parts...)})
	if f := m.Overview.axis.flow; f == nil || f.across {
		t.Fatalf("the flow is laid down: %+v", f)
	}
	bs := boxes(m.Overview)
	if d, s := bs["MOD-0012"], bs[string(id(2))]; d.Y+d.H > s.Y {
		t.Errorf("the decompressor %+v stands above stage 2 %+v, which it uses", d, s)
	}
	if n := climbs(t, m, "overview"); n != 0 {
		t.Errorf("%d lines climb, want none", n)
	}
}

// Across, a box the first stage uses that uses the last stage stands in
// the spine row, before that stage, so no line climbs to the spine.
func TestABoxBetweenStagesStandsOnTheSpine(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "frontend", deps("MOD-0004")...),
		called("MOD-0002", "plugins", deps("MOD-0003")...),
		called("MOD-0003", "libobs"),
		called("MOD-0004", "shared", deps("MOD-0003")...),
	}
	m := Build(Input{Units: units, Figures: primaryStack(band("app", "MOD-0001"), band("sources", "MOD-0002"), band("render", "MOD-0003"))})
	if f := m.Overview.axis.flow; f == nil || !f.across {
		t.Fatalf("the flow is laid across: %+v", f)
	}
	bs := boxes(m.Overview)
	if s, l := bs["MOD-0004"], bs["MOD-0003"]; s.Y != l.Y || s.X > l.X {
		t.Errorf("shared %+v stands in the spine row before libobs %+v", s, l)
	}
	if n := climbs(t, m, "overview"); n != 0 {
		t.Errorf("%d lines climb, want none", n)
	}
}

// A line between two stages that do not stand side by side runs over the
// spine, clear of the lines the stages send down to the boxes below.
func TestALineBetweenStagesApartRunsOverTheSpine(t *testing.T) {
	m := Build(Input{Units: pipeline(), Figures: pipelineStages()})
	d := m.Overview
	spine := boxes(d)["MOD-0001"]
	n := 0
	for _, r := range d.Routes {
		if !slices.Equal(r.ends, []string{"MOD-0001", "MOD-0003"}) && !slices.Equal(r.ends, []string{"MOD-0003", "MOD-0001"}) {
			continue
		}
		n++
		for _, run := range r.runs {
			if run[0].y > spine.Y || run[1].y > spine.Y {
				t.Errorf("the line between config and sources runs %v, below the spine's top %d", run, spine.Y)
			}
		}
	}
	if n == 0 {
		t.Fatal("no line between config and sources")
	}
}

// The boxes under a spine stand in the spine's order, not in the order the
// rows had before the stages set it: each stage's own library stands under
// it, so the lines down run side by side.
func TestTheRowsUnderASpineFollowItsOrder(t *testing.T) {
	var units []model.Unit
	var parts []store.Part[model.Layer]
	var libs []string
	for i := 1; i <= 4; i++ {
		stage, lib := model.ID(fmt.Sprintf("MOD-%04d", 5-i)), model.ID(fmt.Sprintf("MOD-%04d", 10+i))
		units = append(units, called(stage, fmt.Sprint("stage ", i), deps(lib)...), called(lib, fmt.Sprint("library ", i)))
		parts = append(parts, band(fmt.Sprint("stage ", i), stage))
		libs = append(libs, string(lib))
	}
	bs := boxes(Build(Input{Units: units, Figures: primaryStack(parts...)}).Overview)
	for i := 1; i < len(libs); i++ {
		if a, b := bs[libs[i-1]], bs[libs[i]]; b.X < a.X {
			t.Errorf("library %d %+v stands left of library %d %+v, against the stages that use them", i+1, b, i, a)
		}
	}
}

// On an inside, a unit that joins no stage, only the world outside the
// module, stands off the spine's row, as the overview keeps it.
func TestAnInsideKeepsItsSpineToTheStages(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "core"),
		inside("MOD-0002", "MOD-0001"),
		inside("MOD-0003", "MOD-0001", rel("MOD-0002", model.DependsOn)),
		inside("MOD-0004", "MOD-0001", rel("MOD-0003", model.DependsOn), rel("MOD-0009", model.DependsOn)),
		inside("MOD-0006", "MOD-0001", rel("MOD-0009", model.DependsOn)),
		called("MOD-0005", "cli", rel("MOD-0002", model.DependsOn)),
		called("MOD-0009", "lib"),
	}
	m := Build(Input{Units: units, Figures: primaryStack(band("bridge", "MOD-0002"), band("manager", "MOD-0003"), band("ipc", "MOD-0004"))})
	for _, ins := range m.Insides {
		if ins.ID != "MOD-0001" {
			continue
		}
		bs := boxes(ins.Drawing)
		if o, s := bs["MOD-0006"], bs["MOD-0002"]; o.Y == s.Y {
			t.Errorf("a unit tied only to the outside %+v stands in the spine's row %+v", o, s)
		}
		return
	}
	t.Fatal("no inside of MOD-0001")
}

// A later stage that uses an earlier one runs back by the order the stages
// set, which O4 judges: rule 13 leaves such lines out of its count.
func TestLinesBackToAnEarlierStageAreNotCountedAgainstTheAxis(t *testing.T) {
	units := []model.Unit{called("MOD-0001", "config")}
	var parts []store.Part[model.Layer]
	parts = append(parts, band("config", "MOD-0001"))
	for i := 2; i <= 5; i++ {
		id := model.ID(fmt.Sprintf("MOD-%04d", i))
		rels := deps("MOD-0001")
		if i < 5 {
			rels = append(rels, deps(model.ID(fmt.Sprintf("MOD-%04d", i+1)))...)
		}
		units = append(units, called(id, fmt.Sprint("stage ", i), rels...))
		parts = append(parts, band(fmt.Sprint("stage ", i), id))
	}
	vs, err := Lint(Build(Input{Units: units, Figures: primaryStack(parts...)}), "overview")
	if err != nil {
		t.Fatal(err)
	}
	if n := Sum(vs).Back; n != 0 {
		t.Errorf("%d lines counted back, want none", n)
	}
}
