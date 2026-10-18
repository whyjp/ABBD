package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/cli"
	"github.com/whyjp/abbd/internal/mcp"
	"github.com/whyjp/abbd/internal/workflow"
	"gopkg.in/yaml.v3"
)

// TestMCPMatchesCLI covers FR-10: MCP and CLI return the same project state.
func TestMCPMatchesCLI(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init", "--mode", "code-only")
	cliStatus := p.ok("status")

	e, err := workflow.New(p.dir)
	if err != nil {
		t.Fatal(err)
	}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"project_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_feature_context","arguments":{"id":"FEAT-001"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"abbd://confirm-queue"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"ask_user_decision","arguments":{"question":"q","options":["a","b"]}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := mcp.Serve(e, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var replies []map[string]any
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<22), 1<<22)
	for sc.Scan() {
		var r map[string]any
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("reply is not JSON-RPC: %s", sc.Text())
		}
		replies = append(replies, r)
	}
	if len(replies) != 6 {
		t.Fatalf("notifications get no reply: want 6 replies, got %d", len(replies))
	}
	if str(replies[0], "result", "protocolVersion") != mcp.ProtocolVersion {
		t.Fatalf("initialize: %v", replies[0])
	}
	var names []string
	for _, tl := range list(replies[1], "result", "tools") {
		names = append(names, str(tl, "name"))
	}
	for _, want := range []string{"project_status", "resolve_intent", "get_feature_context", "get_architecture_context", "trace_implementation",
		"analyze_change_impact", "get_verification_plan", "sync_project_knowledge", "check_consistency", "ask_user_decision",
		"get_external_contracts", "analyze_contract_impact", "list_unconfirmed", "confirm_item"} {
		if !has(names, want) {
			t.Fatalf("PRD ch.09 MCP tool %s missing", want)
		}
	}
	var mcpStatus map[string]any
	text := str(list(replies[2], "result", "content")[0], "text")
	if err := json.Unmarshal([]byte(text), &mcpStatus); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"revision", "fingerprint", "synced_fingerprint"} {
		if str(mcpStatus, "result", k) != str(cliStatus, k) {
			t.Fatalf("MCP %s = %q, CLI %q", k, str(mcpStatus, "result", k), str(cliStatus, k))
		}
	}
	if get(replies[3], "result", "isError") != false {
		t.Fatalf("get_feature_context: %v", replies[3])
	}
	if len(list(replies[4], "result", "contents")) != 1 {
		t.Fatalf("resources/read: %v", replies[4])
	}
	if get(replies[5], "result", "isError") != true {
		t.Fatal("ask_user_decision outside a turn must fail: bootstrap never asks")
	}
}

// cmdRe matches abbd where a command starts: line start, inside backticks, after a prompt,
// a shell operator or "--". Prose such as "abbd skills" is not a command.
var cmdRe = regexp.MustCompile("(?m)(?:^\\s*|`|\\$ |&& |\\| |\\(|-- )abbd ([a-z][a-z-]+)")

// TestSkillsUseRegisteredCommands encodes the agreement between the skills and the CLI: a
// skill that names a command the CLI does not register fails the build.
func TestSkillsUseRegisteredCommands(t *testing.T) {
	var files []string
	for _, root := range []string{"../plugins", "../docs"} {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".sh") || strings.HasSuffix(p, ".ps1")) {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) < 9 {
		t.Fatalf("expected skills, hosts and docs files, found %d", len(files))
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, m := range cmdRe.FindAllStringSubmatch(string(b), -1) {
			if _, ok := cli.Commands[m[1]]; !ok {
				t.Errorf("%s names `abbd %s`, which the CLI does not register", f, m[1])
			}
		}
	}
}

// TestSkillsFollowAgentSkillsSpec checks the PRD ch.09 skill set and SKILL.md frontmatter.
func TestSkillsFollowAgentSkillsSpec(t *testing.T) {
	want := []string{"session-bootstrap", "understand-project", "change-intent", "architect-change",
		"implement-and-prove", "impact-review", "sync-knowledge", "reflect-and-correct"}
	nameRe := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, name := range want {
		b, err := os.ReadFile(filepath.Join("..", "plugins", "abbd", "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatalf("PRD ch.09 skill %s: %v", name, err)
		}
		text := strings.ReplaceAll(string(b), "\r\n", "\n")
		if !strings.HasPrefix(text, "---\n") {
			t.Fatalf("%s: no frontmatter", name)
		}
		end := strings.Index(text[4:], "\n---")
		var fm struct{ Name, Description string }
		if err := yaml.Unmarshal([]byte(text[4:4+end]), &fm); err != nil {
			t.Fatalf("%s: frontmatter: %v", name, err)
		}
		if fm.Name != name || !nameRe.MatchString(fm.Name) {
			t.Errorf("%s: frontmatter name %q must equal the directory name", name, fm.Name)
		}
		if len(fm.Description) < 40 || len(fm.Description) > 1024 {
			t.Errorf("%s: description must say what and when (40..1024 chars), got %d", name, len(fm.Description))
		}
		if !strings.Contains(text, "origin") || !strings.Contains(text, "등급") {
			t.Errorf("%s: must use the shared provenance and tier vocabulary", name)
		}
	}
}

// TestReservedDomainLayer covers PRD 08A: Phase 1 reserves domain prefixes only.
func TestReservedDomainLayer(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	if _, code, stderr := p.run("domain", "inspect", "Heartbeat"); code == 0 || !strings.Contains(stderr, "Phase 3") {
		t.Fatalf("domain commands are reserved: exit %d %s", code, stderr)
	}
	if _, code, _ := p.run("inspect", "DOM-001"); code == 0 {
		t.Fatal("DOM-* IDs are reserved, not resolvable")
	}
}
