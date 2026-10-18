package tests

import (
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/workflow"
)

func TestAgentEditsAreVerifiedInATurnNotReconciled(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.commit("harness")
	e, _ := workflow.New(p.dir)
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	if err := e.NoteEdit("internal/api/api.go"); err != nil {
		t.Fatal(err)
	}
	if _, code, _ := p.run("reconcile"); code == 0 {
		t.Fatal("reconcile must not absorb the agent's own edit as an outside change")
	}
	plan := p.ok("run", "health 응답 변경")
	if get(plan, "reconciled") != nil {
		t.Fatalf("the agent's edit belongs to the turn, not to a reconcile: %v", get(plan, "reconciled"))
	}
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", strings.Replace(test, `rec.Body.String() != "ok"`, `rec.Body.String() != "ok!"`, 1))
	p.ok("verify")
	s := p.ok("sync")
	if !strings.Contains(strings.Join(strs(s, "receipt", "files_changed"), " "), "internal/api/api.go") {
		t.Fatalf("the turn's receipt covers the edit made before it opened: %v", get(s, "receipt", "files_changed"))
	}
}

func TestFailingFirstNeedsAFailingTest(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "health 응답 버그 고쳐줘")
	if _, code, _ := p.run("verify", "--cmd", "false"); code != 2 {
		t.Fatal("a failing command is still a failed run")
	}
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", strings.Replace(test, `rec.Body.String() != "ok"`, `rec.Body.String() != "ok!"`, 1))
	p.ok("verify")
	if v, code, _ := p.run("sync"); code != 2 {
		t.Fatalf("`false` failing reproduces nothing: the bug fix must not sync, %v", get(v, "reason"))
	}
}

func TestHandEditsStayFlaggedUntilAccepted(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	md := ".abbd/architecture/" + p.moduleAt("internal/storage") + ".md"
	p.replace(md, "(추정) storage 패키지가 노출하는 기능 묶음", "손으로 바꾼 책임")
	p.run("check")
	p.ok("intent", "rule", "--claim", "모든 응답은 1 KiB 이하여야 한다")
	handEdit := func() bool {
		v, _, _ := p.run("check")
		return strings.Contains(strings.Join(strs(v, "errors"), " "), "edited outside abbd")
	}
	if !handEdit() {
		t.Fatal("an engine write after the hand edit must not erase the flag")
	}
	p.ok("check", "--accept-edits")
	if handEdit() {
		t.Fatal("--accept-edits acknowledges the hand edit")
	}
}
