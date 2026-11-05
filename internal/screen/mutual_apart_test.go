package screen

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// Two modules that each use a third, which uses them back, are two
// two-headed lines. Bundled into the third, they would read as joining the
// two, which do not relate (rule 18), so each keeps a line of its own.
func TestLinesOfTwoMutualPairsJoinNoOtherPlaces(t *testing.T) {
	m := build([]model.Unit{
		called("MOD-0001", "app", rel("MOD-0002", model.DependsOn), rel("MOD-0003", model.DependsOn), rel("MOD-0004", model.DependsOn)),
		called("MOD-0002", "left", rel("MOD-0004", model.DependsOn)),
		called("MOD-0003", "right", rel("MOD-0004", model.DependsOn)),
		called("MOD-0004", "core", rel("MOD-0002", model.DependsOn), rel("MOD-0003", model.DependsOn)),
	})
	vl, err := lintView(views(m)[0])
	if err != nil {
		t.Fatal(err)
	}
	if vl.Totals.Phantoms != 0 || vl.Totals.Hard != 0 {
		for _, b := range vl.Breaches {
			t.Log(b.Criterion, b.Text)
		}
		t.Fatalf("phantoms %d, hard %d, want none", vl.Totals.Phantoms, vl.Totals.Hard)
	}
}
