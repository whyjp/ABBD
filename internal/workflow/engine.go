// Package workflow is the Turn Engine and the Project Intelligence API (PRD ch.03-05, 09).
// The CLI and the MCP server both call these functions, so they return the same state (FR-10).
// The engine, not the model's memory, owns turn state and the completion gates.
package workflow

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/analysis"
	"github.com/whyjp/abbd/internal/bootstrap"
	"github.com/whyjp/abbd/internal/graph"
	"github.com/whyjp/abbd/internal/html"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
	"github.com/whyjp/abbd/internal/workspace"
)

type Engine struct {
	Root string
	// Notice is a one-time message for the user, such as the v1.3 directory migration.
	Notice string
}

func New(root string) (*Engine, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	notice, err := ssot.MigrateLegacy(abs)
	if err != nil {
		return nil, err
	}
	return &Engine{Root: abs, Notice: notice}, nil
}

// world is one consistent observation of the project for the duration of a command.
type world struct {
	s   *ssot.Snapshot
	ws  workspace.Snapshot
	res *analysis.Result
}

func (e *Engine) load() (*world, error) {
	if !ssot.Exists(e.Root) {
		return nil, fmt.Errorf("no project knowledge in %s; run `abbd init`", e.Root)
	}
	s, err := ssot.Load(e.Root)
	if err != nil {
		return nil, err
	}
	ws, err := workspace.Observe(e.Root)
	if err != nil {
		return nil, err
	}
	return &world{s: s, ws: ws}, nil
}

func (w *world) analyze() (*analysis.Result, error) {
	if w.res == nil {
		res, err := analysis.Analyze(w.s.Root)
		if err != nil {
			return nil, err
		}
		w.res = res
	}
	return w.res, nil
}

// renames combines content-hash moves (works without git) with git rename detection.
func (w *world) renames() map[string]string {
	r := identity.ContentRenames(w.s.State.Files, w.ws.Files)
	for k, v := range workspace.Renames(w.s.Root, w.s.State.Revision) {
		r[k] = v
	}
	return r
}

// changedSinceSync lists files that differ from the last sync watermark.
func (w *world) changedSinceSync() []string {
	return workspace.Diff(w.s.State.Files, w.ws.Files)
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// countGraphQuery attributes a graph/knowledge query to the active turn (FR-28 telemetry).
// The counter lives in state.yaml, not in the CHG markdown, so a query does not make the
// derived graph and HTML look stale; verify and sync fold it into the receipt.
func (w *world) countGraphQuery() {
	if w.s.ActiveChange() != nil {
		w.s.State.TurnGraphQueries++
		_ = w.s.SaveState()
	}
}

// NotePrompt stores the user's message verbatim for the next turn. It never fails the host: a
// project without .abbd simply has nothing to record yet.
func (e *Engine) NotePrompt(prompt string) error {
	if !ssot.Exists(e.Root) || strings.TrimSpace(prompt) == "" {
		return nil
	}
	s, err := ssot.Load(e.Root)
	if err != nil {
		return nil
	}
	s.State.UserPrompt, s.State.UserPromptAt = prompt, now()
	return s.SaveState()
}

// Init is `abbd init`: bootstrap when there is no knowledge, otherwise a session start.
func (e *Engine) Init(opt bootstrap.Options) (any, error) {
	if ssot.Exists(e.Root) && opt.Mode == "" && opt.From == "" {
		return e.Status()
	}
	return bootstrap.Run(e.Root, opt)
}

func (e *Engine) graph() (*graph.Graph, error) { return graph.Load(e.Root) }

// queryGraph loads the derived graph with its freshness. A stale graph is still served, but
// every answer carries STALE so it is never mistaken for current (NFR-03).
func (e *Engine) queryGraph(w *world) (*graph.Graph, model.Freshness, error) {
	g, err := e.graph()
	if err != nil {
		return nil, model.UnknownFresh, err
	}
	if g.Fingerprint != w.ws.Fingerprint || g.SSOTHash != ssot.Hash(e.Root) {
		return g, model.Stale, nil
	}
	return g, model.Current, nil
}

func htmlFreshness(root, fp, hash string) model.Freshness {
	_, hfp, hhash, ok := html.ReadWatermark(root)
	switch {
	case !ok:
		return model.UnknownFresh
	case hfp != fp || hhash != hash:
		return model.Stale
	default:
		return model.Current
	}
}
