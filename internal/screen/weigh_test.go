package screen

import "testing"

func unitEnd(node string, row int) end {
	return end{node: node, row: row, key: node}
}

// A line that ends on another line of its kind lets a reader follow it into
// the other line's box. That counts against a routing only when the line's
// place does not reach that box anyway.
func TestWeighCountsInkThatLeadsWhereNoLineGoes(t *testing.T) {
	reqs := []wireReq{
		{from: unitEnd("a", 0), to: unitEnd("x", 1), kind: "depends_on"},
		{from: unitEnd("b", 0), to: unitEnd("y", 1), kind: "depends_on"},
	}
	r := routing{drawn: []drawn{
		{parts: [][]pt{{{0, 0}, {0, 100}, {200, 100}}}, reqs: []int{0}, kind: "depends_on"},
		{parts: [][]pt{{{100, 40}, {100, 100}}}, reqs: []int{1}, kind: "depends_on"},
	}}
	if got := weigh(r, reqs).touches; got != 1 {
		t.Fatalf("touches %d; want 1 for b's line ending on a's", got)
	}
	reqs = append(reqs, wireReq{from: unitEnd("a", 0), to: unitEnd("y", 1), kind: "depends_on"}, wireReq{from: unitEnd("b", 0), to: unitEnd("x", 1), kind: "depends_on"})
	if got := weigh(r, reqs).touches; got != 0 {
		t.Fatalf("touches %d; want 0 once a and b both reach x and y", got)
	}
}

func TestWeighCountsTurnsAndJogsOnEachWay(t *testing.T) {
	r := routing{ways: [][]pt{{{0, 0}, {0, 50}, {10, 50}, {10, 100}}}}
	if got := weigh(r, nil).turns; got != 3 {
		t.Fatalf("turns %d; want two bends and one jog", got)
	}
}

// Shared buses are kept only when they spare turns and cost a reader
// nothing else.
func TestSharedBusesMustSpareTurnsAtNoOtherCost(t *testing.T) {
	plain := weight{touches: 0, crossings: 4, braid: 300, turns: 20}
	for _, c := range []struct {
		shared weight
		keep   bool
	}{
		{weight{0, 4, 300, 19}, true},
		{weight{0, 4, 300, 20}, false},
		{weight{1, 4, 300, 10}, false},
		{weight{0, 5, 300, 10}, false},
		{weight{0, 4, 301, 10}, false},
		{weight{0, 3, 200, 19}, true},
	} {
		if got := c.shared.beats(plain); got != c.keep {
			t.Errorf("%+v beats %+v = %v; want %v", c.shared, plain, got, c.keep)
		}
	}
}
