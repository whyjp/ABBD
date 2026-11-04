package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A line says the phrase its relations share. A bundle whose relations
// differ, and more than half of whose relations carry a phrase, says the
// phrase that names more than half of those, with 등. Otherwise it falls
// back to the via word they share (rule 25). Two different phrases on two
// relations name no majority, and one phrase among unphrased relations
// does not speak for them.
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
		{[]string{"a", "c"}, "호출"},
		{[]string{"c", "a"}, "호출"},
		{[]string{"a", "b", "c", "e"}, "Cypher 등"},
		{[]string{"a", "c", "e"}, "호출"},
		{[]string{"a", "d", "c", "x"}, ""},
		{[]string{"a", "b", "d", "x"}, ""},
		{[]string{"a", "x"}, ""},
		{[]string{"c", "x", "y"}, ""},
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
