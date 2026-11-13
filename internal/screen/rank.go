package screen

import (
	"cmp"
	"math"
	"slices"
)

// Rank is where a unit's box stands along the axis its view reads by, so a
// tool reads the boxes in the order a person sees them. Basis names the
// axis, and the view reads top to bottom along it. N is the row the box is
// drawn in, 1 at the top, of Of rows. On "dependency" the layout layers the
// view's lines, each user above what it uses. On "stack" a stored stack
// pins its units' boxes to its layers, from the top, and every other box
// stands by the lines. On "flow" the box stands on the primary flow: N is
// its stage, from 1, of the Of stages stored, and Stage names it, whether
// the view lays the stages across one row or down its rows. Cycle says the
// box shares a cycle of the lines the layout layered with another box of
// the view: no order runs between them, so on "dependency" their rows are
// the layout's choice.
type Rank struct {
	Basis string
	N     int `json:",omitempty"`
	Of    int
	Stage string `json:",omitempty"`
	Cycle bool   `json:",omitempty"`
}

const (
	byDependency = "dependency"
	byStack      = "stack"
	byFlow       = "flow"
)

// axis is what a layout ranked its rows by: the basis and the places that
// share a cycle of the lines it layered.
type axis struct {
	basis string
	cycle map[string]bool
	// flow is how the view laid the primary flow out, nil when it shows
	// fewer than two stages.
	flow *flowAxis
}

// ranks are the Rank of each box d draws, by its unit's ID. A box on the
// primary flow p ranks by its stage, and every other box of a view that
// lays the flow out by its drawn row along the lines.
func ranks(d Drawing, p *PrimaryFlow) map[string]Rank {
	row, rows := drawnRows(d.Boxes)
	out := map[string]Rank{}
	for id, r := range row {
		rk := Rank{Basis: d.axis.basis, N: r + 1, Of: rows, Cycle: d.axis.cycle[id]}
		if f := d.axis.flow; f != nil && p != nil {
			rk.Basis = byDependency
			if n := f.stage[id]; n > 0 {
				rk.Basis, rk.N, rk.Of, rk.Stage = byFlow, n, len(p.Stages), p.Stages[n-1].Name
			}
		}
		out[id] = rk
	}
	return out
}

// drawnRows numbers the rows boxes are drawn in from the top, and counts
// them. A row is a band of boxes whose heights overlap, as lint reads it
// from the page (newLook).
func drawnRows(boxes []Box) (map[string]int, int) {
	bs := slices.Clone(boxes)
	slices.SortStableFunc(bs, func(a, b Box) int { return cmp.Compare(a.Y, b.Y) })
	row, rows, bottom := map[string]int{}, 0, math.MinInt
	for _, b := range bs {
		if b.Y >= bottom {
			rows++
		}
		bottom = max(bottom, b.Y+b.H)
		row[b.ID] = rows - 1
	}
	return row, rows
}

// cyclic says which places stand in a strongly connected component of two
// or more places of the graph uses draws.
func cyclic(uses map[string][]string) map[string]bool {
	out := map[string]bool{}
	for _, c := range components(uses) {
		if len(c) > 1 {
			for _, id := range c {
				out[id] = true
			}
		}
	}
	return out
}

// components are the strongly connected components of the graph uses
// draws, by Tarjan's algorithm.
func components(uses map[string][]string) [][]string {
	index, low, on := map[string]int{}, map[string]int{}, map[string]bool{}
	var stack []string
	var out [][]string
	var visit func(v string)
	visit = func(v string) {
		index[v], low[v] = len(index), len(index)
		stack, on[v] = append(stack, v), true
		for _, w := range uses[v] {
			if _, seen := index[w]; !seen {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if on[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			var c []string
			for {
				w := stack[len(stack)-1]
				stack, on[w] = stack[:len(stack)-1], false
				c = append(c, w)
				if w == v {
					break
				}
			}
			out = append(out, c)
		}
	}
	for v := range uses {
		if _, seen := index[v]; !seen {
			visit(v)
		}
	}
	return out
}
