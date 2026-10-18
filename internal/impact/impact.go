// Package impact computes change impact (PRD ch.08): external contracts first, then the
// modules touched, their dependents, features and the tests to run. Impact Recall is the
// priority metric, so uncertainty is reported as unknowns, never dropped.
package impact

import (
	"path"
	"sort"
	"strings"

	"github.com/whyjp/for-my-harness/internal/analysis"
	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/policy"
	"github.com/whyjp/for-my-harness/internal/ssot"
)

type Report struct {
	ChangedFiles  []string                `json:"changed_files"`
	Contracts     []policy.ContractChange `json:"contracts"`
	Modules       []string                `json:"modules"`
	NewModules    []string                `json:"new_modules"`
	Dependents    []string                `json:"dependents"`
	Features      []string                `json:"features"`
	Tests         []string                `json:"tests"`
	TestCommands  []string                `json:"test_commands"`
	Unknowns      []string                `json:"unknowns"`
	Risks         []string                `json:"risks"`
	HighestTier   model.Tier              `json:"highest_tier"`
	UnconfirmedIn []string                `json:"unconfirmed_touched"`
}

func (r Report) Affected() []string {
	out := append(append(append([]string{}, r.Features...), r.Modules...), r.Dependents...)
	for _, c := range r.Contracts {
		if c.ID != "" {
			out = append(out, c.ID)
		}
	}
	return uniq(out)
}

func Analyze(s *ssot.Snapshot, res *analysis.Result, changed []string) Report {
	rep := Report{ChangedFiles: changed, HighestTier: model.TierA}
	rep.Contracts = ContractDiff(s, res)
	for _, c := range rep.Contracts {
		rep.HighestTier = maxTier(rep.HighestTier, c.Tier)
		if c.Breaking {
			rep.Risks = append(rep.Risks, c.Selector+": "+c.Reason)
		}
	}

	fileModule := map[string]string{}
	for _, m := range res.Modules {
		for _, f := range m.Files {
			fileModule[f] = m.Path
		}
	}
	language := map[string]string{}
	for _, m := range res.Modules {
		language[m.Path] = m.Language
	}
	direct := map[string]bool{}
	newMods := map[string]bool{}
	for _, f := range changed {
		p, ok := fileModule[f]
		if !ok {
			p = path.Dir(f)
		}
		if mod := s.ModuleByPath(p); mod != nil {
			direct[mod.ID] = true
		} else if ok {
			// A module created in this change is not in the SSOT yet; it is still in scope.
			newMods[p] = true
		} else if !strings.HasSuffix(f, "_test.go") {
			rep.Unknowns = append(rep.Unknowns, f+": 어떤 모듈에도 속하지 않는 파일 (설정·매니페스트면 서빙 구조를 확인)")
		}
	}
	for _, c := range rep.Contracts {
		if _, owner := s.Contract(c.ID); owner != nil {
			direct[owner.ID] = true
		}
	}
	rep.Modules = model.SortedKeys(direct)

	reverse := map[string][]string{}
	for _, m := range s.Modules {
		for _, d := range m.DependsOn {
			reverse[d] = append(reverse[d], m.ID)
		}
	}
	dependents := map[string]bool{}
	queue := append([]string{}, rep.Modules...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, up := range reverse[cur] {
			if !direct[up] && !dependents[up] {
				dependents[up] = true
				queue = append(queue, up)
			}
		}
	}
	rep.Dependents = model.SortedKeys(dependents)

	touchedContracts := map[string]bool{}
	for _, c := range rep.Contracts {
		touchedContracts[c.ID] = true
	}
	all := map[string]bool{}
	for k := range direct {
		all[k] = true
	}
	for k := range dependents {
		all[k] = true
	}
	for _, id := range model.SortedKeys(s.Features) {
		f := s.Features[id]
		if f.Status == model.ItemRemoved {
			continue
		}
		hit := false
		for _, m := range f.OwnedBy {
			hit = hit || all[m]
		}
		for _, c := range f.ExternalContracts {
			hit = hit || touchedContracts[c]
		}
		if hit {
			rep.Features = append(rep.Features, id)
			if f.Intent.Purpose.Pending() {
				rep.UnconfirmedIn = append(rep.UnconfirmedIn, id)
			}
		}
	}
	tests := map[string]bool{}
	cmds := map[string]bool{}
	for id := range all {
		m := s.Modules[id]
		for _, t := range m.Tests {
			tests[t] = true
		}
		if m.Language == "go" {
			cmds[goTestCmd(m.Path)] = true
		}
		if m.Responsibility.Pending() {
			rep.UnconfirmedIn = append(rep.UnconfirmedIn, id)
		}
		for _, c := range m.ExternalContracts {
			if c.Origin == model.UnknownOrigin && c.Status != model.ItemRemoved {
				rep.Unknowns = append(rep.Unknowns, c.ID+": "+c.Selector+" 런타임 조립 — 정적 분석으로 소비자를 확정할 수 없음")
			}
		}
	}
	for _, m := range res.Modules {
		if mod := s.ModuleByPath(m.Path); mod == nil || !all[mod.ID] {
			continue
		}
		for _, c := range m.Calls {
			if c.Confidence == "possible" {
				rep.Unknowns = append(rep.Unknowns, "possible call "+c.From+" -> "+c.To+" (동적 디스패치, 관계 없음으로 단정하지 않음)")
			}
		}
	}
	rep.NewModules = model.SortedKeys(newMods)
	for _, p := range rep.NewModules {
		if language[p] == "go" {
			cmds[goTestCmd(p)] = true
		}
	}
	rep.Tests = model.SortedKeys(tests)
	rep.TestCommands = model.SortedKeys(cmds)
	if len(rep.TestCommands) == 0 && len(changed) > 0 {
		rep.TestCommands = append(rep.TestCommands, s.Project.Verification.TestCommands...)
	}
	rep.Unknowns = uniq(rep.Unknowns)
	rep.UnconfirmedIn = uniq(rep.UnconfirmedIn)
	return rep
}

