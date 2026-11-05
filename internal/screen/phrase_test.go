package screen

import (
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A line says the phrase its relations share. A bundle whose relations
// differ says the phrase the most carry, the first by name among equals,
// with 등 when at least half carry it and with 외 and how many do not
// otherwise, so one phrase of ten never passes for the line (rule 25). A
// phrase too wide keeps the names before its commas that fit, else the next
// phrase that fits speaks. A line with no phrase that fits falls back to the
// via word its relations share.
func TestALineSaysWhatCrossesIt(t *testing.T) {
	long := strings.Repeat("x", model.PhraseMax)
	tr := tree{
		vias: map[string]model.Via{"a": "call", "b": "call", "c": "call", "d": "data", "e": "call", "l": "call", "m": "call",
			"t1": "call", "t2": "call", "t3": "call", "t4": "call", "k": "config", "w": "call", "v": "call"},
		phrases: map[string]phrase{"a": {"Cypher", model.Inferred}, "b": {"Cypher", model.Inferred}, "c": {"schema fetch", model.UserStated},
			"l": {long, model.Inferred}, "m": {"WW" + long[2:], model.Inferred},
			"t1": {"delta", model.Inferred}, "t2": {"beta", model.Inferred}, "t3": {"gamma", model.Inferred}, "t4": {"alpha", model.Inferred},
			"k": {"COMP_WORDS, COMP_CWORD", model.Inferred}, "w": {"EnableDecoderUseNumber", model.Inferred}, "v": {"binding.Validator", model.Inferred}},
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
	if got := said(tr, []string{"c", "a", "b", "d", "x"}); !slices.Equal(got, []string{"Cypher (판독) 2/5", "schema fetch (확인) 1/5"}) {
		t.Errorf("the tooltip lists %v, want each phrase once with whose words it is and its share of the line", got)
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
