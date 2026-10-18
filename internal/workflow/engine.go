// Package workflow is the Turn Engine and the Project Intelligence API (PRD ch.03-05, 09).
// The CLI and the MCP server both call these functions, so they return the same state (FR-10).
// The engine, not the model's memory, owns turn state and the completion gates.
package workflow

import (
	"fmt"
	"path/filepath"
	"runtime"
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
	w := &world{s: s, ws: ws}
	// An agent edit that was reverted is no longer pending work; left recorded, it would hold a
	// later human edit of the same file as the agent's.
	if n := len(s.State.AgentEdits); n > 0 {
		if s.State.AgentEdits = w.agentEdited(w.changedSinceSync()); len(s.State.AgentEdits) != n {
			if err := s.SaveState(); err != nil {
				return nil, err
			}
		}
	}
	return w, nil
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
	s.State.LastPrompt = s.State.UserPromptAt
	return s.SaveState()
}

// NoteSession records a host agent session opening ("start", refreshed on each prompt) or
// closing ("end"). Like the other hooks it never fails the host.
func (e *Engine) NoteSession(event, id string) error {
	if !ssot.Exists(e.Root) {
		return nil
	}
	s, err := ssot.Load(e.Root)
	if err != nil {
		return nil
	}
	if id == "" {
		id = "default"
	}
	if event == "end" {
		delete(s.State.Sessions, id)
	} else {
		s.State.Sessions = mapSet(s.State.Sessions, id, now())
	}
	return s.SaveState()
}

// sessionTTL bounds a session whose end hook never ran (a crash): it stops counting after this.
const sessionTTL = 12 * time.Hour

// agentSession returns an open host agent session, or "".
func (w *world) agentSession() string {
	for _, id := range model.SortedKeys(w.s.State.Sessions) {
		if t, err := time.Parse(time.RFC3339, w.s.State.Sessions[id]); err == nil && time.Since(t) < sessionTTL {
			return id
		}
	}
	return ""
}

// duringSession refuses to take a change as outside while a host agent session is open: the
// agent may have made it with any tool, and only the user can say they made it.
func (w *world) duringSession(byUser bool) error {
	if id := w.agentSession(); id != "" && !byUser {
		return fmt.Errorf("%w: %s changed while agent session %s is open. Open a turn (abbd run \"<request>\"), then abbd verify and abbd sync; if the user made the change: abbd reconcile --user", ErrAgentEdits, strings.Join(w.changedSinceSync(), ", "), id)
	}
	return nil
}

// NoteEdit records a file the agent edited (host PostToolUse hook). Edits under .abbd are the
// engine's own business and are not recorded.
func (e *Engine) NoteEdit(path string) error {
	if !ssot.Exists(e.Root) || strings.TrimSpace(path) == "" {
		return nil
	}
	rel := filepath.Clean(path)
	if filepath.IsAbs(rel) {
		r, err := filepath.Rel(e.Root, rel)
		if err != nil {
			return nil
		}
		rel = r
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || samePath(rel, ssot.Dir) || hasPathPrefix(rel, ssot.Dir+"/") {
		return nil
	}
	s, err := ssot.Load(e.Root)
	if err != nil {
		return nil
	}
	for _, f := range s.State.AgentEdits {
		if samePath(f, rel) {
			return nil
		}
	}
	s.State.AgentEdits = append(s.State.AgentEdits, rel)
	return s.SaveState()
}

// foldCase: Windows and macOS file systems ignore case, so INTERNAL/api.go is internal/api.go.
var foldCase = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

func samePath(a, b string) bool { return a == b || foldCase && strings.EqualFold(a, b) }

func hasPathPrefix(p, prefix string) bool {
	return len(p) >= len(prefix) && samePath(p[:len(prefix)], prefix)
}

// agentEdited returns the changed files the agent edited without a completed turn.
func (w *world) agentEdited(changed []string) []string {
	var out []string
	for _, f := range changed {
		for _, a := range w.s.State.AgentEdits {
			if samePath(a, f) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// AcceptHandEdits acknowledges md originals edited outside abbd. Hand-typed user-only
// provenance (USER_STATED, CONFIRMED, an answer, an approval) survives only the user's own
// acceptance (byUser); any other acceptance lowers it.
func (e *Engine) AcceptHandEdits(byUser bool) error {
	w, err := e.load()
	if err != nil {
		return err
	}
	w.s.AcceptHeldEdits(byUser)
	w.s.HandEdited, w.s.State.HandEdits = false, false
	return w.s.Save()
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
