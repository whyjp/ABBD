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
}
