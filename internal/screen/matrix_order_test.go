package screen

import (
	"reflect"
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

func isPermutation(o []int, n int) bool {
	return len(o) == n && slices.Equal(slices.Sorted(slices.Values(o)), identity(n))
}

func k4Pairs(a, b, c, d int) [][2]int {
	return [][2]int{{a, b}, {a, c}, {a, d}, {b, c}, {b, d}, {c, d}}
}

// A gathered order is a permutation of the boxes, and two disjoint cliques are
// each gathered where their first box stood.
func TestGatheredOrderIsAPermutationOfDisjointCliques(t *testing.T) {
	t.Parallel()
	pairs := slices.Concat(k4Pairs(0, 2, 4, 6), k4Pairs(1, 3, 5, 7))
	got := gatherCliques(countOf(8, pairs, nil))
	if want := []int{0, 2, 4, 6, 1, 3, 5, 7}; !slices.Equal(got, want) {
		t.Errorf("two K4 gather to %v, want %v", got, want)
	}
	if !isPermutation(got, 8) {
		t.Errorf("%v is no permutation of 8 boxes", got)
	}
}

// Cliques that share boxes are taken largest first and never share one: a K5
// and a K4 sharing a box gather the K5, and the K4 loses the shared box and
// is left, being then smaller than four.
func TestOverlappingCliquesTakeTheLargestAndShareNoBox(t *testing.T) {
	t.Parallel()
	k5 := [][2]int{{0, 2}, {0, 4}, {0, 6}, {0, 8}, {2, 4}, {2, 6}, {2, 8}, {4, 6}, {4, 8}, {6, 8}}
	pairs := slices.Concat(k5, k4Pairs(8, 1, 3, 5))
	cliques := mutualCliques(countOf(9, pairs, nil))
	if want := [][]int{{0, 2, 4, 6, 8}}; !reflect.DeepEqual(cliques, want) {
		t.Fatalf("cliques = %v, want only the K5 %v", cliques, want)
	}
	got := gatherCliques(countOf(9, pairs, nil))
	if !isPermutation(got, 9) {
		t.Errorf("%v is no permutation of 9 boxes", got)
	}
}

// The gathered order is dropped when it takes the cells farther from the
// diagonal in all. The K4 on boxes 0, 3, 4, 5 is gathered to 0..3, which saves
// 12 on its own cells, but boxes 1 and 2 that lean on box 0 and 3 are thrown
// to the end, which costs 13.
func TestGatheringThatWorsensTheTableIsRejected(t *testing.T) {
	t.Parallel()
	count := countOf(6, k4Pairs(0, 3, 4, 5), [][2]int{{1, 0}, {0, 1}, {2, 0}, {0, 2}, {1, 2}, {2, 1}, {1, 3}})
	if distance(count, []int{0, 3, 4, 5, 1, 2}) <= distance(count, identity(6)) {
		t.Fatal("the fixture's gathered order is not worse")
	}
	if got := gatherCliques(count); got != nil {
		t.Errorf("a worse gathered order %v was kept", got)
	}
}
