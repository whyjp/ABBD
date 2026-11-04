package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

func band(name string, ids ...model.ID) store.Part[model.Layer] {
	return store.Part[model.Layer]{Claim: model.Layer{Name: name, Role: "holds " + name, Units: ids, Evidence: []model.Evidence{{Path: "x", Lines: "1", Quote: name}}}}
}

// A stack is drawn as bands from the top down, each with the units it holds
// linked into the tree. A relation by which a lower unit uses a higher one is
// drawn in the band it leaves, a lower unit providing a higher layer's
// contract is not, and each module's inside lists the layers that hold its
// units.
func TestAStackIsDrawnAsBandsLinkedIntoTheTree(t *testing.T) {
	units := append(ripgrep(), inside("MOD-0006", "MOD-0004", rel("MOD-0002", model.DependsOn), rel("IF-0003", model.Provides)), inside("IF-0003", "MOD-0003"))
	top := band("binary", "MOD-0001")
	top.Changed = []string{"src/main.rs (the lines around it changed)"}
	m := Build(Input{Units: units, Figures: store.Figures{Stacks: []store.Stack{{
		Key: "layers", Known: true, Question: "What are the layers of ripgrep?", Caption: "Each crate uses the ones below.",
		Parts: []store.Part[model.Layer]{top, band("facade", "MOD-0002"), band("engines", "MOD-0003", "MOD-0005"), band("matcher", "MOD-0004", "IF-0001")},
	}}}})
	if len(m.Stacks) != 1 {
		t.Fatalf("one stored stack, one view: %+v", m.Stacks)
	}
	s := m.Stacks[0]
	var names []string
	for _, b := range s.Bands {
		names = append(names, b.Name)
	}
	if !slices.Equal(names, []string{"binary", "facade", "engines", "matcher"}) || s.Layers != 4 {
		t.Errorf("the bands keep their order from the top: %v", names)
	}
	var chips []string
	for _, c := range s.Bands[2].Units {
		chips = append(chips, c.ID+" "+c.Href)
	}
	if want := []string{"MOD-0003 #m-MOD-0003", "MOD-0005 #m-MOD-0005"}; !slices.Equal(chips, want) {
		t.Errorf("a band's units open the inside that shows them: want %v, got %v", want, chips)
	}
	if up := s.Bands[3].Up; s.Breaches != 1 || len(up) != 1 || up[0].From != "MOD-0006" || up[0].To != "MOD-0002" || up[0].Upper != 2 || up[0].UpperName != "facade" {
		t.Errorf("the one relation that runs up the stack is drawn in the band it leaves: %+v", s.Bands[3].Up)
	}
	if s.Stale != 1 || !s.Bands[0].Stale || s.Bands[1].Stale {
		t.Errorf("only the band whose code moved is STALE: %+v", s)
	}
	refs := map[string][]int{}
	for _, ins := range m.Insides {
		for _, r := range ins.Stacks {
			refs[ins.ID] = r.Layers
		}
	}
	if !slices.Equal(refs["MOD-0004"], []int{4}) || !slices.Equal(refs["MOD-0002"], []int{2}) {
		t.Errorf("each inside lists the layers that hold its units: %v", refs)
	}
	page := renderAll(t, m).String()
	for _, want := range []string{`<section class="inside stack" id="stack-layers">`, `id="stack-layers.4"`, "Each crate uses the ones below.", `<a href="stack-layers.html">What are the layers of ripgrep?</a>`, "↑ ", "거꾸로 1"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page must hold %q", want)
		}
	}
	files, err := Files(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files[GraphFile]), `"stacks"`) {
		t.Error("graph.json holds the stacks as the page draws them")
	}
}
