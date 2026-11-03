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
	l := arrange(nodes, edges, 920, nil)
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
// randomGraph is n boxes joined by m lines from a fixed seed.
func randomGraph(seed int64, n, m int) ([]lnode, []ledge) {
	r := rand.New(rand.NewSource(seed))
	var nodes []lnode
	for i := 0; i < n; i++ {
		nodes = append(nodes, lnode{id: fmt.Sprint("n", i), w: 80 + r.Intn(100), h: 40})
	}
	seen := map[ledge]bool{}
	var edges []ledge
	for len(edges) < m {
		a, b := r.Intn(n), r.Intn(n)
		e := ledge{fmt.Sprint("n", min(a, b)), fmt.Sprint("n", max(a, b))}
		if a != b && !seen[e] {
			seen[e] = true
			edges = append(edges, e)
		}
	}
	return nodes, edges
}

// A large graph lays out in well under a second, with a tight budget that
// sheds most chains and with wide ones that keep them all.
func TestALargeGraphLaysOutQuickly(t *testing.T) {
	for _, c := range []struct {
		seed         int64
		n, m, budget int
	}{{1, 200, 400, 1320}, {2, 200, 400, 4000}, {1, 200, 400, 20000}, {1, 100, 300, 100000}, {1, 400, 800, 8000}} {
		nodes, edges := randomGraph(c.seed, c.n, c.m)
		start := time.Now()
		arrange(nodes, edges, c.budget, nil)
		if took := time.Since(start); took > time.Second {
			t.Errorf("%d boxes and %d lines at budget %d took %v to lay out, past 1s", c.n, c.m, c.budget, took)
		}
	}
}

// Moving boxes along their rows never takes a drawing past its budget.
func TestShorteningKeepsTheBudget(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		nodes, edges := randomGraph(seed, 60, 120)
		if l := arrange(nodes, edges, 1320, nil); l.width > 1320 {
			t.Errorf("seed %d: drawn %dpx wide, past the budget of 1320px", seed, l.width)
		}
	}
}
