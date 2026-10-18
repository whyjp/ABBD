package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/workflow"
)

// Bypasses an adversarial review of the density round-2 fixes reproduced.

func TestDriverRefusesTierCCommands(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	if _, code, stderr := p.run("driver", "set", feat, "--run", "git push origin master", "--expect", "x"); code == 0 || !strings.Contains(stderr, "tier C") {
		t.Fatalf("a driver that pushes is a tier C action, not a check: exit %d %s", code, stderr)
	}
	// A driver that reached project.yaml some other way is refused when it would run.
	p.write(".abbd/project.yaml", p.read(".abbd/project.yaml")+"drivers:\n  "+feat+":\n    run: git push origin master\n    expect: x\n")
	p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	if _, code, stderr := p.run("verify", "--drive"); code != 1 || !strings.Contains(stderr, "tier C") {
		t.Fatalf("verify --drive must refuse a tier C driver before running it: exit %d %s", code, stderr)
	}
}

func TestDecisionFileNeedsTheUserFlag(t *testing.T) {
	p, post := breakingSetup(t)
	v, _, _ := p.run("sync")
	card := pendingCard(t, v, post)
	f := filepath.Join(p.dir, "decisions.json")
	os.WriteFile(f, []byte(`{"decisions":[{"delta":"`+card+`","decision":"approve"}]}`), 0o644)
	if _, code, _ := p.run("review", "--apply", f); code == 0 {
		t.Fatal("a decision file the agent can write is not the user's answer without --user")
	}
	p.ok("review", "--apply", f, "--user")
}

func TestConfirmIsTheUsersWord(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("confirm", post, "--external-consumers", "true", "--user")
	if _, code, _ := p.run("confirm", post, "--external-consumers", "false"); code == 0 {
		t.Fatal("dropping external consumers re-tiers C to B: the agent may not do it")
	}
	if _, code, _ := p.run("confirm", p.featureFor("GET /health")); code == 0 {
		t.Fatal("confirming a meaning is the user's word")
	}
	if c := p.ok("inspect", post); get(c, "contract", "external_consumers") != true {
		t.Fatalf("external consumers stay as the user stated: %v", get(c, "contract"))
	}
}

func TestHandRaisedProvenanceIsNotTrusted(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	m := p.moduleAt("internal/storage")
	md := ".abbd/architecture/" + m + ".md"
	raise := func() {
		s := p.read(md)
		i := strings.Index(s, "responsibility:")
		block := s[i:]
		block = strings.Replace(block, "origin: INFERRED", "origin: USER_STATED", 1)
		block = strings.Replace(block, "confirmation: UNCONFIRMED", "confirmation: CONFIRMED", 1)
		p.write(md, s[:i]+block)
	}
	raise()
	p.ok("render")
	for _, line := range strings.Split(p.read(".abbd/BLUEPRINT.md"), "\n") {
		if strings.Contains(line, "`internal/storage`") && strings.Contains(line, "[확정]") {
			t.Fatalf("the blueprint must not show a hand-typed confirmation as confirmed: %s", line)
		}
	}
	p.ok("check", "--accept-edits")
	r := get(p.ok("inspect", m), "module", "responsibility")
	if str(r, "origin") != "INFERRED" || str(r, "confirmation") != "UNCONFIRMED" {
		t.Fatalf("an accept without --user keeps hand-typed provenance down: %v", r)
	}
	raise()
	p.ok("check", "--accept-edits", "--user")
	r = get(p.ok("inspect", m), "module", "responsibility")
	if str(r, "origin") != "USER_STATED" || str(r, "confirmation") != "CONFIRMED" {
		t.Fatalf("the user accepting their own edit keeps it: %v", r)
	}
}

func TestChangesDuringAnAgentSessionAreNotReconciled(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.commit("harness")
	e, _ := workflow.New(p.dir)
	if err := e.NoteSession("start", "s1"); err != nil {
		t.Fatal(err)
	}
	// A Bash edit (sed -i, perl -pi) passes no Edit hook.
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	if _, code, _ := p.run("reconcile"); code == 0 {
		t.Fatal("reconcile must not absorb a change made while an agent session is active")
	}
	if _, code, _ := p.run("sync"); code == 0 {
		t.Fatal("a turnless sync must not absorb it either")
	}
	if plan := p.ok("run", "health 응답 변경"); get(plan, "reconciled") != nil {
		t.Fatalf("the session's change belongs to the turn: %v", get(plan, "reconciled"))
	}
	p.ok("turn", "abort")
	p.ok("reconcile", "--user")
	if err := e.NoteSession("end", "s1"); err != nil {
		t.Fatal(err)
	}
	p.write("README.md", "# heartbeat\n")
	p.ok("reconcile")
}

func TestNotedEditPathsAreNormalised(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.commit("harness")
	e, _ := workflow.New(p.dir)
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	paths := []string{"./internal/kafka/../api/api.go"}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		paths = append(paths, "INTERNAL/API/api.go")
	}
	for _, path := range paths {
		e.NoteEdit(path)
	}
	if _, code, _ := p.run("reconcile"); code == 0 {
		t.Fatal("a non-canonical spelling of an edited path must still be the agent's")
	}
}

