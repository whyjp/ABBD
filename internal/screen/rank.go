package screen

import (
	"cmp"
	"math"
	"slices"

	"github.com/whyjp/abbd/internal/model"
)

// Rank is where a unit's box stands along the axis its view reads by, so a
// tool reads the boxes in the order a person sees them. Basis names the
// axis. On "dependency" the view reads top to bottom, each user above what
// it uses, and N is the row the box is drawn in, 1 at the top, of Of rows.
// Cycle says the box shares a cycle of depends_on relations with another
// box of the view: no dependency order runs between them, so the layout
// chose their rows and they carry no meaning.
type Rank struct {
	Basis string
	N, Of int
	Cycle bool `json:",omitempty"`
}

const byDependency = "dependency"

// ranks are the Rank of each box d draws, by its unit's ID.
func (t tree) ranks(d Drawing) map[string]Rank {
	row, rows := drawnRows(d.Boxes)
	cyclic := t.cyclic(row)
	out := map[string]Rank{}
	for id, r := range row {
		out[id] = Rank{Basis: byDependency, N: r + 1, Of: rows, Cycle: cyclic[id]}
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

// cyclic says which boxes stand in a strongly connected component of two
// or more boxes, by the depends_on relations of every unit each box holds.
func (t tree) cyclic(boxes map[string]int) map[string]bool {
	boxOf := func(id model.ID) (string, bool) {
		c := t.chain(id)
		for i := len(c) - 1; i >= 0; i-- {
			if _, ok := boxes[string(c[i])]; ok {
				return string(c[i]), true
			}
		}
		return "", false
	}
	uses := map[string][]string{}
	for id, u := range t.units {
		from, ok := boxOf(id)
		if !ok {
			continue
		}
		for _, r := range u.Relations {
			if to, ok := boxOf(r.To); ok && r.Kind == model.DependsOn && !r.Dead && to != from && !slices.Contains(uses[from], to) {
				uses[from] = append(uses[from], to)
			}
		}
	}
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
