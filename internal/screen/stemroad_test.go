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
	v = v.grouped()
	r := routing{boxes: []box{{0, 0, 100, 60}, {320, 200, 80, 60}, {560, 400, 80, 60}, {270, 0, 60, 60}, {160, 200, 80, 60}}}
	next, ok := r.gather(ways, v, v.cost(ways))
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
	v = v.grouped()
	r := routing{boxes: []box{{0, 0, 100, 60}, {320, 200, 80, 60}, {560, 400, 80, 60}, {270, 0, 60, 60}, {160, 200, 80, 60}, {330, 0, 40, 60}, {230, 300, 40, 60}}}
	if next, ok := r.gather(ways, v, v.cost(ways)); ok {
		t.Errorf("a line out of p moved onto a track the line from e runs the other way: %v %v", next[0], next[1])
	}
}

// stemCase is a view of routes of one style and kind, each between two
// boxes, with rows 200px high, as gather weighs them.
func stemCase(ways [][]pt, ends [][2][2]int, from, to []string, dest []int, plan []bool, boxes []box) (routing, roadView) {
	n := len(ways)
	v := roadView{style: make([]string, n), plan: plan, kinds: make([]string, n), from: from, to: to, dest: dest, src: make([]int, n),
		position: func(y int) int { return y / 200 }}
	for i := range ways {
		v.style[i], v.kinds[i], v.src[i] = "0", "k", 1
		v.ends = append(v.ends, [2]end{{node: "box", lo: ends[i][0][0], hi: ends[i][0][1]}, {node: "box", lo: ends[i][1][0], hi: ends[i][1][1]}})
	}
	return routing{boxes: boxes}, v.grouped()
}

// A branch whose box stands back the way its stem came, past its flat run,
// is no sibling of the branches that go on: gathered onto their track it
// would run out and back again.
func TestABranchThatTurnsBackKeepsItsOwnTrack(t *testing.T) {
	t.Parallel()
	ways := [][]pt{
		{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
		{{50, 60}, {50, 116}, {600, 116}, {600, 60}},
	}
	r, v := stemCase(ways, [][2][2]int{{{0, 100}, {320, 400}}, {{0, 100}, {560, 640}}}, []string{"p", "p"}, []string{"a", "b"}, []int{3, 1},
		[]bool{false, false}, []box{{0, 0, 100, 60}, {320, 200, 80, 60}, {560, 0, 80, 60}})
	if next, ok := r.gather(ways, v, v.cost(ways)); ok {
		t.Errorf("the line back up to b was gathered with the line down to a: %v %v", next[0], next[1])
	}
}

// A branch moves onto a sibling's track only toward its own box: lifted
// onto a turn further from it, it would drop the longer way alone, down
// through rows it passed on the stem before.
func TestABranchGathersOnlyTowardItsOwnBox(t *testing.T) {
	t.Parallel()
	ways := [][]pt{
		{{50, 60}, {50, 116}, {600, 116}, {600, 400}},
		{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
	}
	r, v := stemCase(ways, [][2][2]int{{{0, 100}, {560, 640}}, {{0, 100}, {320, 400}}}, []string{"p", "p"}, []string{"b", "a"}, []int{5, 3},
		[]bool{false, false}, []box{{0, 0, 100, 60}, {560, 400, 80, 60}, {320, 200, 80, 60}})
	next, ok := r.gather(ways, v, v.cost(ways))
	if !ok {
		t.Fatalf("the lines out of p stay at y=116 and y=100")
	}
	if a, b := next[0][1].y, next[1][1].y; a != 116 || b != 116 {
		t.Errorf("the lines out of p turn off at y=%d and y=%d, want both at y=116, the turn nearer the box at y=200", a, b)
	}
}

// spread counts the heights past the first at which the branches of one
// stem turn, toward any row, one way across apart from the other, and
// leaves out a branch that turns back.
func TestSpreadCountsTheHeightsABranchesStemTurnsAt(t *testing.T) {
	t.Parallel()
	ways := [][]pt{
		{{50, 60}, {50, 100}, {360, 100}, {360, 200}},
		{{50, 60}, {50, 116}, {600, 116}, {600, 400}},
		{{50, 60}, {50, 132}, {800, 132}, {800, 60}},
		{{50, 60}, {50, 140}, {-200, 140}, {-200, 200}},
	}
	_, v := stemCase(ways, [][2][2]int{{{0, 100}, {320, 400}}, {{0, 100}, {560, 640}}, {{0, 100}, {760, 840}}, {{0, 100}, {-240, -160}}},
		[]string{"p", "p", "p", "p"}, []string{"a", "b", "c", "e"}, []int{3, 5, 1, 3}, []bool{false, false, false, false}, nil)
	if got := v.spread(ways); got != 1 {
		t.Errorf("spread %d, want 1: the lines to a and b turn at two heights; c turns back and e leaves the other way", got)
	}
	ways[1] = []pt{{50, 60}, {50, 100}, {600, 100}, {600, 400}}
	if got := v.spread(ways); got != 0 {
		t.Errorf("spread %d, want 0 with the lines to a and b at one height", got)
	}
}

// A move that sets one branch on its sibling's height but leaves another
// at the height it left gathers nothing: the stem still turns at as many
// heights, so the branch keeps its own.
func TestABranchMovesOnlyWhereTheStemTurnsAtFewerHeights(t *testing.T) {
	t.Parallel()
	ways := [][]pt{
		{{100, 60}, {100, 100}, {300, 100}, {300, 200}},
		{{100, 60}, {100, 116}, {450, 116}, {450, 200}},
		{{700, 60}, {700, 100}, {900, 100}, {900, 200}},
	}
	r, v := stemCase(ways, [][2][2]int{{{0, 1000}, {260, 340}}, {{0, 1000}, {410, 490}}, {{0, 1000}, {860, 940}}}, []string{"p", "p", "p"}, []string{"a", "b", "c"},
		[]int{3, 3, 3}, []bool{false, false, false}, []box{{0, 0, 1000, 60}, {260, 200, 80, 60}, {410, 200, 80, 60}, {860, 200, 80, 60}})
	if next, ok := r.gather(ways, v, v.cost(ways)); ok {
		t.Errorf("a line out of p moved while the stem still turns at y=100 and y=116: %v %v %v", next[0], next[1], next[2])
	}
}
