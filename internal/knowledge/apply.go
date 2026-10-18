// Package knowledge folds fresh analysis facts into the SSOT. It is the one place where the
// mechanical layer is refreshed: bootstrap, reconcile and sync all call Apply, so EXTRACTED
// facts are updated deterministically and semantic fields are never touched here (PRD ch.07).
package knowledge

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/whyjp/for-my-harness/internal/analysis"
	"github.com/whyjp/for-my-harness/internal/identity"
	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/ssot"
)

type Options struct {
	Revision string
	Renames  map[string]string
	// SeedFeatures creates INFERRED feature candidates for new entry points. Bootstrap and
	// sync set it; reconcile does not invent features from an external change.
	SeedFeatures bool
}

type Report struct {
	New        []string `json:"new" yaml:"new"`
	Updated    []string `json:"updated" yaml:"updated"`
	Removed    []string `json:"removed" yaml:"removed"`
	Candidates []string `json:"candidates" yaml:"candidates"`
}

func (r *Report) Touched() []string {
	out := append(append(append([]string{}, r.New...), r.Updated...), r.Removed...)
	sort.Strings(out)
	return out
}

// item is the identity-relevant view shared by every object kind.
type item struct {
	anchors []model.Anchor
	name    string
}

// matcher holds what identity resolution needs beyond anchors: renames, which existing objects
// are removed, and which candidate pairs the user already rejected.
type matcher struct {
	s       *ssot.Snapshot
	renames map[string]string
	removed map[string]bool
}

// reconcile matches fresh items to existing IDs in two passes: exact (>=2 anchors) first so a
// one-anchor candidate can never steal an object that has a better match. A removed object can
// only come back on an exact match; one shared anchor with a dead object is not identity.
// A candidate the user already split is New, so the same question is not asked again.
func (mt matcher) reconcile(fresh []item, existing []identity.Existing) []identity.Match {
	out := make([]identity.Match, len(fresh))
	taken := map[string]bool{}
	for i, f := range fresh {
		m := identity.Resolve(f.anchors, existing, mt.renames, taken)
		if m.Verdict == identity.Same {
			out[i] = m
			taken[m.ID] = true
		}
	}
	for id := range mt.removed {
		taken[id] = true
	}
	for i, f := range fresh {
		if out[i].Verdict == identity.Same {
			continue
		}
		m := identity.Resolve(f.anchors, existing, mt.renames, taken)
		if m.Verdict == identity.Candidate && !rejectedPair(mt.s, m.ID, f.anchors) {
			taken[m.ID] = true
			out[i] = m
		} else {
			out[i] = identity.Match{Verdict: identity.New}
		}
	}
	return out
}

func rejectedPair(s *ssot.Snapshot, of string, anchors []model.Anchor) bool {
	for _, c := range s.Candidates {
		if c.CandidateOf == of && c.Confirmation == model.Rejected && sameAnchors(c.Anchors, anchors) {
			return true
		}
	}
	return false
}

// reconcileByKind matches each fresh contract only against existing contracts of the same ID
// kind, so a new route can never inherit the ID of a topic that shares its file.
func (mt matcher) reconcileByKind(fresh []item, existing []identity.Existing, kindOf func(int) model.Kind) []identity.Match {
	groups := map[model.Kind][]int{}
	for i := range fresh {
		groups[kindOf(i)] = append(groups[kindOf(i)], i)
	}
	out := make([]identity.Match, len(fresh))
	for k, idx := range groups {
		var ex []identity.Existing
		for _, e := range existing {
			if model.KindOf(e.ID) == k {
				ex = append(ex, e)
			}
		}
		sub := make([]item, len(idx))
		for j, i := range idx {
			sub[j] = fresh[i]
		}
		for j, m := range mt.reconcile(sub, ex) {
			out[idx[j]] = m
		}
	}
	return out
}

func Apply(s *ssot.Snapshot, res *analysis.Result, opt Options) Report {
	var rep Report
	modByPath := applyModules(s, res, opt, &rep)
	applyContracts(s, res, opt, modByPath, &rep)
	applyTests(s, res, opt, modByPath, &rep)
	if opt.SeedFeatures {
		seedFeatures(s, res, opt, &rep)
	}
	refreshFeatures(s, res, opt, &rep)
	rep.sort()
	return rep
}

