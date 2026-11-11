package screen

import (
	"path/filepath"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A bus says its word instead of its count only when every one of its lines
// says that word, so the word hides no other meaning. A plain via word is
// not said, so a bus whose lines share only it keeps its count.
func TestOnlyAnUncontestedWordTakesACount(t *testing.T) {
	tr := tree{
		vias: map[string]model.Via{"w1": "call", "w2": "call", "e1": "call", "e2": "call", "l1": "call", "l2": "call",
			"c1": "call", "c2": "call", "c3": "call", "d1": "data", "d2": "data", "d3": "data", "p1": "call"},
		phrases: map[string]phrase{"w1": {"write cache", model.Inferred}, "w2": {"write cache", model.Inferred},
			"e1": {"evict cache", model.Inferred}, "e2": {"evict cache", model.Inferred},
			"l1": {"load config", model.Inferred}, "l2": {"load config", model.Inferred}, "p1": {"ping", model.Inferred}},
	}
	for _, tc := range []struct {
		lines [][]string
		takes bool
	}{
		{[][]string{{"w1"}, {"w2"}, {"e1"}, {"e2"}}, false},
		{[][]string{{"l1"}, {"l2"}, {"e1"}}, false},
		{[][]string{{"c1"}, {"c2"}, {"d1"}, {"p1"}}, false},
		{[][]string{{"c1"}, {"c2"}, {"c3"}}, false},
		{[][]string{{"d1"}, {"d2"}, {"d3"}}, true},
		{[][]string{{"l1"}, {"l2"}, {"l1", "c1"}}, true},
	} {
		w, said := word(tr, tc.lines)
		if got := everyUnit(tr, tc.lines, w, said); got != tc.takes {
			t.Errorf("%v says %q, which takes its count: %v, want %v", tc.lines, w, got, tc.takes)
		}
	}
}

// Over every view of each fixture: no line shows ×1 or ×2; no bus shows
// its ×N beside the word every one of its lines says, drawn for it on its
// own ink or its net's; and a bus of three or more lines that gives up its
// ×N has that word drawn so.
func TestABusSaysItsWordOrItsCount(t *testing.T) {
	t.Cleanup(func() { dropCounts = true })
	dropCounts = false
	counted := buildEach(t, fresh)
	dropCounts = true
	drawn := buildEach(t, cached)
	for _, name := range fixtures {
		loaded, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(loaded)
		before := views(counted[name])
		for k, v := range views(drawn[name]) {
			d := v.d
			says := func(i int, s string) bool {
				for j, o := range d.Wires {
					if o.Label != nil && o.Label.S == s && (j == i || o.Net != 0 && o.Net == d.Wires[i].Net) {
						return true
					}
				}
				return false
			}
			for i, w := range d.Wires {
				was := before[k].d.Wires[i]
				s, said := word(tr, w.lines)
				full := everyUnit(tr, w.lines, s, said)
				switch {
				case w.Count == 1 || w.Count == 2:
					t.Errorf("%s %s: wire %d shows ×%d", name, v.name, i, w.Count)
				case w.Count >= 3 && full && says(i, s):
					t.Errorf("%s %s: wire %d shows ×%d beside its word %q", name, v.name, i, w.Count, s)
				case w.Count == 0 && was.Count >= 3 && !(full && says(i, s)):
					t.Errorf("%s %s: wire %d gave up ×%d but its word %q is not drawn for it", name, v.name, i, was.Count, s)
				}
			}
		}
	}
}
