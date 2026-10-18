package workflow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/whyjp/abbd/internal/analysis"
	"github.com/whyjp/abbd/internal/impact"
	"github.com/whyjp/abbd/internal/knowledge"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/review"
	"github.com/whyjp/abbd/internal/ssot"
)

type analysisResult = analysis.Result

type ReconcileResult struct {
	Change    string           `json:"change"`
	Files     []string         `json:"files"`
	Knowledge knowledge.Report `json:"knowledge"`
	Conflicts []string         `json:"conflicts"`
	State     model.TurnState  `json:"state"`
	Questions int              `json:"questions_asked"`
}

// Reconcile absorbs an outside change. byUser says the user made it, the only way to reconcile
// while a host agent session is open.
func (e *Engine) Reconcile(byUser bool) (*ReconcileResult, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	if c := w.s.ActiveChange(); c != nil {
		return nil, fmt.Errorf("turn %s is active; its changes are synced by `abbd sync`", c.ID)
	}
	if len(w.changedSinceSync()) == 0 {
		return &ReconcileResult{State: model.StateCompleted}, nil
	}
	if err := w.duringSession(byUser); err != nil {
		return nil, err
	}
	return e.reconcile(w)
}

// reconcile handles an external or manual change (PRD ch.03 Reconcile): a CHG with
// intent_source external, EXTRACTED facts refreshed, meaning fields untouched, no questions.
// The only tier B record is a clear conflict with a CONFIRMED contract, and even that
// proceeds with its default instead of waiting.
// ErrAgentEdits: the agent's own edits are verified in a turn, not absorbed as external change.
var ErrAgentEdits = errors.New("edited by the agent without a turn")

