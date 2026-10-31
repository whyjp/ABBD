package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	if err := json.NewDecoder(e.stdin).Decode(&in); err != nil || in.SessionID == "" {
		return
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

// Only a human changes the intended topology. These commands and files are
// how it changes, so an agent in an active session may not reach them.
var (
	humanCommand  = regexp.MustCompile(`(?i)(?:^|[^\w.-])abbd(?:\.exe)?["']?\s+(align|approve|confirm|setup|mode\s+off)\b`)
	protectedPath = regexp.MustCompile(`(?i)\.abbd[/\\]+(intended\.yaml|project\.yaml|decisions|changes|sessions)(?:$|[/\\\s"'])`)
)

func (e env) hookPreToolUse(in hookInput) *hookOutput {
	reason := ""
	switch in.ToolName {
	case "Bash", "PowerShell":
		reason = e.deniedCommand(in.ToolInput.Command)
	case "Write", "Edit", "NotebookEdit":
		file := in.ToolInput.FilePath
		if file == "" {
			file = in.ToolInput.NotebookPath
		}
		if file != "" && protectedPath.MatchString(relTo(e.root, file)+" ") {
			reason = fmt.Sprintf("%s is written only by the abbd CLI. Approvals, alignment and the turn record are a human's or the CLI's, not an agent's.", relTo(e.root, file))
		}
	}
	if reason == "" {
		return nil
	}
	return &hookOutput{Specific: &hookSpecific{HookEventName: "PreToolUse", PermissionDecision: "deny", PermissionDecisionReason: reason}}
}

// requestHeredoc is abbd turn start reading the user's request from a heredoc.
// The body is the request text, which abbd stores and never runs.
var requestHeredoc = regexp.MustCompile(`^\s*abbd(?:\.exe)?\s+turn\s+start\b[^\n]*<<-?\s*['"]?(\w+)['"]?[ \t]*\n`)

// dropRequestText removes the request body so a request that mentions a
// human's command does not read as running it.
func dropRequestText(cmd string) string {
	m := requestHeredoc.FindStringSubmatchIndex(cmd)
	if m == nil {
		return cmd
	}
	delim := cmd[m[2]:m[3]]
	head, body := cmd[:m[1]], cmd[m[1]:]
	lines := strings.SplitAfter(body, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == delim {
			return head + strings.Join(lines[i:], "")
		}
	}
	return head
}

func (e env) deniedCommand(cmd string) string {
	cmd = dropRequestText(cmd)
	if m := humanCommand.FindStringSubmatch(cmd); m != nil {
		sub := strings.ToLower(strings.Join(strings.Fields(m[1]), " "))
		if sub == "setup" && e.registersScript(cmd) {
			return ""
		}
		return fmt.Sprintf("abbd %s is a human's command while ABBD mode is on. Stop and give the human the exact command to run.", sub)
	}
	if protectedPath.MatchString(cmd + " ") {
		return "this command names a file only the abbd CLI writes (.abbd/intended.yaml, project.yaml, decisions/, changes/, sessions/). Read it with the Read tool, or use abbd turn show and abbd status."
	}
	return ""
}

// registersScript allows the one setup an agent runs inside the mode: a
// greenfield project's first turn registering its extraction script.
func (e env) registersScript(cmd string) bool {
	p, err := e.st.LoadProject()
	return err == nil && len(p.Extract) == 0 && !strings.Contains(cmd, "--greenfield")
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
