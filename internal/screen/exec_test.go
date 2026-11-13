package screen

import (
	"slices"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// An execution context's box says how it runs where a module's says the
// build artifact it is.
func TestAnExecutionContextSaysHowItRuns(t *testing.T) {
	units := ripgrep()
	exec := inside("EXEC-0001", "MOD-0002")
	exec.Kind, exec.Name.Value = model.Exec, "search workers"
	run := model.Said(model.Supplement, "steal pool")
	exec.Run = &run
	m := build(append(units, exec))
	i := slices.IndexFunc(m.Insides, func(in Inside) bool { return in.ID == "MOD-0002" })
	if i < 0 {
		t.Fatal("MOD-0002 has no inside view")
	}
	if b := boxes(m.Insides[i].Drawing)["EXEC-0001"]; b.Artifact != "STEAL · pool" {
		t.Errorf("the EXEC box says %q, want STEAL · pool", b.Artifact)
	}
	if kindWord(model.Exec) != "실행" || labels["EXEC"] != "실행" {
		t.Errorf("an EXEC is not named 실행")
	}
}

// A line that hands work through a queue, a channel, a signal or a proxy
// is a run-time line, drawn solid, where one that arises only in design is
// dotted.
func TestAHandoffViaIsDrawnAsARunTimeLine(t *testing.T) {
	for via, dotted := range map[model.Via]bool{"queue": false, "channel": false, "signal": false, "proxy": false, "import": true} {
		units := ripgrep()
		units[0].Relations = append(units[0].Relations, guess("MOD-0005", model.DependsOn, via))
		drawn := false
		for _, w := range build(units).Overview.Wires {
			if slices.Contains(w.Rels, "MOD-0001.depends_on.MOD-0005") {
				drawn = true
				if w.Plan != dotted {
					t.Errorf("a line via %s is dotted=%v, want %v", via, w.Plan, dotted)
				}
			}
		}
		if !drawn {
			t.Errorf("no overview line from MOD-0001 to MOD-0005 via %s", via)
		}
	}
}
