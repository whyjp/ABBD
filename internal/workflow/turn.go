package workflow

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/bootstrap"
	"github.com/whyjp/abbd/internal/impact"
	"github.com/whyjp/abbd/internal/intent"
	"github.com/whyjp/abbd/internal/knowledge"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/policy"
	"github.com/whyjp/abbd/internal/review"
	"github.com/whyjp/abbd/internal/ssot"
	"github.com/whyjp/abbd/internal/verify"
	"github.com/whyjp/abbd/internal/workspace"
)

// ErrGate is returned when a completion gate refuses to close the turn.
var ErrGate = errors.New("completion gate refused")

type RunOptions struct {
	TaskType string
	Tier     string
}

type WorkflowPlan struct {
	Change          string         `json:"change"`
	TaskType        string         `json:"task_type"`
	Tier            string         `json:"verification_tier"`
	Steps           []string       `json:"steps"`
	ContextStrategy string         `json:"context_strategy"`
	ContextReason   string         `json:"context_reason"`
	Matches         []intent.Match `json:"matches"`
	Context         []any          `json:"context"`
	Ambiguous       bool           `json:"ambiguous"`
	AmbiguityHint   string         `json:"ambiguity_hint,omitempty"`
	NewFeature      string         `json:"new_feature,omitempty"`
	Requirement     string         `json:"requirement,omitempty"`
	ConfirmIfUsed   []string       `json:"confirm_if_purpose_used"`
	Budget          ssot.Budget    `json:"budget"`
	Rework          []string       `json:"rework,omitempty"`
	// ReadFirst is the current blueprint the agent reads before changing code; history is not on it.
	ReadFirst []string `json:"read_first"`
	// StandingRules are project-wide rules in force; they apply to new paths too.
	StandingRules []string          `json:"standing_rules"`
	Bootstrap     *bootstrap.Report `json:"bootstrap,omitempty"`
	Reconciled    *ReconcileResult  `json:"reconciled,omitempty"`
}

