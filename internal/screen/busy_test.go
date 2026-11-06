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
