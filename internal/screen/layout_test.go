package screen

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
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
	return model.Relation{To: to, Kind: kind, Origin: model.Inferred, Via: via,
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

func build(units []model.Unit) Model { return Build(Input{Units: units}) }

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
	first := build(units)
	for i := 0; i < 5; i++ {
		again := build(units)
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
// module must not move a box, a port or a line. A relation from inside a
// module that crosses to another top-level unit is the module's own on the
// overview: it adds what it stands for to the line already drawn.
func TestAModulesInsideNeverMovesTheOverview(t *testing.T) {
	before := build(ripgrep()).Overview
	grown := append(ripgrep(),
		inside("MOD-0006", "MOD-0003", rel("IF-0001", model.Consumes), rel("MOD-0007", model.DependsOn)),
		inside("MOD-0007", "MOD-0003", rel("IF-0002", model.Provides)),
		inside("DATA-0001", "MOD-0004"),
	)
	after := build(grown).Overview
	type drawnLine struct {
		D, Kind string
		Count   int
	}
	shape := func(d Drawing) ([]drawnLine, []Port) {
		var ws []drawnLine
		for _, w := range d.Wires {
			ws = append(ws, drawnLine{w.D, w.Kind, w.Count})
		}
		var ps []Port
		for _, p := range d.Ports {
			p.Rels = nil
			ps = append(ps, p)
		}
		return ws, ps
	}
	bw, bp := shape(before)
	aw, ap := shape(after)
	if !reflect.DeepEqual(boxes(before), boxes(after)) || !reflect.DeepEqual(bp, ap) || !reflect.DeepEqual(bw, aw) {
		t.Fatalf("the overview moved when only insides changed:\nbefore %+v\nafter  %+v", bw, aw)
	}
	if strings.Contains(svg(t, after), "MOD-0006") || strings.Contains(svg(t, after), "DATA-0001\"") {
		t.Fatalf("an inside unit must never be drawn on the overview")
	}
	var carried bool
	for _, w := range after.Wires {
		carried = carried || slices.Contains(w.Rels, "MOD-0006.consumes.IF-0001")
	}
	if !carried {
		t.Fatalf("the inside unit's use of Matcher should ride the searcher's line to the matcher")
	}
}

// A coupling a reader found is drawn on the overview with the reader's mark
// where it starts, because sync re-checks its quote every time. A dead one,
// whose quote is gone, is drawn nowhere. Each module's inside still lists
// the reader's couplings that touch it, with their quotes.
func TestAReadersCouplingIsDrawnWithItsMark(t *testing.T) {
	units := ripgrep()
	units[0].Relations = append(units[0].Relations, guess("MOD-0005", model.DependsOn, model.Via("import")))
	dead := guess("MOD-0004", model.DependsOn, model.Via("import"))
	dead.Dead = true
	units[4].Relations = append(units[4].Relations, dead)
	m := build(units)
	var found, marked bool
	for _, w := range m.Overview.Wires {
		for _, r := range w.Rels {
			if r == "MOD-0001.depends_on.MOD-0005" {
				found = true
				for _, mk := range w.Marks {
					marked = marked || mk.Origin == "inferred"
				}
			}
			if r == "MOD-0005.depends_on.MOD-0004" {
				t.Errorf("a dead coupling is drawn: %+v", w)
			}
		}
	}
	if !found || !marked {
		t.Fatalf("the overview should draw the reader's coupling ripgrep → grep-printer with a reader's mark: drawn %v, marked %v", found, marked)
	}
	listed := map[string]int{}
	for _, ins := range m.Insides {
		for _, c := range ins.Inferred {
			if c.Via == "" || len(c.Quotes) == 0 {
				t.Errorf("%s lists a coupling without its via and quote: %+v", ins.ID, c)
			}
			listed[ins.ID]++
		}
	}
	if want := map[string]int{"MOD-0001": 1, "MOD-0005": 1}; !reflect.DeepEqual(listed, want) {
		t.Fatalf("each module's inside lists the couplings that touch it: got %v, want %v", listed, want)
	}
}

func TestModulesSitAboveWhatTheyUse(t *testing.T) {
	b := boxes(build(ripgrep()).Overview)
	for _, pair := range [][2]string{{"MOD-0001", "MOD-0002"}, {"MOD-0002", "MOD-0003"}, {"MOD-0005", "MOD-0003"}, {"MOD-0003", "MOD-0004"}} {
		if b[pair[0]].Y >= b[pair[1]].Y {
			t.Errorf("%s uses %s and should sit above it: y %d vs %d", pair[0], pair[1], b[pair[0]].Y, b[pair[1]].Y)
		}
	}
}

// A line between modules goes through a contract. A dependency with no
// contract under it bypasses the boundary and is drawn as its own kind.
func TestADependencyThatBypassesEveryContractStandsOut(t *testing.T) {
	d := build(ripgrep()).Overview
	seen := map[string]bool{}
	kinds := map[string]int{}
	var bypass []string
	for _, w := range d.Wires {
		for _, title := range strings.Split(w.Title, "\n") {
			if !seen[title] {
				seen[title] = true
				kinds[w.Kind]++
				if w.Kind == "bypass" {
					bypass = append(bypass, title)
				}
			}
		}
	}
	// ripgrep → grep and grep → its three crates have no contract; the
	// searcher's and the printer's dependencies are carried by Matcher and Sink.
	if kinds["bypass"] != 4 {
		t.Fatalf("want the 4 contract-free dependencies drawn as bypasses, got %d: %v", kinds["bypass"], bypass)
	}
	if kinds["contract"] != 4 {
		t.Fatalf("want one contract line per user and provider of Matcher and Sink, counting the lines a bundle carries, got %d", kinds["contract"])
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
	d := build(units).Overview
	for _, b := range d.Boxes {
		for _, l := range b.Lines {
			if textWidth(l.S, 13) > b.W-16 {
				t.Errorf("box %s label %q is wider than the box", b.ID, l.S)
			}
		}
	}
	for _, p := range d.Ports {
		for _, l := range p.Lines {
			if textWidth(l, chipPx) > p.W-6 {
				t.Errorf("port %q is wider than its chip", l)
			}
		}
		if strings.Join(p.Lines, "") != p.Label {
			t.Errorf("port %q must keep its whole name: %q", p.Label, p.Lines)
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
	m := build(units)
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

// Two modules that use each other directly are one line with a head at each
// end, read downward, and it stands for both relations.
func TestMutualUseIsOneLine(t *testing.T) {
	m := build([]model.Unit{
		called("MOD-0001", "app", rel("MOD-0002", model.DependsOn), rel("MOD-0003", model.DependsOn)),
		called("MOD-0002", "core", rel("MOD-0003", model.DependsOn)),
		called("MOD-0003", "util", rel("MOD-0002", model.DependsOn)),
	})
	var mutual []Wire
	for _, w := range m.Overview.Wires {
		if w.Kind == "mutual" {
			mutual = append(mutual, w)
		}
		if w.Back {
			t.Errorf("no line should climb, got %q", w.Title)
		}
	}
	if len(mutual) != 1 || !mutual[0].Both || len(mutual[0].Rels) != 2 {
		t.Fatalf("want one two-headed line for core and util, got %+v", mutual)
	}
	if !strings.Contains(mutual[0].Title, "core → util") || !strings.Contains(mutual[0].Title, "util → core") {
		t.Fatalf("the line should name both directions, got %q", mutual[0].Title)
	}
}

// A chip whose name is longer than any edge wraps onto more lines: it stays
// whole, inside the edge it sits on and inside the canvas.
func TestALongChipNameWrapsInsideItsEdge(t *testing.T) {
	units := ripgrep()
	units[5].Name.Value = "trait SearcherSinkMatcherBridgeThatAdaptsEveryMatchIntoAPrinterCallbackWithContextLinesAndByteOffsets (grep-matcher)"
	units[6].Name.Value = "trait sink for every printer that writes matches, context lines, byte offsets, binary detection and summary statistics"
	m := build(units)
	for _, v := range views(m) {
		s, err := readSVG(svg(t, v.d))
		if err != nil {
			t.Fatal(err)
		}
		if mt := measure(s); mt.Escaped+mt.Truncated+mt.Overflow > 0 {
			t.Errorf("%s: %s", v.name, mt)
		}
		for _, sh := range s.shapes {
			if sh.kind == "port" && strings.Contains(sh.title, "SearcherSink") && !strings.Contains(strings.Join(sh.lines, ""), "ByteOffsets") {
				t.Errorf("%s: the chip must keep its whole name: %q", v.name, sh.lines)
			}
		}
	}
}