// Run starts a turn: step 0 session bootstrap, 1 route, 2 understand, 3 resolve intent.
func (e *Engine) Run(request string, o RunOptions) (*WorkflowPlan, error) {
	plan := &WorkflowPlan{}
	if !ssot.Exists(e.Root) {
		rep, err := bootstrap.Run(e.Root, bootstrap.Options{})
		if err != nil {
			return nil, err
		}
		plan.Bootstrap = rep
	}
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if c := w.s.ActiveChange(); c != nil && c.State != model.StateCompleted {
		if !rejectedOnly(c) {
			return nil, fmt.Errorf("turn %s is active (%s); finish it with `abbd sync` or `abbd turn abort`", c.ID, c.State)
		}
		c.State = model.StateBlocked
		c.Blocked = uniqStrings(append(c.Blocked, "closed: its tier C changes were rejected; the next turn reverts them"))
		finish(c)
		w.s.State.ActiveTurn = ""
		if err := w.s.Save(); err != nil {
			return nil, err
		}
		if err := w.s.SaveState(); err != nil {
			return nil, err
		}
	}
	rework := &model.Change{}
	plan.Rework = consumeRejections(w.s, rework)
	if len(plan.Rework) > 0 {
		if err := w.s.Save(); err != nil {
			return nil, err
		}
	}
	if len(w.changedSinceSync()) > 0 {
		rec, err := e.reconcile(w)
		if err != nil {
			return nil, err
		}
		plan.Reconciled = rec
		if w, err = e.load(); err != nil {
			return nil, err
		}
	}

	r := intent.Resolve(request, w.s)
	if o.TaskType != "" {
		r.TaskType = o.TaskType
		for _, t := range intent.Types {
			if t.Name == o.TaskType {
				r.Steps = t.Steps
			}
		}
	}
	if o.Tier != "" {
		r.Tier = o.Tier
	}
	strategy, reason := e.contextStrategy(w, r)
	targets := topTargets(r.Matches)

	chg := &model.Change{
		ID: w.s.NextID(model.KindChange), RequestSummary: request, IntentSource: "user", ChangedBy: "harness",
		State: model.StateIntentResolved, BaseRevision: w.ws.Revision, BaseFingerprint: w.ws.Fingerprint,
		Plan:         model.Plan{TaskType: r.TaskType, VerificationTier: r.Tier, Steps: r.Steps, ContextStrategy: strategy, ContextReason: reason, Targets: targets, Ambiguous: r.Ambiguous},
		IntentDelta:  model.IntentDelta{Additions: []string{request}},
		Impacted:     targets,
		Verification: model.NotRun,
		Telemetry:    model.Telemetry{StartedAt: now(), ModelCallsNote: "host reports with `abbd turn note --model-calls N`"},
		SyncState:    model.SyncState{FeatureMap: model.Draft, ArchitectureMap: model.Draft, Graph: model.Draft, HTML: model.Draft},
		Time:         now(),
	}
	w.s.Changes[chg.ID] = chg
	req := &model.Requirement{ID: w.s.NextID(model.KindRequirement), Statement: model.MustSemantic(request, model.UserStated), Change: chg.ID, Scope: model.ScopeChange}
	if intent.IsProjectRule(request) {
		req.Scope = model.ScopeProject
	}
	w.s.Requirements[req.ID] = req
	chg.Intends = []string{req.ID}
	chg.IntentDelta.Additions = append(chg.IntentDelta.Additions, rework.IntentDelta.Additions...)
	markConsumed(w.s, chg.ID)
	plan.Requirement = req.ID

	if r.NewFeature {
		f := &model.Feature{
			ID: w.s.NextID(model.KindFeature), Name: shortName(request), Status: model.ItemDraft,
			Intent:       model.Intent{Purpose: model.MustSemantic(request, model.UserStated)},
			Requirements: []string{req.ID}, Changes: []string{chg.ID},
		}
		f.Intent.Purpose.ConfirmedIn = chg.ID
		w.s.Features[f.ID] = f
		chg.Delivers = []string{f.ID}
		plan.NewFeature = f.ID
	}
	for _, id := range targets {
		if ctx, err := inspectIn(w, id); err == nil {
			plan.Context = append(plan.Context, ctx)
		}
		if pendingPurpose(w.s, id) {
			plan.ConfirmIfUsed = append(plan.ConfirmIfUsed, id)
		}
	}
	if r.Ambiguous {
		plan.AmbiguityHint = "요청이 둘 이상의 기능과 같은 정도로 맞는다: `abbd decide`로 등급 B 질문(선택지 2~3개 + 기본값)을 남기고 기본값으로 진행"
	}
	if err := review.SaveBase(w.s); err != nil {
		return nil, err
	}
	w.s.State.ActiveTurn = chg.ID
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	if _, err := knowledge.Publish(w.s, res, w.ws, false, "턴 시작: "+chg.ID); err != nil {
		return nil, err
	}
	plan.Change, plan.TaskType, plan.Tier, plan.Steps = chg.ID, r.TaskType, r.Tier, r.Steps
	plan.ContextStrategy, plan.ContextReason, plan.Matches, plan.Ambiguous = strategy, reason, r.Matches, r.Ambiguous
	plan.Budget = w.s.Project.Budgets[r.Tier]
	plan.ReadFirst = readFirst(w.s, targets)
	for _, rule := range knowledge.StandingRules(w.s) {
		plan.StandingRules = append(plan.StandingRules, rule.ID+": "+rule.Statement.Value)
	}
	return plan, nil
}

// readFirst: the blueprint page, then the md originals of the matched targets.
func readFirst(s *ssot.Snapshot, targets []string) []string {
	out := []string{ssot.Dir + "/" + knowledge.BlueprintFile}
	for _, id := range targets {
		switch model.KindOf(id) {
		case model.KindFeature:
			out = append(out, ssot.Dir+"/features/"+id+".md")
		case model.KindModule:
			out = append(out, ssot.Dir+"/architecture/"+id+".md")
		default:
			if _, owner := s.Contract(id); owner != nil {
				out = append(out, ssot.Dir+"/architecture/"+owner.ID+".md")
			}
		}
	}
	return uniqStrings(out)
}

func shortName(request string) string {
	r := []rune(strings.TrimSpace(request))
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return string(r)
}

func topTargets(ms []intent.Match) []string {
	var out []string
	for _, m := range ms {
		if len(out) >= 5 || m.Score < ms[0].Score {
			break
		}
		out = append(out, m.ID)
	}
	return out
}

func pendingPurpose(s *ssot.Snapshot, id string) bool {
	if f := s.Features[id]; f != nil {
		return f.Intent.Purpose.Pending()
	}
	if m := s.Modules[id]; m != nil {
		return m.Responsibility.Pending()
	}
	return false
}

