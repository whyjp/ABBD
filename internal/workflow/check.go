package workflow

import (
	"fmt"
	"sort"

	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/ssot"
	"github.com/whyjp/for-my-harness/internal/verify"
)

type ConsistencyReport struct {
	OK        bool      `json:"ok"`
	Errors    []string  `json:"errors"`
	Warnings  []string  `json:"warnings"`
	Stale     []string  `json:"stale"`
	Freshness Freshness `json:"freshness"`
}

// Check is `myharness check`: schema, provenance rules, reference integrity, evidence links,
// dependency policy and cross-artifact freshness (FR-06, FR-29).
func (e *Engine) Check() (*ConsistencyReport, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	r := checkSnapshot(w.s)
	r.Freshness = w.freshness()
	for name, f := range map[string]model.Freshness{"feature_map": r.Freshness.FeatureMap, "graph": r.Freshness.Graph, "html": r.Freshness.HTML} {
		if f != model.Current {
			r.Stale = append(r.Stale, fmt.Sprintf("%s %s", name, f))
		}
	}
	sort.Strings(r.Stale)
	return r, nil
}

func checkSnapshot(s *ssot.Snapshot) *ConsistencyReport {
	r := &ConsistencyReport{}
	errf := func(format string, a ...any) { r.Errors = append(r.Errors, fmt.Sprintf(format, a...)) }
	warnf := func(format string, a ...any) { r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...)) }

	sem := func(owner, name string, v model.Semantic) {
		if v.Value == "" {
			return
		}
		if !v.Origin.SemanticAllowed() {
			errf("%s.%s: origin %s not allowed for a why-field (USER_STATED|ADOPTED|INFERRED)", owner, name, v.Origin)
		}
		if !v.Confirmation.Valid() {
			errf("%s.%s: invalid confirmation %q", owner, name, v.Confirmation)
		}
	}
	evidence := func(owner string, v model.Verification, ev []string) {
		if !v.Valid() {
			errf("%s: invalid verification %q", owner, v)
		}
		if v == model.Passed && len(ev) == 0 {
			errf("%s: verification PASSED without an evidence link", owner)
		}
		for _, id := range ev {
			if model.KindOf(id) == model.KindTest {
				if s.Tests[id] == nil {
					errf("%s: evidence %s does not exist", owner, id)
				}
			} else if !verify.Exists(s.Root, id) {
				errf("%s: evidence run %s not found", owner, id)
			}
		}
	}
	ref := func(owner, id string, ok bool) {
		if !ok {
			errf("%s: dangling reference %s", owner, id)
		}
	}

	contractSeen := map[string]string{}
	for _, mid := range model.SortedKeys(s.Modules) {
		m := s.Modules[mid]
		sem(mid, "responsibility", m.Responsibility)
		if o := m.DependencyPolicy.Origin; o != "" && !o.SemanticAllowed() {
			errf("%s.dependency_policy: origin %s not allowed", mid, o)
		}
		for _, d := range m.DependsOn {
			ref(mid, d, s.Modules[d] != nil)
			if contains(m.DependencyPolicy.Forbidden, d) {
				warnf("%s depends on forbidden %s (architecture drift)", mid, d)
			}
		}
		for _, t := range m.Tests {
			ref(mid, t, s.Tests[t] != nil)
		}
		for _, c := range m.ExternalContracts {
			if prev, dup := contractSeen[c.ID]; dup {
				errf("%s stored in both %s and %s (a contract lives in its owning module only)", c.ID, prev, mid)
			}
			contractSeen[c.ID] = mid
			if !c.Origin.Valid() {
				errf("%s: invalid origin %q", c.ID, c.Origin)
			}
			sem(c.ID, "compatibility", c.Compatibility)
			if o := c.ConsumersOrigin; o != "" && o != model.UserStated && o != model.Inferred {
				errf("%s.external_consumers: origin %s not allowed (USER_STATED|INFERRED)", c.ID, o)
			}
			var ev []string
			if c.Verification == model.Passed {
				ev = []string{c.VerificationNote}
			}
			evidence(c.ID, c.Verification, ev)
			if c.Verification == model.Unverifiable && c.VerificationNote == "" {
				errf("%s: UNVERIFIABLE without a reason", c.ID)
			}
		}
	}
	for _, fid := range model.SortedKeys(s.Features) {
		f := s.Features[fid]
		sem(fid, "purpose", f.Intent.Purpose)
		for i, inv := range f.Intent.Invariants {
			owner := fmt.Sprintf("%s.invariants[%d]", fid, i)
			sem(owner, "claim", model.Semantic{Value: inv.Claim, Origin: inv.Origin, Confirmation: inv.Confirmation})
			evidence(owner, inv.Verification, inv.Evidence)
		}
		for i, o := range f.Observations {
			if o.Origin != model.Extracted && o.Origin != model.Observed {
				errf("%s.observations[%d]: an observation records what the code does; origin %s", fid, i, o.Origin)
			}
			evidence(fmt.Sprintf("%s.observations[%d]", fid, i), o.Verification, o.Evidence)
		}
		for _, m := range f.OwnedBy {
			ref(fid, m, s.Modules[m] != nil)
		}
		for _, c := range f.ExternalContracts {
			cc, _ := s.Contract(c)
			ref(fid, c, cc != nil)
		}
		for _, t := range f.Verification {
			ref(fid, t.TestID, s.Tests[t.TestID] != nil)
			var ev []string
			if t.Verification == model.Passed {
				ev = []string{t.Evidence}
			}
			evidence(fid+"."+t.TestID, t.Verification, ev)
		}
		for _, c := range f.Changes {
			ref(fid, c, s.Changes[c] != nil)
		}
		if f.Status == model.ItemCurrent && len(f.Anchors) == 0 {
			warnf("%s has no anchors: its identity cannot survive a re-run", fid)
		}
	}
	for _, tid := range model.SortedKeys(s.Tests) {
		t := s.Tests[tid]
		var ev []string
		if t.Verification == model.Passed {
			ev = []string{t.Evidence}
		}
		evidence(tid, t.Verification, ev)
	}
	for _, cid := range model.SortedKeys(s.Changes) {
		c := s.Changes[cid]
		if c.Verification == model.Passed {
			evidence(cid, c.Verification, c.VerificationEvidence)
		}
		for _, id := range c.Intends {
			ref(cid, id, s.Requirements[id] != nil)
		}
		for _, id := range c.Delivers {
			ref(cid, id, s.Features[id] != nil)
		}
		for _, d := range c.Decisions {
			ref(cid, d.ADR, s.Decisions[d.ADR] != nil)
		}
		if c.State == model.StateCompleted && c.Verification == model.Failed {
			errf("%s: COMPLETED with FAILED verification", cid)
		}
	}
	for _, did := range model.SortedKeys(s.Decisions) {
		d := s.Decisions[did]
		sem(did, "rationale", d.Rationale)
		if d.Answered && d.Confirmation != model.Confirmed {
			errf("%s: answered but not CONFIRMED", did)
		}
	}
	for _, id := range s.AllIDs() {
		if model.ReservedKinds[model.KindOf(id)] {
			errf("%s: domain layer object exists, but Phase 1 only reserves the prefix", id)
		}
	}
	r.OK = len(r.Errors) == 0
	return r
}
