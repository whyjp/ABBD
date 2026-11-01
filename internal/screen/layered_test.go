package screen

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// A drawing too wide for its budget gives up chains, and the boxes it then
// places all stand on the canvas at their own width, apart from each other.
func TestGivingUpChainsKeepsEveryBoxOnTheCanvas(t *testing.T) {
	nodes := []lnode{{id: "T", w: 600, h: 40}}
	var edges []ledge
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		nodes = append(nodes, lnode{id: id, w: 140, h: 40}, lnode{id: "m" + id, w: 140, h: 40})
		edges = append(edges, ledge{"T", id}, ledge{id, "m" + id})
	}
	for i := 0; i < 8; i++ {
		id := fmt.Sprint("L", i)
		nodes = append(nodes, lnode{id: id, w: 60, h: 40})
		for k := 0; k < 5; k++ {
			edges = append(edges, ledge{"T", id})
		}
		edges = append(edges, ledge{"me", id})
	}
	l := arrange(nodes, edges, 920)
	if l.width > 920 {
		t.Errorf("width %d past the budget 920", l.width)
	}
	for _, n := range nodes {
		p := l.pos[n.id]
		if p.W != n.w || p.X < padX || p.X+p.W > l.width-padX {
			t.Errorf("%s at %d..%d, %d wide, on a canvas %d wide", n.id, p.X, p.X+p.W, n.w, l.width)
		}
	}
	for _, row := range l.rows {
		for i := 1; i < len(row); i++ {
			a, b := l.pos[row[i-1]], l.pos[row[i]]
			if a.X+a.W+gapX > b.X {
				t.Errorf("%s at %d..%d crowds %s at %d", row[i-1], a.X, a.X+a.W, row[i], b.X)
			}
		}
	}
}

// Laying out a large random graph stays fast: placing every chain once,
// not once per chain given up.
func TestALargeGraphLaysOutQuickly(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	var nodes []lnode
	for i := 0; i < 200; i++ {
		nodes = append(nodes, lnode{id: fmt.Sprint("n", i), w: 80 + r.Intn(100), h: 40})
	}
	seen := map[ledge]bool{}
	var edges []ledge
	for len(edges) < 400 {
		a, b := r.Intn(200), r.Intn(200)
		e := ledge{fmt.Sprint("n", min(a, b)), fmt.Sprint("n", max(a, b))}
		if a != b && !seen[e] {
			seen[e] = true
			edges = append(edges, e)
		}
	}
	start := time.Now()
	arrange(nodes, edges, 1320)
	if took := time.Since(start); took > time.Second {
		t.Errorf("200 boxes and 400 lines took %v to lay out, past 1s", took)
	}
}
