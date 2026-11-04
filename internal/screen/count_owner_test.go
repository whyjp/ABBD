package screen

import (
	"strconv"
	"testing"
)

// A count drawn across another line reads as that line's, rule 22, even
// though it sits nearer that line's stretch than any other.
func TestACountAcrossAnotherLineIsAmbiguous(t *testing.T) {
	draw := func(countX int) string {
		return `<svg width="300" height="220">` +
			`<path class="w depends_on" d="M100 0 L100 200"><title>a</title></path>` +
			`<text class="count" x="` + strconv.Itoa(countX) + `" y="105">×2</text>` +
			`<path class="w consumes" d="M156 0 L156 200"><title>b</title></path>` +
			`</svg>`
	}
	for _, c := range []struct {
		x    int
		want int
	}{{104, 0}, {150, 1}} {
		s, err := readSVG(draw(c.x))
		if err != nil {
			t.Fatal(err)
		}
		if got := countAmbiguity(s, inkGraph(s)); got != c.want {
			t.Errorf("count at x %d: %d ambiguous, want %d", c.x, got, c.want)
		}
	}
}
