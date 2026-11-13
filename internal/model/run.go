package model

import (
	"fmt"
	"slices"
	"strings"
)

// Shape is how an execution context takes its work.
type Shape string

const (
	// Tick is a periodic loop.
	Tick Shape = "tick"
	// Wait blocks on a queue or a condition.
	Wait Shape = "wait"
	// Ready is a readiness event loop, such as select, epoll or a reactor.
	Ready Shape = "ready"
	// Complete is a completion queue, a proactor.
	Complete Shape = "complete"
	// Steal is a work-stealing pool, such as tokio's multi-thread runtime.
	Steal Shape = "steal"
	// Inline runs on its caller.
	Inline Shape = "inline"
)

// Count is how many of an execution context run.
type Count string

const (
	One  Count = "1"
	Many Count = "N"
	Pool Count = "pool"
)

var (
	shapes = []Shape{Tick, Wait, Ready, Complete, Steal, Inline}
	counts = []Count{One, Many, Pool}
)

// Run is how an execution context dispatches its work and how many of it
// run. A unit stores it as its shape and its count, such as "tick 1".
type Run struct {
	Shape Shape
	Count Count
}

// ParseRun reads a run as a unit stores it, "SHAPE COUNT".
func ParseRun(s string) (Run, error) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return Run{}, fmt.Errorf("run %q is not a shape and a count, such as \"tick 1\": the shape is one of %s, the count one of %s", s, wordList(shapes), wordList(counts))
	}
	r := Run{Shape: Shape(f[0]), Count: Count(f[1])}
	if !slices.Contains(shapes, r.Shape) {
		return Run{}, fmt.Errorf("run %q: unknown shape %q, use %s", s, f[0], wordList(shapes))
	}
	if !slices.Contains(counts, r.Count) {
		return Run{}, fmt.Errorf("run %q: unknown count %q, use %s", s, f[1], wordList(counts))
	}
	return r, nil
}

func (r Run) String() string { return string(r.Shape) + " " + string(r.Count) }

// Words is the run as a box shows it, such as "TICK · 1".
func (r Run) Words() string { return strings.ToUpper(string(r.Shape)) + " · " + string(r.Count) }

// Ran is the unit's run, if it records one that parses.
func (u Unit) Ran() (Run, bool) {
	if u.Run == nil {
		return Run{}, false
	}
	r, err := ParseRun(u.Run.Value)
	return r, err == nil
}

func wordList[T ~string](xs []T) string {
	ss := make([]string, len(xs))
	for i, x := range xs {
		ss[i] = string(x)
	}
	return strings.Join(ss[:len(ss)-1], ", ") + " or " + ss[len(ss)-1]
}