// Seed creates INFERRED feature candidates for entry points no feature claims, then refreshes
// feature facts. Sync calls it after binding the features a turn delivered, so a delivered
// feature is never duplicated by a seeded guess.
func Seed(s *ssot.Snapshot, res *analysis.Result, opt Options) Report {
	var rep Report
	seedFeatures(s, res, opt, &rep)
	refreshFeatures(s, res, opt, &rep)
	rep.sort()
	return rep
}

func (r *Report) sort() {
	r.New, r.Updated, r.Removed = dedupe(r.New), dedupe(r.Updated), dedupe(r.Removed)
}

func moduleAnchors(m analysis.ModuleFacts) []model.Anchor {
	sym := m.ImportPath
	if sym == "" {
		sym = m.Path
	}
	return []model.Anchor{
		{Kind: model.AnchorPath, Ref: m.Path},
		{Kind: model.AnchorSymbol, Ref: m.Language + ":" + sym},
		{Kind: model.AnchorName, Ref: m.Language + ":" + m.Name},
	}
}

// candidate presents a one-anchor match instead of applying it (AC-BOOT-03). target is the
// object the fresh facts live in until the user decides: a new module or contract, or for a
// feature the entry contract it would absorb. Nothing semantic moves until confirm --merge.
func candidate(s *ssot.Snapshot, kind model.Kind, of, target, name string, anchors, matched []model.Anchor, rep *Report) {
	for _, c := range s.Candidates {
		if c.CandidateOf == of && sameAnchors(c.Anchors, anchors) {
			return
		}
	}
	id := s.NextID(model.KindCandidate)
	s.Candidates = append(s.Candidates, model.Candidate{
		ID: id, Kind: kind, CandidateOf: of, Target: target, Name: name, Anchors: anchors, Matched: matched,
		Confirmation: model.Unconfirmed,
	})
	rep.Candidates = append(rep.Candidates, id)
}

func applyModules(s *ssot.Snapshot, res *analysis.Result, opt Options, rep *Report) map[string]*model.Module {
	var existing []identity.Existing
	mt := matcher{s: s, renames: opt.Renames, removed: map[string]bool{}}
	for _, id := range model.SortedKeys(s.Modules) {
		existing = append(existing, identity.Existing{ID: id, Anchors: s.Modules[id].Anchors})
		if s.Modules[id].Status == model.ItemRemoved {
			mt.removed[id] = true
		}
	}
	fresh := make([]item, len(res.Modules))
	for i, m := range res.Modules {
		fresh[i] = item{anchors: moduleAnchors(m), name: m.Name}
	}
	matches := mt.reconcile(fresh, existing)
	byPath := map[string]*model.Module{}
	seen := map[string]bool{}
	for i, f := range res.Modules {
		var mod *model.Module
		switch matches[i].Verdict {
		case identity.Same:
			mod = s.Modules[matches[i].ID]
			rep.Updated = append(rep.Updated, mod.ID)
		default:
			id := s.NextID(model.KindModule)
			mod = &model.Module{ID: id, Responsibility: model.MustSemantic(inferResponsibility(f), model.Inferred)}
			s.Modules[id] = mod
			rep.New = append(rep.New, id)
			if matches[i].Verdict == identity.Candidate {
				candidate(s, model.KindModule, matches[i].ID, id, f.Path, fresh[i].anchors, matches[i].Matched, rep)
			}
		}
		seen[mod.ID] = true
		mod.Name, mod.Path, mod.Language, mod.Analyzer = f.Name, f.Path, f.Language, f.Analyzer
		mod.Status = model.ItemCurrent
		mod.Symbols = f.Symbols
		mod.Anchors = fresh[i].anchors
		mod.LastSyncedRevision = opt.Revision
		mod.Serving = nil
		byPath[f.Path] = mod
	}
	for _, e := range existing {
		if !seen[e.ID] && !mt.removed[e.ID] {
			s.Modules[e.ID].Status = model.ItemRemoved
			rep.Removed = append(rep.Removed, e.ID)
		}
	}
	for _, f := range res.Modules {
		mod := byPath[f.Path]
		mod.DependsOn = nil
		for _, imp := range f.Imports {
			if d, ok := byPath[imp]; ok {
				mod.DependsOn = append(mod.DependsOn, d.ID)
			}
		}
		sort.Strings(mod.DependsOn)
	}
	for _, sv := range res.Serving {
		for _, p := range sv.ModulePaths {
			if mod, ok := byPath[p]; ok {
				mod.Serving = append(mod.Serving, model.Serving{DeployUnit: sv.DeployUnit, Source: sv.Source, Origin: sv.Origin})
			}
		}
	}
	return byPath
}

