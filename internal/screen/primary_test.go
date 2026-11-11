package screen

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// A dense overview draws its primary links, about one a box as the
// exemplars do, counts the rest on their boxes, and each count opens the
// focus view that draws them all.
func TestADenseOverviewCountsWhatItDoesNotDraw(t *testing.T) {
	t.Parallel()
	m := fixture(t, "pstack")
	d := m.Overview
	if 10*len(d.ways) > sparse*len(d.Boxes) {
		t.Errorf("the overview draws %d lines over %d boxes, past %d tenths a box", len(d.ways), len(d.Boxes), sparse)
	}
	touched := map[string]bool{}
	for _, r := range d.ways {
		for _, e := range r.ends {
			touched[e] = true
		}
	}
	counted := 0
	for _, b := range d.Boxes {
		if !touched[b.ID] {
			t.Errorf("box %s is drawn with no line", b.ID)
		}
		if b.More == 0 {
			continue
		}
		counted++
		if !slices.ContainsFunc(m.Focus, func(f Focus) bool { return "#"+f.ID == b.MoreHref }) {
			t.Errorf("box %s counts %d lines, and its count opens %q, no focus view", b.ID, b.More, b.MoreHref)
		}
	}
	if counted == 0 {
		t.Error("no box counts a line the overview does not draw")
	}
}

// A relation marked primary is drawn whatever the overview's own ranking
// would count instead.
func TestAMarkedRelationIsDrawn(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	drawn := func(d Drawing, a, b string) bool {
		return slices.ContainsFunc(d.ways, func(r Route) bool {
			return slices.Contains(r.ends, a) && slices.Contains(r.ends, b)
		})
	}
	parent := map[model.ID]model.ID{}
	for _, u := range units {
		parent[u.ID] = u.Parent
	}
	top := func(id model.ID) string {
		for parent[id] != "" {
			id = parent[id]
		}
		return string(id)
	}
	plain := fixture(t, "pstack").Overview
	for _, u := range units {
		for _, r := range u.Relations {
			a, b := top(u.ID), top(r.To)
			if r.Kind != model.DependsOn || r.Dead || a == b || drawn(plain, a, b) {
				continue
			}
			k := key(u.ID, r.Kind, r.To)
			got := Build(Input{Units: units, Primary: map[string]bool{k: true}}).Overview
			if !drawn(got, a, b) {
				t.Errorf("%s is marked primary and the overview does not draw %s to %s", k, a, b)
			}
			return
		}
	}
	t.Fatal("the pstack overview draws every top-level dependency")
}
