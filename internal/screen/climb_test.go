package screen

import (
	"strconv"
	"strings"
	"testing"
)

// A line that runs against the reading axis never shares a road with a line
// that does not where the two run opposite ways: in every fixture view,
// each stretch of ink that carries a climbing route and another route
// carries them all one way.
func TestLinesThatClimbShareRoadsOnlyTheirWay(t *testing.T) {
	t.Parallel()
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
					for h := range ways[true] {
						if ways[false][-h] {
							t.Errorf("%s %s: a line that climbs and one that does not run opposite ways on %v-%v", name, v.name, a, b)
						}
					}
				}
			}
		}
	}
}

// A line that climbs and one that does not, running one way into one place
// and dotted alike, come in on one road; kept apart (tactic.apart), or
// dotted differently, they keep a track each.
func TestAClimbJoinsARoadRunningItsWay(t *testing.T) {
	t.Parallel()
	src := func(node string, lo, hi int) end { return end{node: node, row: 0, lo: lo, hi: hi, key: node} }
	into := end{node: "t", row: 1, lo: 320, hi: 520, key: "t"}
	for _, c := range []struct {
		name          string
		apart, dotted bool
		joined        bool
	}{
		{"shared", false, false, true},
		{"apart", true, false, false},
		{"dotted differently", false, true, false},
	} {
		reqs := []wireReq{
			{from: src("a", 0, 100), to: into, kind: "depends_on", plan: c.dotted},
			{from: src("b", 270, 330), to: into, kind: "depends_on"},
		}
		r := routing{
			ways: [][]pt{
				{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
				{{300, 60}, {300, 116}, {360, 116}, {360, 200}},
			},
			back:  []bool{true, false},
			boxes: []box{{0, 0, 100, 60}, {270, 0, 60, 60}, {320, 200, 200, 60}},
		}
		r.confluent(reqs, func(y int) int { return y / 100 }, 2, c.apart, nil)
		a, b := r.ways[0], r.ways[1]
		if joined := a[len(a)-2].y == b[len(b)-2].y; joined != c.joined {
			t.Errorf("%s: the two lines into t on one road %v, want %v: %v %v", c.name, joined, c.joined, a, b)
		}
	}
}

// The road index counts a line that climbs and one that does not, run
// over each other on one track, as a clash only where they run opposite
// ways or one is dotted and the other is not, which would draw the shared
// ink solid and take the planned line's dots.
func TestAClimbAndAForwardLineClashOnlyOppositeWaysOrDottedApart(t *testing.T) {
	down := []pt{{40, 0}, {40, 100}}
	for _, c := range []struct {
		name    string
		climb   []pt
		planned bool
		want    int
	}{
		{"same way", []pt{{40, 20}, {40, 80}}, false, 0},
		{"opposite ways", []pt{{40, 80}, {40, 20}}, false, 60},
		{"same way, the climb planned", []pt{{40, 20}, {40, 80}}, true, 60},
	} {
		v := roadView{style: []string{"", ""}, back: []bool{true, false}, plan: []bool{c.planned, false}, kinds: []string{"k", "k"},
			ends: make([][2]end, 2), from: []string{"a", "b"}, to: []string{"c", "d"}, dest: []int{1, 1}, src: []int{0, 0},
			position: func(y int) int { return y / 16 }}.grouped()
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
