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

func insideByID(m Model, id string) (Inside, bool) {
	i := slices.IndexFunc(m.Insides, func(ins Inside) bool { return ins.ID == id })
	if i < 0 {
		return Inside{}, false
	}
	return m.Insides[i], true
}

// A box no record may fold twice, a record of the overview, and one citing
// code its units do not hold fold nothing; a boundary group says how many of
// its boxes reach outside.
func TestAFoldTakesEachBoxOnceAndOnlyFromARecordThatHolds(t *testing.T) {
	boundary := model.Layer{Name: "edge", Role: "바깥과 닿는 상자", As: model.AsBoundary, Units: []model.ID{"MOD-0105"}, Evidence: []model.Evidence{{Path: "eng/MOD-0105.go", Quote: "q"}}}
	units := tangled()
	for i := range units {
		if units[i].ID == "MOD-0105" {
			units[i].Anchors = []model.Anchor{{Path: "eng/MOD-0105.go"}}
		}
	}
	second := record("MOD-0100", model.KindCore, clique())
	second.Key = "again"
	in := Input{Units: units}
	in.Figures.Stacks = []store.Stack{record("MOD-0100", model.KindCore, clique(), boundary), second}
	m := Build(in)
	engine, _ := insideByID(m, "MOD-0100")
	got := boxIDs(engine.Drawing)
	slices.Sort(got)
	if want := []string{"MOD-0101~stack-core.1", "MOD-0105~stack-core.2", "MOD-0106"}; !slices.Equal(got, want) {
		t.Errorf("the module draws %v, want %v (the second record's clique shares boxes and folds nothing)", got, want)
	}
	for _, b := range engine.Drawing.Boxes {
		if b.ID == "MOD-0105~stack-core.2" && (b.Sub == nil || b.Sub.S != "바깥과 이어진 1상자") {
			t.Errorf("a boundary group says how many boxes reach outside: %+v", b.Sub)
		}
	}
	for _, ins := range m.Insides {
		for _, b := range ins.Drawing.Boxes {
			if id, ok := strings.CutPrefix(b.Href, "#m-"); ok {
				if _, there := insideByID(m, id); !there {
					t.Errorf("%s: box %s links to %s, which has no view", ins.ID, b.ID, b.Href)
				}
			}
		}
	}
	stale := clique()
	stale.Evidence = []model.Evidence{{Path: "elsewhere.go", Quote: "q"}}
	for name, f := range map[string]store.Stack{
		"overview":    record(model.OverviewView, model.KindCore, clique()),
		"stale layer": record("MOD-0100", model.KindCore, stale),
	} {
		in := Input{Units: tangled()}
		in.Figures.Stacks = []store.Stack{f}
		if got, want := len(Build(in).Insides), len(Build(Input{Units: tangled()}).Insides); got != want {
			t.Errorf("%s folds: %d views, want %d", name, got, want)
		}
	}
}

func TestAMarkIsCutToTheRoomTheBoxHas(t *testing.T) {
	mark := "↻ 안에 순환 5상자 · 바깥과 이어진 3상자"
	if got := fitMark(mark, 1000); got != mark {
		t.Errorf("room for all: %q", got)
	}
	if got := fitMark(mark, subWidth("↻ 안에 순환 5상자")); got != "↻ 안에 순환 5상자" {
		t.Errorf("room for the first part: %q", got)
	}
	if got := fitMark(mark, 4); got != "" {
		t.Errorf("no room: %q", got)
	}
}