// inferResponsibility states only what the structure shows; the wording marks it as a guess.
func inferResponsibility(f analysis.ModuleFacts) string {
	if f.IsMain {
		return fmt.Sprintf("(추정) %s 실행 진입점", path.Base(f.Path))
	}
	return fmt.Sprintf("(추정) %s 패키지가 노출하는 기능 묶음", f.Name)
}

func ContractKind(kind string) model.Kind {
	switch kind {
	case "kafka-topic", "queue", "webhook":
		return model.KindEvent
	case "db-table", "db-view", "cache-key":
		return model.KindData
	case "env-var", "config-key":
		return model.KindConfig
	default:
		return model.KindInterface
	}
}

func contractAnchors(c analysis.ContractFacts) []model.Anchor {
	a := []model.Anchor{{Kind: model.AnchorEntry, Ref: c.Kind + ":" + c.Direction + ":" + c.Selector}}
	if c.Location != "" {
		a = append(a, model.Anchor{Kind: model.AnchorPath, Ref: fileOf(c.Location)})
	}
	if c.Symbol != "" {
		a = append(a, model.Anchor{Kind: model.AnchorSymbol, Ref: c.Symbol})
	}
	return a
}

func fileOf(loc string) string {
	if i := strings.LastIndex(loc, ":"); i > 0 {
		return loc[:i]
	}
	return loc
}

func applyContracts(s *ssot.Snapshot, res *analysis.Result, opt Options, byPath map[string]*model.Module, rep *Report) {
	old := map[string]model.Contract{}
	oldOwner := map[string]*model.Module{}
	reserved := map[string]bool{}
	mt := matcher{s: s, renames: opt.Renames, removed: map[string]bool{}}
	var existing []identity.Existing
	for _, mid := range model.SortedKeys(s.Modules) {
		for _, c := range s.Modules[mid].ExternalContracts {
			// Removed contracts take part in exact matching so a contract that comes back gets its ID back.
			reserved[c.ID] = true
			if c.Status == model.ItemRemoved {
				mt.removed[c.ID] = true
			}
			old[c.ID] = c
			oldOwner[c.ID] = s.Modules[mid]
			existing = append(existing, identity.Existing{ID: c.ID, Anchors: c.Anchors})
		}
		s.Modules[mid].ExternalContracts = nil
	}
	fresh := make([]item, len(res.Contracts))
	for i, c := range res.Contracts {
		fresh[i] = item{anchors: contractAnchors(c), name: c.Selector}
	}
	matches := mt.reconcileByKind(fresh, existing, func(i int) model.Kind { return ContractKind(res.Contracts[i].Kind) })
	seen := map[string]bool{}
	for i, f := range res.Contracts {
		owner := byPath[f.OwnerPath]
		if owner == nil {
			continue
		}
		var c model.Contract
		switch matches[i].Verdict {
		case identity.Same:
			c = old[matches[i].ID]
			rep.Updated = append(rep.Updated, c.ID)
		default:
			c = model.Contract{
				ID:            nextContractID(s, ContractKind(f.Kind), reserved),
				Verification:  model.NotRun,
				Compatibility: model.MustSemantic("backward-compatible (추정)", model.Inferred),
			}
			rep.New = append(rep.New, c.ID)
			if matches[i].Verdict == identity.Candidate {
				candidate(s, ContractKind(f.Kind), matches[i].ID, c.ID, f.Selector, fresh[i].anchors, matches[i].Matched, rep)
			}
		}
		seen[c.ID] = true
		c.Kind, c.Direction, c.Selector, c.Location, c.Symbol = f.Kind, f.Direction, f.Selector, f.Location, f.Symbol
		c.Origin, c.Note = f.Origin, f.Note
		if c.Origin == "" {
			c.Origin = model.Extracted
		}
		c.Status = model.ItemCurrent
		c.Anchors = fresh[i].anchors
		owner.ExternalContracts = append(owner.ExternalContracts, c)
	}
	// Removed contracts stay visible on their last owner so a breaking removal is reviewable.
	for _, id := range model.SortedKeys(old) {
		if seen[id] {
			continue
		}
		c := old[id]
		if c.Status != model.ItemRemoved {
			rep.Removed = append(rep.Removed, id)
		}
		c.Status = model.ItemRemoved
		oldOwner[id].ExternalContracts = append(oldOwner[id].ExternalContracts, c)
	}
	for _, m := range s.Modules {
		sort.Slice(m.ExternalContracts, func(i, j int) bool { return m.ExternalContracts[i].ID < m.ExternalContracts[j].ID })
	}
}

