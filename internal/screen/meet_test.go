package screen

import "testing"

// Lines from two boxes of one row into one box turn onto its stem at one
// height, a fork read from its other end: they come in on one flat road,
// rather than on two flat roads two lanes apart into the same corner. The
// two flat runs share too short a stretch for a long merge to take them.
func TestLinesIntoOnePlaceTurnOntoItsStemAtOneHeight(t *testing.T) {
	t.Parallel()
	src := func(node string, lo, hi int) end { return end{node: node, row: 0, lo: lo, hi: hi, key: node} }
	into := end{node: "t", row: 1, lo: 320, hi: 520, key: "t"}
	reqs := []wireReq{
		{from: src("a", 0, 100), to: into, kind: "depends_on"},
		{from: src("b", 270, 330), to: into, kind: "depends_on"},
	}
	r := routing{
		ways: [][]pt{
			{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
			{{300, 60}, {300, 116}, {360, 116}, {360, 200}},
		},
		back:  []bool{false, false},
		boxes: []box{{0, 0, 100, 60}, {270, 0, 60, 60}, {320, 200, 200, 60}},
	}
	r.confluent(reqs, func(y int) int { return y / 100 }, 2)
	a, b := r.ways[0], r.ways[1]
	if ya, yb := a[len(a)-2].y, b[len(b)-2].y; ya != yb {
		t.Errorf("the lines into t turn onto its stem at y=%d and y=%d, want one height: %v %v", ya, yb, a, b)
	}
	for _, w := range [][]pt{a, b} {
		if end := w[len(w)-1]; end != (pt{360, 200}) || w[len(w)-2].x != 360 {
			t.Errorf("a line into t no longer ends on its stem at x=360: %v", w)
		}
	}
}

// Levelling where lines meet never splits a fork where they leave: a
// route's one flat run is both, and the solid branches of a fork that a
// dotted branch turns off apart from stay on one track. Here moving s1 down
// to m's height would part it from its solid sibling s2, so m and the dotted
// branch on its track move up instead.
func TestLevellingWhereLinesMeetKeepsTheirSolidForkWhole(t *testing.T) {
	t.Parallel()
	at := func(lo, hi int) end { return end{node: "box", lo: lo, hi: hi} }
	ways := [][]pt{
		{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
		{{10, 60}, {10, 100}, {-200, 100}, {-200, 200}},
		{{50, 60}, {50, 116}, {640, 116}, {640, 200}},
		{{180, 60}, {180, 116}, {360, 116}, {360, 200}},
	}
	v := roadView{
		style: []string{"0", "0", "0", "0"}, plan: []bool{false, false, true, false}, kinds: []string{"k", "k", "k", "k"},
		ends:  [][2]end{{at(0, 100), at(320, 400)}, {at(0, 100), at(-240, -160)}, {at(0, 100), at(600, 680)}, {at(150, 210), at(320, 400)}},
		from:  []string{"p", "p", "p", "q"}, to: []string{"t", "u", "w", "t"}, dest: []int{3, 3, 3, 3}, src: []int{1, 1, 1, 1},
		position: func(y int) int { return y / 100 },
	}
	d := v
	d.dotting = true
	v, d = v.grouped(), d.grouped()
	r := routing{boxes: []box{{0, 0, 100, 60}, {150, 0, 60, 60}, {320, 200, 80, 60}, {-240, 200, 80, 60}, {600, 200, 80, 60}}}
	if next, ok := r.level(ways, v, d, v.cost(ways)); ok && next[0][1].y != next[1][1].y {
		t.Errorf("levelling the lines into t moved s1 off its solid sibling's track: %v %v", next[0], next[1])
	}
}
