package screen

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
)

func unit(id model.ID, rels ...model.Relation) model.Unit {
	k, _, _ := model.ParseID(string(id))
	return model.Unit{ID: id, Kind: k, Name: model.Fact{Value: string(id), Origin: model.Extracted}, Anchors: []model.Anchor{{Path: "x"}}, Relations: rels}
}

func called(id model.ID, name string, rels ...model.Relation) model.Unit {
	u := unit(id, rels...)
	u.Name.Value = name
	return u
}

func rel(to model.ID, kind model.RelationKind) model.Relation {
	return model.Relation{To: to, Kind: kind, Origin: model.Extracted}
}

func guess(to model.ID, kind model.RelationKind, via model.Via) model.Relation {
	return model.Relation{To: to, Kind: kind, Origin: model.Inferred, Confirmation: model.Unconfirmed, Via: via,
		Evidence: []model.Evidence{{Path: "src/a.rs", Lines: "1", Quote: "use b;"}}}
}

func inside(id, parent model.ID, rels ...model.Relation) model.Unit {
	u := unit(id, rels...)
	u.Parent = parent
	return u
}

// A small ripgrep: the binary uses the facade, the facade re-exports the
// searcher and the matcher, the searcher uses Matcher and provides Sink, the
// printer implements Sink and uses Matcher.
func ripgrep() []model.Unit {
	return []model.Unit{
		called("MOD-0001", "ripgrep", rel("MOD-0002", model.DependsOn), rel("IF-0001", model.Consumes)),
		called("MOD-0002", "grep", rel("MOD-0003", model.DependsOn), rel("MOD-0004", model.DependsOn), rel("MOD-0005", model.DependsOn)),
		called("MOD-0003", "grep-searcher", rel("IF-0001", model.Consumes), rel("IF-0002", model.Provides), rel("MOD-0004", model.DependsOn)),
		called("MOD-0004", "grep-matcher", rel("IF-0001", model.Provides)),
		called("MOD-0005", "grep-printer", rel("IF-0002", model.Consumes), rel("IF-0001", model.Consumes), rel("MOD-0003", model.DependsOn)),
		called("IF-0001", "trait Matcher (grep-matcher)"),
		called("IF-0002", "trait Sink (grep-searcher)"),
	}
}

func design(units []model.Unit) topology.Intended {
	all := map[model.ID]bool{}
	for _, u := range units {
		all[u.ID] = true
	}
	return topology.Select(units, all)
}

func build(units []model.Unit, in topology.Intended) Model {
	return Build(Input{Units: units, Intended: &in})
}

