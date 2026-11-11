package screen

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Pointing at a shape keeps lit exactly the shapes a relation of the view
// joins it to, and every line with an end there, in every view of every
// real state. A twin is lit with its unit, and a chip that stands for
// several ports with every one of them.
func TestPointingAtAShapeKeepsWhatItJoins(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			d := v.d
			lit := map[string][]string{}
			for _, b := range d.Boxes {
				lit[string(base(model.ID(b.ID)))] = strings.Fields(b.Lit)
			}
			for _, p := range d.Ports {
				for _, k := range p.Keys {
					lit[k] = strings.Fields(p.Lit)
				}
			}
			own := map[string]string{}
			for id, cls := range lit {
				if len(cls) == 0 || cls[0][0] != 'p' {
					t.Fatalf("%s %s: %s takes no part in pointing", name, v.name, id)
				}
				own[id] = cls[0][1:]
			}
			want := map[string][]string{}
			for id := range lit {
				want[own[id]] = []string{"n" + own[id]}
			}
			for _, r := range d.Relations {
				a, b := string(base(model.ID(r[0]))), string(base(model.ID(r[1])))
				_, okA := own[a]
				_, okB := own[b]
				if okA && okB {
					want[own[a]] = append(want[own[a]], "n"+own[b])
					want[own[b]] = append(want[own[b]], "n"+own[a])
				}
			}
			for id, cls := range lit {
				got := slices.Clone(cls[1:])
				w := slices.Clone(want[own[id]])
				slices.Sort(got)
				slices.Sort(w)
				w = slices.Compact(w)
				if !slices.Equal(got, w) {
					t.Errorf("%s %s: pointing at %s keeps %v lit, its relations join %v", name, v.name, id, got, w)
				}
			}
			for _, w := range d.Wires {
				for _, e := range w.ends {
					if o, ok := own[string(base(model.ID(e)))]; ok && !slices.Contains(strings.Fields(w.Lit), "e"+o) {
						t.Errorf("%s %s: a line to %s does not light when it is pointed at", name, v.name, e)
					}
				}
			}
		}
	}
}

// pairs is an overview of 2n boxes where each of the first n uses two of
// the last n, so its lines meet on roads.
func pairs(n int) Model {
	var units []model.Unit
	for i := 1; i <= 2*n; i++ {
		var rels []model.Relation
		if i <= n {
			for _, j := range []int{n + i, n + 1 + i%n} {
				rels = append(rels, rel(model.ID(fmt.Sprintf("MOD-%04d", j)), model.DependsOn))
			}
		}
		units = append(units, called(model.ID(fmt.Sprintf("MOD-%04d", i)), fmt.Sprintf("m%d", i), rels...))
	}
	return Build(Input{Units: units})
}

// However many boxes and lines a drawing on roads holds, it keeps its
// routes, so pointing lights each line and no join on its roads reads as a
// coupling. The page carries a rule for each of its shapes and routes, and
// no more.
func TestALargeRoadDrawingKeepsItsRoutes(t *testing.T) {
	t.Parallel()
	for _, n := range []int{26} {
		m := pairs(n)
		d := m.Overview
		if !d.roads || len(d.Wires) == 0 {
			t.Fatalf("n=%d: the overview is not drawn on roads", n)
		}
		if len(d.Routes) != 2*n {
			t.Errorf("n=%d: %d boxes route %d of %d lines", n, len(d.Boxes), len(d.Routes), 2*n)
		}
		vl, err := Lint(m, "overview")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range vl {
			if j := v.Totals.Criteria["J3"]; j != 0 {
				t.Errorf("n=%d: %d false joins", n, j)
			}
		}
		page := pages(t, m)[PageFile]
		k := len(d.Routes)
		if !strings.Contains(page, fmt.Sprintf(".w.r%d:hover", k-1)) || strings.Contains(page, fmt.Sprintf(".w.r%d:hover", k)) {
			t.Errorf("n=%d: the page's rules do not light exactly its %d routes", n, k)
		}
		s := len(d.Boxes) + len(d.Ports)
		if !strings.Contains(page, fmt.Sprintf(":focus-visible>.p%d)", s-1)) || strings.Contains(page, fmt.Sprintf(":focus-visible>.p%d)", s)) {
			t.Errorf("n=%d: the page's rules do not name exactly its %d shapes", n, s)
		}
	}
}

// A drawing of 200 boxes and 200 routes, past the 48 shapes and 96 routes
// the rules once stopped at, still numbers every shape and lights every
// route.
func TestEveryShapeAndRouteTakesPart(t *testing.T) {
	var d Drawing
	for i := range 200 {
		id := fmt.Sprintf("MOD-%04d", i+1)
		d.Boxes = append(d.Boxes, Box{ID: id, X: 100 * i, W: 50, H: 20})
		d.Routes = append(d.Routes, Route{D: fmt.Sprintf("M%d 0 L%d 50", 100*i, 100*i), Kind: "dep", ends: []string{id}})
	}
	d.hover()
	if len(d.Routes) != 200 || len(d.numbers) != 200 {
		t.Fatalf("hover kept %d routes and numbered %d shapes, want 200 each", len(d.Routes), len(d.numbers))
	}
	if r := d.Routes[199].Lit; r != "e199 r199" {
		t.Errorf("the last route lights %q, want e199 r199", r)
	}
	if b := d.Boxes[199].Lit; !strings.HasPrefix(b, "p199") {
		t.Errorf("the last box is numbered %q, want p199", b)
	}
}

// Only a page whose drawing holds a shape to point at carries the pointing
// rules.
func TestOnlyPagesWithADrawingCarryThePointingRules(t *testing.T) {
	for name, page := range pages(t, Build(Input{Units: ripgrep()})) {
		has := strings.Contains(page, ":focus-visible>.p0")
		drawn := regexp.MustCompile(`[" ]p0[" ]`).MatchString(page)
		if has != drawn {
			t.Errorf("%s: pointing rules %v, drawing %v", name, has, drawn)
		}
	}
}