// nextContractID must account for every contract ID detached at the start of the pass and
// those assigned earlier in it; neither is visible to Snapshot.AllIDs while detached.
func nextContractID(s *ssot.Snapshot, k model.Kind, pending map[string]bool) string {
	ids := s.AllIDs()
	for id := range pending {
		ids = append(ids, id)
	}
	id := model.NextID(k, ids)
	pending[id] = true
	return id
}

func testAnchors(t analysis.TestFacts) []model.Anchor {
	return []model.Anchor{
		{Kind: model.AnchorSymbol, Ref: t.ModulePath + "." + t.Name},
		{Kind: model.AnchorPath, Ref: fileOf(t.Location)},
		{Kind: model.AnchorName, Ref: t.Name},
	}
}

func applyTests(s *ssot.Snapshot, res *analysis.Result, opt Options, byPath map[string]*model.Module, rep *Report) {
	var existing []identity.Existing
	for _, id := range model.SortedKeys(s.Tests) {
		if s.Tests[id].Status != model.ItemRemoved {
			existing = append(existing, identity.Existing{ID: id, Anchors: s.Tests[id].Anchors})
		}
	}
	fresh := make([]item, len(res.Tests))
	for i, t := range res.Tests {
		fresh[i] = item{anchors: testAnchors(t), name: t.Name}
	}
	matches := matcher{s: s, renames: opt.Renames}.reconcile(fresh, existing)
	seen := map[string]bool{}
	for _, m := range s.Modules {
		m.Tests = nil
	}
	for i, f := range res.Tests {
		var t *model.Test
		if matches[i].Verdict != identity.Same {
			id := s.NextID(model.KindTest)
			t = &model.Test{ID: id, Verification: model.NotRun}
			s.Tests[id] = t
			rep.New = append(rep.New, id)
		} else {
			t = s.Tests[matches[i].ID]
		}
		seen[t.ID] = true
		t.Name, t.Location, t.Command, t.Origin = f.Name, f.Location, f.Command, model.Extracted
		t.Anchors = fresh[i].anchors
		t.Status = model.ItemCurrent
		if mod := byPath[f.ModulePath]; mod != nil {
			t.Module = mod.ID
			mod.Tests = append(mod.Tests, t.ID)
		}
	}
	for _, e := range existing {
		if !seen[e.ID] {
			s.Tests[e.ID].Status = model.ItemRemoved
			rep.Removed = append(rep.Removed, e.ID)
		}
	}
	for _, m := range s.Modules {
		sort.Strings(m.Tests)
	}
}

// entryKinds are contracts through which the outside world reaches the code; each one is a
// feature candidate. Produced topics, tables and env vars are used by features, not features.
var entryKinds = map[string]bool{"exposes": true, "consumes": true}

type entryFacts struct {
	contract model.Contract
	module   *model.Module
	anchors  []model.Anchor
}

func entryPoints(s *ssot.Snapshot) []entryFacts {
	var out []entryFacts
	for _, mid := range model.SortedKeys(s.Modules) {
		m := s.Modules[mid]
		if m.Status == model.ItemRemoved {
			continue
		}
		for _, c := range m.ExternalContracts {
			if c.Status == model.ItemRemoved || !entryKinds[c.Direction] {
				continue
			}
			a := []model.Anchor{{Kind: model.AnchorEntry, Ref: c.Kind + ":" + c.Selector}}
			if c.Symbol != "" {
				a = append(a, model.Anchor{Kind: model.AnchorSymbol, Ref: c.Symbol})
			}
			if c.Location != "" {
				a = append(a, model.Anchor{Kind: model.AnchorPath, Ref: fileOf(c.Location)})
			}
			out = append(out, entryFacts{contract: c, module: m, anchors: a})
		}
	}
	return out
}

