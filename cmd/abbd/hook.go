package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/whyjp/abbd/internal/store"
	"github.com/whyjp/abbd/internal/turn"
)

// hookInput is the part of Claude Code's hook JSON that abbd reads.
type hookInput struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	ToolName       string `json:"tool_name"`
	StopHookActive bool   `json:"stop_hook_active"`
	ToolInput      struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Command      string `json:"command"`
	} `json:"tool_input"`
	// raw is tool_input as sent, for tools whose fields abbd does not know.
	raw string
}

// hookOutput is the JSON a hook prints on stdout. Stop blocks through the
// top-level decision; the other events speak through hookSpecificOutput.
type hookOutput struct {
	Decision string        `json:"decision,omitempty"`
	Reason   string        `json:"reason,omitempty"`
	Specific *hookSpecific `json:"hookSpecificOutput,omitempty"`
}

type hookSpecific struct {
	HookEventName            string `json:"hookEventName"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

func hookContext(event, text string) *hookOutput {
	return &hookOutput{Specific: &hookSpecific{HookEventName: event, AdditionalContext: text}}
}

// hooks maps each Claude Code event to its handler. A nil result prints nothing.
var hooks = map[string]func(e env, in hookInput) *hookOutput{
	"SessionStart":     env.hookSessionStart,
	"UserPromptSubmit": env.hookPromptSubmit,
	"PreToolUse":       env.hookPreToolUse,
	"PostToolUse":      env.hookPostToolUse,
	"Stop":             env.hookStop,
}

// hook never fails the host. Outside a session whose mode is on it does nothing
// at all. Once the mode is on, drift the session itself causes does not switch
// the hooks off, or an agent could lift the guard by drifting the project.
func (e env) hook(event string) {
	var in hookInput
	b, err := io.ReadAll(e.stdin)
	if err != nil || json.Unmarshal(b, &in) != nil || in.SessionID == "" {
		return
	}
	var raw struct {
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if json.Unmarshal(b, &raw) == nil {
		in.raw = string(raw.ToolInput)
	}
	e.root = e.hookRoot(in.Cwd)
	e.st = store.Open(e.root)
	handle, ok := hooks[event]
	if !ok || !e.st.ModeOn(in.SessionID) {
		return
	}
	e.session = in.SessionID
	if out := handle(e, in); out != nil {
		if err := json.NewEncoder(e.stdout).Encode(out); err != nil {
			fmt.Fprintln(e.stderr, "abbd hook:", err)
		}
	}
}

// hookRoot prefers an explicit --root, then the project dir Claude Code exports,
// then the nearest ancestor of the hook's cwd that holds .abbd.
func (e env) hookRoot(cwd string) string {
	if e.root != "." {
		return e.root
	}
	if d := os.Getenv("CLAUDE_PROJECT_DIR"); d != "" {
		return d
	}
	for d := cwd; d != ""; d = filepath.Dir(d) {
		if fi, err := os.Stat(filepath.Join(d, store.Dir)); err == nil && fi.IsDir() {
			return d
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return e.root
}

func relTo(root, p string) string {
	if !filepath.IsAbs(p) {
		return filepath.ToSlash(filepath.Clean(p))
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.ToSlash(p)
	}
	if r, err := filepath.Rel(abs, p); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

func (e env) openTurn() *turn.Turn {
	t, err := e.st.LatestTurn()
	if err != nil {
		return nil
	}
	return t
}

func (e env) hookSessionStart(in hookInput) *hookOutput {
	v, err := e.load()
	if err != nil {
		return nil
	}
	text := fmt.Sprintf("ABBD mode is on for this session. Phase: %s.", v.phase)
	if t := e.openTurn(); t != nil && t.State.Resumable() {
		text += fmt.Sprintf(" %s is %s. Continue it under /abbd-mode and finish with abbd turn end --session %s.", t.ID, t.State, in.SessionID)
	} else {
		text += " No turn is open."
	}
	return hookContext("SessionStart", text)
}

func (e env) hookPromptSubmit(in hookInput) *hookOutput {
	if t := e.openTurn(); t != nil && t.State.Live() {
		return nil
	}
	return hookContext("UserPromptSubmit", fmt.Sprintf(
		"ABBD mode is on and no turn is open. Before changing code, open one under /abbd-mode: abbd turn start --session %s --seeds IDS --request -", in.SessionID))
}

func (e env) hookPostToolUse(in hookInput) *hookOutput {
	file := in.ToolInput.FilePath
	if file == "" {
		file = in.ToolInput.NotebookPath
	}
	if file != "" {
		if err := e.st.AppendJournal(in.SessionID, "changed "+relTo(e.root, file)); err != nil {
			fmt.Fprintln(e.stderr, "abbd hook:", err)
		}
	}
	return nil
}

// Only a human changes the intended topology, and only the CLI writes the
// turn record and the session's mode, so an agent in an active session may
// not run those commands or write those files.
func (e env) hookPreToolUse(in hookInput) *hookOutput {
	g := e.guard(in.Cwd)
	reason := ""
	switch in.ToolName {
	case "Bash", "PowerShell":
		reason = g.command(in.ToolInput.Command)
	case "Write", "Edit", "NotebookEdit":
		file := in.ToolInput.FilePath
		if file == "" {
			file = in.ToolInput.NotebookPath
		}
		reason = g.toolPath(file)
	default:
		// A browser or fetch tool could drive the screen as the human would.
		if g.reachesScreen(in.raw) {
			reason = screenReason
		}
	}
	if reason == "" {
		return nil
	}
	return &hookOutput{Specific: &hookSpecific{HookEventName: "PreToolUse", PermissionDecision: "deny", PermissionDecisionReason: reason}}
}

// hookStop blocks the session from ending once while a turn it opened has
// changed the working tree and never reached abbd turn end. A turn whose
// question waits for a human is meant to stop there.
func (e env) hookStop(in hookInput) *hookOutput {
	t := e.openTurn()
	if in.StopHookActive || t == nil || t.State != turn.Open || t.Session != in.SessionID || t.Waiting() {
		return nil
	}
	tree, err := e.tree()
	if err != nil || tree == t.Baseline.Fingerprint {
		return nil
	}
	return &hookOutput{Decision: "block", Reason: fmt.Sprintf(
		"%s is still OPEN and has changed the working tree. Run abbd turn verify --session %s, then abbd turn end --session %s, and report the state it prints. A turn that cannot finish still ends, as DRAFT or BLOCKED.",
		t.ID, in.SessionID, in.SessionID)}
}