func svg(t *testing.T, d Drawing) string {
	t.Helper()
	var b bytes.Buffer
	if err := page.ExecuteTemplate(&b, "drawing", d); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func boxes(d Drawing) map[string]Box {
	out := map[string]Box{}
	for _, b := range d.Boxes {
		out[b.ID] = b
	}
	return out
}

func TestTheSameDataAlwaysDrawsTheSamePicture(t *testing.T) {
	units := ripgrep()
	in := design(units)
	first := build(units, in)
	for i := 0; i < 5; i++ {
		again := build(units, in)
		if svg(t, again.Overview) != svg(t, first.Overview) {
			t.Fatalf("drawing the same data twice gave two overviews")
		}
		for j := range again.Insides {
			if svg(t, again.Insides[j].Drawing) != svg(t, first.Insides[j].Drawing) {
				t.Fatalf("drawing the same data twice gave two insides of %s", again.Insides[j].ID)
			}
		}
	}
}

// A human compares two screenshots of the overview, so what happens inside a
// module, and any guess an agent registers, must not move a box or a port.
func TestAModulesInsideNeverMovesTheOverview(t *testing.T) {
	units := ripgrep()
	in := design(units)
	before := build(units, in).Overview

	grown := append(ripgrep(),
		inside("MOD-0006", "MOD-0003", rel("IF-0001", model.Consumes), rel("MOD-0007", model.DependsOn)),
		inside("MOD-0007", "MOD-0003", rel("IF-0002", model.Provides)),
		inside("DATA-0001", "MOD-0004"),
	)
	grown[0].Relations = append(grown[0].Relations, guess("IF-0002", model.Consumes, model.Via("trait")), guess("MOD-0005", model.DependsOn, model.Via("import")))
	grown[2].Relations = append(grown[2].Relations, guess("DATA-0001", model.Consumes, model.Via("convention")))
	after := build(grown, in).Overview

	if !reflect.DeepEqual(boxes(before), boxes(after)) || !reflect.DeepEqual(before.Ports, after.Ports) {
		t.Fatalf("the overview moved when only insides and guesses changed:\nbefore %+v\nafter  %+v", before.Boxes, after.Boxes)
	}
	if strings.Contains(svg(t, after), "MOD-0006") || strings.Contains(svg(t, after), "DATA-0001\"") {
		t.Fatalf("an inside unit must never be drawn on the overview")
	}
	semantic := 0
	for _, w := range after.Wires {
		if w.Kind == "semantic" {
			semantic++
		}
	}
	if semantic != 3 {
		t.Fatalf("each guessed coupling between two modules is drawn once, lifted to the modules; got %d", semantic)
	}
}

func TestModulesSitAboveWhatTheyUse(t *testing.T) {
	b := boxes(build(ripgrep(), design(ripgrep())).Overview)
	for _, pair := range [][2]string{{"MOD-0001", "MOD-0002"}, {"MOD-0002", "MOD-0003"}, {"MOD-0005", "MOD-0003"}, {"MOD-0003", "MOD-0004"}} {
		if b[pair[0]].Y >= b[pair[1]].Y {
			t.Errorf("%s uses %s and should sit above it: y %d vs %d", pair[0], pair[1], b[pair[0]].Y, b[pair[1]].Y)
		}
	}
}

// A line between modules goes through a contract. A dependency with no
// contract under it bypasses the boundary and is drawn as its own kind.
func TestADependencyThatBypassesEveryContractStandsOut(t *testing.T) {
	d := build(ripgrep(), design(ripgrep())).Overview
	kinds := map[string]int{}
	var bypass []string
	for _, w := range d.Wires {
		kinds[w.Kind]++
		if w.Kind == "bypass" {
			bypass = append(bypass, w.Title)
		}
	}
	// ripgrep → grep and grep → its three crates have no contract; the
	// searcher's and the printer's dependencies are carried by Matcher and Sink.
	if kinds["bypass"] != 4 {
		t.Fatalf("want the 4 contract-free dependencies drawn as bypasses, got %d: %v", kinds["bypass"], bypass)
	}
	if kinds["contract"] != 4 {
		t.Fatalf("want one contract line per user and provider of Matcher and Sink, got %d", kinds["contract"])
	}
	exits, entries := 0, 0
	for _, p := range d.Ports {
		if p.Exit {
			exits++
		} else {
			entries++
		}
	}
	if exits != 2 || entries != 4 {
		t.Fatalf("each contract is an exit on its provider and an entry on each user: exits %d entries %d", exits, entries)
	}
}

func TestLabelsFitTheirBoxes(t *testing.T) {
	units := ripgrep()
	units[0].Name.Value = "a module whose name is far too long to fit any box drawn on the overview at all"
	units[5].Name.Value = "trait AnExtremelyLongContractNameThatWouldOverflowItsChip"
	d := build(units, design(units)).Overview
	for _, b := range d.Boxes {
		if textWidth(b.Label, 13) > b.W-16 {
			t.Errorf("box %s label %q is wider than the box", b.ID, b.Label)
		}
	}
	for _, p := range d.Ports {
		if textWidth(p.Label, chipPx) > p.W-6 {
			t.Errorf("port %q is wider than its chip", p.Label)
		}
	}
}

// The second level puts the module's contracts on its frame and joins each
// to the inside unit that implements or uses it.
func TestAModulesInsideJoinsItsContractsToItsUnits(t *testing.T) {
	units := append(ripgrep(),
		inside("MOD-0006", "MOD-0003", rel("IF-0001", model.Consumes), rel("MOD-0007", model.DependsOn)),
		inside("MOD-0007", "MOD-0003", rel("IF-0002", model.Provides)),
	)
	m := build(units, design(ripgrep()))
	var searcher Inside
	for _, ins := range m.Insides {
		if ins.ID == "MOD-0003" {
			searcher = ins
		}
	}
	ports := map[string]bool{}
	for _, p := range searcher.Drawing.Ports {
		ports[p.Contract] = p.Exit
	}
	if exit, ok := ports["IF-0001"]; !ok || exit {
		t.Fatalf("Matcher, which the searcher uses, belongs on the bottom of its frame: %+v", searcher.Drawing.Ports)
	}
	if exit, ok := ports["IF-0002"]; !ok || !exit {
		t.Fatalf("Sink, which the searcher provides, belongs on the top of its frame: %+v", searcher.Drawing.Ports)
	}
	titles := []string{}
	for _, w := range searcher.Drawing.Wires {
		titles = append(titles, w.Title)
	}
	for _, want := range []string{"MOD-0006 consumes trait Matcher (grep-matcher)", "MOD-0007 provides trait Sink (grep-searcher)", "MOD-0006 depends_on MOD-0007"} {
		if !strings.Contains(strings.Join(titles, "\n"), want) {
			t.Errorf("missing the line %q among %v", want, titles)
		}
	}
	if len(searcher.Unwired) != 0 {
		t.Errorf("every contract on the frame has an inside unit: %v", searcher.Unwired)
	}
}

func TestAContractChangeShowsOnlyTheMembersThatChanged(t *testing.T) {
	old := "pub trait Sink { type Error: SinkError; fn matched(&mut self, _searcher: &Searcher) -> Result<bool, Self::Error>; fn finish(&mut self) -> Result<(), Self::Error> }"
	new := "pub trait Sink { type Error: SinkError; fn matched(&mut self, _searcher: &Searcher, at: u64) -> Result<bool, Self::Error>; fn finish(&mut self) -> Result<(), Self::Error> }"
	diff, same := diffSignature(old, new)
	if len(diff) != 2 || diff[0].Op != "+" && diff[0].Op != "-" || same != 3 {
		t.Fatalf("one changed method shows as one removal and one addition, the rest folded: %+v same %d", diff, same)
	}
	for _, d := range diff {
		if strings.Contains(d.Parts[0].Text, "finish") {
			t.Fatalf("an unchanged member was shown: %+v", d)
		}
	}
}
