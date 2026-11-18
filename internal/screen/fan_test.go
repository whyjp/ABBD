package screen

import (
	"bytes"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

// fanned is a fixture built as its project.yaml says, with the overview's
// strands in shape.
func fanned(t *testing.T, name, shape string) Model {
	t.Helper()
	v, _ := built.LoadOrStore(name+" strands "+shape, &builtModel{})
	b := v.(*builtModel)
	b.once.Do(func() {
		st := store.Open(filepath.Join("testdata", name))
		units, err := st.LoadUnits()
		if err != nil {
			b.err = err
			return
		}
		p, err := st.LoadProject()
		p.OverviewStrands = shape
		b.m, b.err = Build(Input{Units: units, Project: p}), err
	})
	if b.err != nil {
		t.Fatal(b.err)
	}
	return b.m
}

// Strands change only the overview's departures: every box, route, word,
// number and key stands where it stood, each fanned path keeps its whole
// D, and a path takes strands only where its lines all leave its start,
// two or more of them, none climbing, mutual or on the primary flow.
func TestStrandsMoveNothing(t *testing.T) {
	t.Parallel()
	total := 0
	for _, name := range fixtures {
		for _, shape := range []string{"fan", "twist"} {
			plain, got := fixture(t, name).Overview, fanned(t, name, shape).Overview
			if !reflect.DeepEqual(plain.Boxes, got.Boxes) || !reflect.DeepEqual(plain.Routes, got.Routes) || !reflect.DeepEqual(plain.Lead, got.Lead) || !reflect.DeepEqual(plain.Card, got.Card) || !reflect.DeepEqual(plain.Dots, got.Dots) {
				t.Errorf("%s %s: strands moved a box, route, number, key or dot", name, shape)
			}
			lead := map[int]bool{}
			if got.Lead != nil {
				for _, h := range got.Lead.Hops {
					lead[h.way] = true
				}
			}
			for i, w := range got.Wires {
				was := plain.Wires[i]
				if w.D != was.D || !reflect.DeepEqual(w.Label, was.Label) {
					t.Errorf("%s %s: path %d changed its D or its word", name, shape, i)
				}
				if w.Fan == nil {
					if w.Count != was.Count || !reflect.DeepEqual(w.Marks, was.Marks) {
						t.Errorf("%s %s: path %d has no strands but its count or marks moved", name, shape, i)
					}
					continue
				}
				total++
				n := len(w.rides)
				if n < 2 || w.Back || w.Both || !w.Joins || slices.Contains(w.leave, "") || slices.ContainsFunc(w.rides, func(k int) bool { return lead[k] }) {
					t.Errorf("%s %s: path %d takes strands it may not: %d lines, back %v, both %v, joins %v, leave %q", name, shape, i, n, w.Back, w.Both, w.Joins, w.leave)
				}
				if len(w.Fan.Strands) != min(n, fanMost) {
					t.Errorf("%s %s: path %d of %d lines draws %d strands", name, shape, i, n, len(w.Fan.Strands))
				}
				if n <= fanMost && w.Count != 0 || n > fanMost && w.Count != was.Count {
					t.Errorf("%s %s: path %d of %d lines shows ×%d, had ×%d", name, shape, i, n, w.Count, was.Count)
				}
				if len(w.Marks) != len(was.Marks) {
					t.Errorf("%s %s: path %d has %d origin marks, had %d", name, shape, i, len(w.Marks), len(was.Marks))
				}
				if shape == "fan" && len(w.Fan.Plies) > 0 {
					t.Errorf("%s fan: path %d twists", name, i)
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("no fixture overview drew a strand")
	}
}

// A path's strands start on the edge it leaves, apart from each other,
// and gather at one point on the path, so the trunk the page draws runs on
// from there.
func TestStrandsGatherOnThePath(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		d := fanned(t, name, "fan").Overview
		for i, w := range d.Wires {
			if w.Fan == nil {
				continue
			}
			ps := pointsOf(w.D)
			knot := pointsOf(w.Fan.Trunk)[0]
			if !on(knot, ps[0], ps[1]) || knot == ps[0] {
				t.Errorf("%s: path %d's trunk starts at %v, off its first stretch %v-%v", name, i, knot, ps[0], ps[1])
			}
			lo, hi, ok := d.edgeSpan(ps[0], pt{sign(ps[1].x - ps[0].x), sign(ps[1].y - ps[0].y)})
			var starts []int
			for _, s := range w.Fan.Strands {
				f := strings.Fields(strings.NewReplacer("M", "", "C", "").Replace(s.D))
				x, y := atoi(f[0]), atoi(f[1])
				end := pt{atoi(f[len(f)-2]), atoi(f[len(f)-1])}
				along := x
				if ps[0].x != ps[1].x {
					along = y
				}
				if !ok || along < lo || along > hi || end != (pt{ps[0].x + fanReach*sign(ps[1].x-ps[0].x), ps[0].y + fanReach*sign(ps[1].y-ps[0].y)}) {
					t.Errorf("%s: path %d's strand %s leaves off its edge %d..%d or meets off the path", name, i, s.D, lo, hi)
				}
				starts = append(starts, along)
			}
			slices.Sort(starts)
			for k := 1; k < len(starts); k++ {
				if starts[k]-starts[k-1] < fanPitchMin {
					t.Errorf("%s: path %d's strands start %dpx apart", name, i, starts[k]-starts[k-1])
				}
			}
		}
	}
}

// The strands are a trial: with no shape set, and on every page whose
// drawings have none, the pages are byte for byte what they were.
func TestPagesWithoutStrandsStayTheSame(t *testing.T) {
	t.Parallel()
	for _, name := range fixtures {
		plain, err := Pages(fixture(t, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := Pages(fanned(t, name, "fan"))
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for f, b := range plain {
			if bytes.Equal(b, got[f]) {
				continue
			}
			changed++
			if f != "index.html" || !bytes.Contains(got[f], []byte(`class="strand `)) {
				t.Errorf("%s: %s changed though it draws no strand", name, f)
			}
		}
		if len(got) != len(plain) {
			t.Errorf("%s: %d pages, had %d", name, len(got), len(plain))
		}
		t.Logf("%s: %d of %d pages changed", name, changed, len(plain))
	}
}

func atoi(s string) int {
	n, neg := 0, strings.HasPrefix(s, "-")
	for _, c := range strings.TrimPrefix(s, "-") {
		n = n*10 + int(c-'0')
	}
	if neg {
		return -n
	}
	return n
}
