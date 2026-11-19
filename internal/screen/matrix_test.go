package screen

import (
	"bytes"
	"strings"
	"testing"
)

// A full view gets a matrix only when its joined pairs stand past
// 1/matrixPart of its own range and half its boxes share a cycle; the
// table has a row and a column per box, marks the diagonal, and marks a
// cell as a cycle only between two boxes that share one. The page draws the
// table under its drawing and no other page carries one.
func TestADenseCyclicViewAddsAMatrix(t *testing.T) {
	t.Parallel()
	seen := 0
	for _, name := range fixtures {
		m := fixture(t, name)
		ds := append([]*Drawing{&m.Overview}, insides(m.Insides)...)
		for _, d := range ds {
			mx := d.Matrix
			if mx == nil {
				continue
			}
			seen++
			n := len(d.Boxes)
			if n < matrixMin || 2*mx.Loops < n || len(mx.Heads) != n || len(mx.Rows) != n {
				t.Errorf("%s: matrix of %d heads and %d rows, %d loops, on %d boxes", name, len(mx.Heads), len(mx.Rows), mx.Loops, n)
				continue
			}
			for i, r := range mx.Rows {
				for j, c := range r.Cells {
					if c.Self != (i == j) {
						t.Errorf("%s: cell %d,%d self %v", name, i, j, c.Self)
					}
					if c.Loop && (c.N == 0 || !r.Head.Loop || !mx.Heads[j].Loop) {
						t.Errorf("%s: cell %s → %s marks a cycle its boxes do not share", name, r.Head.Name, mx.Heads[j].Name)
					}
				}
			}
		}
		for _, f := range m.Focus {
			if f.Drawing.Matrix != nil {
				t.Errorf("%s: focus view %s carries a matrix", name, f.ID)
			}
		}
	}
	// A view built here: four boxes in one cycle that all use each other.
	d := &Drawing{axis: axis{cycle: map[string]bool{"a": true, "b": true, "c": true, "d": true}}}
	for i, id := range []string{"a", "b", "c", "d"} {
		d.Boxes = append(d.Boxes, Box{ID: id, Name: id, X: 100 * i, W: 80, H: 40})
	}
	for _, e := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "a"}, {"a", "c"}} {
		d.ways = append(d.ways, Route{ends: []string{e[0], e[1]}, rels: []string{e[0] + e[1]}})
	}
	mx := d.matrix()
	if mx == nil || mx.Loops != 4 || !mx.Rows[0].Cells[1].Loop || mx.Rows[0].Cells[1].N != 1 {
		t.Fatalf("a four-box cycle with 5 of 6 pairs joined gets %+v", mx)
	}
	var b bytes.Buffer
	if err := site.ExecuteTemplate(&b, "matrix", mx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `<table class="dsm">`) || strings.Count(b.String(), "<tr>") != 5 {
		t.Errorf("matrix page part:\n%s", b.String())
	}
	d.ways = d.ways[:4]
	if d.matrix() != nil {
		t.Error("a four-box ring, the fewest pairs a joined view has, gets a matrix")
	}
	t.Logf("%d fixture views carry a matrix", seen)
}
