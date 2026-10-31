package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fire runs one hook the way Claude Code does: the event as the argument and
// the hook JSON on stdin. A hook must always exit 0.
func (p *project) fire(event string, fields map[string]any) (hookOutput, string) {
	p.t.Helper()
	in := map[string]any{"session_id": "s1", "cwd": p.root, "hook_event_name": event, "permission_mode": "default"}
	for k, v := range fields {
		in[k] = v
	}
	b, err := json.Marshal(in)
	if err != nil {
		p.t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"hook", event, "--root", p.root}, bytes.NewReader(b), &out, &errb); code != 0 {
		p.t.Fatalf("hook %s exit %d: %s", event, code, errb.String())
	}
	var got hookOutput
	if s := strings.TrimSpace(out.String()); s != "" {
		if err := json.Unmarshal([]byte(s), &got); err != nil {
			p.t.Fatalf("hook %s printed something other than one JSON object: %q", event, s)
		}
	}
	return got, out.String()
}

func tool(name string, input map[string]any) map[string]any {
	return map[string]any{"tool_name": name, "tool_input": input}
}

func (p *project) bash(cmd string) map[string]any {
	return tool("Bash", map[string]any{"command": cmd, "description": "run it"})
}

func denied(o hookOutput) string {
	if o.Specific == nil || o.Specific.HookEventName != "PreToolUse" || o.Specific.PermissionDecision != "deny" {
		return ""
	}
	return o.Specific.PermissionDecisionReason
}

func TestHooksDoNothingWhileTheModeIsOff(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	p.want(0, "mode off", "mode", "off", "--session", "s1")
	p.editCode("billing/charge.go")
	for _, c := range []struct {
		event  string
		fields map[string]any
	}{
		{"SessionStart", map[string]any{"source": "startup"}},
		{"UserPromptSubmit", map[string]any{"prompt": "change billing"}},
		{"PreToolUse", p.bash("abbd approve new-relation:MOD-0001.depends_on.MOD-0002")},
		{"PreToolUse", tool("Write", map[string]any{"file_path": filepath.Join(p.root, ".abbd", "intended.yaml"), "content": "x"})},
		{"PostToolUse", tool("Edit", map[string]any{"file_path": filepath.Join(p.root, "billing", "charge.go")})},
		{"Stop", map[string]any{"stop_hook_active": false}},
	} {
		if _, out := p.fire(c.event, c.fields); out != "" {
			t.Errorf("%s with the mode off must print nothing, got %q", c.event, out)
		}
	}
	if p.journal("s1") != "" {
		t.Fatalf("PostToolUse with the mode off must not journal:\n%s", p.journal("s1"))
	}
}

func TestSessionStartReportsThePhaseAndTheTurn(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	o, _ := p.fire("SessionStart", map[string]any{"source": "resume"})
	if o.Specific == nil || o.Specific.HookEventName != "SessionStart" ||
		!strings.Contains(o.Specific.AdditionalContext, "Phase: aligned") || !strings.Contains(o.Specific.AdditionalContext, "No turn is open") {
		t.Fatalf("SessionStart should add the phase as context: %+v", o.Specific)
	}
	p.start("MOD-0001")
	o, _ = p.fire("SessionStart", map[string]any{"source": "resume"})
	if !strings.Contains(o.Specific.AdditionalContext, "CHG-0001 is OPEN") || !strings.Contains(o.Specific.AdditionalContext, "abbd turn end --session s1") {
		t.Fatalf("SessionStart should name the open turn: %q", o.Specific.AdditionalContext)
	}
}

func TestPromptSubmitRemindsToOpenATurn(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	o, _ := p.fire("UserPromptSubmit", map[string]any{"prompt": "rename the charge field"})
	if o.Specific == nil || o.Specific.HookEventName != "UserPromptSubmit" || !strings.Contains(o.Specific.AdditionalContext, "abbd turn start --session s1") {
		t.Fatalf("a prompt with no open turn should get a reminder: %+v", o.Specific)
	}
	p.start("MOD-0001")
	if _, out := p.fire("UserPromptSubmit", map[string]any{"prompt": "go on"}); out != "" {
		t.Fatalf("a prompt inside an open turn needs no reminder, got %q", out)
	}
}