// contextStrategy is the Graph-on-demand router of PRD appendix B, as a simple heuristic.
// The chosen strategy and its reason are recorded on the CHG (FR-21).
func (e *Engine) contextStrategy(w *world, r intent.Resolution) (string, string) {
	_, fresh, err := e.queryGraph(w)
	if err != nil || fresh != model.Current {
		return "lsp-source", fmt.Sprintf("graph %s: LSP·원본 소스·테스트로 대체하고 불확실성을 표시", fresh)
	}
	modules, contracts, history := 0, 0, 0
	for _, m := range r.Matches {
		switch m.Kind {
		case "module":
			modules++
		case "contract":
			contracts++
		case "feature":
			if f := w.s.Features[m.ID]; f != nil && len(f.Changes) >= 3 {
				history++
			}
		}
	}
	switch {
	case r.TaskType == "interface-change" || contracts > 0:
		return "contract-view", "외부 계약 변경 가능: 계약 뷰 + 소비자 + 호환성"
	case history > 0:
		return "graph+history", "반복 변경 대상: 영속 그래프 + 결정 + 과거 증거"
	case (r.Tier == "T0" || r.Tier == "T1") && modules <= 1:
		return "lsp-source", "국소 저위험 변경: LSP + 관련 소스 + 단위 테스트"
	default:
		return "graph", "교차 모듈 변경: Architecture + callers + 계약 영향"
	}
}

// Plan is a dry run of route + intent + impact. It writes nothing.
func (e *Engine) Plan(request string) (map[string]any, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	r := intent.Resolve(request, w.s)
	strategy, reason := e.contextStrategy(w, r)
	imp := impact.Analyze(w.s, res, w.changedSinceSync())
	return map[string]any{"resolution": r, "context_strategy": strategy, "context_reason": reason, "impact": imp,
		"verification_commands": imp.TestCommands, "budget": w.s.Project.Budgets[r.Tier]}, nil
}

type VerifyOptions struct {
	All          bool
	Commands     []string
	Unverifiable string
	Reason       string
}

type EvidenceReport struct {
	Change      string             `json:"change,omitempty"`
	Overall     model.Verification `json:"overall"`
	Runs        []model.Evidence   `json:"runs"`
	Ran         []string           `json:"ran"`
	CouldNotRun []string           `json:"could_not_run"`
	FailedTests []string           `json:"failed_tests"`
	// Failures carries each failing test's own output, so the report shows what disagreed.
	Failures []Failure `json:"failures"`
	Warnings []string  `json:"warnings,omitempty"`
}

// Verify is step 6. "Ran and failed" and "could not run" stay distinct (PRD ch.04).
func (e *Engine) Verify(o VerifyOptions) (*EvidenceReport, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	chg := w.s.ActiveChange()
	rep := &EvidenceReport{}
	changeID := ""
	if chg != nil {
		changeID = chg.ID
		rep.Change = chg.ID
	}
	if o.Unverifiable != "" {
		ev := model.Evidence{
			ID: "", Command: o.Unverifiable, Dir: ".", Revision: w.ws.Revision, Fingerprint: w.ws.Fingerprint,
			Verification: model.Unverifiable, Reason: o.Reason, Change: changeID, StartedAt: now(), ExitCode: -1,
		}
		if err := verify.Record(e.Root, &ev); err != nil {
			return nil, err
		}
		rep.Runs = append(rep.Runs, ev)
	} else {
		cmds, err := e.verificationCommands(w, o)
		if err != nil {
			return nil, err
		}
		for _, cmd := range cmds {
			if tier, why := policy.CommandTier(cmd); tier == model.TierC {
				return nil, fmt.Errorf("refusing to run %q as verification: %s (tier C)", cmd, why)
			}
		}
		for _, cmd := range cmds {
			ev := verify.Run(e.Root, cmd, w.ws.Revision, w.ws.Fingerprint, changeID)
			if err := verify.Save(e.Root, ev); err != nil {
				return nil, err
			}
			verify.ApplyResults(w.s, ev)
			rep.Runs = append(rep.Runs, ev)
		}
	}
	rep.Overall = aggregate(rep.Runs)
	for _, ev := range rep.Runs {
		switch ev.Verification {
		case model.Unverifiable:
			rep.CouldNotRun = append(rep.CouldNotRun, ev.Command+": "+ev.Reason)
		default:
			rep.Ran = append(rep.Ran, fmt.Sprintf("%s → %s (%s)", ev.Command, ev.Verification, ev.ID))
		}
		for _, t := range ev.Tests {
			if t.Result == model.Failed {
				rep.FailedTests = append(rep.FailedTests, t.Package+"."+t.Name)
				rep.Failures = append(rep.Failures, Failure{Test: t.Package + "." + t.Name, Output: t.Output, Evidence: ev.ID})
			}
		}
	}
	if chg != nil {
		for _, ev := range rep.Runs {
			chg.VerificationEvidence = append(chg.VerificationEvidence, ev.ID)
			if ev.Verification != model.Unverifiable {
				chg.Telemetry.TestRuns++
			}
		}
		// An UNVERIFIABLE-only record does not override an earlier PASSED or FAILED run.
		if rep.Overall != model.Unverifiable || chg.Verification == model.NotRun {
			chg.Verification = rep.Overall
			chg.ResultFingerprint = w.ws.Fingerprint
		}
		chg.State = model.StateVerifying
		chg.Telemetry.GraphQueries += w.s.State.TurnGraphQueries
		w.s.State.TurnGraphQueries = 0
		if b, ok := w.s.Project.Budgets[chg.Plan.VerificationTier]; ok && b.TestRuns > 0 && chg.Telemetry.TestRuns > b.TestRuns {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("테스트 실행 %d회가 %s 예산 %d회를 넘음", chg.Telemetry.TestRuns, chg.Plan.VerificationTier, b.TestRuns))
		}
	}
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	settle := chg == nil && w.s.State.Fingerprint == w.ws.Fingerprint
	if _, err := knowledge.Publish(w.s, res, w.ws, settle, "검증 반영"); err != nil {
		return nil, err
	}
	if rep.Overall == model.Failed {
		return rep, ErrFailed
	}
	return rep, nil
}

