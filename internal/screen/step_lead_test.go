package screen

import "testing"

// On internal/store's page the chips ← group and ← identity each provide
// ID high-water mark, and both lines would run down one chain to it, a
// lane apart. ← identity's line rides its chip's departure trunk, with its
// line to merge candidates file, into the box's gap; it leads the arrival
// bus there, so the drop stands under its trunk and it runs straight on
// down, while ← group's line passes the chip row on its own and joins on
// the track. Neither line jogs onto the drop.
func TestASteppingLineLeadsItsDrop(t *testing.T) {
	t.Parallel()
	for _, v := range views(fixture(t, "self")) {
		if v.name != "MOD-0041 internal/store" {
			continue
		}
		s, err := sheetOf(v.d)
		if err != nil {
			t.Fatal(err)
		}
		l := newLook(v, s)
		seen := 0
		for _, w := range l.trace.Ways {
			from := l.shapeName(w.from)
			if l.shapeName(w.to) != `box "ID high-water mark" (DATA-0018)` || from != `chip "← identity"` && from != `chip "← group"` {
				continue
			}
			seen++
			if n := len(w.pts) - 2; n > 2 {
				t.Errorf("%s to ID high-water mark turns %d times, want at most 2: %v", from, n, w.pts)
			}
			for j := 1; j+2 < len(w.pts); j++ {
				a, b := w.pts[j], w.pts[j+1]
				if d := abs(int(b.x-a.x)) + abs(int(b.y-a.y)); d < jogUnder {
					t.Errorf("%s to ID high-water mark jogs %dpx at %v: %v", from, d, a, w.pts)
				}
			}
		}
		if seen != 2 {
			t.Fatalf("want lines from ← identity and ← group to ID high-water mark, got %d", seen)
		}
		return
	}
	t.Fatal("no view MOD-0041 internal/store")
}
