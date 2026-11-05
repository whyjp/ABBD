package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A line says the phrase its relations share. A bundle whose relations
// differ says the phrase most of them carry, the first by name among equals,
// with 등, even when most carry none, since a bundle that names what crosses
// it reads better than one that names how (rule 25). A line with no phrase
// falls back to the via word its relations share.
func TestALineSaysWhatCrossesIt(t *testing.T) {
	tr := tree{
		vias:    map[string]model.Via{"a": "call", "b": "call", "c": "call", "d": "data", "e": "call"},
		phrases: map[string]phrase{"a": {"Cypher", model.Inferred}, "b": {"Cypher", model.Inferred}, "c": {"schema fetch", model.UserStated}},
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
		{[]string{"a", "c", "e"}, "Cypher 등"},
		{[]string{"a", "d", "c", "x"}, "Cypher 등"},
		{[]string{"a", "x"}, "Cypher 등"},
		{[]string{"c", "x", "y"}, "schema fetch 등"},
		{[]string{"d", "e"}, ""},
		{[]string{"e", "x"}, ""},
		{[]string{"d"}, "데이터"},
		{[]string{"x"}, ""},
	} {
		if got := word(tr, tc.rels); got != tc.want {
			t.Errorf("%v says %q, want %q", tc.rels, got, tc.want)
		}
	}
	if got := said(tr, []string{"c", "a", "b", "d"}); len(got) != 2 || got[0] != "Cypher (판독)" || got[1] != "schema fetch (확인)" {
		t.Errorf("the tooltip lists %v, want each phrase once with whose words it is", got)
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
