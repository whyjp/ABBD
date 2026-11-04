package screen

import "testing"

// src/'s bottom edge as the loose cover tactic lays it out: eight chips
// that fill the frame. In the order of their mean x the contract chip with
// one line into desktop main comes first and the crowd to its right pushes
// it to the frame's left end, far off that line. reorder lets the chip
// whose lines spread from orca CLI to the renderer give way, so the
// contract chip stands over its line and no chip misses more of its lines.
func TestReorderSetsAChipOverItsOnlyLine(t *testing.T) {
	frame := rect{X: 6, W: 1120}
	edge := func() []attach {
		return []attach{
			{key: "connector", label: "@orca-cloud/connector-contract", other: 292, runs: []int{292}},
			{key: "native", label: "→ native/", other: 384, runs: []int{292, 292, 570}},
			{key: "rpc", label: "Orca runtime RPC methods", other: 422, runs: []int{94, 292, 880}},
			{key: "push", label: "@orca-cloud/push-contract", other: 717, runs: []int{717}},
			{key: "cloud", label: "→ cloud/", other: 717, runs: []int{717, 717}},
			{key: "mobile", label: "→ mobile/", other: 717, runs: []int{717}},
			{key: "preload", label: "preload bridge", other: 880, runs: []int{880}},
			{key: "ipc", label: "Electron IPC channels", other: 1010, runs: []int{1010}},
		}
	}
	off := func(ps map[string]Port, as []attach) (n int) {
		for _, a := range as {
			p, over := ps[a.key], false
			for _, x := range a.runs {
				over = over || x >= p.X+4 && x <= p.X+p.W-4
			}
			if !over {
				n++
			}
		}
		return n
	}
	meets := func(ps map[string]Port, k string, x int) bool { return x >= ps[k].X+4 && x <= ps[k].X+ps[k].W-4 }

	plain := edge()
	before := settle(spread(frame, true, plain, 1), plain, frame)
	if meets(before, "connector", 292) {
		t.Fatalf("settle alone already sets the contract chip over its line: %+v", before["connector"])
	}
	items := edge()
	after, moved := reorder(settle(spread(frame, true, items, 1), items, frame), items, frame)
	if !moved {
		t.Fatal("reorder moved no chip")
	}
	if !meets(after, "connector", 292) {
		t.Errorf("the contract chip stands at %d..%d, off its line at 292", after["connector"].X, after["connector"].X+after["connector"].W)
	}
	if b, a := off(before, plain), off(after, items); a >= b {
		t.Errorf("%d chips stand over none of their lines after reorder, %d before", a, b)
	}
	for i := 1; i < len(items); i++ {
		if p, q := after[items[i-1].key], after[items[i].key]; p.X+p.W+4 > q.X {
			t.Errorf("%s and %s overlap: %d..%d, %d..", items[i-1].key, items[i].key, p.X, p.X+p.W, q.X)
		}
	}
}
