package model

import "testing"

func TestEveryViaHasAWordAndEveryWordAVia(t *testing.T) {
	if len(ViaNames) != len(viaWords) {
		t.Fatalf("%d vias listed, %d with words", len(ViaNames), len(viaWords))
	}
	for _, v := range ViaNames {
		if v.Word() == "" {
			t.Errorf("via %s has no word", v)
		}
	}
}

func TestArtifactsAreAModulesOnlyInOneOrder(t *testing.T) {
	if err := ValidateArtifacts(Module, SortArtifacts([]Artifact{"shared", "static", "shared"})); err != nil {
		t.Error(err)
	}
	if err := ValidateArtifacts(Interface, []Artifact{"exe"}); err == nil {
		t.Error("a contract listed artifacts")
	}
	if err := ValidateArtifacts(Module, []Artifact{"shared", "static"}); err == nil {
		t.Error("artifacts out of order passed")
	}
}
