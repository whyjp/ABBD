package screen

import (
	"strconv"
	"strings"
	"testing"
)

// A line that runs against the reading axis shares a road with lines that
// do not wherever the two run the same way, and never where they run
// opposite ways: in every fixture view, each stretch of ink that carries a
// climbing route and another route carries them all one way.
func TestLinesThatClimbShareRoadsOnlyTheirWay(t *testing.T) {
	t.Parallel()
	shared := 0
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			s, err := sheetOf(v.d)
			if err != nil {
				t.Fatal(err)
			}
			route := map[int]stroke{}
			for _, r := range s.routes {
				for _, c := range strings.Fields(r.class) {
					if k, ok := routeNumber(c); ok {
						route[k] = r
					}
				}
			}
			for _, w := range s.strokes {
				var rs []int
				climbs, not := false, false
				for _, c := range strings.Fields(w.class) {
					if k, ok := routeNumber(c); ok && k < len(v.d.Routes) {
						rs = append(rs, k)
						climbs, not = climbs || v.d.Routes[k].Back, not || !v.d.Routes[k].Back
					}
				}
				if !climbs || !not {
					continue
				}
				for i := 1; i < len(w.pts); i++ {
					a, b := w.pts[i-1], w.pts[i]
					ways := map[bool]map[int]bool{true: {}, false: {}}
					for _, k := range rs {
						r := route[k]
						for j := 1; j < len(r.pts); j++ {
							if p, q := r.pts[j-1], r.pts[j]; collinear(p, q, a, b) {
								ways[v.d.Routes[k].Back][int(fsign(q.x-p.x+q.y-p.y))] = true
							}
						}
					}
					if len(ways[true]) == 0 || len(ways[false]) == 0 {
						continue
					}
					shared++
					for h := range ways[true] {
						if ways[false][-h] {
							t.Errorf("%s %s: a line that climbs and one that does not run opposite ways on %v-%v", name, v.name, a, b)
						}
					}
				}
			}
		}
	}
	if shared == 0 {
		t.Error("no line that climbs shares a road with one that does not in any fixture")
	}
}

// The road index counts a line that climbs and one that does not, run
// over each other on one track, as a clash only where they run opposite
// ways.
func TestAClimbAndAForwardLineClashOnlyOppositeWays(t *testing.T) {
	v := roadView{style: []string{"", ""}, back: []bool{true, false}, plan: make([]bool, 2), kinds: []string{"k", "k"},
		ends: make([][2]end, 2), from: []string{"a", "b"}, to: []string{"c", "d"}, dest: []int{1, 1}, src: []int{0, 0},
		position: func(y int) int { return y / 16 }}.grouped()
	down := []pt{{40, 0}, {40, 100}}
	for _, c := range []struct {
		name  string
		climb []pt
		want  int
	}{
		{"same way", []pt{{40, 20}, {40, 80}}, 0},
		{"opposite ways", []pt{{40, 80}, {40, 20}}, 60},
	} {
		if got := newRoadIndex(v, [][]pt{c.climb, down}).cost.clash; got != c.want {
			t.Errorf("%s: clash %d, want %d", c.name, got, c.want)
		}
	}
}

// A view keeps its climbing lines on shared roads only where that reads
// no worse than keeping them apart on every total the gate holds and on
// crossings.
func TestClimbsShareRoadsOnlyWhereThatReadsNoWorse(t *testing.T) {
	apart := cost{gate: []int{1, 5, 3}, crossed: 4}
	for _, c := range []struct {
		name   string
		shared cost
		want   bool
	}{
		{"no worse anywhere", cost{gate: []int{1, 4, 3}, crossed: 4}, true},
		{"a gated total rises", cost{gate: []int{1, 6, 2}, crossed: 3}, false},
		{"more crossings", cost{gate: []int{1, 4, 3}, crossed: 5}, false},
		{"not judged", cost{crossed: 4}, false},
	} {
		if got := sharedOrApart(c.shared, apart); got != c.want {
			t.Errorf("%s: shared %v, want %v", c.name, got, c.want)
		}
	}
}

// routeNumber is the number of a class rN, the class that names route N.
func routeNumber(c string) (int, bool) {
	if !strings.HasPrefix(c, "r") {
		return 0, false
	}
	k, err := strconv.Atoi(c[1:])
	return k, err == nil
}
