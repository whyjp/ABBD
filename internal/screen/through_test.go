package screen

import "testing"

// farChain lays out a line from A, in row 0, to Z, in row 2, whose chain
// the layout set down row 1 at x 1000, past the end of that row's boxes
// and 460px right of both ends, so riding the chain runs it round the row.
func farChain(through bool) (*layout, []wireReq) {
	l := &layout{
		pos:     map[string]rect{"A": {300, 0, 150, 60}, "M": {200, 100, 600, 60}, "Z": {300, 200, 150, 60}},
		rows:    [][]string{{"A"}, {"M"}, {"Z"}},
		chains:  []chain{{upper: "A", lower: "Z", top: 1, bottom: 1, lanes: 1}},
		pass:    map[[2]int]int{{0, 1}: 1000},
		width:   1200,
		through: through,
	}
	return l, []wireReq{{from: end{node: "A", row: 0, lo: 340, hi: 400}, to: end{node: "Z", row: 2, lo: 340, hi: 400}, kind: "contract"}}
}

// A line between two boxes rides the chain the layout set for it, however
// far off its way, unless the view is drawn through the rows' gaps: then
// it passes the rows on its own, as a line to a frame's chip always does.
func TestALineLeavesAFarChainOnlyWhenDrawnThroughTheGaps(t *testing.T) {
	l, reqs := farChain(false)
	if got := l.chainsOf(reqs)[0]; got != 0 {
		t.Errorf("drawn plainly, the line runs down chain %d, want its own chain 0", got)
	}
	l, reqs = farChain(true)
	if got := l.chainsOf(reqs)[0]; got != -1 {
		t.Errorf("drawn through the gaps, the line runs down chain %d, want none", got)
	}
}

// pickThrough is what choose keeps of a view whose kept drawing, on roads,
// has one line far out of its way, where the drawing through the rows'
// gaps has far such lines and reads gate totals.
func pickThrough(far int, gate []int) int {
	return choose([]tactic{{}}, func(t tactic, read *cost) (int, bool) {
		if t.through {
			read.far = far
			return 1, true
		}
		read.far, read.strays = 1, true
		return 0, true
	}, func(v int, read *cost) {
		read.gate = []int{0, 5, 5}
		if v == 1 {
			read.gate = gate
		}
	})
}

// The drawing through the rows' gaps takes the place of the drawing on
// roads only where fewer lines run far out of their way and no total the
// gate holds the view to reads worse.
func TestADrawingThroughTheGapsIsKeptOnlyWhereTheGateReadsItNoWorse(t *testing.T) {
	if got := pickThrough(0, []int{0, 5, 4}); got != 1 {
		t.Errorf("through the gaps, no line far and a total lower: kept %d, want the drawing through the gaps", got)
	}
	if got := pickThrough(0, []int{0, 4, 6}); got != 0 {
		t.Errorf("through the gaps, one total higher: kept %d, want the drawing on roads", got)
	}
	if got := pickThrough(1, []int{0, 4, 4}); got != 0 {
		t.Errorf("through the gaps, as many lines far: kept %d, want the drawing on roads", got)
	}
	if got := pickThrough(0, nil); got != 0 {
		t.Errorf("through the gaps, unread by lint: kept %d, want the drawing on roads", got)
	}
}
