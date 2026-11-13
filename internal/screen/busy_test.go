package screen

import (
	"fmt"
	"testing"
)

// Two drawn lines from A to B and from A to D that drop side by side 8px
// apart are 60px of parallel ink, and no more once they share one stem;
// stretches of one path, or 12px apart, are not.
func TestParallelInkCountsLinesSideBySide(t *testing.T) {
	boxes := []string{"A 40 0 80 20", "B 0 140 40 20", "D 120 140 40 20"}
	rels := [][2]string{{"A", "B"}, {"A", "D"}}
	for _, c := range []struct {
		body string
		want int
	}{
		{`<path class="w contract" d="M76 20 V80 H20 V140"/><path class="w contract" d="M84 20 V80 H140 V140"/>`, 60},
		{`<path class="w contract" d="M80 20 V80 H20 V140"/><path class="w contract" d="M80 80 H140 V140"/>`, 0},
		{`<path class="w contract" d="M74 20 V80 H20 V140"/><path class="w contract" d="M86 20 V80 H140 V140"/>`, 0},
		{`<path class="w contract" d="M76 20 V80 H20 V140 M84 20 V80"/>`, 0},
		{`<path class="w contract" d="M80 20 V80 H20 V140"/><path class="w contract" d="M80 20 V80 H140 V140"/>`, 60},
	} {
		l := lookAt(t, boxes, c.body, rels)
		if got := parallelInk(l.s); got != c.want {
			t.Errorf("%s: parallel ink %d, want %d", c.body, got, c.want)
		}
	}
}

// A line from A down to B and one from B back up to A, 6px apart with each
// on its own right, are a two-lane road (#232): no parallel ink and none
// off the road. At the lane pitch of 8px, or each on the other's right,
// they are a line drawn twice, and two lines down 6px apart still are.
func TestTwoLaneRoadIsNoParallelInk(t *testing.T) {
	boxes := []string{"A 40 0 80 20", "B 40 140 80 20"}
	rels := [][2]string{{"A", "B"}, {"B", "A"}}
	down := func(x int) string { return fmt.Sprintf(`<path class="w contract" d="M%d 20 V140"/>`, x) }
	up := func(x int) string { return fmt.Sprintf(`<path class="w contract back" d="M%d 140 V20"/>`, x) }
	for _, c := range []struct {
		name, body string
		ink, off   int
	}{
		{"two-lane road", down(76) + up(82), 0, 0},
		{"at the lane pitch", down(76) + up(84), 120, 120},
		{"each on the other's right", down(82) + up(76), 120, 120},
		{"two lines down", down(76) + down(82), 120, 0},
	} {
		l := lookAt(t, boxes, c.body, rels)
		if got := parallelInk(l.s); got != c.ink {
			t.Errorf("%s: parallel ink %d, want %d", c.name, got, c.ink)
		}
		if got := twoLaneOff(l.s); got != c.off {
			t.Errorf("%s: two-lane ink off the road %d, want %d", c.name, got, c.off)
		}
	}
}

// The two lanes of a two-lane road stand closer than the pitch of lines
// that run near each other, and no closer than rule 6 lets two lines run,
// and the renderer draws them where the lint looks for them.
func TestTwoLaneGapIsUnderTheLanePitch(t *testing.T) {
	if !(pairGap < lane && pairGap >= minGap && float64(twoLaneGap) == pairGap) {
		t.Errorf("two-lane gap: renderer %dpx, lint %gpx, want equal, under the lane pitch %dpx and at least rule 6's %gpx", twoLaneGap, pairGap, lane, minGap)
	}
}

// Three drawn lines that drop side by side within 30px are lines abreast
// for as far as all three run together; two lines, or three spread wider,
// are not, and two groups of three apart count twice.
func TestAbreastInkCountsThreeLinesSideBySide(t *testing.T) {
	boxes := []string{"A 40 0 160 20", "B 0 140 40 20", "C 100 140 40 20", "D 200 140 40 20"}
	rels := [][2]string{{"A", "B"}, {"A", "C"}, {"A", "D"}}
	drop := func(x, to, bend int) string {
		return fmt.Sprintf(`<path class="w contract" d="M%d 20 V%d H%d V140"/>`, x, bend, to)
	}
	for _, c := range []struct {
		name, body string
		want       int
	}{
		{"three within 30px", drop(70, 20, 80) + drop(80, 120, 100) + drop(96, 220, 80), 60},
		{"two", drop(70, 20, 80) + drop(80, 120, 100), 0},
		{"three across 40px", drop(60, 20, 80) + drop(80, 120, 100) + drop(100, 220, 80), 0},
		{"two groups", drop(60, 60, 80) + drop(70, 70, 80) + drop(80, 80, 80) + drop(150, 150, 80) + drop(160, 160, 80) + drop(170, 170, 80), 240},
	} {
		l := lookAt(t, boxes, c.body, rels)
		if got := abreastInk(l.s); got != c.want {
			t.Errorf("%s: abreast ink %d, want %d", c.name, got, c.want)
		}
	}
}

// A fork from A into B and D of the row below splits off one track; its
// branches turning onto two tracks are one fork level too many.
func TestForkLevelsCountTracksPastTheFirst(t *testing.T) {
	boxes := []string{"A 60 0 40 20", "B 0 140 40 20", "D 120 140 40 20"}
	rels := [][2]string{{"A", "B"}, {"A", "D"}}
	fork := func(y int) string {
		return fmt.Sprintf(`<path class="w contract" d="M80 20 V80 H20 V140"/><path class="w contract" d="M80 80 V%d H140 V140"/><circle class="junction contract" cx="80" cy="80" r="3"/>`, y)
	}
	for y, want := range map[int]int{80: 0, 100: 1} {
		l := lookAt(t, boxes, fork(y), rels)
		if got := forkLevels(l); got != want {
			t.Errorf("track at %d: fork levels %d, want %d", y, got, want)
		}
	}
}

// Every straight piece of every drawn path counts as one segment.
func TestSegmentsCountStraightPieces(t *testing.T) {
	l := lookAt(t, []string{"A 60 0 40 20", "B 0 140 40 20"}, `<path class="w contract" d="M80 20 V80 H20 V140"/>`, [][2]string{{"A", "B"}})
	if got := len(inkStretches(l.s)); got != 3 {
		t.Fatalf("%d segments, want 3", got)
	}
}

// Two lines of one kind that leave A for B and D by stubs 8 or 30px apart
// are a comb of one tooth past the first; one stem, or stubs 60px apart,
// are none.
func TestCombsCountStubsOfOnePlaceSideBySide(t *testing.T) {
	boxes := []string{"A 20 0 140 20", "B 0 140 40 20", "D 140 140 40 20"}
	rels := [][2]string{{"A", "B"}, {"A", "D"}}
	for _, c := range []struct {
		left, right, want int
	}{{76, 84, 1}, {70, 100, 1}, {80, 80, 0}, {50, 110, 0}} {
		body := fmt.Sprintf(`<path class="w contract" d="M%d 20 V80 H20 V140"/><path class="w contract" d="M%d 20 V80 H160 V140"/>`, c.left, c.right)
		l := lookAt(t, boxes, body, rels)
		if got := combs(l.trace.Ways); got != c.want {
			t.Errorf("stubs at %d and %d: %d combs, want %d", c.left, c.right, got, c.want)
		}
	}
}
