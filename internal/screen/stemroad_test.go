package screen

import "testing"

// Lines that leave one box by one stem the same way across turn off it at
// one height, even toward boxes of different rows, where the track one of
// them turns off onto also carries a line the other way: only the line
// going their way moves, so the opposite line keeps its own track.
func TestBranchesOfAStemTowardDifferentRowsLeaveAtOneHeight(t *testing.T) {
	t.Parallel()
	at := func(lo, hi int) end { return end{node: "box", lo: lo, hi: hi} }
	ways := [][]pt{
		{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
		{{50, 60}, {50, 116}, {600, 116}, {600, 400}},
		{{300, 60}, {300, 100}, {200, 100}, {200, 200}},
	}
	v := roadView{
		style: []string{"0", "0", "0"}, plan: []bool{false, false, false}, kinds: []string{"k", "k", "k"},
		ends: [][2]end{{at(0, 100), at(320, 400)}, {at(0, 100), at(560, 640)}, {at(270, 330), at(160, 240)}},
		from: []string{"p", "p", "c"}, to: []string{"a", "b", "d"}, dest: []int{3, 5, 3}, src: []int{1, 1, 1},
		position: func(y int) int { return y / 200 },
	}
	d := v
	d.dotting = true
	v, d = v.grouped(), d.grouped()
	r := routing{boxes: []box{{0, 0, 100, 60}, {320, 200, 80, 60}, {560, 400, 80, 60}, {270, 0, 60, 60}, {160, 200, 80, 60}}}
	next, ok := r.gather(ways, v, d, v.cost(ways))
	if !ok {
		t.Fatalf("the lines out of p stay at y=100 and y=116")
	}
	if a, b := next[0], next[1]; a[1].y != b[1].y {
		t.Errorf("the lines out of p turn off its stem at y=%d and y=%d, want one height: %v %v", a[1].y, b[1].y, a, b)
	}
	if o := next[2]; o[1].y == next[1][1].y {
		t.Errorf("the line from c shares a track with p's lines the other way: %v %v", o, next[1])
	}
}

// A branch that leaves an opposite line on its own track does not move onto
// a track where another line runs the other way: the stretch of opposite
// lines side by side is weighed on each track, not summed over the view.
func TestBranchesOfAStemFindNoNewOppositePartner(t *testing.T) {
	t.Parallel()
	at := func(lo, hi int) end { return end{node: "box", lo: lo, hi: hi} }
	ways := [][]pt{
		{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
		{{50, 60}, {50, 116}, {600, 116}, {600, 400}},
		{{300, 60}, {300, 100}, {200, 100}, {200, 200}},
		{{350, 60}, {350, 116}, {250, 116}, {250, 300}},
	}
	v := roadView{
		style: []string{"0", "0", "0", "0"}, plan: []bool{false, false, false, false}, kinds: []string{"k", "k", "k", "k"},
		ends: [][2]end{{at(0, 100), at(320, 400)}, {at(0, 100), at(560, 640)}, {at(270, 330), at(160, 240)}, {at(330, 370), at(230, 270)}},
		from: []string{"p", "p", "c", "e"}, to: []string{"a", "b", "d", "f"}, dest: []int{3, 5, 3, 3}, src: []int{1, 1, 1, 1},
		position: func(y int) int { return y / 200 },
	}
	d := v
	d.dotting = true
	v, d = v.grouped(), d.grouped()
	r := routing{boxes: []box{{0, 0, 100, 60}, {320, 200, 80, 60}, {560, 400, 80, 60}, {270, 0, 60, 60}, {160, 200, 80, 60}, {330, 0, 40, 60}, {230, 300, 40, 60}}}
	if next, ok := r.gather(ways, v, d, v.cost(ways)); ok {
		t.Errorf("a line out of p moved onto a track the line from e runs the other way: %v %v", next[0], next[1])
	}
}