func TestPreToolUseKeepsTheIntendedTopologyHumanOnly(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	abs := func(rel ...string) string { return filepath.Join(append([]string{p.root}, rel...)...) }
	deny := []map[string]any{
		p.bash("abbd approve new-relation:MOD-0001.depends_on.MOD-0002"),
		p.bash("abbd align --all"),
		p.bash(`abbd confirm MOD-0001.purpose --value "takes payments"`),
		p.bash("cd /repo && /usr/local/bin/abbd.exe setup --supplement s.json -- python x.py"),
		p.bash("abbd mode off --session s1"),
		p.bash("go run ./cmd/abbd approve contract-changed:IF-0001"),
		p.bash("cat .abbd/intended.yaml"),
		p.bash("rm .abbd/sessions/s1.on"),
		tool("PowerShell", map[string]any{"command": `Set-Content .abbd\changes\CHG-0001.md "state: SYNCED"`}),
		tool("PowerShell", map[string]any{"command": "& abbd.exe Approve contract-changed:IF-0001"}),
		tool("Write", map[string]any{"file_path": abs(".abbd", "intended.yaml"), "content": "units: []"}),
		tool("Edit", map[string]any{"file_path": abs(".abbd", "decisions", "ADR-0001.md"), "old_string": "a", "new_string": "b"}),
		tool("Write", map[string]any{"file_path": abs("billing", "..", ".abbd", "project.yaml"), "content": "extract: [true]"}),
		tool("NotebookEdit", map[string]any{"notebook_path": abs(".abbd", "changes", "x.ipynb"), "new_source": "x"}),
	}
	for _, f := range deny {
		o, _ := p.fire("PreToolUse", f)
		if denied(o) == "" {
			t.Errorf("PreToolUse should deny %v, got %+v", f["tool_input"], o.Specific)
		}
	}
	allow := []map[string]any{
		p.bash("abbd turn diff --session s1"),
		p.bash("abbd status && abbd turn show"),
		p.bash("abbd mode on --session s1"),
		p.bash("go test ./... # abbdapprove is not a command"),
		p.bash("git add -A && git commit -m 'charge in cents'"),
		tool("Write", map[string]any{"file_path": abs("billing", "charge.go"), "content": "package billing"}),
		tool("Write", map[string]any{"file_path": abs(".abbd", "extractor", "extract.py"), "content": "print('{}')"}),
		tool("Read", map[string]any{"file_path": abs(".abbd", "intended.yaml")}),
	}
	for _, f := range allow {
		if _, out := p.fire("PreToolUse", f); out != "" {
			t.Errorf("PreToolUse should leave %v alone, got %q", f["tool_input"], out)
		}
	}
}

// A greenfield project's first turn registers its extraction script with
// abbd setup, so that one setup stays open to the agent.
func TestPreToolUseLetsAGreenfieldTurnRegisterItsScript(t *testing.T) {
	p := activeTurnProject(t, topoV1).project
	register := p.bash("abbd setup -- python .abbd/extractor/extract.py")
	if o, _ := p.fire("PreToolUse", register); denied(o) == "" {
		t.Fatalf("setup must be denied once the project records a script")
	}
	write(t, filepath.Join(p.root, ".abbd", "project.yaml"), "extract: []\n")
	if _, out := p.fire("PreToolUse", register); out != "" {
		t.Fatalf("registering the first script should be allowed, got %q", out)
	}
	if o, _ := p.fire("PreToolUse", p.bash("abbd setup --greenfield units.json")); denied(o) == "" {
		t.Fatalf("restating the greenfield units changes the intended topology and must be denied")
	}
}