// ErrFailed is returned when verification ran and failed; the CLI exits 2 like a refused gate.
var ErrFailed = errors.New("verification FAILED")

type Failure struct {
	Test     string `json:"test"`
	Output   string `json:"output"`
	Evidence string `json:"evidence"`
}

func aggregate(runs []model.Evidence) model.Verification {
	out := model.Unverifiable
	for _, r := range runs {
		switch r.Verification {
		case model.Failed:
			return model.Failed
		case model.Passed:
			out = model.Passed
		}
	}
	return out
}

func (e *Engine) verificationCommands(w *world, o VerifyOptions) ([]string, error) {
	if len(o.Commands) > 0 {
		return o.Commands, nil
	}
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	// A greenfield project learns its build and test commands from the code its first turn wrote.
	p := w.s.Project.Verification
	if len(p.TestCommands) == 0 {
		p.TestCommands = res.TestCommands
	}
	if len(p.BuildCommands) == 0 {
		p.BuildCommands = res.BuildCommands
	}
	w.s.Project.Verification = p
	tier := ""
	if c := w.s.ActiveChange(); c != nil {
		tier = c.Plan.VerificationTier
	}
	// T2 and T3 widen to the whole regression suite (PRD ch.04 verification tiers).
	if o.All || tier == "T2" || tier == "T3" {
		return append(append([]string{}, p.BuildCommands...), p.TestCommands...), nil
	}
	imp := impact.Analyze(w.s, res, w.changedSinceSync())
	cmds := imp.TestCommands
	if len(cmds) == 0 {
		cmds = p.TestCommands
	}
	return append(append([]string{}, p.BuildCommands...), cmds...), nil
}

type SyncOptions struct {
	Commit bool
}

type SyncResult struct {
	Completed   bool               `json:"completed"`
	Reason      string             `json:"reason,omitempty"`
	Receipt     *model.Change      `json:"receipt,omitempty"`
	Knowledge   knowledge.Report   `json:"knowledge"`
	Impact      *impact.Report     `json:"impact,omitempty"`
	Consistency *ConsistencyReport `json:"consistency,omitempty"`
	Freshness   Freshness          `json:"freshness"`
	Commit      string             `json:"commit,omitempty"`
	Reconciled  *ReconcileResult   `json:"reconciled,omitempty"`
	// Review is set only when the turn has structural changes to look at; a turn that only
	// touched module internals produces no review notice (AC-REV-05).
	Review *ReviewSummary `json:"review,omitempty"`
}

var docExts = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".rst": true, ".adoc": true}

func docsOnly(files []string) bool {
	for _, f := range files {
		if !docExts[strings.ToLower(path.Ext(f))] {
			return false
		}
	}
	return true
}

