package screen

import (
	"strconv"
	"testing"
)

// ambiguous counts the ambiguous ×N of one drawing.
func ambiguous(t *testing.T, svg string) int {
	t.Helper()
	s, err := readSVG(svg)
	if err != nil {
		t.Fatal(err)
	}
	return countAmbiguity(s, inkGraph(s))
}

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
		if got := ambiguous(t, draw(c.x)); got != c.want {
			t.Errorf("count at x %d: %d ambiguous, want %d", c.x, got, c.want)
		}
	}
}

// A count set apart from its path, here by a junction drawn between them,
// has no wire, so only the stretch test holds it: beside one line and clear
// of the next it reads as that line's.
func TestACountWithNoWireKeepsTheStretchTest(t *testing.T) {
	svg := `<svg width="300" height="220">` +
		`<path class="w depends_on" d="M100 0 L100 200"><title>a</title></path>` +
		`<circle class="dot" cx="100" cy="60" r="3"/>` +
		`<text class="count" x="150" y="105">×2</text>` +
		`<path class="w consumes" d="M156 0 L156 200"><title>b</title></path>` +
		`</svg>`
	if got := ambiguous(t, svg); got != 0 {
		t.Errorf("count with no wire: %d ambiguous, want 0", got)
	}
}

// The paths of one bus share a data-net, so a count drawn after one of them
// belongs to the whole net: beside another path of its own bus it is not
// across a foreign line.
func TestACountBelongsToItsWholeNet(t *testing.T) {
	svg := `<svg width="300" height="220">` +
		`<path class="w depends_on" data-net="n1" d="M100 0 L100 200"><title>a</title></path>` +
		`<text class="count" x="150" y="105">×2</text>` +
		`<path class="w depends_on" data-net="n1" d="M156 0 L156 200"><title>a</title></path>` +
		`</svg>`
	if got := ambiguous(t, svg); got != 0 {
		t.Errorf("count beside its own net: %d ambiguous, want 0", got)
	}
}
