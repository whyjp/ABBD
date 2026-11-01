package topology

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

// A dead relation's quote is no longer in the code, so a phrase from it
// finds nothing.
func TestAPhraseNeverMatchesTheQuoteOfADeadRelation(t *testing.T) {
	quoted := func(dead bool) []model.Unit {
		return []model.Unit{
			{ID: "MOD-0001", Kind: model.Module, Name: model.Fact{Value: "billing", Origin: model.Extracted}, Relations: []model.Relation{{
				To: "MOD-0002", Kind: model.DependsOn, Origin: model.Inferred, Dead: dead,
				Evidence: []model.Evidence{{Path: "billing/charge.go", Lines: "3", Quote: "settlement webhook"}},
			}}},
			{ID: "MOD-0002", Kind: model.Module, Name: model.Fact{Value: "ledger", Origin: model.Extracted}},
		}
	}
	none := func() []string { return nil }
	if got := Find(quoted(false), none, "settlement webhook"); len(got) != 1 || got[0].Unit.ID != "MOD-0001" {
		t.Fatalf("a live quote should find its unit: %+v", got)
	}
	if got := Find(quoted(true), none, "settlement webhook"); len(got) != 0 {
		t.Errorf("a dead quote found %+v", got)
	}
}