// Sync is step 7, the completion gate of PRD ch.07 on_verified_change. It refuses to complete
// unless the latest verification PASSED on exactly the current workspace, and it only reports
// completion after re-reading the published artifacts and finding them all CURRENT.
func (e *Engine) Sync(o SyncOptions) (*SyncResult, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	chg := w.s.ActiveChange()
	if chg == nil {
		if len(w.changedSinceSync()) > 0 {
			rec, err := e.reconcile(w)
			if err != nil {
				return nil, err
			}
			return &SyncResult{Completed: true, Reason: "활성 턴 없음: 외부 변경으로 재조정", Reconciled: rec, Freshness: e.freshnessNow()}, nil
		}
		res, err := w.analyze()
		if err != nil {
			return nil, err
		}
		if _, err := knowledge.Publish(w.s, res, w.ws, true, "render"); err != nil {
			return nil, err
		}
		return &SyncResult{Completed: true, Reason: "변경 없음: 생성물만 재생성", Freshness: e.freshnessNow()}, nil
	}
	changed := w.changedSinceSync()
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}

	switch {
	case len(changed) == 0:
		// Nothing changed: there is nothing to prove, so nothing is claimed PASSED.
		chg.Risks = uniqStrings(append(chg.Risks, "작업 트리 변경 없음: 검증 주장 없이 종료"))
	case docsOnly(changed):
		// T0: documents only. The proof is the consistency check, recorded as evidence.
		if chg.Verification != model.Passed || chg.ResultFingerprint != w.ws.Fingerprint {
			cons := checkSnapshot(w.s)
			ev := model.Evidence{Command: "abbd check", Dir: ".", Revision: w.ws.Revision, Fingerprint: w.ws.Fingerprint, Change: chg.ID, StartedAt: now(), Verification: model.Passed}
			if !cons.OK {
				ev.Verification, ev.OutputTail = model.Failed, strings.Join(cons.Errors, "\n")
			}
			if err := verify.Record(e.Root, &ev); err != nil {
				return nil, err
			}
			chg.VerificationEvidence = append(chg.VerificationEvidence, ev.ID)
			chg.Verification, chg.ResultFingerprint = ev.Verification, w.ws.Fingerprint
			if ev.Verification != model.Passed {
				return e.draft(w, chg, res, "T0 검사 실패")
			}
		}
	case chg.Verification != model.Passed || chg.ResultFingerprint != w.ws.Fingerprint:
		reason := fmt.Sprintf("검증 %s", chg.Verification)
		if chg.Verification == model.Passed {
			reason = "검증 이후 작업 트리가 바뀜: 다시 `abbd verify`"
		}
		return e.draft(w, chg, res, reason)
	}

	chg.State = model.StateSynchronizing
	imp := impact.Analyze(w.s, res, changed)

	// The turn is applied to a copy first. Its structural deltas decide whether the copy may
	// become the SSOT: an unapproved tier C delta leaves the original untouched (AC-REV-04).
	orig, origChg := w, chg
	trial, err := w.s.Clone()
	if err != nil {
		return nil, err
	}
	before := review.LoadBase(w.s)
	w = &world{s: trial, ws: w.ws, res: res}
	chg = trial.Changes[chg.ID]

	w.s.Project.Languages = res.Languages
	opt := knowledge.Options{Revision: w.ws.Revision, Renames: w.renames()}
	krep := knowledge.Apply(w.s, res, opt)
	for p, sem := range chg.ModuleIntents {
		if m := w.s.ModuleByPath(p); m != nil && !confirmed(m.Responsibility) {
			m.Responsibility = sem
		}
	}
	delivered := e.bindDelivered(w, chg, krep, changed, res)
	seed := knowledge.Seed(w.s, res, opt)
	krep.New = append(krep.New, seed.New...)
	krep.Candidates = append(krep.Candidates, seed.Candidates...)
	for _, id := range chg.VerificationEvidence {
		if ev, err := verify.Load(e.Root, id); err == nil {
			verify.ApplyResults(w.s, ev)
		}
	}
	last := ""
	if n := len(chg.VerificationEvidence); n > 0 {
		last = chg.VerificationEvidence[n-1]
	}
	impacted := append(imp.Affected(), delivered...)
	for _, id := range impacted {
		if f := w.s.Features[id]; f != nil && f.Status != model.ItemRemoved {
			if !contains(f.Changes, chg.ID) {
				f.Changes = append(f.Changes, chg.ID)
			}
			f.LastSynced = model.SyncMark{Revision: w.ws.Revision, Fingerprint: w.ws.Fingerprint, Evidence: last, Change: chg.ID}
		}
	}
	chg.Impacted = uniqStrings(append(chg.Impacted, impacted...))
	chg.FilesChanged = changed
	chg.ResultRevision = w.ws.Revision
	chg.IntentFidelity = fidelity(w.s, chg)
	chg.NextSteps = append(chg.NextSteps, imp.Unknowns...)
	chg.Risks = uniqStrings(append(chg.Risks, imp.Risks...))

	deltas := carryDecisions(review.Diff(before, review.Project(w.s), chg.ID), origChg.StructuralDeltas)
	if blocked := pendingTierC(w.s, chg, origChg.StructuralDeltas, deltas); len(blocked) > 0 {
		// Nothing of this trial reached the blueprint, so no card may claim "applied".
		for i := range deltas {
			if deltas[i].Status == model.DeltaApplied || deltas[i].Status == model.DeltaDefaultApplied {
				deltas[i].Status = model.DeltaPending
			}
		}
		origChg.StructuralDeltas = deltas
		origChg.State = model.StateBlocked
		origChg.Blocked = uniqStrings(append(origChg.Blocked, blocked...))
		return e.draft(orig, origChg, res, "등급 C: 명시적 승인 필요 — "+strings.Join(blocked, "; "))
	}
	chg.StructuralDeltas = deltas
	recordDeltaDecisions(w.s, chg)
	chg.Blocked = openTierC(w.s, chg)

	if cons := checkSnapshot(w.s); !cons.OK {
		return e.draft(orig, origChg, res, "스키마/계약 검사 실패: "+strings.Join(cons.Errors, "; "))
	}

	chg.State = model.StateCompleted
	for _, d := range chg.Decisions {
		if d.Tier == model.TierB && !d.Answered {
			chg.State = model.StateNeedsUser
		}
	}
	w.s.State.TurnGraphQueries, chg.Telemetry.GraphQueries = 0, chg.Telemetry.GraphQueries+w.s.State.TurnGraphQueries
	finish(chg)
	chg.SyncState = model.SyncState{FeatureMap: model.Current, ArchitectureMap: model.Current, Graph: model.Current, HTML: model.Current}
	w.s.State.ActiveTurn = ""
	chg.NewUnconfirmed += len(krep.Candidates) + countNewPending(w.s, krep.New)

	result := &SyncResult{Receipt: chg, Knowledge: krep, Impact: &imp}
	_, err = knowledge.Publish(w.s, res, w.ws, true, "동기화 완료: "+chg.ID)
	if err == nil {
		result.Freshness = e.freshnessNow()
		result.Consistency, err = e.Check()
	}
	if err != nil || !result.Freshness.AllCurrent() || !result.Consistency.OK {
		// Never leave a COMPLETED receipt behind a failed or inconsistent publish (PRD ch.07).
		reason := "동기화 후 산출물 불일치"
		if err != nil {
			reason = "산출물 게시 실패: " + err.Error()
		}
		// The trial is discarded; the original SSOT and its watermark stay as they were.
		origChg.State = model.StateImplementing
		res2, derr := e.draft(orig, origChg, res, reason)
		if res2 != nil {
			res2.Consistency = result.Consistency
		}
		if derr == ErrGate && err != nil {
			return res2, fmt.Errorf("%w: %v", ErrGate, err)
		}
		return res2, derr
	}
	result.Completed = true
	if len(chg.StructuralDeltas) > 0 {
		result.Review = summarize(e.Root, chg)
	}
	if o.Commit {
		// The turn is complete either way; a refused or failed commit is reported, not raised.
		var cerr error
		result.Commit, cerr = e.commit(w, chg)
		if cerr != nil {
			result.Reason = "commit 안 함: " + cerr.Error()
		} else {
			// The receipt keeps the verified revision; the commit that records it lives in state.
			w.s.State.Commits = mapSet(w.s.State.Commits, chg.ID, result.Commit)
			err = w.s.SaveState()
		}
	}
	return result, err
}