// ContractDiff compares stored contracts with freshly extracted ones. A contract whose symbol
// survives but whose selector changed is "modified", the breaking kind most often missed.
func ContractDiff(s *ssot.Snapshot, res *analysis.Result) []policy.ContractChange {
	type key struct{ kind, dir, sel string }
	stored := map[key]model.Contract{}
	bySymbol := map[string]model.Contract{}
	for _, mid := range model.SortedKeys(s.Modules) {
		for _, c := range s.Modules[mid].ExternalContracts {
			if c.Status == model.ItemRemoved {
				continue
			}
			stored[key{c.Kind, c.Direction, c.Selector}] = c
			if c.Symbol != "" {
				bySymbol[c.Kind+"|"+c.Symbol] = c
			}
		}
	}
	freshKeys := map[key]analysis.ContractFacts{}
	for _, f := range res.Contracts {
		freshKeys[key{f.Kind, f.Direction, f.Selector}] = f
	}
	var out []policy.ContractChange
	modified := map[string]bool{}
	for k, f := range freshKeys {
		if _, ok := stored[k]; ok {
			continue
		}
		if old, ok := bySymbol[f.Kind+"|"+f.Symbol]; ok && f.Symbol != "" {
			if _, still := freshKeys[key{old.Kind, old.Direction, old.Selector}]; !still {
				tier, br, why := policy.ContractTier(old, "modified")
				out = append(out, policy.ContractChange{ID: old.ID, Kind: old.Kind, Selector: old.Selector + " -> " + f.Selector, Change: "modified", Breaking: br, Tier: tier, Reason: why})
				modified[old.ID] = true
				continue
			}
		}
		tier, br, why := policy.ContractTier(model.Contract{}, "added")
		out = append(out, policy.ContractChange{Kind: f.Kind, Selector: f.Direction + " " + f.Selector, Change: "added", Breaking: br, Tier: tier, Reason: why})
	}
	for k, c := range stored {
		if _, ok := freshKeys[k]; ok || modified[c.ID] {
			continue
		}
		tier, br, why := policy.ContractTier(c, "removed")
		out = append(out, policy.ContractChange{ID: c.ID, Kind: c.Kind, Selector: c.Selector, Change: "removed", Breaking: br, Tier: tier, Reason: why})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Selector < out[j].Selector
	})
	return out
}

func goTestCmd(p string) string {
	if p == "." {
		return "go test ."
	}
	return "go test ./" + p
}

func maxTier(a, b model.Tier) model.Tier {
	rank := map[model.Tier]int{model.TierA: 0, model.TierB: 1, model.TierC: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