// featureMatcher includes removed features so a route that comes back finds its old, possibly
// CONFIRMED, feature again, but only on an exact match.
func featureMatcher(s *ssot.Snapshot, opt Options) (matcher, []identity.Existing) {
	mt := matcher{s: s, renames: opt.Renames, removed: map[string]bool{}}
	var existing []identity.Existing
	for _, id := range model.SortedKeys(s.Features) {
		f := s.Features[id]
		if len(f.Anchors) == 0 {
			continue
		}
		existing = append(existing, identity.Existing{ID: id, Anchors: f.Anchors})
		if f.Status == model.ItemRemoved {
			mt.removed[id] = true
		}
	}
	return mt, existing
}

func seedFeatures(s *ssot.Snapshot, res *analysis.Result, opt Options, rep *Report) {
	entries := entryPoints(s)
	fresh := make([]item, len(entries))
	for i, e := range entries {
		fresh[i] = item{anchors: e.anchors}
	}
	mt, existing := featureMatcher(s, opt)
	matches := mt.reconcile(fresh, existing)
	for i, e := range entries {
		if matches[i].Verdict != identity.New {
			continue
		}
		if claimed := claimedBy(s, e.contract.ID); claimed != "" {
			continue
		}
		id := s.NextID(model.KindFeature)
		f := &model.Feature{
			ID:      id,
			Name:    featureName(e.contract),
			Status:  model.ItemCurrent,
			Intent:  model.Intent{Purpose: model.MustSemantic(fmt.Sprintf("(추정) %s `%s` 진입점이 제공하는 동작", e.contract.Kind, e.contract.Selector), model.Inferred)},
			Anchors: e.anchors,
		}
		s.Features[id] = f
		rep.New = append(rep.New, id)
	}
}

// claimedBy finds a feature that already lists the contract (e.g. a greenfield feature bound
// by sync), so seeding never duplicates it.
func claimedBy(s *ssot.Snapshot, contractID string) string {
	for _, id := range model.SortedKeys(s.Features) {
		f := s.Features[id]
		if f.Status == model.ItemRemoved {
			continue
		}
		for _, c := range f.ExternalContracts {
			if c == contractID {
				return id
			}
		}
	}
	return ""
}

func featureName(c model.Contract) string {
	switch c.Direction {
	case "consumes":
		return c.Selector + " 소비"
	default:
		return c.Selector + " 제공"
	}
}

// refreshFeatures recomputes every EXTRACTED field of features from current entry points:
// anchors, entry points, owning module, used contracts, observations, linked tests.
func refreshFeatures(s *ssot.Snapshot, res *analysis.Result, opt Options, rep *Report) {
	entries := entryPoints(s)
	fresh := make([]item, len(entries))
	for i, e := range entries {
		fresh[i] = item{anchors: e.anchors}
	}
	mt, existing := featureMatcher(s, opt)
	matches := mt.reconcile(fresh, existing)
	bound := map[string][]entryFacts{}
	held := map[string]bool{}
	for i, e := range entries {
		switch matches[i].Verdict {
		case identity.Same:
			bound[matches[i].ID] = append(bound[matches[i].ID], e)
		case identity.Candidate:
			// Presented, not applied: the feature keeps its last facts until the user decides.
			candidate(s, model.KindFeature, matches[i].ID, e.contract.ID, featureName(e.contract), e.anchors, matches[i].Matched, rep)
			held[matches[i].ID] = true
		default:
			if id := claimedBy(s, e.contract.ID); id != "" {
				bound[id] = append(bound[id], e)
			}
		}
	}
	calls := callIndex(res)
	for _, ex := range existing {
		f := s.Features[ex.ID]
		es := bound[ex.ID]
		if len(es) == 0 {
			if hasEntryAnchor(f) && !held[f.ID] && f.Status != model.ItemRemoved {
				f.Status = model.ItemRemoved
				rep.Removed = append(rep.Removed, f.ID)
			}
			continue
		}
		bindFeature(s, f, es, calls, opt.Revision)
		if !contains(rep.New, f.ID) {
			rep.Updated = append(rep.Updated, f.ID)
		}
	}
}

