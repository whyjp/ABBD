package screen

import (
	"testing"
)

// A line runs straight into its arrowhead for the chevron's depth and 12px
// more, so a reader sees which way it enters before the head (user-19: the
// line into /abbd-setup turned 8px under its head).
func TestLinesEnterTheirHeadsStraight(t *testing.T) {
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			for _, w := range v.d.Wires {
				rs := runs(w.D)
				if w.Head == "" || len(rs) == 0 {
					continue
				}
				ends := [][2]pt{rs[len(rs)-1]}
				if w.Both {
					ends = append(ends, rs[0])
				}
				for _, s := range ends {
					if d := abs(s[0].x-s[1].x) + abs(s[0].y-s[1].y); d < straightIn {
						t.Errorf("%s %s: %s ends %dpx straight into its head, under %dpx", name, v.name, w.D, d, straightIn)
					}
				}
			}
		}
	}
}
