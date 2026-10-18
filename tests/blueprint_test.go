package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The agent's context is the current blueprint; per-request history stays out of it.

func TestBlueprintPageAndHistoryIgnore(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	bp := p.read(".abbd/BLUEPRINT.md")
	for _, want := range []string{"## Standing rules", "## Modules", "## External contracts", "## Features", "POST /messages"} {
		if !strings.Contains(bp, want) {
			t.Fatalf("BLUEPRINT.md lacks %q", want)
		}
	}
	ignore := p.read(".abbd/.ignore")
	for _, dir := range []string{"changes/", "requirements/", "decisions/", "verification/", "generated/"} {
		if !strings.Contains(ignore, dir) {
			t.Fatalf(".abbd/.ignore must exclude %s", dir)
		}
	}
	plan := p.ok("run", "POST /messages 응답 형식 변경")
	if first := strs(plan, "read_first"); len(first) == 0 || first[0] != ".abbd/BLUEPRINT.md" {
		t.Fatalf("run must point at the blueprint first: %v", first)
	}
	for _, f := range strs(plan, "read_first") {
		for _, h := range []string{"/changes/", "/requirements/", "/decisions/"} {
			if strings.Contains(f, h) {
				t.Fatalf("read_first must not send the agent to history: %s", f)
			}
		}
	}
}

func TestStandingRuleReachesLaterTurnsAndTheBlueprint(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "앞으로 모든 HTTP 에러 응답은 JSON {\"error\": ...} 형식이어야 한다")
	p.write("README.md", "# heartbeat\n")
	p.ok("sync")
	if !strings.Contains(p.read(".abbd/BLUEPRINT.md"), "모든 HTTP 에러 응답은 JSON") {
		t.Fatal("a rule the user stated for the whole project must be on the blueprint")
	}
	later := p.ok("run", "GET /heartbeats/{id} 조회 API 추가")
	if !strings.Contains(strings.Join(strs(later, "standing_rules"), " "), "모든 HTTP 에러 응답은 JSON") {
		t.Fatalf("a later turn that does not mention the rule must still be shown it: %v", get(later, "standing_rules"))
	}
	p.ok("turn", "abort")
	p.ok("run", "규칙 추가")
	r := p.ok("intent", "rule", "--claim", "source가 test인 heartbeat는 어떤 경로로도 저장하지 않는다")
	if str(r, "scope") != "project" || str(r, "statement", "origin") != "USER_STATED" {
		t.Fatalf("intent rule records a USER_STATED project rule: %v", r)
	}
}

func TestHistoryGuardRefusesHistoryReads(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	guard, _ := filepath.Abs("../plugins/abbd/scripts/history-guard.sh")
	cases := map[string]bool{
		`{"tool_name":"Read","tool_input":{"file_path":"C:\\p\\.abbd\\changes\\CHG-001.md"}}`:     true,
		`{"tool_name":"Grep","tool_input":{"pattern":"x","path":".abbd/requirements"}}`:           true,
		`{"tool_name":"Glob","tool_input":{"pattern":".abbd/decisions/*.md"}}`:                    true,
		`{"tool_name":"Bash","tool_input":{"command":"cat .abbd/verification/runs/run-1.yaml"}}`:  true,
		`{"tool_name":"Bash","tool_input":{"command":"abbd review CHG-001 && ls .abbd/changes"}}`: true,
		`{"tool_name":"Read","tool_input":{"file_path":".abbd/BLUEPRINT.md"}}`:                    false,
		`{"tool_name":"Read","tool_input":{"file_path":".abbd/features/FEAT-001.md"}}`:            false,
		`{"tool_name":"Bash","tool_input":{"command":"git add .abbd/changes"}}`:                   false,
		`{"tool_name":"Bash","tool_input":{"command":"abbd inspect CHG-001"}}`:                    false,
		`{"tool_name":"Read","tool_input":{"file_path":"internal/changes/changes.go"}}`:           false,
	}
	for in, deny := range cases {
		cmd := exec.Command(sh, guard)
		cmd.Stdin = strings.NewReader(in)
		out, _ := cmd.Output()
		if got := strings.Contains(string(out), `"permissionDecision":"deny"`); got != deny {
			t.Errorf("%s: deny=%v, want %v", in, got, deny)
		}
	}
}

// ripgrep (Claude Code's Grep, Codex, editors) skips history through .abbd/.ignore.
func TestSearchSkipsHistory(t *testing.T) {
	rg, err := exec.LookPath("rg")
	if err != nil {
		t.Skip("rg not installed")
	}
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "zzuniquetoken 문서 정리")
	p.write("README.md", "# x\n")
	p.ok("sync")
	if _, err := os.Stat(filepath.Join(p.dir, ".abbd", "requirements")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(rg, "-l", "--hidden", "zzuniquetoken", ".")
	cmd.Dir = p.dir
	out, _ := cmd.Output()
	if strings.Contains(string(out), "requirements") || strings.Contains(string(out), "changes") {
		t.Fatalf("search must not return history: %s", out)
	}
}
