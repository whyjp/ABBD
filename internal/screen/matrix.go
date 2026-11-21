package screen

import (
	"cmp"
	"fmt"
	"slices"
)

// A view whose boxes use each other many times over, round cycles, reads
// poorly as lines: a reader asking which boxes form one cycle, or what two
// boxes both use, follows every line through every other. A dependency
// structure matrix answers those at a glance (Sangal et al. 2005), while
// the drawing still answers what connects to what and in which order
// (Okoe et al. 2019). So such a view keeps its drawing as it is and adds a
// table below it: one row and one column per box in the order a reader
// meets them, boxes that all use one another gathered into neighbouring rows
// (matrix_order.go), and in each cell how many stored relations the row's box
// uses the column's by.
//
// Which views get one is read against each view's own range, never a fixed
// count. A view of n boxes that joins them at all joins at least n-1 pairs
// and at most n(n-1)/2, so its density is where its joined pairs stand
// between those two. A view gets the table when that density stands past
// 1/matrixPart of its range and at least half of its boxes share a cycle of
// the lines its layout layered (axis.cycle, graph.json's Cycle).

// Matrix is the table a dense cyclic view adds below its drawing.
type Matrix struct {
	Heads []MatrixHead
	Rows  []MatrixRow
	// Loops is how many of the boxes share a cycle.
	Loops int
}

// MatrixHead is one box as a row or a column names it: its number in
// reading order, its name, and whether it shares a cycle.
type MatrixHead struct {
	N    int
	ID   string
	Name string
	Loop bool
}

// MatrixRow is one box and what it uses: a cell per column.
type MatrixRow struct {
	Head  MatrixHead
	Cells []MatrixCell
}

// MatrixCell is how many stored relations the row's box uses the column's
// by, 0 for none. Self marks the diagonal and Loop a use the column's box
// reaches back along the drawn lines, between two boxes the layout found on
// a cycle, so the two stand on one cycle. Title says it in words.
type MatrixCell struct {
	N     int
	Self  bool
	Loop  bool
	Title string
}

// A view's joined pairs must stand past 1/matrixPart of its range of
// density before it gets a matrix, and it must draw matrixMin boxes at
// least, as for level L's lines per box. Across the 99 full views of four
// boxes or more that 18 evaluation sets draw, 20 have half their boxes on a
// cycle. Their density stands at 19% of its range or less in all but five:
// a four-box ring at exactly a third, which reads plainly as lines, and four
// views between 40% and 67%, in four repositories, which past a third picks.
const (
	matrixPart = 3
	matrixMin  = 4
)

// matrix is d's table, or nil where d is not dense and cyclic enough.
func (d *Drawing) matrix() *Matrix {
	row, _ := drawnRows(d.Boxes)
	boxes := slices.Clone(d.Boxes)
	slices.SortStableFunc(boxes, func(a, b Box) int {
		return cmp.Or(cmp.Compare(row[a.ID], row[b.ID]), cmp.Compare(a.X, b.X))
	})
	n := len(boxes)
	count, reach, joined := d.tally(boxes)
	loops := 0
	for _, b := range boxes {
		if d.axis.cycle[b.ID] {
			loops++
		}
	}
	lo, hi, p := n-1, n*(n-1)/2, len(joined)
	if n < matrixMin || hi <= lo || matrixPart*(p-lo) <= hi-lo || 2*loops < n {
		return nil
	}
	if order := gatherCliques(count); order != nil {
		gathered := make([]Box, n)
		for p, v := range order {
			gathered[p] = boxes[v]
		}
		boxes = gathered
		count, reach, _ = d.tally(boxes)
	}
	for k := range n {
		for i := range n {
			for j := range n {
				reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
			}
		}
	}
	m := &Matrix{Loops: loops}
	for i, b := range boxes {
		name := b.Name
		if name == "" {
			name = b.ID
		}
		m.Heads = append(m.Heads, MatrixHead{N: i + 1, ID: b.ID, Name: name, Loop: d.axis.cycle[b.ID]})
	}
	for i, h := range m.Heads {
		r := MatrixRow{Head: h}
		for j, c := range m.Heads {
			cell := MatrixCell{N: count[i][j], Self: i == j}
			if cell.N > 0 {
				cell.Loop = reach[j][i] && h.Loop && c.Loop
				cell.Title = fmt.Sprintf("%s → %s: 관계 %d개", h.Name, c.Name, cell.N)
				if cell.Loop {
					cell.Title += ", 서로 이어지는 순환 의존"
				}
			}
			r.Cells = append(r.Cells, cell)
		}
		m.Rows = append(m.Rows, r)
	}
	return m
}

// tally counts, for boxes in the order given, how many stored relations each
// box uses each other by, which of them reach, and which pairs are joined.
func (d *Drawing) tally(boxes []Box) (count [][]int, reach [][]bool, joined map[[2]int]bool) {
	n := len(boxes)
	at := map[string]int{}
	for i, b := range boxes {
		at[b.ID] = i
	}
	count, reach = make([][]int, n), make([][]bool, n)
	for i := range count {
		count[i], reach[i] = make([]int, n), make([]bool, n)
	}
	joined = map[[2]int]bool{}
	for _, u := range d.uses() {
		i, ok := at[u.from]
		j, ok2 := at[u.to]
		if !ok || !ok2 || i == j {
			continue
		}
		count[i][j] += u.weight
		reach[i][j] = true
		joined[[2]int{min(i, j), max(i, j)}] = true
	}
	return count, reach, joined
}
