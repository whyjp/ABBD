package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

func step(unit model.ID, does string, calls ...model.ID) store.Part[model.Step] {
	return store.Part[model.Step]{Claim: model.Step{Unit: unit, Does: does, Calls: calls, Evidence: []model.Evidence{{Path: "x", Lines: "1", Quote: does}}}}
}

// A flow is drawn as numbered steps in their order. Consecutive steps on one
// unit share its cell, a click on a unit opens the inside that shows it, a
// call is drawn only through a stored relation, and each module's inside
// lists the steps on the units it holds.
func TestAFlowIsDrawnAsNumberedStepsLinkedIntoTheTree(t *testing.T) {
	units := append(ripgrep(), inside("MOD-0006", "MOD-0003", rel("IF-0002", model.Consumes)))
	stale := step("MOD-0006", "reads a sink")
	stale.Changed = []string{"src/a.rs (the lines around it changed)"}
	in := Input{Units: units, Figures: store.Figures{Flows: []store.Flow{{
		Key: "search", Known: true, Question: "How does a search run?", Caption: "From argv to output.",
		Parts: []store.Part[model.Step]{
			step("MOD-0001", "parses argv", "IF-0001", "MOD-0005"),
			step("MOD-0002", "builds the searcher"),
			step("MOD-0002", "hands each file to it"),
			stale,
		},
	}}}}
	m := Build(in)
	if len(m.Flows) != 1 {
		t.Fatalf("one stored flow, one view: %+v", m.Flows)
	}
	f := m.Flows[0]
	var lanes []string
	var numbers []int
	for _, l := range f.Lanes {
		lanes = append(lanes, l.Unit+" "+l.Href)
		for _, s := range l.Steps {
			numbers = append(numbers, s.N)
		}
	}
	if want := []string{"MOD-0001 #m-MOD-0001", "MOD-0002 #m-MOD-0002", "MOD-0006 #m-MOD-0003"}; !slices.Equal(lanes, want) {
		t.Errorf("lanes: want %v, got %v", want, lanes)
	}
	if !slices.Equal(numbers, []int{1, 2, 3, 4}) {
		t.Errorf("steps are numbered in their order: %v", numbers)
	}
	if calls := f.Lanes[0].Steps[0].Calls; len(calls) != 1 || calls[0].ID != "IF-0001" || calls[0].Verb != "사용" {
		t.Errorf("only the call a stored relation carries is drawn, named by its kind: %+v", calls)
	}
	if f.Stale != 2 || !f.Lanes[2].Steps[0].Stale || !strings.Contains(strings.Join(f.Lanes[0].Steps[0].Reasons, ""), "no live relation joins MOD-0001 to MOD-0005") {
		t.Errorf("the step whose code moved and the step whose call has no relation are STALE, and only they: %+v", f)
	}
	refs := map[string][]int{}
	for _, ins := range m.Insides {
		for _, r := range ins.Flows {
			refs[ins.ID] = r.Steps
		}
	}
	if !slices.Equal(refs["MOD-0002"], []int{2, 3}) || !slices.Equal(refs["MOD-0003"], []int{4}) || refs["MOD-0004"] != nil {
		t.Errorf("each inside lists the steps on the units it holds: %v", refs)
	}
	b := renderAll(t, m)
	page := b.String()
	for _, want := range []string{`<section class="inside flow" id="flow-search">`, `id="flow-search.4"`, "hands each file to it", "From argv to output.", `<a href="flow-search.html">How does a search run?</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page must hold %q", want)
		}
	}
}

// A step's anchor cannot be another flow's: flow a's step 1 and flow a-1
// each have their own.
// A figure has a page of its own, and the page shows it on opening, though
// the page CSS hides a view no link targets. The overview links the page.
func TestAFigureHasAPageThatShowsItOpen(t *testing.T) {
	two := []store.Part[model.Step]{step("MOD-0001", "parses argv"), step("MOD-0002", "builds the searcher")}
	m := Build(Input{Units: ripgrep(), Figures: store.Figures{Flows: []store.Flow{{Key: "a", Known: true, Question: "A?", Parts: two}}}})
	ps := pages(t, m)
	if !strings.Contains(ps["flow-a.html"], `<div class="solo"><section class="inside flow" id="flow-a">`) || !strings.Contains(pageCSS, ".solo > .inside") {
		t.Errorf("flow-a.html must show its flow open:\n%.600s", ps["flow-a.html"])
	}
	if strings.Contains(ps[PageFile], `id="flow-a"`) || !strings.Contains(ps[PageFile], `href="flow-a.html"`) {
		t.Error("the overview links the flow's page and holds no flow")
	}
}

func TestFlowAnchorsNeverCollide(t *testing.T) {
	two := []store.Part[model.Step]{step("MOD-0001", "parses argv"), step("MOD-0002", "builds the searcher")}
	m := Build(Input{Units: ripgrep(), Figures: store.Figures{Flows: []store.Flow{
		{Key: "a", Known: true, Question: "A?", Parts: two},
		{Key: "a-1", Known: true, Question: "A1?", Parts: two},
	}}})
	seen := map[string]bool{}
	for _, f := range m.Flows {
		ids := []string{f.ID}
		for _, l := range f.Lanes {
			for _, s := range l.Steps {
				ids = append(ids, s.ID)
			}
		}
		for _, id := range ids {
			if seen[id] {
				t.Errorf("two anchors are %q", id)
			}
			seen[id] = true
		}
	}
}