func (e *Engine) reconcile(w *world) (*ReconcileResult, error) {
	files := w.changedSinceSync()
	if mine := w.agentEdited(files); len(mine) > 0 {
		return nil, fmt.Errorf("%w: %s. Open a turn (abbd run \"<request>\"), then abbd verify and abbd sync; or revert them", ErrAgentEdits, strings.Join(mine, ", "))
	}
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	orig := w
	trial, err := w.s.Clone()
	if err != nil {
		return nil, err
	}
	w = &world{s: trial, ws: w.ws, res: res}
	diff := impact.ContractDiff(w.s, res)
	chg := &model.Change{
		ID: w.s.NextID(model.KindChange), RequestSummary: "external change reconciled", IntentSource: "external", ChangedBy: "external",
		State: model.StateSynchronizing, BaseRevision: w.s.State.Revision, BaseFingerprint: w.s.State.Fingerprint,
		Plan:         model.Plan{TaskType: "reconcile", VerificationTier: "T0", ContextStrategy: "lsp-source", ContextReason: "외부 변경: EXTRACTED 사실만 재추출"},
		Verification: model.NotRun, FilesChanged: files,
		Telemetry: model.Telemetry{StartedAt: now()},
		Time:      now(),
	}
	prev := lastBlockedReconcile(orig.s)
	if prev != nil {
		chg.ID = prev.ID
	}
	w.s.Changes[chg.ID] = chg
	before := review.Project(w.s)
	krep := knowledge.Apply(w.s, res, knowledge.Options{Revision: w.ws.Revision, Renames: w.renames()})
	// The code already changed outside the harness, so nothing is held back; the cards show
	// what changed structurally and a rejection becomes the next turn's work.
	var prevDeltas []model.StructuralDelta
	if prev != nil {
		prevDeltas = prev.StructuralDeltas
	}
	chg.StructuralDeltas = carryDecisions(review.Diff(before, review.Project(w.s), chg.ID), prevDeltas)
	// An external change can't be stopped, but an unapproved tier C change is not taken into the
	// blueprint either: the record waits BLOCKED on the original until someone approves it.
	if blocked := pendingTierC(w.s, chg, prevDeltas, chg.StructuralDeltas); len(blocked) > 0 {
		for i := range chg.StructuralDeltas {
			if chg.StructuralDeltas[i].Status == model.DeltaApplied || chg.StructuralDeltas[i].Status == model.DeltaDefaultApplied {
				chg.StructuralDeltas[i].Status = model.DeltaPending
			}
		}
		chg.State, chg.Blocked = model.StateBlocked, blocked
		chg.Impacted = nil
		orig.s.Changes[chg.ID] = chg
		if _, err := knowledge.Publish(orig.s, res, orig.ws, false, "외부 변경 대기 (등급 C): "+chg.ID); err != nil {
			return nil, err
		}
		return &ReconcileResult{Change: chg.ID, Files: files, State: model.StateBlocked, Conflicts: blocked}, ErrGate
	}
	recordDeltaDecisions(w.s, chg)
	chg.Risks = uniqStrings(append(chg.Risks, untestedNewPaths(w.s, chg)...))
	rr := &ReconcileResult{Change: chg.ID, Files: files, Knowledge: krep}

	for _, id := range krep.Removed {
		if f := w.s.Features[id]; f != nil && confirmed(f.Intent.Purpose) {
			rr.Conflicts = append(rr.Conflicts, fmt.Sprintf("%s (%s): 확정된 기능의 진입점이 외부 변경으로 사라짐", id, f.Name))
		}
	}
	for _, c := range diff {
		if c.ID == "" || c.Change == "added" {
			continue
		}
		if old, _ := w.s.Contract(c.ID); old != nil && confirmed(old.Compatibility) {
			rr.Conflicts = append(rr.Conflicts, fmt.Sprintf("%s `%s` %s: CONFIRMED 호환성 정책(%s)과 충돌", c.ID, c.Selector, c.Change, old.Compatibility.Value))
		}
	}
	for _, conflict := range rr.Conflicts {
		recordDecision(w.s, chg, DecideInput{
			Title: "외부 변경과 확정 계약 충돌", Tier: model.TierB, Question: conflict + ". 어떻게 할까?",
			Options: []model.Option{
				{Key: "accept", Label: "외부 변경을 수용", Effect: "맵이 코드를 따르고 확정 계약은 removed/변경으로 남음"},
				{Key: "restore", Label: "계약 복구", Effect: "코드를 되돌리는 후속 턴 필요"},
			},
			Default: "accept", ProceedsWithout: "EXTRACTED 사실 갱신과 HTML 재생성. 계약의 의미 판단만 보류",
		}, false)
	}
	chg.Impacted = krep.Touched()
	chg.State = model.StateCompleted
	for _, d := range chg.Decisions {
		if !d.Answered {
			chg.State = model.StateNeedsUser
		}
	}
	finish(chg)
	chg.SyncState = model.SyncState{FeatureMap: model.Current, ArchitectureMap: model.Current, Graph: model.Current, HTML: model.Current}
	if _, err := knowledge.Publish(w.s, res, w.ws, true, "외부 변경 재조정: "+chg.ID); err != nil {
		return nil, err
	}
	rr.State = chg.State
	return rr, nil
}

func lastBlockedReconcile(s *ssot.Snapshot) *model.Change {
	ids := model.SortedKeys(s.Changes)
	for i := len(ids) - 1; i >= 0; i-- {
		c := s.Changes[ids[i]]
		// A record whose tier C card was rejected is closed: the next outside change gets its own.
		if c.IntentSource == "external" && c.State == model.StateBlocked && !hasRejectedTierC(c) {
			return c
		}
		if c.IntentSource != "external" {
			return nil
		}
	}
	return nil
}

func hasRejectedTierC(c *model.Change) bool {
	for _, d := range c.StructuralDeltas {
		if d.Tier == model.TierC && (d.Status == model.DeltaRejected || d.Status == model.DeltaRevertAsked) {
			return true
		}
	}
	return false
}

func confirmed(s model.Semantic) bool {
	return s.Value != "" && (s.Origin == model.UserStated || s.Confirmation == model.Confirmed)
}
