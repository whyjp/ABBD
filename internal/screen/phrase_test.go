package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A line says the phrase its relations share. A line whose relations differ
// says the phrase the most carry, the first by name among equals, with 등
// when at least half carry it, and otherwise by its first name with 외 and
// how many relations do not carry it, so one phrase of ten never
// passes for the line (rule 25). A bus counts its lines instead, so its
// label adds up to its ×N. A phrase too wide keeps the names before its
// commas that fit, else the next phrase speaks if at least half carry it.
// A line with no phrase that fits falls back to the via word its relations
// share.
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
		{[]string{"a", "b", "c"}, "Cypher 등"},
		{[]string{"a", "c"}, "Cypher 등"},
		{[]string{"c", "a"}, "Cypher 등"},
		{[]string{"a", "b", "c", "e"}, "Cypher 등"},
		{[]string{"a", "c", "e"}, "Cypher 외 2"},
		{[]string{"a", "d", "c", "x"}, "Cypher 외 3"},
		{[]string{"a", "x"}, "Cypher 등"},
		{[]string{"c", "x", "y"}, "schema fetch 외 2"},
		{[]string{"d", "e"}, ""},
		{[]string{"e", "x"}, ""},
		{[]string{"d"}, "데이터"},
		{[]string{"x"}, ""},
		{[]string{"l"}, long},
		{[]string{"l", "e"}, "호출"},
		{[]string{"m"}, "호출"},
		{[]string{"l", "d"}, ""},
		{[]string{"k"}, "COMP_WORDS 등"},
		{[]string{"w", "v"}, "binding.Validator 등"},
		{[]string{"n", "x", "y"}, "Evidence 외 2"},
		{[]string{"n", "x"}, "Evidence, Entry 등"},
		{[]string{"f", "x", "y"}, "render(ctx, w) 외 2"},
		{[]string{"l", "l2", "c"}, "호출"},
	} {
		if got := wordOf(tr, tc.rels); got != tc.want {
			t.Errorf("%v says %q, want %q", tc.rels, got, tc.want)
		}
	}
	// Map order changes from run to run, so only many runs show a tie broken
	// by chance.
	for range 200 {
		if got := wordOf(tr, []string{"t1", "t2", "t3", "t4"}); got != "alpha 외 3" {
			t.Fatalf("four phrases carried once each say %q, want the first by name, \"alpha 외 3\"", got)
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
		{[][]string{{"a", "e"}, {"b"}}, "Cypher 등"},
		{[][]string{{"a", "e"}, {"x"}, {"y"}}, "Cypher 외 2"},
		{[][]string{{"a"}, {"b"}, {"c"}, {"x"}, {"y"}}, "Cypher 외 3"},
	} {
		if got, _ := word(tr, tc.lines); got != tc.want {
			t.Errorf("a bus of %v says %q, want %q, whose lines add up to its ×%d", tc.lines, got, tc.want, len(tc.lines))
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
