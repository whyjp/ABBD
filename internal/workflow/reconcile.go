package workflow

import (
	"fmt"

	"github.com/whyjp/abbd/internal/analysis"
	"github.com/whyjp/abbd/internal/impact"
	"github.com/whyjp/abbd/internal/knowledge"
	"github.com/whyjp/abbd/internal/model"
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

func (e *Engine) Reconcile() (*ReconcileResult, error) {
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
	return e.reconcile(w)
}

// reconcile handles an external or manual change (PRD ch.03 Reconcile): a CHG with
// intent_source external, EXTRACTED facts refreshed, meaning fields untouched, no questions.
// The only tier B record is a clear conflict with a CONFIRMED contract, and even that
// proceeds with its default instead of waiting.
func (e *Engine) reconcile(w *world) (*ReconcileResult, error) {
	files := w.changedSinceSync()
	res, err := w.analyze()
	if err != nil {
		return nil, err
	}
	diff := impact.ContractDiff(w.s, res)
	chg := &model.Change{
		ID: w.s.NextID(model.KindChange), RequestSummary: "external change reconciled", IntentSource: "external", ChangedBy: "external",
		State: model.StateSynchronizing, BaseRevision: w.s.State.Revision, BaseFingerprint: w.s.State.Fingerprint,
		Plan:         model.Plan{TaskType: "reconcile", VerificationTier: "T0", ContextStrategy: "lsp-source", ContextReason: "외부 변경: EXTRACTED 사실만 재추출"},
		Verification: model.NotRun, FilesChanged: files,
		Telemetry: model.Telemetry{StartedAt: now()},
		Time:      now(),
	}
	w.s.Changes[chg.ID] = chg
	krep := knowledge.Apply(w.s, res, knowledge.Options{Revision: w.ws.Revision, Renames: w.renames()})
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
	if len(rr.Conflicts) > 0 {
		chg.State = model.StateNeedsUser
	}
	finish(chg)
	chg.SyncState = model.SyncState{FeatureMap: model.Current, ArchitectureMap: model.Current, Graph: model.Current, HTML: model.Current}
	if _, err := knowledge.Publish(w.s, res, w.ws, true, "외부 변경 재조정: "+chg.ID); err != nil {
		return nil, err
	}
	rr.State = chg.State
	return rr, nil
}

func confirmed(s model.Semantic) bool {
	return s.Value != "" && (s.Origin == model.UserStated || s.Confirmation == model.Confirmed)
}
