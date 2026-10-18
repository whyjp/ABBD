package workflow

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/graph"
	"github.com/whyjp/abbd/internal/impact"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/policy"
	"github.com/whyjp/abbd/internal/ssot"
	"github.com/whyjp/abbd/internal/verify"
)

type FeatureContext struct {
	Feature   *model.Feature    `json:"feature"`
	Modules   []ModuleSummary   `json:"modules"`
	Contracts []model.Contract  `json:"contracts"`
	Tests     []*model.Test     `json:"tests"`
	Changes   []ChangeSummary   `json:"changes"`
	Decisions []*model.Decision `json:"decisions"`
}

type ModuleSummary struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Path           string         `json:"path"`
	Responsibility model.Semantic `json:"responsibility"`
}

type ChangeSummary struct {
	ID      string          `json:"id"`
	Request string          `json:"request"`
	State   model.TurnState `json:"state"`
	Source  string          `json:"intent_source"`
}

type ArchitectureContext struct {
	Module     *model.Module     `json:"module"`
	Features   []string          `json:"related_features"`
	Dependent  []string          `json:"dependents"`
	Decisions  []*model.Decision `json:"decisions"`
	Violations []string          `json:"dependency_violations"`
}

type ContractContext struct {
	Contract *model.Contract `json:"contract"`
	Owner    string          `json:"owner"`
	UsedBy   []string        `json:"used_by"`
}

func (e *Engine) Inspect(id string) (any, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if model.ReservedKinds[model.KindOf(id)] {
		return nil, fmt.Errorf("%s: domain layer (%s) is reserved for Phase 3", id, model.KindOf(id))
	}
	w.countGraphQuery()
	return inspectIn(w, id)
}

func decisionsAbout(s *ssot.Snapshot, id string) []*model.Decision {
	var out []*model.Decision
	for _, did := range model.SortedKeys(s.Decisions) {
		if d := s.Decisions[did]; contains(d.Subjects, id) {
			out = append(out, d)
		}
	}
	return out
}

