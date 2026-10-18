package workflow

import (
	"github.com/whyjp/for-my-harness/internal/graph"
	"github.com/whyjp/for-my-harness/internal/html"
	"github.com/whyjp/for-my-harness/internal/model"
	"github.com/whyjp/for-my-harness/internal/ssot"
)

type Freshness struct {
	FeatureMap      model.Freshness `json:"feature_map"`
	ArchitectureMap model.Freshness `json:"architecture_map"`
	Graph           model.Freshness `json:"graph"`
	HTML            model.Freshness `json:"html"`
}

func (f Freshness) AllCurrent() bool {
	return f.FeatureMap == model.Current && f.ArchitectureMap == model.Current && f.Graph == model.Current && f.HTML == model.Current
}

type TurnSummary struct {
	ID      string          `json:"id"`
	Request string          `json:"request"`
	State   model.TurnState `json:"state"`
	Plan    model.Plan      `json:"plan"`
}

type Status struct {
	Root              string         `json:"root"`
	Mode              string         `json:"mode"`
	Revision          string         `json:"revision"`
	Dirty             bool           `json:"dirty"`
	Fingerprint       string         `json:"fingerprint"`
	SyncedRevision    string         `json:"synced_revision"`
	SyncedFingerprint string         `json:"synced_fingerprint"`
	SyncedAt          string         `json:"synced_at"`
	Freshness         Freshness      `json:"freshness"`
	PendingChanges    []string       `json:"pending_changes"`
	ExternalChange    bool           `json:"external_change"`
	ActiveTurn        *TurnSummary   `json:"active_turn,omitempty"`
	Knowledge         map[string]int `json:"knowledge"`
	Unconfirmed       int            `json:"unconfirmed"`
	Hint              string         `json:"hint,omitempty"`
}

func (e *Engine) Status() (*Status, error) {
	w, err := e.load()
	if err != nil {
		return nil, err
	}
	return w.status(), nil
}

// freshness implements PRD ch.07: maps are CURRENT only when the sync watermark equals the
// workspace now; an unfinished turn makes them DRAFT; anything else is STALE.
func (w *world) freshness() Freshness {
	s := w.s
	maps := model.Current
	if s.State.Fingerprint != w.ws.Fingerprint {
		maps = model.Stale
		if s.ActiveChange() != nil {
			maps = model.Draft
		}
	} else if c := s.ActiveChange(); c != nil && c.State != model.StateCompleted {
		maps = model.Draft
	}
	hash := ssot.Hash(s.Root)
	g := model.UnknownFresh
	if gr, err := graph.Load(s.Root); err == nil {
		g = model.Current
		if gr.Fingerprint != w.ws.Fingerprint || gr.SSOTHash != hash {
			g = model.Stale
		}
	}
	h := htmlFreshness(s.Root, w.ws.Fingerprint, hash)
	// A page or graph built for a draft turn is not a confirmed view of the workspace.
	if maps == model.Draft {
		if g == model.Current {
			g = model.Draft
		}
		if h == model.Current {
			h = model.Draft
		}
	}
	if maps == model.Stale {
		if g == model.Current {
			g = model.Stale
		}
		if h == model.Current {
			h = model.Stale
		}
	}
	return Freshness{FeatureMap: maps, ArchitectureMap: maps, Graph: g, HTML: h}
}

func (w *world) status() *Status {
	s := w.s
	st := &Status{
		Root: s.Root, Mode: s.Project.Mode, Revision: w.ws.Revision, Dirty: w.ws.Dirty, Fingerprint: w.ws.Fingerprint,
		SyncedRevision: s.State.Revision, SyncedFingerprint: s.State.Fingerprint, SyncedAt: s.State.SyncedAt,
		Freshness: w.freshness(), PendingChanges: w.changedSinceSync(),
	}
	if c := s.ActiveChange(); c != nil {
		st.ActiveTurn = &TurnSummary{ID: c.ID, Request: c.RequestSummary, State: c.State, Plan: c.Plan}
	} else if len(st.PendingChanges) > 0 {
		st.ExternalChange = true
		st.Hint = "외부 변경 감지: `myharness reconcile` (또는 `myharness run`이 자동 재조정)"
	}
	st.Knowledge = map[string]int{}
	for _, f := range s.Features {
		if f.Status != model.ItemRemoved {
			st.Knowledge["features"]++
		}
	}
	for _, m := range s.Modules {
		if m.Status != model.ItemRemoved {
			st.Knowledge["modules"]++
			for _, c := range m.ExternalContracts {
				if c.Status != model.ItemRemoved {
					st.Knowledge["contracts"]++
				}
			}
		}
	}
	st.Knowledge["tests"] = len(s.Tests)
	st.Knowledge["changes"] = len(s.Changes)
	st.Knowledge["decisions"] = len(s.Decisions)
	st.Unconfirmed = len(html.Pending(s))
	return st
}
