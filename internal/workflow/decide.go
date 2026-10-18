package workflow

import (
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/html"
	"github.com/whyjp/abbd/internal/impact"
	"github.com/whyjp/abbd/internal/knowledge"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/policy"
	"github.com/whyjp/abbd/internal/ssot"
)

type DecideInput struct {
	// ByUser marks the answer as the user's own. Without it an answer is the agent's choice:
	// recorded, INFERRED, and still open for the user (an agent cannot approve tier C).
	ByUser          bool
	ADR             string
	Reason          string
	Title           string
	Tier            model.Tier
	Question        string
	Options         []model.Option
	Default         string
	Answer          string
	Subjects        []string
	ProceedsWithout string
}

type DecideResult struct {
	Decision *model.Decision `json:"decision"`
	Ask      bool            `json:"ask"`
	Reused   bool            `json:"reused"`
	Blocked  bool            `json:"blocked"`
	Message  string          `json:"message"`
}

func normQuestion(q string) string { return strings.Join(strings.Fields(strings.ToLower(q)), " ") }

// recordDecision stores an ADR and links it to the turn. Unanswered means the default is
// taken and the ADR stays UNCONFIRMED and reversible.
func recordDecision(s *ssot.Snapshot, chg *model.Change, in DecideInput, asked bool) *model.Decision {
	d := &model.Decision{
		ID: s.NextID(model.KindDecision), Title: in.Title, Tier: in.Tier, Question: in.Question, Options: in.Options,
		Default: in.Default, Chosen: in.Default, Asked: asked, ProceedsWithout: in.ProceedsWithout,
		Rationale: model.MustSemantic("기본값으로 진행 (답 없음)", model.Inferred), Change: chg.ID, Subjects: in.Subjects,
		Confirmation: model.Unconfirmed,
	}
	if d.Title == "" {
		d.Title = in.Question
	}
	switch {
	case in.Answer != "" && in.ByUser:
		answer(d, in.Answer, chg)
	case in.Answer != "":
		d.Chosen = in.Answer
		d.Rationale = model.MustSemantic("에이전트 선택 (사용자 확인 대기)", model.Inferred)
	}
	s.Decisions[d.ID] = d
	chg.Decisions = append(chg.Decisions, model.DecisionRef{ADR: d.ID, Tier: d.Tier, Answered: d.Answered})
	return d
}

func answer(d *model.Decision, key string, chg *model.Change) {
	d.Chosen, d.Answered, d.Confirmation = key, true, model.Confirmed
	d.Rationale = model.MustSemantic("사용자 답변", model.UserStated)
	chg.IntentDelta.Additions = append(chg.IntentDelta.Additions, fmt.Sprintf("%s: %s → %s", d.ID, d.Question, key))
	for i := range chg.Decisions {
		if chg.Decisions[i].ADR == d.ID {
			chg.Decisions[i].Answered = true
		}
	}
}

