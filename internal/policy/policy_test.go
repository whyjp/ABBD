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
		{"an addition is a structural change to review", nil, "added", model.TierB},
		{"an addition with external consumers stops", &yes, "added", model.TierC},
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

func TestCommandTiersMatchCommandsNotWords(t *testing.T) {
	for cmd, want := range map[string]model.Tier{
		"git -c core.x=y push origin main":  model.TierC,
		"rtk git push":                      model.TierC,
		"kubectl apply -f k8s.yaml":         model.TierC,
		"deploy --prod":                     model.TierC,
		"go vet ./internal/deploy/...":      model.TierA,
		"go test ./internal/push/...":       model.TierA,
		"grep -rn 'terraform' docs":         model.TierA,
		"make deploy":                       model.TierC,
		"npm run deploy":                    model.TierC,
		"yarn publish":                      model.TierC,
		"pnpm publish --access public":      model.TierC,
		"npm publish":                       model.TierC,
		"docker buildx build -t x --push .": model.TierC,
		"git send-pack origin main":         model.TierC,
		"rm -rf /*":                         model.TierC,
		"rm -r -f /":                        model.TierC,
		"rm -rf ~":                          model.TierC,
		"rm -rf ./build":                    model.TierA,
		"rm -rf ~/project/tmp":              model.TierA,
		"make test":                         model.TierA,
		"npm run test":                      model.TierA,
	} {
		if got, _ := CommandTier(cmd); got != want {
			t.Errorf("%q: %s, want %s", cmd, got, want)
		}
	}
}
