package screen

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// undrawnOwners are the boxes of d that list undrawn pairs, by ID.
func undrawnOwners(d Drawing) map[string]LeftOut {
	out := map[string]LeftOut{}
	for _, u := range d.Undrawn {
		out[strings.TrimPrefix(u.Href, "#"+focusID("", ""))] = u
	}
	return out
}

// undrawnPairs are the pairs of box IDs d's list of undrawn lines names.
func undrawnPairs(d Drawing) map[pair]bool {
	id := map[string]string{}
	for _, b := range d.Boxes {
		id[b.Name] = b.ID
	}
	out := map[pair]bool{}
	for owner, u := range undrawnOwners(d) {
		for _, w := range u.With {
			out[pairOf(model.ID(owner), model.ID(id[w]))] = true
		}
	}
	return out
}

// Each pair the overview counts is listed once, under one box, and no list
// stands where nothing is counted.
func TestEveryCountedPairIsListedOnce(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		m := fixture(t, name)
		for _, d := range m.drawings() {
			want := map[pair]bool{}
			for _, c := range d.Counted {
				if c[0] != c[1] {
					want[pairOf(model.ID(c[0]), model.ID(c[1]))] = true
				}
			}
			n := 0
			for _, u := range d.Undrawn {
				n += len(u.With)
			}
			if n != len(want) || d.UndrawnPairs() != n {
				t.Errorf("%s: %d pairs counted, %d listed", name, len(want), n)
			}
			if len(want) == 0 && len(d.Undrawn) > 0 {
				t.Errorf("%s: a list of undrawn lines where none is counted", name)
			}
		}
	}
}
