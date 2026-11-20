package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// bandUnits are five boxes that all use log, which uses nothing.
func bandUnits() []model.Unit {
	return []model.Unit{
		called("MOD-0001", "api", rel("MOD-0002", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0003", "store", rel("MOD-0005", model.DependsOn)),
		called("MOD-0004", "cli", rel("MOD-0001", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0005", "log"),
	}
}

// A common module the classification calls a utility stands as a
// foundation band under every other box and as wide as they span, with
// none of its lines drawn and its last line saying how many boxes use it.
// No record and no switch asks for it. A box many use that uses as much
// back is no common module and stays a box.
func TestAUtilityStandsAsAFoundationBand(t *testing.T) {
	units := bandUnits()
	on := Build(Input{Units: units})
	if len(on.Common) != 1 || on.Common[0].Unit != "MOD-0005" || !on.Common[0].Utility() {
		t.Fatalf("log is not the one utility: %+v", on.Common)
	}
	var band *Box
	lowest, left, right := 0, 1<<30, 0
	for i, b := range on.Overview.Boxes {
		if b.Ambient {
			band = &on.Overview.Boxes[i]
			continue
		}
		lowest, left, right = max(lowest, b.Y+b.H), min(left, b.X), max(right, b.X+b.W)
	}
	if band == nil || band.ID != "MOD-0005" {
		t.Fatalf("no band for the utility log: %+v", on.Overview.Boxes)
	}
	if band.Y <= lowest || band.X != left || band.X+band.W != max(right, left+boxMax) {
		t.Errorf("the band stands at %d,%d %dpx wide; the boxes end at %d and span %d..%d", band.X, band.Y, band.W, lowest, left, right)
	}
	if band.Sub == nil || band.Sub.S != foundationText(4) {
		t.Errorf("the band says %+v (box %+v)", band.Sub, *band)
	}
	for _, r := range on.Overview.ways {
		for _, e := range r.ends {
			if e == "MOD-0005" {
				t.Errorf("a route still ends on the band: %+v", r.ends)
			}
		}
	}
	units[4].Relations = []model.Relation{rel("MOD-0001", model.DependsOn), rel("MOD-0002", model.DependsOn), rel("MOD-0003", model.DependsOn)}
	coupled := Build(Input{Units: units})
	if len(coupled.Common) != 0 {
		t.Errorf("log uses as much as it is used, yet is common: %+v", coupled.Common)
	}
	for _, b := range coupled.Overview.Boxes {
		if b.Ambient {
			t.Errorf("%s stands as a band though it is no common module", b.Name)
		}
	}
}

// A box a line of the record ends on stays a box, whatever its class, so
// the record's word is never lost to the band.
func TestARecordedLineKeepsAUtilityABox(t *testing.T) {
	var rec store.Overview
	rec.Known = true
	rec.Lines = []store.Part[model.Line]{{Claim: model.Line{From: "MOD-0001", To: "MOD-0005", Word: "logs"}}}
	m := Build(Input{Units: bandUnits(), Overview: rec})
	for _, b := range m.Overview.Boxes {
		if b.Ambient {
			t.Errorf("%s stands as a band though the record draws a line to it", b.Name)
		}
	}
}

// A common module that holds state another box reaches affects the logic:
// it keeps its box and its lines, and its tooltip says why. A setting
// inside it that no other box reaches leaves it a utility.
func TestReachedStateMakesACommonModuleAffectTheLogic(t *testing.T) {
	units := append(bandUnits(), model.Unit{ID: "CFG-0001", Kind: model.Config, Name: model.Said(model.Script, "PORT"), Parent: "MOD-0005"})
	m := Build(Input{Units: units})
	if len(m.Common) != 1 || !m.Common[0].Utility() {
		t.Fatalf("a setting no box reaches made log %+v", m.Common)
	}
	units = append(units, model.Unit{ID: "DATA-0001", Kind: model.Data, Name: model.Said(model.Script, "registry"), Parent: "MOD-0005"})
	units[0].Relations = append(units[0].Relations, rel("DATA-0001", model.DependsOn))
	m = Build(Input{Units: units})
	if len(m.Common) != 1 || m.Common[0].Kind != CommonLogic || !strings.Contains(strings.Join(m.Common[0].Reasons, " "), "registry") {
		t.Fatalf("state api reaches left log %+v", m.Common)
	}
	for _, b := range m.Overview.Boxes {
		if b.ID == "MOD-0005" && (b.Ambient || !strings.Contains(b.Title, "공통 모듈 (로직 영향)")) {
			t.Errorf("log is a band or its tooltip says nothing of its kind: %+v", b)
		}
	}
}

// The common modules' page lists each common module with its kind, the
// boxes that use it and those that do not, and the libraries two boxes or
// more share; the overview links it. A blueprint with neither has no page.
func TestTheCommonPageListsWhoUsesEachCommonModule(t *testing.T) {
	units := append(bandUnits(),
		model.Unit{ID: "MOD-0006", Kind: model.Module, Name: model.Said(model.Script, "docs")},
		model.Unit{ID: "EXT-0001", Kind: model.External, Name: model.Said(model.Script, "yaml")},
		model.Unit{ID: "EXT-0002", Kind: model.External, Name: model.Said(model.Script, "color")})
	units[0].Relations = append(units[0].Relations, rel("EXT-0001", model.DependsOn), rel("EXT-0002", model.DependsOn))
	units[2].Relations = append(units[2].Relations, rel("EXT-0001", model.DependsOn))
	fs, err := Files(Build(Input{Units: units}))
	if err != nil {
		t.Fatal(err)
	}
	page := string(fs[CommonFile])
	for _, want := range []string{`id="c-MOD-0005"`, "유틸리티", "안 쓰는 상자", ">docs<", ">yaml<"} {
		if !strings.Contains(page, want) {
			t.Errorf("the common page lacks %q", want)
		}
	}
	if strings.Contains(page, ">color<") {
		t.Error("the common page lists a library one box uses")
	}
	if !strings.Contains(string(fs[PageFile]), `href="common.html`) {
		t.Error("the overview does not link the common page")
	}
	if !strings.Contains(string(fs[GraphFile]), `"kind": "utility"`) {
		t.Error("graph.json does not give the common module's kind")
	}
	plain, err := Files(Build(Input{Units: bandUnits()[:3]}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain[CommonFile]; ok || strings.Contains(string(plain[PageFile]), "common.html") {
		t.Error("a blueprint with no common module has a common page")
	}
}

// A contract outside every box that two boxes provide counts each use
// once out of the user and once into each provider, and a provider's own
// relation that provides it counts as no use of the other.
func TestAContractTwoBoxesProvideCountsEachUseOnce(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "a", rel("IF-0001", model.Consumes)),
		called("MOD-0002", "b", rel("IF-0001", model.Consumes)),
		called("MOD-0003", "c", rel("IF-0001", model.Consumes)),
		called("MOD-0004", "d", rel("IF-0001", model.Consumes)),
		called("MOD-0005", "disk", rel("IF-0001", model.Provides)),
		called("MOD-0006", "memory", rel("IF-0001", model.Provides)),
		{ID: "IF-0001", Kind: model.Interface, Name: model.Said(model.Script, "Store")},
	}
	in := Input{Units: units}
	o := planScoped(in.Units, "", nil, tactic{}, layerings(in), walks(in), nil, scopeOf(in))
	cs, _ := o.commons()
	got := map[string][2]int{}
	for _, c := range cs {
		got[c.Unit] = [2]int{c.In, c.Out}
	}
	for _, p := range []string{"MOD-0005", "MOD-0006"} {
		if got[p] != [2]int{4, 0} {
			t.Errorf("%s counts in, out %v; want the 4 uses in and nothing out: %+v", p, got[p], cs)
		}
	}
}
