package screen

import (
	"strings"
	"testing"
)

// portBendDebt is how many traced lines to or from a frame chip, across
// the fixtures, turn more than once, and preloadBendDebt how often Orca's
// line from `desktop preload bridge` up to its `preload bridge` chip turns
// in `src/`. A person draws that line straight up, or with one bend. Both
// may only fall (docs/blueprint-svg.md, "Known gaps", "Seated chips in
// src/").
const portBendDebt, preloadBendDebt = 782, 2

func TestFramePortLinesTurnLittle(t *testing.T) {
	over, preload := 0, -1
	for _, name := range fixtures {
		for _, v := range views(fixture(t, name)) {
			got := traced(t, v.d)
			for i, w := range got.Ways {
				if !strings.HasPrefix(w.from, "port|") && !strings.HasPrefix(w.to, "port|") {
					continue
				}
				if got.Bends[i] > 1 {
					over++
				}
				if name == "orca" && v.name == "MOD-0098 src/" && w.from == "MOD-0071" && w.to == "port|IF-0024true" {
					preload = got.Bends[i]
				}
			}
		}
	}
	if over > portBendDebt {
		t.Errorf("%d frame chip lines turn more than once, recorded %d", over, portBendDebt)
	}
	switch {
	case preload < 0:
		t.Fatal("src/ draws no line from desktop preload bridge to its preload bridge chip")
	case preload > preloadBendDebt:
		t.Errorf("the preload bridge line turns %d times, recorded %d", preload, preloadBendDebt)
	}
	t.Logf("frame chip lines turning more than once %d, preload bridge line bends %d", over, preload)
}