func mapSet(m map[string]string, k, v string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	m[k] = v
	return m
}

// openTierC keeps only tier C stops that are still unapproved; contract stops that passed the
// gate in this sync are gone.
func openTierC(s *ssot.Snapshot, chg *model.Change) []string {
	var out []string
	for _, ref := range chg.Decisions {
		d := s.Decisions[ref.ADR]
		if d != nil && d.Tier == model.TierC && !(d.Answered && d.Chosen == "approve") {
			out = append(out, d.ID+": "+d.Question)
		}
	}
	return out
}

func finish(chg *model.Change) {
	chg.Telemetry.FinishedAt = now()
	if t, err := time.Parse(time.RFC3339, chg.Telemetry.StartedAt); err == nil {
		chg.Telemetry.DurationSec = int64(time.Since(t).Seconds())
	}
}

// draft keeps the turn open: work-in-progress is saved and rendered as DRAFT, the sync
// watermark does not move, and confirmed contracts are not overwritten by failing behavior.
func (e *Engine) draft(w *world, chg *model.Change, res *analysisResult, reason string) (*SyncResult, error) {
	chg.SyncState = model.SyncState{FeatureMap: model.Draft, ArchitectureMap: model.Draft, Graph: model.Draft, HTML: model.Draft}
	if chg.State == model.StateSynchronizing || chg.State == model.StateIntentResolved {
		chg.State = model.StateImplementing
	}
	if _, err := knowledge.Publish(w.s, res, w.ws, false, "초안: "+reason); err != nil {
		return nil, err
	}
	out := &SyncResult{Completed: false, Reason: reason, Receipt: chg, Freshness: e.freshnessNow()}
	if len(chg.StructuralDeltas) > 0 {
		out.Review = summarize(e.Root, chg)
	}
	return out, ErrGate
}

