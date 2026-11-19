package screen

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// route lays a view many times over to keep one drawing, and between two
// lays puts back the layout it kept (keep). Over every view of every
// fixture: what keep copies is all a lay changes, so a layout put back reads
// as it did before the lay; and a lay depends on nothing an earlier lay
// left, so laying the lines again over a laid layout draws them the same.
func TestALayLeavesNothingKeepDoesNotPutBack(t *testing.T) {
	var views, bad int64
	seen := func(l *layout, reqs []wireReq) {
		atomic.AddInt64(&views, 1)
		for _, o := range []layOpts{{}, {Shared: true, Drops: true, Bent: map[int]bool{}}, {Apart: true, Tweak: true, Early: true}} {
			was := fmt.Sprintf("%#v", *l)
			k := l.keep()
			first := fmt.Sprintf("%#v", l.lay(reqs, o))
			laid := fmt.Sprintf("%#v", *l)
			again := fmt.Sprintf("%#v", l.lay(reqs, o))
			if again != first || fmt.Sprintf("%#v", *l) != laid {
				atomic.AddInt64(&bad, 1)
				t.Errorf("laid again over a laid layout with %+v, %d lines draw otherwise", o, len(reqs))
			}
			*l = k
			if fmt.Sprintf("%#v", *l) != was {
				atomic.AddInt64(&bad, 1)
				t.Errorf("a layout put back after a lay with %+v reads otherwise", o)
			}
		}
	}
	buildEach(t, func(name string) (Model, error) { return freshWith(name, &hooks{routing: seen}) })
	if views == 0 {
		t.Fatal("no view was routed")
	}
	t.Logf("%d views routed, %d broke", views, bad)
}
