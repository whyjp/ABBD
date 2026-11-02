package model

import (
	"strings"
	"testing"
)

func TestAStepIsOneLineOnLiveEvidence(t *testing.T) {
	ok := Step{Unit: "MOD-0001", Does: "parses argv", Evidence: []Evidence{{Path: "a.go", Quote: "x"}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for want, s := range map[string]Step{
		"says what it does":    {Unit: "MOD-0001", Evidence: ok.Evidence},
		"is one line":          {Unit: "MOD-0001", Does: "a\nb", Evidence: ok.Evidence},
		"cites the code":       {Unit: "MOD-0001", Does: "a"},
		"names MOD-0001 twice": {Unit: "MOD-0001", Does: "a", Calls: []ID{"MOD-0001"}, Evidence: ok.Evidence},
	} {
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error saying %q, got %v", want, err)
		}
	}
	if _, err := ParseFlowKey("Sync Flow"); err == nil {
		t.Error("a flow key is lower-case letters, digits and hyphens")
	}
}

// A call rides a live relation from the step's unit or a unit inside it to
// the called unit or a unit inside that; the strongest kind names it.
func TestAStepReachesOnlyWhatTheGraphJoins(t *testing.T) {
	units := []Unit{
		{ID: "MOD-0001", Kind: Module, Relations: []Relation{{To: "DATA-0001", Kind: Consumes}}},
		{ID: "MOD-0002", Kind: Module, Parent: "MOD-0001", Relations: []Relation{{To: "DATA-0001", Kind: Provides}, {To: "MOD-0004", Kind: DependsOn, Dead: true}}},
		{ID: "MOD-0003", Kind: Module},
		{ID: "MOD-0004", Kind: Module},
		{ID: "DATA-0001", Kind: Data},
	}
	r := Step{Unit: "MOD-0001", Calls: []ID{"DATA-0001", "MOD-0003", "MOD-0004", "MOD-0009"}}.Against(units)
	if r.Calls["DATA-0001"] != Provides || len(r.Calls) != 1 {
		t.Errorf("DATA-0001 is reached, by the strongest kind; the rest are not: %v", r.Calls)
	}
	if len(r.Gaps) != 3 || !strings.Contains(strings.Join(r.Gaps, "|"), "MOD-0009, which the step calls, is no longer in the state") {
		t.Errorf("a call with no live relation or no unit is a gap: %v", r.Gaps)
	}
	if g := (Step{Unit: "MOD-0007"}).Against(units).Gaps; len(g) != 1 {
		t.Errorf("a step on a unit the state lacks is a gap: %v", g)
	}
}
