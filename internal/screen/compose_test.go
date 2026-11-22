package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

func viaUse(to model.ID) model.Relation {
	return model.Relation{To: to, Kind: model.DependsOn, Origin: model.Extracted, Via: "call"}
}

// tangled is a module of six boxes: four that all use one another and a fifth
// and sixth that stand beside them, the fifth also reaching outside.
func tangled() []model.Unit {
	four := []model.ID{"MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104"}
	us := []model.Unit{
		called("MOD-0100", "engine"),
		called("MOD-0200", "client", viaUse("MOD-0105")),
		inside("MOD-0105", "MOD-0100", viaUse("MOD-0101"), viaUse("MOD-0200")),
		inside("MOD-0106", "MOD-0100", viaUse("MOD-0105")),
	}
	for _, a := range four {
		var rs []model.Relation
		for _, b := range four {
			if a != b {
				rs = append(rs, viaUse(b))
			}
		}
		u := inside(a, "MOD-0100", rs...)
		u.Anchors = []model.Anchor{{Path: "eng/" + string(a) + ".go"}}
		us = append(us, u)
	}
	return named(us)
}

func record(view, kind string, layers ...model.Layer) store.Stack {
	f := store.Stack{Key: "core", Known: true, Question: "q", View: view, Kind: kind}
	for _, l := range layers {
		f.Parts = append(f.Parts, store.Part[model.Layer]{Claim: l})
	}
	return f
}

func clique() model.Layer {
	return model.Layer{Name: "engine core", Role: "공통 엔진", As: model.AsClique, Units: []model.ID{"MOD-0101", "MOD-0102", "MOD-0103", "MOD-0104"}, Evidence: []model.Evidence{{Path: "eng/MOD-0101.go", Quote: "q"}}}
}

func boxIDs(d Drawing) []string {
	var out []string
	for _, b := range d.Boxes {
		out = append(out, b.ID)
	}
	return out
}

// A record of kind core whose clique the relations bear out folds its boxes
// into one box of the module's view, which says at rest what its inside
// holds and has no view of its own yet; a view with no record draws as before.
func TestACompositionRecordFoldsItsGroupIntoOneBox(t *testing.T) {
	in := Input{Units: tangled()}
	plain := Build(in)
	in.Figures.Stacks = []store.Stack{record("MOD-0100", model.KindCore, clique())}
	m := Build(in)
	byID := func(m Model, id string) (Inside, bool) {
		i := slices.IndexFunc(m.Insides, func(ins Inside) bool { return ins.ID == id })
		if i < 0 {
			return Inside{}, false
		}
		return m.Insides[i], true
	}
	engine, ok := byID(m, "MOD-0100")
	if !ok {
		t.Fatal("the module has no view")
	}
	got, want := boxIDs(engine.Drawing), []string{"MOD-0101~stack-core.1", "MOD-0105", "MOD-0106"}
	if !slices.Equal(slices.Sorted(slices.Values(got)), want) {
		t.Errorf("the module's view draws %v, want %v", got, want)
	}
	var folded Box
	for _, b := range engine.Drawing.Boxes {
		if b.ID == "MOD-0101~stack-core.1" {
			folded = b
		}
	}
	if folded.Href != "" || folded.Sub == nil || folded.Sub.S != "↻ 안에 순환 4상자" || !strings.Contains(folded.Title, "공통 엔진") {
		t.Errorf("the folded box says at rest what is inside, and opens nothing yet: %+v", folded)
	}
	if _, ok := byID(m, "MOD-0101~stack-core.1"); ok {
		t.Error("a folded group has no view of its own yet")
	}
	before, _ := byID(plain, "MOD-0100")
	if len(before.Drawing.Boxes) != 6 {
		t.Errorf("without a record the module draws its six boxes, drew %d", len(before.Drawing.Boxes))
	}
}

// A record the state does not bear out, or one that is not of kind core, or
// not of the view's own boxes, folds nothing.
func TestACompositionRecordTheStateDoesNotBearOutFoldsNothing(t *testing.T) {
	base := Build(Input{Units: tangled()})
	broken := clique()
	broken.Units = []model.ID{"MOD-0101", "MOD-0102", "MOD-0105", "MOD-0106"}
	for name, f := range map[string]store.Stack{
		"not core":      record("MOD-0100", model.KindLayers, clique()),
		"no view":       record("", model.KindCore, clique()),
		"not a clique":  record("MOD-0100", model.KindCore, broken),
		"another view":  record("MOD-0200", model.KindCore, clique()),
		"primary":       func() store.Stack { s := record("MOD-0100", model.KindCore, clique()); s.Primary = true; return s }(),
		"not known key": func() store.Stack { s := record("MOD-0100", model.KindCore, clique()); s.Known = false; return s }(),
	} {
		in := Input{Units: tangled()}
		in.Figures.Stacks = []store.Stack{f}
		m := Build(in)
		if len(m.Insides) != len(base.Insides) {
			t.Errorf("%s: %d views, want the plain %d", name, len(m.Insides), len(base.Insides))
		}
	}
}
