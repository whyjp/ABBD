package screen

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// withFigures builds the self fixture with a stored flow and a stored stack.
func withFigures(t *testing.T, flow []model.ID, layers map[string][]model.ID, order []string) Model {
	t.Helper()
	units, err := store.Open(filepath.Join("testdata", "self")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	var f store.Flow
	f.Key, f.Known, f.Question = "walk", true, "What runs, in order?"
	for _, u := range flow {
		f.Parts = append(f.Parts, store.Part[model.Step]{Claim: model.Step{Unit: u, Does: "runs"}})
	}
	var s store.Stack
	s.Key, s.Known, s.Question = "layers", true, "What are the layers?"
	for _, name := range order {
		s.Parts = append(s.Parts, store.Part[model.Layer]{Claim: model.Layer{Name: name, Units: layers[name]}})
	}
	return Build(Input{Units: units, Figures: store.Figures{Flows: []store.Flow{f}, Stacks: []store.Stack{s}}})
}

// A stored flow leads the overview along its steps: each new box joins by
// the line from the box on the lead that ran a step last and uses it, a
// step back on a box already on the lead adds no hop, and the hops are
// numbered in that order from the box the first hop leaves, which is filled.
func TestAStoredFlowLeadsInItsOrder(t *testing.T) {
	cli, st, gitx, extract := model.ID("MOD-0001"), model.ID("MOD-0041"), model.ID("MOD-0022"), model.ID("MOD-0017")
	m := withFigures(t, []model.ID{cli, st, gitx, cli, extract}, nil, nil)
	l := m.Overview.Lead
	if l == nil || l.Key != "walk" || l.Entry != string(cli) {
		t.Fatalf("lead %+v, want the stored flow from %s", l, cli)
	}
	want := [][2]model.ID{{cli, st}, {st, gitx}, {cli, extract}}
	if len(l.Hops) != len(want) {
		t.Fatalf("%d hops, want %d", len(l.Hops), len(want))
	}
	for i, h := range l.Hops {
		ends := m.Overview.ways[h.way].ends
		if h.N != i+1 || !slices.Equal(ends, []string{string(want[i][0]), string(want[i][1])}) {
			t.Errorf("hop %d runs %v, want %v", h.N, ends, want[i])
		}
	}
	if f := m.Overview.Filled(); f == nil || f.ID != string(cli) || f.Role != "entry" {
		t.Errorf("filled box %+v, want %s as the entry", f, cli)
	}
}

// A flow whose next step sits on a box no drawn line joins to the lead
// stops there, and one hop is no lead.
func TestAFlowStopsWhereNoLineJoinsIt(t *testing.T) {
	gitx, external := model.ID("MOD-0022"), model.ID("MOD-0055")
	m := withFigures(t, []model.ID{gitx, external, gitx}, nil, nil)
	if m.Overview.Lead != nil {
		t.Errorf("a broken flow draws a lead of %d hops", len(m.Overview.Lead.Hops))
	}
}

// A stored stack's layers band the boxes they name, numbered from the
// top, and a band is drawn only where it holds no other box.
func TestAStackBandsItsLayers(t *testing.T) {
	layers := map[string][]model.ID{"commands": {"MOD-0001"}, "store": {"MOD-0041", "MOD-0017"}}
	m := withFigures(t, nil, layers, []string{"commands", "store"})
	d := m.Overview
	groups := d.layered(newTreeFor(t), []layering{{names: []string{"commands", "store"}, units: [][]model.ID{layers["commands"], layers["store"]}}})
	if len(groups) != 2 || groups[0].n != 1 || groups[1].n != 2 || !slices.Equal(groups[0].ids, []string{"MOD-0001"}) {
		t.Fatalf("layered gives %+v", groups)
	}
	if len(d.Zones) == 0 {
		t.Fatal("no layer is banded")
	}
	for _, z := range d.Zones {
		if z.N == 0 && z.Title == "외부 계약" {
			continue
		}
		if z.N < 1 || z.N > len(groups) {
			t.Fatalf("band %q is numbered %d, not one of the %d stored layers", z.Title, z.N, len(groups))
		}
		g := groups[z.N-1]
		if z.Title != []string{"commands", "store"}[z.N-1] {
			t.Errorf("band %d is titled %q", z.N, z.Title)
		}
		for _, b := range d.Boxes {
			if !slices.Contains(g.ids, b.ID) && hit(box{z.X, z.Y, z.W, z.H}, box{b.X, b.Y, b.W, b.H}) {
				t.Errorf("band %q holds %s, which its layer does not name", z.Title, b.ID)
			}
		}
	}
}

func newTreeFor(t *testing.T) tree {
	t.Helper()
	units, err := store.Open(filepath.Join("testdata", "self")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	return newTree(units)
}

// A view past hover's caps has no routes to light its lines again, so it
// draws its lead but never rests its other lines grey.
func TestAViewPastTheHoverCapsIsNeverQuiet(t *testing.T) {
	cli, st, gitx := model.ID("MOD-0001"), model.ID("MOD-0041"), model.ID("MOD-0022")
	m := withFigures(t, []model.ID{cli, st, gitx}, nil, nil)
	if !m.Overview.Quiet {
		t.Fatal("the overview with a lead and routes is not quiet")
	}
	d := withFigures(t, []model.ID{cli, st, gitx}, nil, nil).Overview
	d.Lead, d.Quiet, d.Routes = nil, false, nil
	d.lead(newTreeFor(t), []flowWalk{{key: "walk", units: []model.ID{cli, st, gitx}}})
	if d.Lead == nil {
		t.Fatal("a view past the caps draws no lead")
	}
	if d.Quiet {
		t.Error("a view past the caps rests its lines grey with no way to light them")
	}
}

// A view's card lists a sample only for a mark its rendered drawing
// shows as the sample does, read from the SVG the page draws: the lead's
// bold way, a filled box, an orange bypass line, a dotted line that does
// not climb, a climbing line. Its area keeps cardRoom from all other ink
// and every number, as the card promises.
func TestACardListsOnlyWhatItsViewDraws(t *testing.T) {
	marks := map[string]*regexp.Regexp{
		"lead":   regexp.MustCompile(`<g class="leadway"`),
		"fill":   regexp.MustCompile(`<g class="box [^"]*\blead\b`),
		"bypass": regexp.MustCompile(`<path class="w (bypass|mutual)\b`),
		"plan":   regexp.MustCompile(`<path class="w [^"]*\bplan\b[^"]*"`),
		"back":   regexp.MustCompile(`<path class="w [^"]*\bback\b`),
	}
	climbs := regexp.MustCompile(`\bback\b`)
	cards := 0
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			c := d.Card
			if c == nil {
				continue
			}
			cards++
			var b strings.Builder
			if err := site.ExecuteTemplate(&b, "drawing", d); err != nil {
				t.Fatal(err)
			}
			svg := b.String()
			for _, it := range c.Items {
				found := marks[it.Kind].FindAllString(svg, -1)
				if it.Kind == "plan" {
					found = slices.DeleteFunc(found, func(s string) bool { return climbs.MatchString(s) })
				}
				if len(found) == 0 {
					t.Errorf("%s: a card lists %s, which its drawing does not show", name, it.Kind)
				}
			}
			area := grow(box{c.X, c.Y, c.W, c.H}, cardRoom)
			if !d.clearOfInk(area, nil) || d.onBadge(area) {
				t.Errorf("%s: a card at %d,%d comes within %dpx of other ink", name, c.X, c.Y, cardRoom)
			}
			for _, bx := range d.Boxes {
				if hit(area, box{bx.X, bx.Y, bx.W, bx.H}) {
					t.Errorf("%s: a card covers box %s", name, bx.ID)
				}
			}
		}
	}
	if cards == 0 {
		t.Error("no fixture view keeps a card")
	}
}