func approved(s *ssot.Snapshot, chg *model.Change, subject string, tier model.Tier) bool {
	for _, ref := range chg.Decisions {
		d := s.Decisions[ref.ADR]
		if d != nil && d.Tier == tier && d.Answered && d.Chosen == "approve" && contains(d.Subjects, subject) {
			return true
		}
	}
	return false
}

// bindDelivered turns the turn's draft features into current ones, attached to the entry
// contracts that appeared in the changed files, or to the changed modules when there are none.
func (e *Engine) bindDelivered(w *world, chg *model.Change, krep knowledge.Report, changed []string, res *analysisResult) []string {
	changedSet := map[string]bool{}
	for _, f := range changed {
		changedSet[f] = true
	}
	newSet := map[string]bool{}
	for _, id := range krep.New {
		newSet[id] = true
	}
	// Entry points that appeared in this turn bind first; entry points merely living in a
	// changed file are the fallback, since an edited file also holds older, claimed routes.
	var fresh, inChanged []string
	for _, mid := range model.SortedKeys(w.s.Modules) {
		for _, c := range w.s.Modules[mid].ExternalContracts {
			if c.Status == model.ItemRemoved || (c.Direction != "exposes" && c.Direction != "consumes") || claimedByOther(w.s, c.ID, chg.Delivers) != "" {
				continue
			}
			file := c.Location
			if i := strings.LastIndex(file, ":"); i > 0 {
				file = file[:i]
			}
			switch {
			case newSet[c.ID]:
				fresh = append(fresh, c.ID)
			case changedSet[file]:
				inChanged = append(inChanged, c.ID)
			}
		}
	}
	entryIDs := fresh
	if len(entryIDs) == 0 {
		entryIDs = inChanged
	}
	var out []string
	for _, fid := range chg.Delivers {
		f := w.s.Features[fid]
		if f == nil {
			continue
		}
		knowledge.BindEntries(w.s, res, f, entryIDs, w.ws.Revision)
		if len(f.Anchors) == 0 {
			bindToModules(w.s, f, changed)
		}
		f.Status = model.ItemCurrent
		out = append(out, fid)
	}
	return out
}

func claimedByOther(s *ssot.Snapshot, contractID string, mine []string) string {
	for _, id := range model.SortedKeys(s.Features) {
		if contains(mine, id) || s.Features[id].Status == model.ItemRemoved {
			continue
		}
		if contains(s.Features[id].ExternalContracts, contractID) {
			return id
		}
	}
	return ""
}

