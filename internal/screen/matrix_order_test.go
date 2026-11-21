package screen

import (
	"slices"
	"testing"
)

func countOf(n int, mutual [][2]int, oneWay [][2]int) [][]int {
	c := make([][]int, n)
	for i := range c {
		c[i] = make([]int, n)
	}
	for _, p := range mutual {
		c[p[0]][p[1]], c[p[1]][p[0]] = 1, 1
	}
	for _, p := range oneWay {
		c[p[0]][p[1]] = 1
	}
	return c
}

// The boxes of an exact mutual clique of four or more are gathered where the
// first of them stood; a near-clique, three boxes, or an order that leaves the
// cells no nearer the diagonal stays as drawn.
func TestAMutualCliqueGathersIntoNeighbouringRows(t *testing.T) {
	t.Parallel()
	k4 := [][2]int{{0, 2}, {0, 4}, {0, 5}, {2, 4}, {2, 5}, {4, 5}}
	got := gatherCliques(countOf(6, k4, [][2]int{{1, 0}, {3, 5}}))
	if want := []int{0, 2, 4, 5, 1, 3}; !slices.Equal(got, want) {
		t.Errorf("K4 at 0,2,4,5 of 6 gathers to %v, want %v", got, want)
	}
	if got := gatherCliques(countOf(6, k4[:5], nil)); got != nil {
		t.Errorf("a clique missing one pair is gathered to %v", got)
	}
	if got := gatherCliques(countOf(5, [][2]int{{0, 1}, {0, 2}, {1, 2}}, nil)); got != nil {
		t.Errorf("a triangle is gathered to %v", got)
	}
	if got := gatherCliques(countOf(4, [][2]int{{0, 1}, {0, 2}, {0, 3}, {1, 2}, {1, 3}, {2, 3}}, nil)); got != nil {
		t.Errorf("a clique already in neighbouring rows is reordered to %v", got)
	}
}