func hasEntryAnchor(f *model.Feature) bool {
	for _, a := range f.Anchors {
		if a.Kind == model.AnchorEntry {
			return true
		}
	}
	return false
}

// BindFeature attaches entry contracts to a feature (used by sync for greenfield features).
func bindFeature(s *ssot.Snapshot, f *model.Feature, es []entryFacts, calls map[string][]string, rev string) {
	f.Status = model.ItemCurrent
	f.Anchors, f.EntryPoints, f.ExternalContracts, f.OwnedBy, f.Observations = nil, nil, nil, nil, nil
	seenMod := map[string]bool{}
	var tests []string
	for _, e := range es {
		f.Anchors = append(f.Anchors, e.anchors...)
		f.EntryPoints = append(f.EntryPoints, model.EntryPoint{Kind: e.contract.Kind, Selector: e.contract.Selector, Contract: e.contract.ID})
		f.ExternalContracts = append(f.ExternalContracts, e.contract.ID)
		if !seenMod[e.module.ID] {
			seenMod[e.module.ID] = true
			f.OwnedBy = append(f.OwnedBy, e.module.ID)
			tests = append(tests, e.module.Tests...)
			// Contracts the owning module produces, owns or reads are used by the feature.
			for _, c := range e.module.ExternalContracts {
				if c.Status != model.ItemRemoved && !entryKinds[c.Direction] {
					f.ExternalContracts = append(f.ExternalContracts, c.ID)
				}
			}
		}
		handler := e.contract.Symbol
		if handler == "" {
			handler = e.contract.Selector
		}
		f.Observations = append(f.Observations, model.Observation{
			Behavior: fmt.Sprintf("%s %s `%s` is handled by %s", e.contract.Kind, e.contract.Direction, e.contract.Selector, handler),
			Origin:   model.Extracted, Verification: model.NotRun, Location: e.contract.Location,
		})
		for _, callee := range calls[e.contract.Symbol] {
			f.Observations = append(f.Observations, model.Observation{
				Behavior: fmt.Sprintf("%s calls %s", handler, callee),
				Origin:   model.Extracted, Verification: model.NotRun, Location: e.contract.Location,
			})
		}
	}
	f.ExternalContracts = dedupe(f.ExternalContracts)
	prev := map[string]model.TestRef{}
	for _, t := range f.Verification {
		prev[t.TestID] = t
	}
	f.Verification = nil
	for _, tid := range dedupe(tests) {
		t := s.Tests[tid]
		if t == nil || t.Status == model.ItemRemoved {
			continue
		}
		ref := model.TestRef{TestID: tid, Command: t.Command, Verification: t.Verification, Evidence: t.Evidence}
		if p, ok := prev[tid]; ok {
			ref.Expected = p.Expected
		}
		f.Verification = append(f.Verification, ref)
		f.Anchors = append(f.Anchors, model.Anchor{Kind: model.AnchorTest, Ref: tid})
	}
	f.Anchors = dedupeAnchors(f.Anchors)
	f.LastSynced.Revision = rev
}

// BindEntries binds the entry contracts with the given IDs to feature f. Sync uses it when a
// turn delivers a feature whose contracts appeared in the changed files.
func BindEntries(s *ssot.Snapshot, res *analysis.Result, f *model.Feature, contractIDs []string, rev string) {
	want := map[string]bool{}
	for _, id := range contractIDs {
		want[id] = true
	}
	for _, id := range f.ExternalContracts {
		want[id] = true
	}
	var es []entryFacts
	for _, e := range entryPoints(s) {
		if want[e.contract.ID] {
			es = append(es, e)
		}
	}
	if len(es) > 0 {
		bindFeature(s, f, es, callIndex(res), rev)
	}
}

func callIndex(res *analysis.Result) map[string][]string {
	out := map[string][]string{}
	if res == nil {
		return out
	}
	for _, m := range res.Modules {
		for _, c := range m.Calls {
			out[c.From] = append(out[c.From], c.To)
		}
	}
	for k := range out {
		out[k] = dedupe(out[k])
	}
	return out
}

func dedupe(in []string) []string {
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

func dedupeAnchors(in []model.Anchor) []model.Anchor {
	seen := map[model.Anchor]bool{}
	var out []model.Anchor
	for _, a := range in {
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func sameAnchors(a, b []model.Anchor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