func TestRevertedAgentEditDoesNotBlockALaterHumanEdit(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.commit("harness")
	e, _ := workflow.New(p.dir)
	orig := p.read("internal/api/api.go")
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	e.NoteEdit("internal/api/api.go")
	p.write("internal/api/api.go", orig)
	p.ok("status")
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok"))
	_ = 1`)
	if _, code, stderr := p.run("reconcile"); code != 0 {
		t.Fatalf("a reverted agent edit is not pending work: exit %d %s", code, stderr)
	}
}

func TestRejectedOutsideChangeRecordIsKept(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("confirm", post, "--external-consumers", "true", "--user")
	p.commit("harness")
	orig := p.read("internal/api/api.go")
	p.replace("internal/api/api.go", `"POST /messages"`, `"POST /v2/messages"`)
	v, code, _ := p.run("reconcile")
	if code != 2 {
		t.Fatalf("an outside breaking change of a consumed contract waits: exit %d", code)
	}
	blocked := str(v, "change")
	review := p.ok("review", blocked)
	card := ""
	for _, d := range list(review, "deltas") {
		if str(d, "tier") == "C" {
			card = str(d, "id")
		}
	}
	p.ok("review", "--decide", card, "--decision", "reject", "--reason", "소비자 있음", "--user")
	p.write("internal/api/api.go", orig)
	p.write("README.md", "# heartbeat\n")
	next := p.ok("reconcile")
	if str(next, "change") == blocked {
		t.Fatalf("an unrelated outside change must not reuse the rejected record %s", blocked)
	}
	for _, d := range list(p.ok("review", blocked), "deltas") {
		if str(d, "id") == card && str(d, "status") != "rejected" {
			t.Fatalf("the rejected card stays on its record: %v", d)
		}
	}
}

func TestDeletedReproTestIsNotFailingFirst(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "health 응답 버그 고쳐줘")
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/repro_test.go", "package api\n\nimport \"testing\"\n\nfunc TestRepro(t *testing.T) { t.Fatal(\"bug\") }\n")
	if _, code, _ := p.run("verify"); code != 2 {
		t.Fatal("the reproduction fails")
	}
	os.Remove(filepath.Join(p.dir, "internal/api/repro_test.go"))
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	p.write("internal/api/api_test.go", strings.Replace(test, `rec.Body.String() != "ok"`, `rec.Body.String() != "ok!"`, 1))
	p.ok("verify")
	if v, code, _ := p.run("sync"); code != 2 {
		t.Fatalf("a failing test deleted after the repro proves nothing: %v", get(v, "reason"))
	}
}

func TestSameTurnDriverIsNotFailingFirst(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("GET /health")
	p.ok("run", "health 응답 버그 고쳐줘")
	p.ok("driver", "set", feat, "--run", "false", "--expect", "x")
	p.run("verify", "--drive")
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("ok!"))`)
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", strings.Replace(test, `rec.Body.String() != "ok"`, `rec.Body.String() != "ok!"`, 1))
	p.ok("verify")
	if v, code, _ := p.run("sync"); code != 2 {
		t.Fatalf("a driver shaped in this turn to fail reproduces nothing: %v", get(v, "reason"))
	}
}

func TestDriverOfTheSameRequestDoesNotProveT3(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	feat := p.featureFor("POST /messages")
	e, _ := workflow.New(p.dir)
	e.NotePrompt("POST /messages 재시도 시 중복 발행 방지")
	// Registered before the turn opens, in answer to the same user message.
	p.ok("driver", "set", feat, "--run", "go test ./internal/api -run TestPostMessagePublishes -v", "--expect", "PASS: TestPostMessagePublishes")
	p.ok("run", "POST /messages 재시도 시 중복 발행 방지")
	p.replace("internal/api/api.go", "	w.WriteHeader(http.StatusAccepted)\n}", "	w.WriteHeader(http.StatusAccepted)\n}\n\nfunc dedupeKey(k string) string { return k }")
	p.ok("verify")
	p.ok("verify", "--drive")
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatal("a driver set for the same request must not satisfy its T3 gate")
	}
}

func TestRouteNamedOnlyInACommentIsUntested(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("intent", "rule", "--claim", "모든 응답은 1 KiB 이하여야 한다")
	p.commit("harness")
	p.replace("internal/api/api.go", `s.mux.HandleFunc("GET /health", s.handleHealth)`, `s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ping", s.handleHealth)`)
	p.write("internal/api/api_test.go", p.read("internal/api/api_test.go")+"\n// GET /ping is covered elsewhere.\n")
	v := p.ok("reconcile")
	risks := strings.Join(strs(p.ok("inspect", str(v, "change")), "risks"), " ")
	if !strings.Contains(risks, "/ping") {
		t.Fatalf("a route an outside change added, named only in a comment, is untested: %v", risks)
	}
	p.ok("run", "GET /pong 경로 추가")
	p.replace("internal/api/api.go", `s.mux.HandleFunc("GET /ping", s.handleHealth)`, `s.mux.HandleFunc("GET /ping", s.handleHealth)
	s.mux.HandleFunc("GET /pong", s.handleHealth)`)
	p.write("internal/api/api_test.go", p.read("internal/api/api_test.go")+"\n// GET /pong too.\n")
	p.ok("verify")
	s, _, _ := p.run("sync")
	if risks := strings.Join(strs(s, "receipt", "risks"), " "); !strings.Contains(risks, "/pong") {
		t.Fatalf("a test comment is not a test of the route: %v", risks)
	}
}

func TestHandApprovedTierCCardStaysPending(t *testing.T) {
	p, post := breakingSetup(t)
	v, _, _ := p.run("sync")
	card := pendingCard(t, v, post)
	md := ".abbd/changes/" + strings.Split(card, "#")[0] + ".md"
	s := p.read(md)
	i := strings.Index(s, "id: "+card)
	j := i + strings.Index(s[i:], "status: pending")
	indent := s[strings.LastIndex(s[:j], "\n")+1 : j]
	approval := "status: approved\n" + indent + "decision:\n" + indent + "    decision: approve\n" + indent + "    at: x\n" + indent + "    revision: x\n" + indent + "    channel: serve"
	p.write(md, s[:j]+approval+s[j+len("status: pending"):])
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatal("a tier C card approved by editing the md is not approved")
	}
}
