package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A line says the phrase its relations share. A bundle whose relations
// differ says the phrase that names at least half of them, with 등, and
// otherwise falls back to the via word they share (rule 25).
func TestALineSaysWhatCrossesIt(t *testing.T) {
	tr := tree{
		vias:    map[string]model.Via{"a": "call", "b": "call", "c": "call", "d": "data"},
		phrases: map[string]string{"a": "Cypher", "b": "Cypher", "c": "schema fetch"},
	}
	for _, tc := range []struct {
		rels []string
		want string
	}{
		{[]string{"a", "b"}, "Cypher"},
		{[]string{"a", "b", "c"}, "Cypher 등"},
		{[]string{"a", "c"}, "Cypher 등"},
		{[]string{"a", "d", "c", "x"}, ""},
		{[]string{"a", "b", "d", "x"}, "Cypher 등"},
		{[]string{"c", "x", "y"}, ""},
		{[]string{"d"}, "데이터"},
		{[]string{"x"}, ""},
	} {
		if got := word(tr, tc.rels); got != tc.want {
			t.Errorf("%v says %q, want %q", tc.rels, got, tc.want)
		}
	}
	if got := said(tr, []string{"c", "a", "b", "d"}); len(got) != 2 || got[0] != "Cypher" || got[1] != "schema fetch" {
		t.Errorf("the tooltip lists %v, want each phrase once", got)
	}
}
