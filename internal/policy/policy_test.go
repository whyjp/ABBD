package policy

import (
	"testing"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
)

func TestContractTiers(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name      string
		consumers *bool
		change    string
		want      model.Tier
	}{
		{"breaking with external consumers stops", &yes, "removed", model.TierC},
		{"breaking with unknown consumers asks", nil, "modified", model.TierB},
		{"breaking without consumers is automatic", &no, "removed", model.TierA},
		{"additions never break", &yes, "added", model.TierA},
	}
	for _, c := range cases {
		got, _, _ := ContractTier(model.Contract{ExternalConsumers: c.consumers}, c.change)
		if got != c.want {
			t.Errorf("%s: tier %s, want %s", c.name, got, c.want)
		}
	}
}

func TestActionTiersAndOverrides(t *testing.T) {
	p := ssot.DefaultProject("x", "code-only")
	for action, want := range map[string]model.Tier{"push": model.TierC, "deploy": model.TierC, "map-sync": model.TierA, "local-commit": model.TierA, "add-dependency": model.TierB} {
		if r, _ := Classify(p, action); r.Tier != want {
			t.Errorf("%s: %s, want %s", action, r.Tier, want)
		}
	}
	p.Policy.LocalCommitTier = model.TierB
	if r, _ := Classify(p, "local-commit"); r.Tier != model.TierB {
		t.Error("project.yaml decides the local commit tier")
	}
	if _, err := Classify(p, "teleport"); err == nil {
		t.Error("unknown actions are an error, not a silent tier")
	}
}

func TestQuestionBudget(t *testing.T) {
	p := ssot.DefaultProject("x", "code-only")
	if !CanAsk(p, 2) || CanAsk(p, 3) {
		t.Fatal("at most 3 questions per turn")
	}
}
