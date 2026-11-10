package screen

import (
	"math"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// Where a line leaves or arrives it rides a local road: every piece of
// road along a route's first or last run carries lines of that route's
// kind only, and is dotted just when the route's relations all arise in
// design, unless it is too short to dot (planMin), so each line keeps its
// own look at its ends, in every view of every fixture.
func TestARouteEndRidesOnlyItsKind(t *testing.T) {
	for _, name := range fixtures {
		units, err := store.Open(filepath.Join("testdata", name)).LoadUnits()
		if err != nil {
			t.Fatal(err)
		}
		tr := newTree(units)
		for _, v := range views(fixture(t, name)) {
			if !v.d.roads {
				continue
			}
			for _, w := range v.d.Wires {
				ks := classNumbers(w.Lit, "r")
				kinds := map[string]bool{}
				for _, k := range ks {
					kinds[v.d.Routes[k].Kind] = true
				}
				p := svgPoints(w.D)
				long := 0.0
				for i := 1; i < len(p); i++ {
					long += math.Abs(p[i].x-p[i-1].x) + math.Abs(p[i].y-p[i-1].y)
				}
				for _, k := range ks {
					r := v.d.Routes[k]
					q := svgPoints(r.D)
					for _, e := range [][2]fpt{{q[0], q[1]}, {q[len(q)-2], q[len(q)-1]}} {
						rides := false
						for i := 1; i < len(p); i++ {
							rides = rides || sharedRun(p[i-1], p[i], e[0], e[1]) > 0
						}
						if !rides {
							continue
						}
						if len(kinds) > 1 {
							t.Errorf("%s %s: r%d's end %v rides a piece of %d kinds: %s", name, v.name, k, e, len(kinds), w.Lit)
						}
						// Lines between the same two places read as one line,
						// dotted only where all of them are (dress).
						plan, own := tr.planOnly(r.rels), true
						for _, o := range ks {
							own = own && (tr.planOnly(v.d.Routes[o].rels) == plan || slices.Equal(v.d.Routes[o].ends, r.ends))
						}
						if w.Plan != plan && (w.Plan || long >= planMin) && !own {
							t.Errorf("%s %s: r%d's end %v, dotted %v, rides a %.0fpx piece dotted %v: %s", name, v.name, k, e, plan, long, w.Plan, w.Lit)
						}
					}
				}
			}
		}
	}
}

func svgPoints(d string) []fpt {
	var out []fpt
	for _, m := range regexp.MustCompile(`[ML](-?[\d.]+) (-?[\d.]+)`).FindAllStringSubmatch(d, -1) {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		out = append(out, fpt{x, y})
	}
	return out
}

// sharedRun is how far two axis-aligned segments run on one another.
func sharedRun(a, b, c, d fpt) float64 {
	switch {
	case a.x == b.x && c.x == d.x && a.x == c.x:
		return min(max(a.y, b.y), max(c.y, d.y)) - max(min(a.y, b.y), min(c.y, d.y))
	case a.y == b.y && c.y == d.y && a.y == c.y:
		return min(max(a.x, b.x), max(c.x, d.x)) - max(min(a.x, b.x), min(c.x, d.x))
	}
	return 0
}

// A piece of road wears the kind most of its riders are: no other kind
// among the lines it carries outnumbers its own, in every view of every
// fixture.
func TestARoadPieceWearsItsRidersMajorityKind(t *testing.T) {
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			if !v.d.roads {
				continue
			}
			for _, w := range v.d.Wires {
				count := map[string]int{}
				for _, k := range classNumbers(w.Lit, "r") {
					count[v.d.Routes[k].Kind]++
				}
				for kind, n := range count {
					if n > count[w.Kind] {
						t.Errorf("%s %s: a %s piece carries %d %s lines and %d of its own: %s", name, v.name, w.Kind, n, kind, count[w.Kind], w.Lit)
					}
				}
			}
		}
	}
}
