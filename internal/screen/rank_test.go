package screen

import (
	"encoding/json"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// graphOf is graph.json as a tool reads it.
func graphOf(t *testing.T, m Model) Graph {
	t.Helper()
	files, err := Files(m)
	if err != nil {
		t.Fatal(err)
	}
	var g Graph
	if err := json.Unmarshal(files[GraphFile], &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func ranksOf(g Graph) map[string]Rank {
	out := map[string]Rank{}
	for _, u := range g.Units {
		if u.Rank != nil {
			out[u.ID] = *u.Rank
		}
	}
	return out
}

func TestGraphRanksAChainTopToBottom(t *testing.T) {
	g := graphOf(t, build([]model.Unit{
		called("MOD-0001", "cli", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "engine", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "store"),
	}))
	got := ranksOf(g)
	for id, n := range map[string]int{"MOD-0001": 1, "MOD-0002": 2, "MOD-0003": 3} {
		if want := (Rank{Basis: "dependency", N: n, Of: 3}); got[id] != want {
			t.Errorf("%s ranks %+v, want %+v", id, got[id], want)
		}
	}
}

func TestGraphMarksBoxesInADependencyCycle(t *testing.T) {
	g := graphOf(t, build([]model.Unit{
		called("MOD-0001", "cli", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "engine", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "store"),
		inside("MOD-0004", "MOD-0003", rel("MOD-0002", model.DependsOn)),
		inside("MOD-0005", "MOD-0003", rel("MOD-0004", model.DependsOn)),
		called("MOD-0006", "docs"),
	}))
	got := ranksOf(g)
	for id, cyclic := range map[string]bool{"MOD-0001": false, "MOD-0002": true, "MOD-0003": true, "MOD-0006": false} {
		r, ok := got[id]
		if !ok || r.Basis != "dependency" || r.N < 1 || r.N > r.Of {
			t.Fatalf("%s is drawn on the overview and needs a rank there: %+v", id, r)
		}
		if r.Cycle != cyclic {
			t.Errorf("%s ranks %+v: the engine and the store use each other through the store's unit, so their rows are the layout's choice", id, r)
		}
	}
	for _, id := range []string{"MOD-0004", "MOD-0005"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s is not drawn on the overview, so it has no rank there", id)
		}
	}
	for _, ins := range g.Modules {
		if ins.ID != "MOD-0003" {
			continue
		}
		in := map[string]*Rank{}
		for _, u := range ins.Units {
			in[u.ID] = u.Rank
		}
		want := map[string]Rank{"MOD-0005": {Basis: "dependency", N: 1, Of: 2}, "MOD-0004": {Basis: "dependency", N: 2, Of: 2}}
		for id, w := range want {
			if in[id] == nil || *in[id] != w {
				t.Errorf("%s on the store's inside ranks %+v, want %+v", id, in[id], w)
			}
		}
		return
	}
	t.Fatal("the store's inside is missing from graph.json")
}

func againstOn(t *testing.T, m Model, view string) Totals {
	t.Helper()
	vs, err := Lint(m, view)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.View == view {
			return v.Totals
		}
	}
	t.Fatalf("no view %q", view)
	return Totals{}
}

func TestAgainstAxisCountsALineThatClimbs(t *testing.T) {
	m := build([]model.Unit{
		called("MOD-0001", "a", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "b", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "c", rel("MOD-0001", model.DependsOn)),
	})
	got := againstOn(t, m, "overview")
	if got.AgainstAxis != 1 || got.AgainstAxisInk <= 0 {
		t.Errorf("a cycle of three draws one line against the axis: %d lines, %dpx", got.AgainstAxis, got.AgainstAxisInk)
	}
}

func TestAgainstAxisLeavesOutMutualLines(t *testing.T) {
	m := build([]model.Unit{
		called("MOD-0001", "a", rel("MOD-0002", model.DependsOn)),
		called("MOD-0002", "b", rel("MOD-0001", model.DependsOn)),
	})
	if got := againstOn(t, m, "overview"); got.AgainstAxis != 0 || got.AgainstAxisInk != 0 {
		t.Errorf("two modules that use each other run both ways, so neither against: %d lines, %dpx", got.AgainstAxis, got.AgainstAxisInk)
	}
}

func TestAgainstAxisReadsAProvidesLineTheWayItsContractIsUsed(t *testing.T) {
	m := build(append(ripgrep(),
		inside("MOD-0006", "MOD-0003", rel("IF-0001", model.Consumes), rel("MOD-0007", model.DependsOn)),
		inside("MOD-0007", "MOD-0003", rel("IF-0002", model.Provides)),
	))
	climbs := false
	for _, ins := range m.Insides {
		if ins.ID != "MOD-0003" {
			continue
		}
		for _, r := range ins.Drawing.Routes {
			if rs := runs(r.D); r.Kind == "provides" && len(rs) > 0 && rs[len(rs)-1][1].y < rs[0][0].y {
				climbs = true
			}
		}
	}
	if !climbs {
		t.Fatal("the searcher's inside must draw its provides line climbing to the Sink chip on the frame top")
	}
	if got := againstOn(t, m, "MOD-0003 grep-searcher"); got.AgainstAxis != 0 || got.AgainstAxisInk != 0 {
		t.Errorf("a provides line read from its contract down to its provider runs with the axis: %d lines, %dpx", got.AgainstAxis, got.AgainstAxisInk)
	}
}
