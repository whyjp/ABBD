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

// A stored stack's layers stand in rows of their own, so each layer of two
// boxes or more bands every box it names, numbered from the top with no
// gap, and its band holds no other box. A layer of one box takes no band.
func TestAStackBandsItsLayers(t *testing.T) {
	order := []string{"commands", "rules", "apart", "store"}
	layers := map[string][]model.ID{
		"commands": {"MOD-0001"},
		"rules":    {"MOD-0013", "MOD-0054", "MOD-0026"},
		"apart":    {"MOD-0022", "MOD-0009"},
		"store":    {"MOD-0041", "MOD-0017"},
	}
	m := withFigures(t, nil, layers, order)
	d := m.Overview
	var l layering
	for _, name := range order {
		l.names, l.units = append(l.names, name), append(l.units, layers[name])
	}
	bare := d
	bare.Zones = nil
	groups := bare.layered(newTreeFor(t), []layering{l})
	want := map[string][]string{"rules": {"MOD-0013", "MOD-0026", "MOD-0054"}, "apart": {"MOD-0009", "MOD-0022"}, "store": {"MOD-0017", "MOD-0041"}}
	if len(groups) != len(want) {
		t.Fatalf("layered gives %+v, want every box of rules, apart and store", groups)
	}
	for _, g := range groups {
		ids := slices.Sorted(slices.Values(g.ids))
		if !slices.Equal(ids, want[g.title]) {
			t.Errorf("layer %q bands %v, want %v", g.title, ids, want[g.title])
		}
	}
	var drawn []string
	for _, z := range d.Zones {
		if z.N == 0 && z.Title == "외부 계약" {
			continue
		}
		if z.N != len(drawn)+1 {
			t.Errorf("band %q is numbered %d, want %d", z.Title, z.N, len(drawn)+1)
		}
		drawn = append(drawn, z.Title)
		g := groups[slices.IndexFunc(groups, func(g group) bool { return g.title == z.Title })]
		for _, b := range d.Boxes {
			if !slices.Contains(g.ids, b.ID) && hit(box{z.X, z.Y, z.W, z.H}, box{b.X, b.Y, b.W, b.H}) {
				t.Errorf("band %q holds %s, which its layer does not name", z.Title, b.ID)
			}
		}
	}
	if !slices.Equal(drawn, []string{"rules", "apart", "store"}) {
		t.Fatalf("bands %v are drawn, want rules, apart and store", drawn)
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
// bold way, a filled box, a bypass line, a dotted line that does
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

// Every card's words fit inside the card.
func TestACardsWordsFitIt(t *testing.T) {
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			if c := d.Card; c != nil {
				for _, it := range c.Items {
					if it.TX+textWidth(it.Text, 11) > c.X+c.W-6 {
						t.Errorf("%s: the card's %q runs past its edge", name, it.Text)
					}
				}
			}
		}
	}
}
