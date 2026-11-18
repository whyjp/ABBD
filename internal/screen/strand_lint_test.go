package screen

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// A path that starts as strands lints as the whole path did: lint reads
// the path from the stem the page writes on it, the strands are no ink, a
// mark on a strand marks its line, and a count drawn with the path stays
// its own. Only the fans and strands lint counts tell the two apart.
func TestStrandsLintAsTheirPath(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		d := fixture(t, name).Overview
		v := view{name: "overview", d: d, budget: overviewWidth, room: screenH}
		plain, err := lintView(v)
		if err != nil {
			t.Fatal(err)
		}
		v.d.Wires = slices.Clone(d.Wires)
		fans, counted, marked := 0, 0, 0
		for i := range v.d.Wires {
			w := &v.d.Wires[i]
			rs := runs(w.D)
			if strings.Count(w.D, "M") != 1 || len(rs) == 0 || rs[0][0].x != rs[0][1].x || abs(rs[0][1].y-rs[0][0].y) < 20 {
				continue
			}
			a, dir := rs[0][0], 1
			if rs[0][1].y < a.y {
				dir = -1
			}
			knot := pt{a.x, a.y + dir*13}
			trunk := polyline(append([]pt{knot}, func() []pt {
				var ps []pt
				for _, r := range rs {
					ps = append(ps, r[1])
				}
				return ps
			}()...))
			fan := &Fan{Trunk: trunk, Stem: fmt.Sprintf("%d %d", a.x, a.y)}
			for _, dx := range []int{-8, 8} {
				fan.Strands = append(fan.Strands, Strand{D: fmt.Sprintf("M%d %d C%d %d %d %d %d %d", a.x+dx, a.y, a.x+dx, a.y+dir*8, a.x, a.y+dir*5, knot.x, knot.y), Lit: w.Lit})
			}
			w.Fan = fan
			w.Marks = slices.Clone(w.Marks)
			for k := range w.Marks {
				if w.Marks[k].X == a.x && w.Marks[k].Y == a.y {
					w.Marks[k].X = a.x - 8
					marked++
				}
			}
			if w.Count > 0 {
				counted++
			}
			fans++
		}
		if fans == 0 {
			t.Fatalf("%s: no path to fan", name)
		}
		was, err := sheetOf(d)
		if err != nil {
			t.Fatal(err)
		}
		now, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(now.strokes, was.strokes) {
			t.Errorf("%s: lint reads the fanned paths' ink otherwise", name)
		}
		got, err := lintView(v)
		if err != nil {
			t.Fatal(err)
		}
		if got.Totals.Fans != fans || got.Totals.Strands != 2*fans {
			t.Errorf("%s: lint counts %d fans and %d strands, want %d and %d", name, got.Totals.Fans, got.Totals.Strands, fans, 2*fans)
		}
		got.Totals.Fans, got.Totals.Strands = 0, 0
		if !reflect.DeepEqual(got.Totals, plain.Totals) {
			t.Errorf("%s: fanning %d paths (%d counted, %d marks moved) changed the totals\n got %+v\nwant %+v", name, fans, counted, marked, got.Totals, plain.Totals)
		}
		if !reflect.DeepEqual(got.Breaches, plain.Breaches) {
			t.Errorf("%s: fanning changed the breaches\n got %v\nwant %v", name, got.Breaches, plain.Breaches)
		}
		t.Logf("%s: %d fans, %d with a count, %d marks moved onto a strand", name, fans, counted, marked)
	}
}

// A strand is drawn only for a path that has a fan: a drawing with none
// writes no stem and no strand.
func TestNoFanDrawsNoStrand(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		s, err := svgOf(fixture(t, name).Overview)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(s, "data-stem") || strings.Contains(s, `class="strand`) || strings.Contains(s, `class="ply`) {
			t.Errorf("%s: a drawing with no fan writes a stem or a strand", name)
		}
	}
}

func svgOf(d Drawing) (string, error) {
	var b strings.Builder
	err := site.ExecuteTemplate(&b, "drawing", d)
	return b.String(), err
}