// Decide records a tier B question (ask with a default) or a tier C stop. A question already
// answered in any ADR is never asked again; past the per-turn budget it is not asked at all.
func (e *Engine) Decide(in DecideInput) (*DecideResult, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if in.ADR != "" {
		return e.answerADR(w, in)
	}
	chg := w.s.ActiveChange()
	if chg == nil {
		return nil, fmt.Errorf("decisions belong to a turn; start one with `abbd run` (bootstrap never asks); an open ADR is answered with --adr ADR-x --answer KEY")
	}
	if in.Tier == "" {
		in.Tier = model.TierB
	}
	if in.Tier == model.TierB && (len(in.Options) < 2 || len(in.Options) > 3) {
		return nil, fmt.Errorf("a tier B question offers 2 or 3 options, got %d", len(in.Options))
	}
	if in.Default == "" && len(in.Options) > 0 {
		in.Default = in.Options[0].Key
	}
	q := normQuestion(in.Question)
	for _, id := range model.SortedKeys(w.s.Decisions) {
		d := w.s.Decisions[id]
		if normQuestion(d.Question) != q {
			continue
		}
		if d.Answered {
			if !contains(decisionIDs(chg), d.ID) {
				chg.Decisions = append(chg.Decisions, model.DecisionRef{ADR: d.ID, Tier: d.Tier, Answered: true})
			}
			return &DecideResult{Decision: d, Reused: true, Message: "이미 답한 질문: " + d.ID + " → " + d.Chosen}, w.s.Save()
		}
		if in.Answer != "" && in.ByUser {
			answer(d, in.Answer, chg)
			return &DecideResult{Decision: d, Message: "답변 반영: " + d.ID}, w.s.Save()
		}
		if d.Tier == model.TierC {
			return &DecideResult{Decision: d, Blocked: true, Message: "등급 C: 아직 승인되지 않음 (" + d.ID + ")"}, nil
		}
		return &DecideResult{Decision: d, Message: "같은 질문이 이미 기본값으로 진행 중: " + d.ID}, nil
	}

	if in.Tier == model.TierC {
		d := recordDecision(w.s, chg, in, true)
		res := &DecideResult{Decision: d, Ask: !d.Answered}
		if d.Answered && d.Chosen == "approve" && in.ByUser {
			res.Message = "등급 C 승인됨: " + d.ID
		} else {
			res.Blocked = true
			chg.Blocked = append(chg.Blocked, d.ID+": "+d.Question)
			res.Message = "등급 C: 명시적 승인 전까지 실행하지 않는다. 가역적 준비 작업까지만 진행 (승인: --answer approve)"
		}
		return res, w.s.Save()
	}

	ask := (in.Answer == "" || !in.ByUser) && policy.CanAsk(w.s.Project, chg.Telemetry.QuestionsAsked)
	if ask {
		chg.Telemetry.QuestionsAsked++
	}
	d := recordDecision(w.s, chg, in, ask)
	res := &DecideResult{Decision: d, Ask: ask}
	switch {
	case d.Answered:
		res.Message = "답변 반영: " + d.ID
	case ask:
		res.Message = fmt.Sprintf("사용자에게 묻되 이 턴 안에 답이 없으면 기본값 %q로 진행 (%s, UNCONFIRMED)", d.Default, d.ID)
	default:
		res.Message = fmt.Sprintf("턴당 질문 상한 초과: 묻지 않고 기본값 %q로 진행 (%s, UNCONFIRMED)", d.Default, d.ID)
	}
	return res, w.s.Save()
}

// answerADR answers a decision recorded earlier (by sync, reconcile or a past turn) without
// retyping its question, inside or outside a turn.
func (e *Engine) answerADR(w *world, in DecideInput) (*DecideResult, error) {
	d := w.s.Decisions[in.ADR]
	if d == nil {
		return nil, fmt.Errorf("%s not found", in.ADR)
	}
	if in.Answer == "" {
		return &DecideResult{Decision: d, Ask: !d.Answered, Message: "현재 상태"}, nil
	}
	if !in.ByUser {
		return nil, fmt.Errorf("answering %s records the user's decision; pass --user only with the user's own answer", d.ID)
	}
	valid := d.Tier == model.TierC && (in.Answer == "approve" || in.Answer == "reject")
	for _, o := range d.Options {
		valid = valid || o.Key == in.Answer
	}
	if !valid {
		return nil, fmt.Errorf("%s has no option %q", d.ID, in.Answer)
	}
	// Every channel writes the same record: an ADR that belongs to a structural change card is
	// decided on the card, so the card status, rework and invalidation all follow (08B §6).
	if delta := deltaForADR(w.s, d.ID); delta != "" {
		decision := in.Answer
		if decision != "approve" && decision != "reject" {
			decision = "option:" + decision
		}
		reason := in.Reason
		if reason == "" {
			reason = "decide --adr " + d.ID
		}
		if _, _, err := e.decideIn(w, DeltaDecisionInput{Delta: delta, Decision: decision, Reason: reason, Channel: "cli", ByUser: true}); err != nil {
			return nil, err
		}
		return &DecideResult{Decision: d, Message: "카드 결정 반영: " + delta}, e.republish(w, "결정 반영 "+d.ID)
	}
	chg := w.s.Changes[d.Change]
	if chg == nil {
		chg = &model.Change{}
	}
	answer(d, in.Answer, chg)
	return &DecideResult{Decision: d, Message: "답변 반영: " + d.ID}, e.republish(w, "결정 반영 "+d.ID)
}

func deltaForADR(s *ssot.Snapshot, adr string) string {
	for _, c := range s.Changes {
		for _, d := range c.StructuralDeltas {
			if d.ADR == adr {
				return d.ID
			}
		}
	}
	return ""
}

