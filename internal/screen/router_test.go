package screen

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

// Over every view of every fixture: replaying the moves that led the
// built-in policy to the drawing it kept, from the router's start, draws
// that drawing again, lays nothing on the view's own layout, and logs every
// drawing as plain values a policy outside the package could read.
func TestReplayingTheKeptMovesDrawsTheKeptDrawing(t *testing.T) {
	var mu sync.Mutex
	views, moved := 0, 0
	seen := func(l *layout, reqs []wireReq) {
		was := fmt.Sprintf("%#v", *l)
		rt := newRouter(*l, reqs)
		kept := builtin{}.route(rt)
		again, ok := rt.replay(kept.path)
		mu.Lock()
		defer mu.Unlock()
		views++
		if len(kept.path) > 0 {
			moved++
		}
		switch {
		case !ok:
			t.Errorf("replaying %v: a move had nothing to do", kept.path)
		case fmt.Sprintf("%#v", again.r) != fmt.Sprintf("%#v", kept.r) || fmt.Sprintf("%#v", again.at) != fmt.Sprintf("%#v", kept.at):
			t.Errorf("replaying %v draws another drawing", kept.path)
		}
		if fmt.Sprintf("%#v", *l) != was {
			t.Errorf("routing wrote the view's layout")
		}
		if _, err := json.Marshal(rt.log); err != nil {
			t.Errorf("the log does not marshal: %v", err)
		}
	}
	buildEach(t, func(name string) (Model, error) { return freshWith(name, &hooks{routing: seen}) })
	if views == 0 || moved == 0 {
		t.Fatalf("%d views routed, %d of them by moves", views, moved)
	}
	t.Logf("%d views routed, %d by moves", views, moved)
}

// A lay in any modes depends on nothing an earlier lay in other modes left
// on the layout, so a router may lay every drawing from the one state.
func TestALayDependsOnNoEarlierLay(t *testing.T) {
	var mu sync.Mutex
	modes := []layOpts{{}, {Shared: true, Drops: true, Bent: map[int]bool{}}, {Apart: true, Tweak: true, Early: true}}
	seen := func(l *layout, reqs []wireReq) {
		for _, a := range modes {
			for _, b := range modes {
				fresh := l.keep()
				want := fmt.Sprintf("%#v", fresh.lay(reqs, b))
				over := l.keep()
				over.lay(reqs, a)
				if got := fmt.Sprintf("%#v", over.lay(reqs, b)); got != want || fmt.Sprintf("%#v", over) != fmt.Sprintf("%#v", fresh) {
					mu.Lock()
					t.Errorf("laid in %+v over a lay in %+v, %d lines draw otherwise", b, a, len(reqs))
					mu.Unlock()
				}
			}
		}
	}
	buildEach(t, func(name string) (Model, error) { return freshWith(name, &hooks{routing: seen}) })
}
