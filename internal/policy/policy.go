// Package policy is the approval structure of PRD ch.10: tiers A/B/C as a table, not as
// branches spread over the commands.
package policy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whyjp/cccv/internal/model"
	"github.com/whyjp/cccv/internal/ssot"
)

type Rule struct {
	Tier   model.Tier `json:"tier"`
	Reason string     `json:"reason"`
}

// Actions is the default action table. project.yaml policy.action_tiers overrides entries.
var Actions = map[string]Rule{
	"read":                 {model.TierA, "읽기·탐색"},
	"edit-local":           {model.TierA, "가역적 로컬 코드/테스트 수정"},
	"map-sync":             {model.TierA, "Map 동기화"},
	"render":               {model.TierA, "그래프·HTML 재생성"},
	"bootstrap-candidates": {model.TierA, "bootstrap INFERRED 후보 생성"},
	"verify-local":         {model.TierA, "로컬 검증"},
	"local-commit":         {model.TierA, "로컬 commit (project.yaml policy.local_commit_tier)"},
	"ambiguous-request":    {model.TierB, "요청 해석이 둘 이상이고 결과가 실질적으로 다름"},
	"confirmed-conflict":   {model.TierB, "CONFIRMED 계약·불변조건과 충돌"},
	"compatibility-change": {model.TierB, "외부 인터페이스 호환성 정책 변경"},
	"value-judgement":      {model.TierB, "데이터 보존·비용·의존성 추가 같은 가치 판단"},
	"add-dependency":       {model.TierB, "의존성 추가"},
	"push":                 {model.TierC, "공유 브랜치 push"},
	"force-push":           {model.TierC, "공유 브랜치 force-push"},
	"deploy":               {model.TierC, "배포"},
	"delete-external":      {model.TierC, "외부 리소스 삭제"},
	"secret":               {model.TierC, "비밀정보·권한 변경"},
	"breaking-external":    {model.TierC, "external_consumers: true 인터페이스의 breaking change"},
	"production-data":      {model.TierC, "실서비스 데이터 변경"},
}

func Classify(p ssot.Project, action string) (Rule, error) {
	if t, ok := p.Policy.ActionTiers[action]; ok {
		return Rule{model.Tier(t), "project.yaml override"}, nil
	}
	if action == "local-commit" && p.Policy.LocalCommitTier != "" {
		return Rule{p.Policy.LocalCommitTier, Actions[action].Reason}, nil
	}
	r, ok := Actions[action]
	if !ok {
		return Rule{}, fmt.Errorf("unknown action %q; known: %v", action, ActionNames())
	}
	return r, nil
}

func ActionNames() []string {
	out := make([]string, 0, len(Actions))
	for k := range Actions {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// externalEffects are command fragments that act outside the workspace. A verification command
// containing one is a tier C action, not a check (PRD ch.10).
var externalEffects = []string{"git push", "deploy", "kubectl apply", "kubectl delete", "terraform apply", "helm upgrade", "helm install", "docker push", "rm -rf /", "drop table", "npm publish"}

func CommandTier(command string) (model.Tier, string) {
	low := strings.ToLower(command)
	for _, f := range externalEffects {
		if strings.Contains(low, f) {
			return model.TierC, "외부 효과가 있는 명령: " + f
		}
	}
	return model.TierA, "로컬 검증"
}

// ContractChange is one external contract touched by a diff.
type ContractChange struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Selector string     `json:"selector"`
	Change   string     `json:"change"` // added | removed | modified
	Breaking bool       `json:"breaking"`
	Tier     model.Tier `json:"tier"`
	Reason   string     `json:"reason"`
}

// ContractTier applies PRD 08A: a breaking change to an interface with external consumers
// stops (C); unknown consumers or a compatibility-relevant change asks with a default (B).
func ContractTier(c model.Contract, change string) (model.Tier, bool, string) {
	breaking := change == "removed" || change == "modified"
	if !breaking {
		return model.TierA, false, "추가는 기존 소비자를 깨지 않음"
	}
	switch {
	case c.ExternalConsumers != nil && *c.ExternalConsumers:
		return model.TierC, true, "external_consumers: true 인 계약의 breaking change"
	case c.ExternalConsumers == nil:
		return model.TierB, true, "외부 소비자 유무 미상 + breaking change: 호환성 확인 필요"
	case c.Compatibility.Value != "" && c.Compatibility.Confirmation == model.Confirmed:
		return model.TierB, true, "CONFIRMED 호환성 정책(" + c.Compatibility.Value + ")을 건드림"
	default:
		return model.TierA, true, "외부 소비자 없음으로 확인됨"
	}
}

// CanAsk enforces the per-turn question budget. Questions past it are not asked; they proceed
// with their default and stay UNCONFIRMED.
func CanAsk(p ssot.Project, askedThisTurn int) bool {
	max := p.Policy.MaxQuestionsPerTurn
	if max <= 0 {
		max = 3
	}
	return askedThisTurn < max
}
