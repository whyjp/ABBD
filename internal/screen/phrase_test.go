package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A line says one name at rest: the first of the phrase the most of its
// relations carry, the first by name among equals, never a list, 등 or 외 N,
// and only a phrase at least half of them carry.
// A bus counts its lines instead of relations. A name too wide gives way to
// the next phrase's. A line with no phrase that fits falls back to the via
// word its relations share. The tooltip keeps every phrase and its share.
func TestALineSaysWhatCrossesIt(t *testing.T) {
	long := strings.Repeat("x", model.PhraseMax)
	tr := tree{
		vias: map[string]model.Via{"a": "call", "b": "call", "c": "call", "d": "data", "e": "call", "l": "call", "m": "call",
			"t1": "call", "t2": "call", "t3": "call", "t4": "call", "k": "config", "w": "call", "v": "call", "l2": "call"},
		phrases: map[string]phrase{"a": {"Cypher", model.Inferred}, "b": {"Cypher", model.Inferred}, "c": {"schema fetch", model.UserStated},
			"l": {long, model.Inferred}, "m": {"WW" + long[2:], model.Inferred},
			"t1": {"delta", model.Inferred}, "t2": {"beta", model.Inferred}, "t3": {"gamma", model.Inferred}, "t4": {"alpha", model.Inferred},
			"k": {"COMP_WORDS, COMP_CWORD", model.Inferred}, "w": {"EnableDecoderUseNumber", model.Inferred}, "v": {"binding.Validator", model.Inferred},
			"l2": {long, model.Inferred}, "n": {"Evidence, Entry", model.Inferred}, "f": {"render(ctx, w), flush", model.Inferred}},
	}
	for _, tc := range []struct {
		rels []string
		want string
	}{
		{[]string{"a", "b"}, "Cypher"},
		{[]string{"a", "b", "c"}, "Cypher"},
		{[]string{"a", "c"}, "Cypher"},
		{[]string{"c", "a"}, "Cypher"},
		{[]string{"a", "b", "c", "e"}, "Cypher"},
		{[]string{"a", "c", "e"}, "호출"},
		{[]string{"a", "d", "c", "x"}, "호출"},
		{[]string{"a", "d", "c", "x", "y"}, ""},
		{[]string{"a", "x"}, "Cypher"},
		{[]string{"c", "x", "y"}, ""},
		{[]string{"d", "e"}, ""},
		{[]string{"e", "x"}, ""},
		{[]string{"d"}, "데이터"},
		{[]string{"x"}, ""},
		{[]string{"l"}, long},
		{[]string{"l", "e"}, long},
		{[]string{"m"}, "호출"},
		{[]string{"l", "d"}, long},
		{[]string{"k"}, "COMP_WORDS"},
		{[]string{"w", "v"}, "EnableDecoderUseNumber"},
		{[]string{"n", "x", "y"}, ""},
		{[]string{"n", "x"}, "Evidence"},
		{[]string{"f", "x", "y"}, ""},
		{[]string{"f", "x"}, "render(ctx, w)"},
		{[]string{"l", "l2", "c"}, long},
	} {
		if got := wordOf(tr, tc.rels); got != tc.want {
			t.Errorf("%v says %q, want %q", tc.rels, got, tc.want)
		}
	}
	// Map order changes from run to run, so only many runs show a tie broken
	// by chance.
	for range 200 {
		if got := wordOf(tr, []string{"t1", "t2"}); got != "beta" {
			t.Fatalf("two phrases carried once each say %q, want the first by name, \"beta\"", got)
		}
	}
	if got := said(tr, [][]string{{"c", "a", "b", "d", "x"}}); !slices.Equal(got, []string{"Cypher (판독) 2/5", "schema fetch (사람) 1/5"}) {
		t.Errorf("the tooltip lists %v, want each phrase once with whose words it is and its share of the line", got)
	}
	for _, tc := range []struct {
		lines [][]string
		want  string
	}{
		{[][]string{{"a"}, {"b"}}, "Cypher"},
		{[][]string{{"a", "e"}, {"b"}}, "Cypher"},
		{[][]string{{"a", "e"}, {"x"}}, "Cypher"},
		{[][]string{{"a", "e"}, {"x"}, {"c"}}, "호출"},
		{[][]string{{"c"}, {"b"}, {"c"}, {"x"}}, "schema fetch"},
		{[][]string{{"c"}, {"b"}, {"c"}, {"x"}, {"y"}}, "호출"},
		{[][]string{{"c"}, {"b"}, {"d"}, {"x"}, {"y"}}, ""},
	} {
		if got, _ := word(tr, tc.lines); got != tc.want {
			t.Errorf("a bus of %v says %q, want %q, the phrase most of its lines carry if half do, else the via word most carry if half do", tc.lines, got, tc.want)
		}
	}
	if got := said(tr, [][]string{{"a", "c"}, {"b"}, {"x"}}); !slices.Equal(got, []string{"Cypher (판독) 2/3", "schema fetch (사람) 1/3"}) {
		t.Errorf("a bus's tooltip lists %v, want each phrase's share of its lines", got)
	}
}

// A line merged from requests that share relations holds each relation once,
// and the path that shows a bus's count speaks for every line it counts,
// those its other drops draw too.
func TestALineHoldsEachRelationOnce(t *testing.T) {
	rels := [][]string{{"a", "a", "b"}, {"b", "c"}}
	of := func(i int) []string { return rels[i] }
	var bus Wire
	bus.carry(drawn{reqs: []int{0, 1}, count: 2, counted: []int{0, 1}}, of)
	if !slices.Equal(bus.Rels, []string{"a", "b", "c"}) || len(bus.lines) != 2 || !slices.Equal(bus.lines[0], []string{"a", "b"}) {
		t.Errorf("a bus of two lines holds %v in lines %v, want a, b, c once and each line's own", bus.Rels, bus.lines)
	}
	var drop Wire
	drop.carry(drawn{reqs: []int{0}, count: 2, counted: []int{0, 1}}, of)
	if !slices.Equal(drop.Rels, []string{"a", "b"}) || len(drop.lines) != 2 {
		t.Errorf("a drop that draws one of the two lines its ×2 counts holds %v and speaks for %v", drop.Rels, drop.lines)
	}
}

// A label's glyphs are centred on its X, so the area it keeps clear covers
// half a pixel more on each side when its word is of odd width.
func TestALabelsAreaCoversItsGlyphs(t *testing.T) {
	for _, s := range []string{"skill handoff", "quote check", "ab", "a"} {
		tw := float64(textWidth(s, labelPx))
		a := Label{S: s, X: 100, Y: 50}.area()
		if float64(a.x) > 100-tw/2 || float64(a.x+a.w) < 100+tw/2 {
			t.Errorf("%q is %.0fpx wide about x=100, and its area spans %d to %d", s, tw, a.x, a.x+a.w)
		}
	}
}
