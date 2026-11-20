package screen

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Only two lines between the same two boxes, one each way, of one kind and
// one dotting, both ending on the boxes themselves, are a mutual pair.
// The line that runs the way the eye reads is kept.
func TestOnlyATrueReversePairFolds(t *testing.T) {
	box := func(id string, row int) end { return end{node: id, row: row, lo: 0, hi: 100, key: id} }
	a, b, c := box("a", 0), box("b", 1), box("c", 1)
	chip := end{row: -1, lo: 0, hi: 50, key: "port|x"}
	for _, tc := range []struct {
		name string
		reqs []wireReq
		want [][2]int
	}{
		{"pair", []wireReq{{from: b, to: a, kind: kindDependsOn}, {from: a, to: b, kind: kindDependsOn}}, [][2]int{{1, 0}}},
		{"two kinds", []wireReq{{from: a, to: b, kind: kindDependsOn}, {from: b, to: a, kind: kindConsumes}}, nil},
		{"two dottings", []wireReq{{from: a, to: b, kind: kindDependsOn}, {from: b, to: a, kind: kindDependsOn, plan: true}}, nil},
		{"a chip end", []wireReq{{from: a, to: chip, kind: kindDependsOn}, {from: chip, to: a, kind: kindDependsOn}}, nil},
		{"one end shared", []wireReq{{from: a, to: b, kind: kindDependsOn}, {from: c, to: a, kind: kindDependsOn}}, nil},
		{"two lines one way", []wireReq{{from: a, to: b, kind: kindDependsOn}, {from: a, to: b, kind: kindDependsOn}, {from: b, to: a, kind: kindDependsOn}}, nil},
		{"a lead", []wireReq{{from: a, to: b, kind: kindDependsOn, lead: true}, {from: b, to: a, kind: kindDependsOn}}, nil},
	} {
		if got := foldable(tc.reqs, readFirst); !slices.Equal(got, tc.want) {
			t.Errorf("%s: folds %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Over every view of every fixture the router folds: each folded pair is
// drawn as its kept line's ink alone, which no third line rides, the rider
// runs the kept line's way backward and has no start of its own, and
// replaying the kept moves draws the same drawing.
func TestAFoldedPairIsOneLineOfItsOwn(t *testing.T) {
	var mu sync.Mutex
	folded := 0
	seen := func(l *layout, reqs []wireReq) {
		if len(foldable(reqs, readFirst)) == 0 {
			return
		}
		at := *l
		at.pairs = foldAll
		rt := newRouter(at, reqs)
		kept := builtin{}.route(rt)
		r := kept.r
		mu.Lock()
		defer mu.Unlock()
		if r.rider == nil {
			t.Errorf("a view with %d mutual pairs folds none", len(foldable(reqs, readFirst)))
			return
		}
		for j, k := range r.rider {
			if k < 0 {
				continue
			}
			folded++
			back := slices.Clone(r.ways[k])
			slices.Reverse(back)
			if !slices.Equal(back, r.ways[j]) {
				t.Errorf("rider %d does not run line %d's way backward", j, k)
			}
			for _, d := range r.drawn {
				on := slices.Index(d.reqs, j)
				if on < 0 && !slices.Contains(d.reqs, k) {
					continue
				}
				if on < 0 || !slices.Contains(d.reqs, k) || len(d.reqs) != 2 || !d.both {
					t.Errorf("a path carries %v with folded pair %d, %d", d.reqs, k, j)
				}
				if on >= 0 && d.starts[on] != noStart {
					t.Errorf("rider %d starts a path of its own", j)
				}
			}
		}
		if again, ok := rt.replay(kept.path); !ok || !slices.Equal(again.r.rider, r.rider) {
			t.Errorf("replaying %v does not fold again", kept.path)
		}
	}
	buildEach(t, func(name string) (Model, error) { return freshWith(name, &hooks{routing: seen}) })
	if folded == 0 {
		t.Fatal("no fixture view folds a pair")
	}
}

// Two units that use each other directly, inside a module, are drawn as
// one folded line: a head at each end, both relations in its tooltip and
// lit when pointed at, no origin mark under its start head, and lint traces
// both relations along it and finds no phantom pair.
func TestAFoldedLineHeadsBothEndsAndLightsBoth(t *testing.T) {
	units := []model.Unit{
		called("MOD-0001", "app"),
		inside("MOD-0002", "MOD-0001", rel("MOD-0003", model.DependsOn)),
		inside("MOD-0003", "MOD-0001", rel("MOD-0002", model.DependsOn), rel("MOD-0004", model.DependsOn)),
		inside("MOD-0004", "MOD-0001"),
	}
	m := build(units)
	if len(m.Insides) != 1 {
		t.Fatalf("app's view is missing: %+v", m.Insides)
	}
	d := m.Insides[0].Drawing
	n := 0
	for k, r := range d.Routes {
		if !r.Both || r.Kind == kindMutual {
			continue
		}
		n++
		if len(r.Tips) != 2 || len(r.rels) != 2 {
			t.Errorf("folded route %d has %d heads and %d relations", k, len(r.Tips), len(r.rels))
		}
		lit := false
		for _, w := range d.Wires {
			if !slices.Contains(w.rides, k) {
				continue
			}
			lit = true
			if w.folded() && len(w.Marks) > 0 {
				t.Errorf("an origin mark stands on folded line %q", w.Title)
			}
			if strings.Count(w.Title, "depends_on") < 2 {
				t.Errorf("folded line %q names one relation", w.Title)
			}
		}
		if !lit {
			t.Errorf("no ink lights folded route %d", k)
		}
	}
	if n != 2 {
		t.Fatalf("%d folded routes, want the pair's two", n)
	}
	vl, err := lintView(view{d: d, budget: insideWidth, room: screenH})
	if err != nil {
		t.Fatal(err)
	}
	if vl.Totals.Phantoms != 0 || vl.Totals.OppositeShared != 0 || vl.Totals.TwoLaneOff != 0 {
		t.Errorf("the folded view reads phantoms %d, opposite_shared %d, two_lane_off %d", vl.Totals.Phantoms, vl.Totals.OppositeShared, vl.Totals.TwoLaneOff)
	}
	for _, b := range vl.Breaches {
		if b.Hard {
			t.Errorf("the folded view breaks %s: %s", b.Criterion, b.Text)
		}
	}
}

// A folded line says each way's word: once where both say it, both centred
// where they differ, and, set near its ends, the word of the way arriving
// at its end beside its last stretch and the other beside its first.
func TestAFoldedLineSaysBothWays(t *testing.T) {
	tr := tree{phrases: map[string]phrase{"ab": {text: "콜백"}, "ba": {text: "설정 읽기"}, "ba2": {text: "콜백"}}}
	if s, _, ends := twoWays(tr, [][]string{{"ab"}, {"ba2"}}); s != "콜백" || ends != [2]string{} {
		t.Errorf("one word both ways reads %q %v", s, ends)
	}
	s, _, ends := twoWays(tr, [][]string{{"ab"}, {"ba"}})
	if s != "콜백 · 설정 읽기" || ends != [2]string{"콜백", "설정 읽기"} {
		t.Errorf("two words read %q %v", s, ends)
	}
	// A line from (100,20) down to (100,200), then right to (300,200).
	segs := []seg{{a: pt{100, 20}, b: pt{100, 200}, wire: 0, aEnd: true}, {a: pt{100, 200}, b: pt{300, 200}, wire: 0, bEnd: true}}
	var w Wire
	clear := func(seg, Label, int, int) bool { return true }
	got := w.endWords(0, ends, segs, clear, func(Label) bool { return false })
	if got == nil || w.Far == nil {
		t.Fatal("no clear spot for either word")
	}
	if got.S != "콜백" || got.X < 200 || abs(got.Y-200) > labelH+labelOff {
		t.Errorf("the end's word stands at %+v, want it beside the last stretch near x=300", *got)
	}
	if w.Far.S != "설정 읽기" || w.Far.Y > 110 {
		t.Errorf("the start's word stands at %+v, want it beside the first stretch near y=20", *w.Far)
	}
}
