package screen

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// With ABBD_ACCEPT_WAYS set to a file, every traced line of every fixture
// view goes there as a table: the view as TestLintFixtures names it, the
// two places as a breach names them, the kind of ink, and the turning
// points. eval/lint/accept.py reads its geometry measures from it.
func TestAcceptWays(t *testing.T) {
	f := os.Getenv("ABBD_ACCEPT_WAYS")
	if f == "" {
		t.Skip("ABBD_ACCEPT_WAYS is not set")
	}
	rows := []string{"view\tfrom\tto\tkind\tpoints"}
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			s, err := sheetOf(v.d)
			if err != nil {
				t.Fatal(err)
			}
			l := newLook(v, s)
			for _, w := range l.trace.Ways {
				pts := make([]string, len(w.pts))
				for i, p := range w.pts {
					pts[i] = fmt.Sprintf("%g,%g", p.x, p.y)
				}
				rows = append(rows, strings.Join([]string{name + " " + v.name, l.shapeName(w.from), l.shapeName(w.to), w.kind, strings.Join(pts, " ")}, "\t"))
			}
		}
	}
	if err := os.WriteFile(f, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
