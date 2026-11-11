package screen

import (
	"bytes"
	"path/filepath"
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// cutOn builds a fixture with the overview's density cut switched on.
func cutOn(t *testing.T, name string, marked map[string]bool) Model {
	t.Helper()
	units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	on := true
	return Build(Input{Units: units, Primary: marked, Project: store.Project{OverviewCut: &on}})
}

// With the cut switched on, a dense overview draws its primary links, about
// one a box as the exemplars do, counts the rest on their boxes, and each
// count opens the focus view that draws them all. Each road still lights
// exactly its riders, and no count stands by an arrowhead. Switched off, the
// overview counts nothing.
func TestADenseOverviewCountsWhatItDoesNotDraw(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"pstack", "self"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			plain := drawAll(t, name).Overview
			if slices.ContainsFunc(plain.Boxes, func(b Box) bool { return b.More > 0 }) {
				t.Error("the overview counts lines with the cut switched off")
			}
			m := fixture(t, name)
			d := m.Overview
			if 10*len(d.ways) > sparse*len(d.Boxes) || len(d.ways) >= len(plain.ways) {
				t.Errorf("the overview draws %d lines over %d boxes, against %d uncut, past %d tenths a box", len(d.ways), len(d.Boxes), len(plain.ways), sparse)
			}
			touched := func(d Drawing) map[string]bool {
				out := map[string]bool{}
				for _, r := range d.ways {
					for _, e := range r.ends {
						out[e] = true
					}
				}
				return out
			}
			joined, kept := touched(plain), touched(d)
			counted := 0
			for _, b := range d.Boxes {
				if joined[b.ID] && !kept[b.ID] {
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
			for _, v := range views(m) {
				lightsItsRiders(t, v.name, v.d)
				var b bytes.Buffer
				if err := site.ExecuteTemplate(&b, "drawing", v.d); err != nil {
					t.Fatal(err)
				}
				if c := countsByHeads(b.String()); len(c) > 0 {
					t.Errorf("%s: counts by arrowheads %q", v.name, c)
				}
			}
		})
	}
}

// With the cut switched on, a relation marked primary is drawn whatever
// the overview's own ranking would count instead.
func TestAMarkedRelationIsDrawn(t *testing.T) {
	t.Parallel()
	units, err := store.Open(filepath.Join("testdata", "pstack")).LoadUnits()
	if err != nil {
		t.Fatal(err)
	}
	plain := fixture(t, "pstack").Overview
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
	for _, u := range units {
		for _, r := range u.Relations {
			a, b := top(u.ID), top(r.To)
			if r.Kind != model.DependsOn || r.Dead || a == b || drawn(plain, a, b) {
				continue
			}
			k := key(u.ID, r.Kind, r.To)
			got := cutOn(t, "pstack", map[string]bool{k: true}).Overview
			if !drawn(got, a, b) {
				t.Errorf("%s is marked primary and the overview does not draw %s to %s", k, a, b)
			}
			return
		}
	}
	t.Fatal("the pstack overview draws every top-level dependency")
}

// With the cut switched on, every relation the overview stops drawing is
// counted on the pill of a box at one of its ends, and the focus view each
// such pill opens draws it.
func TestACountedRelationIsDrawnWhereItsPillLeads(t *testing.T) {
	t.Parallel()
	rels := func(d Drawing) map[string]bool {
		out := map[string]bool{}
		for _, w := range d.Wires {
			for _, r := range w.Rels {
				out[r] = true
			}
		}
		for _, p := range d.Ports {
			for _, r := range p.Rels {
				out[r] = true
			}
		}
		return out
	}
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
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
		ends := map[string][2]string{}
		for _, u := range units {
			for _, r := range u.Relations {
				ends[key(u.ID, r.Kind, r.To)] = [2]string{top(u.ID), top(r.To)}
			}
		}
		m := fixture(t, name)
		drawn := rels(m.Overview)
		pill := map[string]string{}
		for _, b := range m.Overview.Boxes {
			if b.More > 0 {
				pill[b.ID] = b.MoreHref
			}
		}
		focus := map[string]map[string]bool{}
		for _, f := range m.Focus {
			focus["#"+f.ID] = rels(f.Drawing)
		}
		counted := 0
		for r := range rels(drawAll(t, name).Overview) {
			if drawn[r] {
				continue
			}
			counted++
			led := false
			for _, e := range ends[r] {
				href, ok := pill[e]
				if !ok {
					continue
				}
				led = true
				if !focus[href][r] {
					t.Errorf("%s: %s is counted on %s's pill, and %s does not draw it", name, r, e, href)
				}
			}
			if !led {
				t.Errorf("%s: the overview neither draws %s nor counts it on a pill at %v", name, r, ends[r])
			}
		}
		if (counted > 0) != (len(pill) > 0) {
			t.Errorf("%s: the overview leaves %d relations undrawn and %d boxes count some", name, counted, len(pill))
		}
		t.Logf("%s: %d relations counted on pills", name, counted)
	}
}
