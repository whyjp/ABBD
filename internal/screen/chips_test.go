package screen

import "testing"

// A chip counts as escaped when it leaves its box and the frame, or the
// canvas; one that sits on its box's edge does not.
func TestTheCheckerCountsAChipThatEscapesItsEdge(t *testing.T) {
	s, err := readSVG(`<svg width="300" height="200">
<g class="box" data-id="MOD-0001"><rect x="20" y="20" width="160" height="80"/><text x="30" y="40">billing</text></g>
<g class="port" data-id="IF-0001"><rect x="60" y="90" width="40" height="10"/><text class="chip" x="62" y="98">ok</text></g>
<g class="port" data-id="IF-0002"><rect x="170" y="90" width="40" height="10"/><text class="chip" x="172" y="98">out</text></g>
<g class="port" data-id="IF-0003"><rect x="280" y="150" width="40" height="10"/><text class="chip" x="282" y="158">off</text></g>
</svg>`)
	if err != nil {
		t.Fatal(err)
	}
	if m := measure(s); m.Escaped != 2 {
		t.Errorf("two chips run off their box and one stays on it, so 2 escape; got %d", m.Escaped)
	}
}