// inspectIn computes reverse relations (related_features, used_by) on read, because the
// SSOT stores each relation in one direction only (PRD ch.06 schema principle).
func inspectIn(w *world, id string) (any, error) {
	s := w.s
	if strings.HasPrefix(id, "run-") {
		ev, err := verify.Load(s.Root, id)
		if err != nil {
			return nil, fmt.Errorf("%s not found", id)
		}
		return ev, nil
	}
	switch model.KindOf(id) {
	case model.KindFeature:
		f := s.Features[id]
		if f == nil {
			break
		}
		ctx := &FeatureContext{Feature: f, Decisions: decisionsAbout(s, id)}
		for _, mid := range f.OwnedBy {
			if m := s.Modules[mid]; m != nil {
				ctx.Modules = append(ctx.Modules, ModuleSummary{m.ID, m.Name, m.Path, m.Responsibility})
			}
		}
		for _, cid := range f.ExternalContracts {
			if c, _ := s.Contract(cid); c != nil {
				ctx.Contracts = append(ctx.Contracts, *c)
			}
		}
		for _, t := range f.Verification {
			if tt := s.Tests[t.TestID]; tt != nil {
				ctx.Tests = append(ctx.Tests, tt)
			}
		}
		for _, cid := range f.Changes {
			if c := s.Changes[cid]; c != nil {
				ctx.Changes = append(ctx.Changes, ChangeSummary{c.ID, c.RequestSummary, c.State, c.IntentSource})
			}
		}
		return ctx, nil
	case model.KindModule:
		m := s.Modules[id]
		if m == nil {
			break
		}
		ctx := &ArchitectureContext{Module: m, Decisions: decisionsAbout(s, id)}
		for _, fid := range model.SortedKeys(s.Features) {
			if contains(s.Features[fid].OwnedBy, id) {
				ctx.Features = append(ctx.Features, fid)
			}
		}
		for _, oid := range model.SortedKeys(s.Modules) {
			if contains(s.Modules[oid].DependsOn, id) {
				ctx.Dependent = append(ctx.Dependent, oid)
			}
		}
		for _, d := range m.DependsOn {
			if contains(m.DependencyPolicy.Forbidden, d) {
				ctx.Violations = append(ctx.Violations, d)
			}
		}
		return ctx, nil
	case model.KindInterface, model.KindEvent, model.KindData, model.KindConfig:
		c, owner := s.Contract(id)
		if c == nil {
			break
		}
		ctx := &ContractContext{Contract: c, Owner: owner.ID}
		for _, fid := range model.SortedKeys(s.Features) {
			if contains(s.Features[fid].ExternalContracts, id) {
				ctx.UsedBy = append(ctx.UsedBy, fid)
			}
		}
		return ctx, nil
	case model.KindTest:
		if t := s.Tests[id]; t != nil {
			return t, nil
		}
	case model.KindDecision:
		if d := s.Decisions[id]; d != nil {
			return d, nil
		}
	case model.KindChange:
		if c := s.Changes[id]; c != nil {
			return c, nil
		}
	case model.KindRequirement:
		if r := s.Requirements[id]; r != nil {
			return r, nil
		}
	case model.KindCandidate:
		for _, c := range s.Candidates {
			if c.ID == id {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("%s not found", id)
}

type ContractRow struct {
	model.Contract
	Owner string `json:"owner"`
}

func (e *Engine) Contracts(module string) ([]ContractRow, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	w.countGraphQuery()
	var out []ContractRow
	for _, mid := range model.SortedKeys(w.s.Modules) {
		if module != "" && mid != module {
			continue
		}
		for _, c := range w.s.Modules[mid].ExternalContracts {
			out = append(out, ContractRow{c, mid})
		}
	}
	return out, nil
}

type ContractImpact struct {
	Changes     []policy.ContractChange `json:"changes"`
	HighestTier model.Tier              `json:"highest_tier"`
}

func (e *Engine) ContractsImpact() (*ContractImpact, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	w.countGraphQuery()
	ci := &ContractImpact{Changes: impact.ContractDiff(w.s, res), HighestTier: model.TierA}
	for _, c := range ci.Changes {
		if c.Tier == model.TierC || (c.Tier == model.TierB && ci.HighestTier == model.TierA) {
			ci.HighestTier = c.Tier
		}
	}
	return ci, nil
}

func (e *Engine) Impact() (*impact.Report, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	w.countGraphQuery()
	r := impact.Analyze(w.s, res, w.changedSinceSync())
	return &r, nil
}

type GraphAnswer struct {
	Freshness model.Freshness `json:"freshness"`
	Path      []graph.Edge    `json:"path,omitempty"`
	Subgraph  *graph.Subgraph `json:"subgraph,omitempty"`
	Found     bool            `json:"found"`
	Note      string          `json:"note,omitempty"`
}

func (e *Engine) Trace(from, to string) (*GraphAnswer, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	g, fresh, err := e.queryGraph(w)
	if err != nil {
		return nil, err
	}
	w.countGraphQuery()
	p, ok := g.Trace(from, to)
	ans := &GraphAnswer{Freshness: fresh, Path: p, Found: ok}
	if !ok {
		ans.Note = "경로 없음: 정적 그래프에 없다는 뜻이지 관계가 없다는 뜻은 아니다 (UNVERIFIED)"
	}
	return ans, nil
}

func (e *Engine) Neighbors(id string, depth int) (*GraphAnswer, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	g, fresh, err := e.queryGraph(w)
	if err != nil {
		return nil, err
	}
	w.countGraphQuery()
	if depth <= 0 {
		depth = 2
	}
	sub := g.Neighbors(id, depth, 60)
	_, found := g.Node(id)
	return &GraphAnswer{Freshness: fresh, Subgraph: &sub, Found: found}, nil
}

type VerificationPlan struct {
	Change   string   `json:"change"`
	Tier     string   `json:"verification_tier"`
	Commands []string `json:"commands"`
	Tests    []string `json:"tests"`
	Expected []string `json:"expected"`
	Unknowns []string `json:"unknowns"`
}

func (e *Engine) VerificationPlan() (*VerificationPlan, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	imp := impact.Analyze(w.s, res, w.changedSinceSync())
	vp := &VerificationPlan{Commands: append(append([]string{}, w.s.Project.Verification.BuildCommands...), imp.TestCommands...), Tests: imp.Tests, Unknowns: imp.Unknowns}
	if c := w.s.ActiveChange(); c != nil {
		vp.Change, vp.Tier = c.ID, c.Plan.VerificationTier
		if vp.Tier == "T2" || vp.Tier == "T3" {
			vp.Commands = append(vp.Commands, w.s.Project.Verification.TestCommands...)
		}
	}
	for _, fid := range imp.Features {
		for _, t := range w.s.Features[fid].Verification {
			if t.Expected != "" {
				vp.Expected = append(vp.Expected, fid+": "+t.Expected)
			}
		}
	}
	vp.Commands = uniqStrings(vp.Commands)
	return vp, nil
}

func (e *Engine) Classify(action string) (policy.Rule, error) {
	w, err := e.load()
	if err != nil {
		return policy.Classify(ssot.DefaultProject("", ""), action)
	}
	return policy.Classify(w.s.Project, action)
}