func TestStopBlocksOnceUntilTheOpenTurnEnds(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	stop := func(active bool) hookOutput {
		o, _ := p.fire("Stop", map[string]any{"stop_hook_active": active, "last_assistant_message": "done"})
		return o
	}
	if o := stop(false); o.Decision != "" {
		t.Fatalf("no turn, nothing to block: %+v", o)
	}
	p.want(0, "active", "mode", "on", "--session", "s2")
	p.start("MOD-0001")
	if o := stop(false); o.Decision != "" {
		t.Fatalf("an open turn that changed nothing may stop: %+v", o)
	}
	p.editCode("billing/charge.go")
	o := stop(false)
	if o.Decision != "block" || !strings.Contains(o.Reason, "CHG-0001 is still OPEN") || !strings.Contains(o.Reason, "abbd turn end --session s1") {
		t.Fatalf("an open turn that changed the tree should block the stop: %+v", o)
	}
	if o := stop(true); o.Decision != "" {
		t.Fatalf("a stop already continued by this hook must not be blocked again: %+v", o)
	}
	in, _ := json.Marshal(map[string]any{"session_id": "s2", "hook_event_name": "Stop", "stop_hook_active": false})
	var out bytes.Buffer
	run([]string{"hook", "Stop", "--root", p.root}, bytes.NewReader(in), &out, &bytes.Buffer{})
	if out.Len() != 0 {
		t.Fatalf("another session's stop is not this turn's to block: %s", out.String())
	}

	p.verify("pass", 0, "PASSED")
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if o := stop(false); o.Decision != "" {
		t.Fatalf("an ended turn must not block: %+v", o)
	}
}

func TestModeOnResumesAnUnfinishedTurn(t *testing.T) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	p.extract(strings.Replace(topoV1, `{"from":"billing","to":"api","kind":"provides"}`,
		`{"from":"billing","to":"api","kind":"provides"},{"from":"billing","to":"ledger","kind":"depends_on"}`, 1))

	out := p.want(0, "active: resuming CHG-0001 (OPEN)", "mode", "on", "--session", "s1")
	if !strings.Contains(out, "C BLOCKED new-relation:MOD-0001.depends_on.MOD-0002") {
		t.Fatalf("resuming should show what the turn has changed so far:\n%s", out)
	}
	p.want(0, "CHG-0001 moves from session s1 to s2", "mode", "on", "--session", "s2")
	p.want(0, "mode: off", "status", "--session", "s1")
	p.want(1, "CHG-0001 belongs to session s2, not s1", "turn", "diff", "--session", "s1")
	p.want(2, "C BLOCKED", "turn", "diff", "--session", "s2")

	p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s2")
	p.want(0, "active: resuming CHG-0001 (BLOCKED)", "mode", "on", "--session", "s3")
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "sessions", "s2.on")); err == nil {
		t.Fatalf("the session that gave the turn up must be off")
	}
}

// The plugin registers every hook abbd handles, each through abbd hook EVENT.
func TestPluginHooksRouteToAbbdHook(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "plugins", "abbd", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type, Command string
				Args          []string
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	for event := range hooks {
		groups := cfg.Hooks[event]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("%s should have exactly one hook, got %+v", event, groups)
		}
		h := groups[0].Hooks[0]
		if h.Type != "command" || h.Command != "abbd" || fmt.Sprint(h.Args) != fmt.Sprint([]string{"hook", event}) {
			t.Errorf("%s should run abbd hook %s in exec form, got %+v", event, event, h)
		}
		if _, err := parse(h.Args, &bytes.Buffer{}); err != nil {
			t.Errorf("%s: %v", event, err)
		}
	}
	if len(cfg.Hooks) != len(hooks) {
		t.Errorf("hooks.json registers %d events, abbd handles %d", len(cfg.Hooks), len(hooks))
	}
	for _, name := range []string{"Bash", "PowerShell", "Write", "Edit", "NotebookEdit"} {
		if !strings.Contains("|"+cfg.Hooks["PreToolUse"][0].Matcher+"|", "|"+name+"|") {
			t.Errorf("PreToolUse must match %s, matcher is %q", name, cfg.Hooks["PreToolUse"][0].Matcher)
		}
	}
}
