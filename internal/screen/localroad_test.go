package screen

import (
	"bytes"
	"regexp"
	"strconv"
	"testing"
)

// Where a line leaves or arrives it rides a local road: every piece of
// road along a route's first or last run carries lines of that route's
// kind only, so each line keeps its own look at its ends, in every view
// of every fixture.
func TestARouteEndRidesOnlyItsKind(t *testing.T) {
	t.Parallel()
	route := regexp.MustCompile(`<g class="route (\S+)[^"]*\br(\d+)\b[^"]*"[^>]*><path class="way" d="([^"]*)"`)
	piece := regexp.MustCompile(`<path class="w ([^"]*)" d="([^"]*)"`)
	rider := regexp.MustCompile(`\br(\d+)\b`)
	for _, name := range []string{"orca", "pstack", "ripgrep", "self"} {
		for _, v := range views(fixture(t, name)) {
			if !v.d.roads {
				continue
			}
			var b bytes.Buffer
			if err := site.ExecuteTemplate(&b, "drawing", v.d); err != nil {
				t.Fatal(err)
			}
			kinds, ends := map[string]string{}, map[string][][2]fpt{}
			for _, m := range route.FindAllStringSubmatch(b.String(), -1) {
				p := svgPoints(m[3])
				kinds[m[2]] = m[1]
				ends[m[2]] = [][2]fpt{{p[0], p[1]}, {p[len(p)-2], p[len(p)-1]}}
			}
			for _, m := range piece.FindAllStringSubmatch(b.String(), -1) {
				rs := rider.FindAllStringSubmatch(m[1], -1)
				seen := map[string]bool{}
				for _, r := range rs {
					seen[kinds[r[1]]] = true
				}
				if len(seen) < 2 {
					continue
				}
				p := svgPoints(m[2])
				for _, r := range rs {
					for _, e := range ends[r[1]] {
						for k := 1; k < len(p); k++ {
							if sharedRun(p[k-1], p[k], e[0], e[1]) > 0 {
								t.Errorf("%s %s: r%s's end %v rides a piece of %d kinds: %s", name, v.name, r[1], e, len(seen), m[1])
							}
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
	t.Parallel()
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
