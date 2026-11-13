package screen

import "testing"

// A place whose lines leave on one stem, solid ones and a dotted one, beside
// a solid road the solid ones could ride: the solid ones join that road and
// the dotted one keeps its own track, where no solid ink hides its dots. A
// dotted branch never holds its solid neighbours off a road, and never rides
// under one.
func TestADottedBranchLetsItsSolidNeighboursJoinARoad(t *testing.T) {
	ways := [][]pt{
		{{100, 0}, {100, 40}, {600, 40}, {600, 80}},
		{{300, 0}, {300, 32}, {500, 32}, {500, 80}},
		{{300, 0}, {300, 32}, {150, 32}, {150, 56}},
	}
	box := func(lo, hi int) end { return end{node: "box", lo: lo, hi: hi} }
	v := roadView{
		style: []string{"0", "0", "0"}, plan: []bool{false, false, true}, kinds: []string{"k", "k", "k"},
		ends: [][2]end{{box(80, 120), box(580, 620)}, {box(280, 320), box(480, 520)}, {box(280, 320), box(130, 170)}},
		from: []string{"a", "s", "s"}, to: []string{"x", "y", "z"}, dest: []int{3, 3, 3},
		position: func(y int) int { return y / 16 },
	}.grouped()
	r := &routing{}
	for range roadMoves {
		next, ok := r.step(ways, v, v.cost(ways), false)
		if !ok {
			break
		}
		for i := range next {
			next[i] = straight(next[i])
		}
		ways = next
	}
	if y := ways[1][1].y; y != 40 {
		t.Errorf("the solid line from s turns at y=%d, want 40 on the road from a: %v", y, ways[1])
	}
	if y := ways[2][1].y; y == 40 {
		t.Errorf("the dotted line from s rides the solid road at y=40, where its dots do not show: %v", ways[2])
	}
}
