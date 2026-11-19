package screen

import "testing"

// A box over the middle of the two boxes it uses below stands centred; one
// over the left box stands off their middle by more than a quarter of their
// span; and a row whose boxes differ in height but share a top is off.
func TestSymmetryReadsCentresAndRows(t *testing.T) {
	t.Parallel()
	draw := func(px, kh int) Drawing {
		d := Drawing{Boxes: []Box{
			{ID: "p", X: px, Y: 0, W: 100, H: 40},
			{ID: "a", X: 0, Y: 100, W: 100, H: 40},
			{ID: "b", X: 200, Y: 100, W: 100, H: kh},
		}}
		for _, to := range []string{"a", "b"} {
			d.ways = append(d.ways, Route{ends: []string{"p", to}, rels: []string{"p" + to}})
		}
		return d
	}
	if s := symmetryOf(draw(100, 40)); s.parents != 1 || s.offCentre != 0 || s.rowsOff != 0 {
		t.Errorf("centred parent: %+v", s)
	}
	if s := symmetryOf(draw(0, 40)); s.parents != 1 || s.offCentre != 1 {
		t.Errorf("parent over the left box: %+v, want one off centre", s)
	}
	if s := symmetryOf(draw(100, 60)); s.rowsOff != 1 {
		t.Errorf("row of a 40px and a 60px box with one top: %+v, want one row off", s)
	}
}