// bindToModules anchors a feature without entry points (a library capability) to the source
// files and tests it changed.
func bindToModules(s *ssot.Snapshot, f *model.Feature, changed []string) {
	mods := map[string]bool{}
	for _, file := range changed {
		if m := s.ModuleByPath(path.Dir(file)); m != nil {
			mods[m.ID] = true
			f.Anchors = append(f.Anchors, model.Anchor{Kind: model.AnchorPath, Ref: file})
		}
	}
	f.OwnedBy = model.SortedKeys(mods)
	for _, mid := range f.OwnedBy {
		for _, tid := range s.Modules[mid].Tests {
			t := s.Tests[tid]
			f.Verification = append(f.Verification, model.TestRef{TestID: tid, Command: t.Command, Verification: t.Verification, Evidence: t.Evidence})
			f.Anchors = append(f.Anchors, model.Anchor{Kind: model.AnchorTest, Ref: tid})
		}
	}
}

// fidelity is the intent-fidelity proxy of appendix A: delta items, items covered by a
// PASSED test, items the user stated or confirmed.
func fidelity(s *ssot.Snapshot, chg *model.Change) model.IntentFidelity {
	fi := model.IntentFidelity{DeltaItems: len(chg.IntentDelta.Additions) + len(chg.IntentDelta.Removals)}
	covered := false
	for _, id := range append(append([]string{}, chg.Delivers...), chg.Impacted...) {
		if f := s.Features[id]; f != nil {
			for _, t := range f.Verification {
				if t.Verification == model.Passed {
					covered = true
				}
			}
		}
	}
	if covered && chg.Verification == model.Passed {
		fi.CoveredByPassedTests = fi.DeltaItems
	}
	fi.UserConfirmed = 1 // the request itself is USER_STATED
	for _, ref := range chg.Decisions {
		if ref.Answered {
			fi.UserConfirmed++
		}
	}
	if fi.UserConfirmed > fi.DeltaItems {
		fi.UserConfirmed = fi.DeltaItems
	}
	return fi
}

func countNewPending(s *ssot.Snapshot, ids []string) int {
	n := 0
	for _, id := range ids {
		if pendingPurpose(s, id) {
			n++
		}
	}
	return n
}

func (e *Engine) freshnessNow() Freshness {
	w, err := e.load()
	if err != nil {
		return Freshness{model.UnknownFresh, model.UnknownFresh, model.UnknownFresh, model.UnknownFresh}
	}
	return w.freshness()
}

// commit records the turn as one local Git change when policy puts local commits in tier A.
func (e *Engine) commit(w *world, chg *model.Change) (string, error) {
	rule, err := policy.Classify(w.s.Project, "local-commit")
	if err != nil {
		return "", err
	}
	if rule.Tier != model.TierA {
		return "", fmt.Errorf("local commit is tier %s in project.yaml; not committing", rule.Tier)
	}
	if _, err := workspace.Git(e.Root, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	if _, err := workspace.Git(e.Root, "commit", "-m", chg.ID+": "+chg.RequestSummary); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	return workspace.Git(e.Root, "rev-parse", "--short", "HEAD")
}

// TurnShow returns the active turn, or the latest receipt when none is active.
func (e *Engine) TurnShow() (*model.Change, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if c := w.s.ActiveChange(); c != nil {
		return c, nil
	}
	ids := model.SortedKeys(w.s.Changes)
	if len(ids) == 0 {
		return nil, fmt.Errorf("no turns yet")
	}
	return w.s.Changes[ids[len(ids)-1]], nil
}

func (e *Engine) TurnNote(modelCalls int) (*model.Change, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	c := w.s.ActiveChange()
	if c == nil {
		return nil, fmt.Errorf("no active turn")
	}
	c.Telemetry.ModelCalls += modelCalls
	c.Telemetry.ModelCallsNote = "host-reported"
	return c, w.s.Save()
}

// TurnAbort abandons the active turn. Draft features it created are dropped; the CHG stays
// as a BLOCKED record so the attempt is not lost.
func (e *Engine) TurnAbort(reason string) (*model.Change, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	c := w.s.ActiveChange()
	if c == nil {
		return nil, fmt.Errorf("no active turn")
	}
	var kept []string
	for _, fid := range c.Delivers {
		if f := w.s.Features[fid]; f != nil && f.Status == model.ItemDraft {
			delete(w.s.Features, fid)
			continue
		}
		kept = append(kept, fid)
	}
	c.Delivers = kept
	c.State = model.StateBlocked
	c.Blocked = uniqStrings(append(c.Blocked, "aborted: "+reason))
	finish(c)
	w.s.State.ActiveTurn = ""
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	settle := len(w.changedSinceSync()) == 0
	_, err = knowledge.Publish(w.s, res, w.ws, settle, "턴 중단: "+c.ID)
	return c, err
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func uniqStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
