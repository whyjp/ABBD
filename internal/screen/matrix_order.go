package screen

import "slices"

// A matrix reads best when boxes that all use one another stand in
// neighbouring rows: their cells then fill one block on the diagonal instead
// of lying scattered across the table. So after the boxes are put in the order
// the drawing shows them, each exact mutual clique of four boxes or more (every
// pair uses each other both ways) is gathered where its first box stood, its
// boxes keeping the drawn order among themselves. Cliques are taken largest
// first and never share a box; a near-clique is left where it is.
//
// The gathered order is kept only when it brings the table's cells nearer its
// diagonal, summed over every cell in use; otherwise the drawn order stays, so
// no view's table reads worse for it.
const cliqueMin = 4

// gatherCliques returns the order of 0..n-1 (indexes in drawn order) that
// puts each exact mutual clique together, or nil when no clique is found or
// the gathered order does not bring the cells nearer the diagonal. count[i][j]
// is how many relations box i uses box j by.
func gatherCliques(count [][]int) []int {
	n := len(count)
	chosen := mutualCliques(count)
	if len(chosen) == 0 {
		return nil
	}
	group := make([]int, n)
	for i := range group {
		group[i] = -1
	}
	for g, c := range chosen {
		for _, v := range c {
			group[v] = g
		}
	}
	var order []int
	done := make([]bool, len(chosen))
	for i := range n {
		g := group[i]
		switch {
		case g < 0:
			order = append(order, i)
		case !done[g]:
			done[g] = true
			order = append(order, chosen[g]...)
		}
	}
	if distance(count, order) >= distance(count, identity(n)) {
		return nil
	}
	return order
}

// mutualCliques are the exact mutual cliques of four boxes or more that
// count holds, largest first and never sharing a box, each in index order.
// The matrix gathers them, and a view's level reports them as a tangle.
func mutualCliques(count [][]int) [][]int {
	n := len(count)
	mutual := make([][]bool, n)
	for i := range mutual {
		mutual[i] = make([]bool, n)
		for j := range n {
			mutual[i][j] = i != j && count[i][j] > 0 && count[j][i] > 0
		}
	}
	// Bron-Kerbosch with a pivot lists each maximal clique once.
	var maximal [][]int
	var grow func(cur, cand, seen []int)
	grow = func(cur, cand, seen []int) {
		if len(cand) == 0 {
			if len(seen) == 0 && len(cur) >= cliqueMin {
				maximal = append(maximal, slices.Sorted(slices.Values(cur)))
			}
			return
		}
		pivot, best := -1, -1
		for _, u := range slices.Concat(cand, seen) {
			c := 0
			for _, w := range cand {
				if mutual[u][w] {
					c++
				}
			}
			if c > best {
				pivot, best = u, c
			}
		}
		for _, v := range slices.Clone(cand) {
			if mutual[pivot][v] {
				continue
			}
			var nc, ns []int
			for _, w := range cand {
				if mutual[v][w] {
					nc = append(nc, w)
				}
			}
			for _, w := range seen {
				if mutual[v][w] {
					ns = append(ns, w)
				}
			}
			grow(append(slices.Clone(cur), v), nc, ns)
			cand = slices.DeleteFunc(cand, func(w int) bool { return w == v })
			seen = append(seen, v)
		}
	}
	grow(nil, identity(n), nil)
	if len(maximal) == 0 {
		return nil
	}
	slices.SortStableFunc(maximal, func(a, b []int) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return slices.Compare(a, b)
	})
	taken := make([]bool, n)
	var chosen [][]int
	for _, c := range maximal {
		if slices.ContainsFunc(c, func(v int) bool { return taken[v] }) {
			continue
		}
		for _, v := range c {
			taken[v] = true
		}
		chosen = append(chosen, c)
	}
	return chosen
}

func identity(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}

// distance sums, over every cell in use, how far it stands from the diagonal
// when the boxes are taken in the given order.
func distance(count [][]int, order []int) int {
	at := make([]int, len(order))
	for p, v := range order {
		at[v] = p
	}
	sum := 0
	for i := range count {
		for j := range count[i] {
			if count[i][j] > 0 {
				sum += max(at[i]-at[j], at[j]-at[i])
			}
		}
	}
	return sum
}