func decisionIDs(chg *model.Change) []string {
	var out []string
	for _, d := range chg.Decisions {
		out = append(out, d.ADR)
	}
	return out
}

func (e *Engine) Decisions(query string) ([]*model.Decision, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	var out []*model.Decision
	q := strings.ToLower(query)
	for _, id := range model.SortedKeys(w.s.Decisions) {
		d := w.s.Decisions[id]
		if q == "" || strings.Contains(strings.ToLower(d.Question+" "+d.Title+" "+strings.Join(d.Subjects, " ")), q) {
			out = append(out, d)
		}
	}
	return out, nil
}

type ConfirmInput struct {
	ID                string
	Value             string
	Reject            bool
	Merge             string
	Split             string
	ExternalConsumers string
	All               bool
}

// Confirm is lazy confirmation. Only this path promotes an INFERRED meaning to CONFIRMED.
func (e *Engine) Confirm(in ConfirmInput) ([]string, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	where := "confirm-" + now()[:10]
	if c := w.s.ActiveChange(); c != nil {
		where = c.ID
	}
	var done []string
	switch {
	case in.Merge != "":
		err = e.resolveCandidate(w, in.Merge, true)
		done = append(done, in.Merge)
	case in.Split != "":
		err = e.resolveCandidate(w, in.Split, false)
		done = append(done, in.Split)
	case in.All:
		for _, p := range html.Pending(w.s) {
			if p.Kind == "candidate" || p.Field == "external_consumers" {
				continue
			}
			if err := confirmOne(w.s, p.ID, p.Field, "", false, where); err == nil {
				done = append(done, p.ID+"."+p.Field)
			}
		}
	default:
		if in.ExternalConsumers != "" {
			c, _ := w.s.Contract(in.ID)
			if c == nil {
				return nil, fmt.Errorf("%s is not a contract", in.ID)
			}
			v := in.ExternalConsumers == "true"
			c.ExternalConsumers, c.ConsumersOrigin = &v, model.UserStated
			done = append(done, in.ID+".external_consumers")
		}
		if in.ExternalConsumers == "" || in.Value != "" {
			if err = confirmOne(w.s, in.ID, "", in.Value, in.Reject, where); err == nil {
				done = append(done, in.ID)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return done, e.republish(w, "확인 반영")
}

// field returns the primary meaning field of an object.
func field(s *ssot.Snapshot, id string) (*model.Semantic, error) {
	if f := s.Features[id]; f != nil {
		return &f.Intent.Purpose, nil
	}
	if m := s.Modules[id]; m != nil {
		return &m.Responsibility, nil
	}
	if c, _ := s.Contract(id); c != nil {
		return &c.Compatibility, nil
	}
	if d := s.Decisions[id]; d != nil {
		return &d.Rationale, nil
	}
	return nil, fmt.Errorf("%s has no confirmable meaning field", id)
}

func confirmOne(s *ssot.Snapshot, id, fieldName, value string, reject bool, where string) error {
	if fieldName == "invariant" {
		f := s.Features[id]
		for i := range f.Intent.Invariants {
			if f.Intent.Invariants[i].Confirmation == model.Unconfirmed {
				f.Intent.Invariants[i].Confirmation = model.Confirmed
			}
		}
		return nil
	}
	sem, err := field(s, id)
	if err != nil {
		return err
	}
	switch {
	case reject && value == "":
		sem.Confirmation = model.Rejected
	case value != "":
		if reject {
			sem.Replaced = sem.Value
		}
		*sem = model.Semantic{Value: value, Origin: model.UserStated, Confirmation: model.Confirmed, ConfirmedIn: where, Replaced: sem.Replaced}
	default:
		// The user saw the inferred text and accepted it: origin stays INFERRED, confirmation moves.
		sem.Confirmation, sem.ConfirmedIn = model.Confirmed, where
	}
	if d := s.Decisions[id]; d != nil {
		d.Confirmation = sem.Confirmation
	}
	return nil
}

// resolveCandidate settles a one-anchor match. Merge keeps the old ID (that is what a stable ID
// is for): the fresh facts move under it and its anchors are refreshed so the next re-run
// matches exactly. Split rejects the pair; the fresh object keeps its own new ID, and a
// rejected feature pair is seeded as a new INFERRED feature on the next sync.
func (e *Engine) resolveCandidate(w *world, id string, merge bool) error {
	for i := range w.s.Candidates {
		c := &w.s.Candidates[i]
		if c.ID != id {
			continue
		}
		if c.Confirmation != model.Unconfirmed {
			return fmt.Errorf("%s already resolved (%s)", id, c.Confirmation)
		}
		if !merge {
			c.Confirmation = model.Rejected
			return nil
		}
		switch c.Kind {
		case model.KindFeature:
			f := w.s.Features[c.CandidateOf]
			if f == nil {
				return fmt.Errorf("feature %s not found", c.CandidateOf)
			}
			f.ExternalContracts = uniqStrings(append(f.ExternalContracts, c.Target))
			f.Anchors = append(f.Anchors, c.Anchors...)
			f.Status = model.ItemCurrent
		case model.KindModule:
			old, fresh := w.s.Modules[c.CandidateOf], w.s.Modules[c.Target]
			if old == nil || fresh == nil {
				return fmt.Errorf("candidate %s refers to a missing module", id)
			}
			old.Name, old.Path, old.Language, old.Analyzer, old.Status = fresh.Name, fresh.Path, fresh.Language, fresh.Analyzer, model.ItemCurrent
			old.Anchors, old.Symbols, old.Tests, old.DependsOn, old.Serving = fresh.Anchors, fresh.Symbols, fresh.Tests, fresh.DependsOn, fresh.Serving
			old.ExternalContracts = append(old.ExternalContracts, fresh.ExternalContracts...)
			delete(w.s.Modules, fresh.ID)
			repoint(w.s, fresh.ID, old.ID)
		default:
			fresh, freshOwner := w.s.Contract(c.Target)
			old, oldOwner := w.s.Contract(c.CandidateOf)
			if fresh == nil || old == nil {
				return fmt.Errorf("candidate %s refers to a missing contract", id)
			}
			merged := *fresh
			merged.ID, merged.Compatibility, merged.ExternalConsumers, merged.ConsumersOrigin = old.ID, old.Compatibility, old.ExternalConsumers, old.ConsumersOrigin
			oldOwner.ExternalContracts = dropContract(oldOwner.ExternalContracts, old.ID)
			freshOwner.ExternalContracts = append(dropContract(freshOwner.ExternalContracts, fresh.ID), merged)
			repoint(w.s, c.Target, c.CandidateOf)
		}
		c.Confirmation = model.Confirmed
		return nil
	}
	return fmt.Errorf("candidate %s not found", id)
}

func dropContract(cs []model.Contract, id string) []model.Contract {
	var out []model.Contract
	for _, c := range cs {
		if c.ID != id {
			out = append(out, c)
		}
	}
	return out
}

// repoint rewrites references from one ID to another after a merge.
func repoint(s *ssot.Snapshot, from, to string) {
	swap := func(xs []string) []string {
		for i := range xs {
			if xs[i] == from {
				xs[i] = to
			}
		}
		return uniqStrings(xs)
	}
	for _, f := range s.Features {
		f.OwnedBy, f.ExternalContracts = swap(f.OwnedBy), swap(f.ExternalContracts)
		for i := range f.EntryPoints {
			if f.EntryPoints[i].Contract == from {
				f.EntryPoints[i].Contract = to
			}
		}
	}
	for _, m := range s.Modules {
		m.DependsOn = swap(m.DependsOn)
	}
	for _, t := range s.Tests {
		if t.Module == from {
			t.Module = to
		}
	}
}

func (e *Engine) republish(w *world, note string) error {
	res, err := w.analyze()
	if err != nil {
		return err
	}
	// Meaning edits never move the source watermark: settle only if the workspace already matches.
	settle := w.s.ActiveChange() == nil && w.s.State.Fingerprint == w.ws.Fingerprint
	_, err = knowledge.Publish(w.s, res, w.ws, settle, note)
	return err
}

type IntentFeatureInput struct {
	ID, Name, Purpose string
	Origin            model.Origin
	Anchors           []model.Anchor
}

// IntentFeature records the resulting intent of the active turn. An agent-authored meaning is
// INFERRED; replacing a CONFIRMED purpose that way is refused: it is a tier B decision.
func (e *Engine) IntentFeature(in IntentFeatureInput) (*model.Feature, error) {
	w, chg, err := e.activeTurn()
	if err != nil {
		return nil, err
	}
	origin := in.Origin
	if origin == "" {
		origin = model.Inferred
	}
	f := w.s.Features[in.ID]
	if f == nil {
		f = &model.Feature{ID: w.s.NextID(model.KindFeature), Status: model.ItemDraft, Changes: []string{chg.ID}, Requirements: chg.Intends}
		w.s.Features[f.ID] = f
		chg.Delivers = append(chg.Delivers, f.ID)
	}
	if in.Name != "" {
		f.Name = in.Name
	}
	if in.Purpose != "" && origin != model.UserStated && f.Status != model.ItemDraft {
		if err := e.inScope(w, chg, f.ID); err != nil {
			return nil, err
		}
	}
	if in.Purpose != "" {
		if confirmed(f.Intent.Purpose) && origin != model.UserStated && f.Intent.Purpose.Value != in.Purpose {
			return nil, fmt.Errorf("%s purpose is CONFIRMED (%q); changing it is a tier B decision: `abbd decide --subject %s`", f.ID, f.Intent.Purpose.Value, f.ID)
		}
		sem, err := model.NewSemantic(in.Purpose, origin)
		if err != nil {
			return nil, err
		}
		sem.ConfirmedIn = chg.ID
		f.Intent.Purpose = sem
	}
	f.Anchors = append(f.Anchors, in.Anchors...)
	if !contains(f.Changes, chg.ID) {
		f.Changes = append(f.Changes, chg.ID)
	}
	if !contains(chg.Delivers, f.ID) && !contains(chg.Impacted, f.ID) {
		chg.Impacted = append(chg.Impacted, f.ID)
	}
	return f, w.s.Save()
}

type IntentInvariantInput struct {
	Feature, Claim, Test string
	Origin               model.Origin
}

func (e *Engine) IntentInvariant(in IntentInvariantInput) (*model.Feature, error) {
	w, chg, err := e.activeTurn()
	if err != nil {
		return nil, err
	}
	f := w.s.Features[in.Feature]
	if f == nil {
		return nil, fmt.Errorf("feature %s not found", in.Feature)
	}
	origin := in.Origin
	if origin == "" {
		origin = model.Inferred
	}
	if !origin.SemanticAllowed() {
		return nil, fmt.Errorf("an invariant claim cannot have origin %s", origin)
	}
	inv := model.Invariant{Claim: in.Claim, Origin: origin, Confirmation: model.Unconfirmed, Verification: model.NotRun}
	if origin == model.UserStated {
		inv.Confirmation = model.Confirmed
	}
	if in.Test != "" {
		if t := w.s.Tests[in.Test]; t != nil {
			inv.Verification, inv.Evidence = t.Verification, []string{in.Test}
		}
	}
	f.Intent.Invariants = append(f.Intent.Invariants, inv)
	chg.IntentDelta.PreservedInvariants = append(chg.IntentDelta.PreservedInvariants, f.ID+": "+in.Claim)
	return f, w.s.Save()
}

type IntentModuleInput struct {
	ID, Path, Responsibility string
	Allowed, Forbidden       []string
	Origin                   model.Origin
}

func (e *Engine) IntentModule(in IntentModuleInput) (*model.Module, error) {
	w, chg, err := e.activeTurn()
	if err != nil {
		return nil, err
	}
	origin := in.Origin
	if origin == "" {
		origin = model.Inferred
	}
	if !origin.SemanticAllowed() {
		return nil, fmt.Errorf("module meaning fields are USER_STATED, ADOPTED or INFERRED, not %s", origin)
	}
	m := w.s.Modules[in.ID]
	if m == nil && in.Path != "" {
		m = w.s.ModuleByPath(in.Path)
	}
	if m == nil {
		if in.Path == "" {
			return nil, fmt.Errorf("module %s not found (a module this turn creates is addressed by --path)", in.ID)
		}
		// The module gets its ID at sync; its meaning waits on the turn until then.
		sem, err := model.NewSemantic(in.Responsibility, origin)
		if err != nil {
			return nil, err
		}
		if chg.ModuleIntents == nil {
			chg.ModuleIntents = map[string]model.Semantic{}
		}
		chg.ModuleIntents[in.Path] = sem
		return &model.Module{Path: in.Path, Responsibility: sem, Status: model.ItemDraft}, w.s.Save()
	}
	if in.Responsibility != "" && origin != model.UserStated {
		if err := e.inScope(w, chg, m.ID); err != nil {
			return nil, err
		}
	}
	if in.Responsibility != "" {
		if confirmed(m.Responsibility) && origin != model.UserStated && m.Responsibility.Value != in.Responsibility {
			return nil, fmt.Errorf("%s responsibility is CONFIRMED; changing it is a tier B decision", m.ID)
		}
		sem, err := model.NewSemantic(in.Responsibility, origin)
		if err != nil {
			return nil, err
		}
		m.Responsibility = sem
	}
	if len(in.Allowed)+len(in.Forbidden) > 0 {
		m.DependencyPolicy = model.DependencyPolicy{Allowed: in.Allowed, Forbidden: in.Forbidden, Origin: origin}
	}
	chg.Impacted = uniqStrings(append(chg.Impacted, m.ID))
	return m, w.s.Save()
}

type IntentContractInput struct {
	ID, Compatibility, ExternalConsumers string
	Origin                               model.Origin
}

func (e *Engine) IntentContract(in IntentContractInput) (*model.Contract, error) {
	w, chg, err := e.activeTurn()
	if err != nil {
		return nil, err
	}
	c, _ := w.s.Contract(in.ID)
	if c == nil {
		return nil, fmt.Errorf("contract %s not found", in.ID)
	}
	origin := in.Origin
	if origin == "" {
		origin = model.Inferred
	}
	if !origin.SemanticAllowed() {
		return nil, fmt.Errorf("contract attributes are USER_STATED or INFERRED, not %s (PRD 08A)", origin)
	}
	if in.Compatibility != "" && origin != model.UserStated {
		if err := e.inScope(w, chg, c.ID); err != nil {
			return nil, err
		}
	}
	if in.Compatibility != "" {
		if confirmed(c.Compatibility) && origin != model.UserStated && c.Compatibility.Value != in.Compatibility {
			return nil, fmt.Errorf("%s compatibility is CONFIRMED (%q); changing it is a tier B decision: `abbd decide --subject %s`", c.ID, c.Compatibility.Value, c.ID)
		}
		sem, err := model.NewSemantic(in.Compatibility, origin)
		if err != nil {
			return nil, err
		}
		c.Compatibility = sem
	}
	if in.ExternalConsumers != "" {
		if in.ExternalConsumers != "true" && in.ExternalConsumers != "false" {
			return nil, fmt.Errorf("--external-consumers must be true or false")
		}
		if c.ConsumersOrigin == model.UserStated && origin != model.UserStated {
			return nil, fmt.Errorf("%s external_consumers was stated by the user; changing it is a tier B decision", c.ID)
		}
		v := in.ExternalConsumers == "true"
		c.ExternalConsumers, c.ConsumersOrigin = &v, origin
	}
	chg.Impacted = uniqStrings(append(chg.Impacted, c.ID))
	return c, w.s.Save()
}

type IntentRuleInput struct {
	Claim, Req string
	Retire     bool
}

// IntentRule records a standing rule the user stated, promotes an existing requirement to project
// scope, or retires one. A rule's text is the user's: origin USER_STATED.
func (e *Engine) IntentRule(in IntentRuleInput) (*model.Requirement, error) {
	// A rule may be stated in a question that changes no code, so no turn is required.
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	chg := w.s.ActiveChange()
	if chg == nil {
		chg = &model.Change{}
	}
	var r *model.Requirement
	switch {
	case in.Req != "":
		if r = w.s.Requirements[in.Req]; r == nil {
			return nil, fmt.Errorf("%s not found", in.Req)
		}
	case in.Claim != "":
		r = &model.Requirement{ID: w.s.NextID(model.KindRequirement), Statement: model.MustSemantic(in.Claim, model.UserStated), Change: chg.ID}
		w.s.Requirements[r.ID] = r
		chg.Intends = append(chg.Intends, r.ID)
	default:
		return nil, fmt.Errorf("give --claim TEXT or --req REQ-x")
	}
	r.Scope, r.Auto = model.ScopeProject, false
	if in.Retire {
		r.Scope = model.ScopeChange
		chg.IntentDelta.Removals = append(chg.IntentDelta.Removals, "rule retired: "+r.ID+" "+r.Statement.Value)
	} else {
		chg.IntentDelta.Additions = append(chg.IntentDelta.Additions, "standing rule: "+r.ID+" "+r.Statement.Value)
	}
	if w.s.ActiveChange() == nil {
		return r, e.republish(w, "규칙 기록 "+r.ID)
	}
	return r, w.s.Save()
}

func (e *Engine) IntentDelta(add, remove, preserve []string) (*model.Change, error) {
	w, chg, err := e.activeTurn()
	if err != nil {
		return nil, err
	}
	chg.IntentDelta.Additions = append(chg.IntentDelta.Additions, add...)
	chg.IntentDelta.Removals = append(chg.IntentDelta.Removals, remove...)
	chg.IntentDelta.PreservedInvariants = append(chg.IntentDelta.PreservedInvariants, preserve...)
	if chg.State == model.StateIntentResolved {
		chg.State = model.StateImplementing
	}
	return chg, w.s.Save()
}

// inScope: a meaning field may be (re)written only for items this turn targets, delivers or
// affects; anything else is outside the IntentDelta and stays as recorded.
func (e *Engine) inScope(w *world, chg *model.Change, id string) error {
	if contains(chg.Impacted, id) || contains(chg.Delivers, id) || contains(chg.Plan.Targets, id) {
		return nil
	}
	if res, err := w.analyze(); err == nil {
		if contains(impact.Analyze(w.s, res, w.changedSinceSync()).Affected(), id) {
			return nil
		}
	}
	return fmt.Errorf("%s is outside this turn (not targeted, delivered or affected); its meaning stays as recorded unless the user asks: then record it with --origin USER_STATED", id)
}

func (e *Engine) activeTurn() (*world, *model.Change, error) {
	w, err := e.load()
	if err != nil {
		return nil, nil, err
	}
	chg := w.s.ActiveChange()
	if chg == nil {
		return nil, nil, fmt.Errorf("no active turn; start one with `abbd run`")
	}
	return w, chg, nil
}

type LearnInput struct {
	Proposal   string
	Kind       string
	Preference string
}

type Learning struct {
	Proposal string `yaml:"proposal" json:"proposal"`
	Kind     string `yaml:"kind" json:"kind"`
	Change   string `yaml:"change" json:"change"`
	Status   string `yaml:"status" json:"status"`
	At       string `yaml:"at" json:"at"`
}

// learnOrder is the correct-style order: a structural constraint beats more prose.
var learnOrder = []string{"architecture", "type", "lint", "test", "doc", "skill"}

// Learn is step 9: records a structural correction proposal, or promotes a repeated
// preference into project.yaml. Skill changes stay proposals until evaluated locally.
func (e *Engine) Learn(in LearnInput) (any, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if in.Preference != "" {
		k, v, ok := strings.Cut(in.Preference, "=")
		if !ok {
			return nil, fmt.Errorf("--preference must be key=value")
		}
		if w.s.Project.Preferences == nil {
			w.s.Project.Preferences = map[string]string{}
		}
		w.s.Project.Preferences[k] = v
		return w.s.Project.Preferences, w.s.Save()
	}
	if !contains(learnOrder, in.Kind) {
		return nil, fmt.Errorf("--kind must be one of %v (prefer the earliest that can encode the lesson)", learnOrder)
	}
	var log []Learning
	_ = ssot.ReadYAMLFile(e.Root, "learning.yaml", &log)
	l := Learning{Proposal: in.Proposal, Kind: in.Kind, Status: "proposed", At: now()}
	if c := w.s.ActiveChange(); c != nil {
		l.Change = c.ID
		c.NextSteps = append(c.NextSteps, "learn("+in.Kind+"): "+in.Proposal)
		if err := w.s.Save(); err != nil {
			return nil, err
		}
	}
	log = append(log, l)
	return l, ssot.WriteYAMLFile(e.Root, "learning.yaml", log)
}

// Pending is the confirm queue (`abbd confirm --list`, MCP list_unconfirmed).
func (e *Engine) Pending() ([]html.PendingItem, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	return html.Pending(w.s), nil
}

// Render regenerates graph and HTML from the md originals (`abbd render`).
func (e *Engine) Render() (*Freshness, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if err := e.republish(w, "render"); err != nil {
		return nil, err
	}
	f := e.freshnessNow()
	return &f, nil
}
