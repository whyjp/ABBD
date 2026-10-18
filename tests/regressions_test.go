package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each test here reproduces a defect an independent review or a skill author found, and fails
// if it comes back.

func TestAbortedNewFeatureTurnDoesNotPoisonLaterSyncs(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "GET /stats 통계 API 추가")
	p.ok("turn", "abort", "--reason", "changed my mind")
	if chk := p.ok("check"); get(chk, "ok") != true {
		t.Fatalf("abort left dangling references: %v", get(chk, "errors"))
	}
	p.ok("run", "README 문서 보강")
	p.write("README.md", "# heartbeat\n")
	if s := p.ok("sync"); get(s, "completed") != true {
		t.Fatalf("a later turn must complete: %v", get(s, "reason"))
	}
}

func TestEmptyChangeSetNeverClaimsPassed(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "heartbeat 처리 성능 개선")
	s := p.ok("sync")
	if str(s, "receipt", "verification") == "PASSED" {
		t.Fatal("a turn that changed nothing must not record PASSED verification without evidence")
	}
	if chk := p.ok("check"); get(chk, "ok") != true {
		t.Fatalf("check: %v", get(chk, "errors"))
	}
}

func TestBootstrapRefusesToResettleUnderAnActiveTurn(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "POST /messages 응답 형식 변경")
	p.replace("internal/api/api.go", "w.WriteHeader(http.StatusAccepted)", "w.WriteHeader(undefinedSymbol)")
	if _, code, _ := p.run("bootstrap", "--mode", "code-only"); code == 0 {
		t.Fatal("bootstrap mid-turn must refuse: it would move the watermark under unverified code")
	}
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatalf("broken unverified code must not sync, exit %d", code)
	}
}

func TestFailingRouteTestNeverMarksItsContractPassed(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.replace("internal/api/api.go", `w.Write([]byte("ok"))`, `w.Write([]byte("nope"))`)
	if _, code, _ := p.run("verify", "--all"); code != 2 {
		t.Fatalf("TestHealth must fail, exit %d", code)
	}
	c := p.ok("inspect", p.findContract("GET /health"))
	if str(c, "contract", "verification") == "PASSED" {
		t.Fatalf("a sibling test passing must not prove GET /health: %v", get(c, "contract"))
	}
}

func TestFailedPublishLeavesNoCompletedReceipt(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	plan := p.ok("run", "README 문서 보강")
	p.write("README.md", "# heartbeat\n")
	staging := filepath.Join(p.dir, ".cccv", "generated", "explorer.html.staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, code, _ := p.run("sync"); code == 0 {
		t.Fatal("sync must fail when the HTML cannot be written")
	}
	chg := p.ok("inspect", str(plan, "change"))
	if str(chg, "state") == "COMPLETED" || str(chg, "sync_state", "html") == "CURRENT" {
		t.Fatalf("receipt claims completion after a failed publish: %v %v", get(chg, "state"), get(chg, "sync_state"))
	}
	if str(p.ok("status"), "active_turn", "id") != str(plan, "change") {
		t.Fatal("the turn must stay active after a failed publish")
	}
	os.RemoveAll(staging)
	if s := p.ok("sync"); get(s, "completed") != true {
		t.Fatalf("retry completes: %v", get(s, "reason"))
	}
}

func TestOneSharedAnchorNeverMovesConfirmedMeaning(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	health := p.findContract("GET /health")
	p.ok("confirm", health, "--external-consumers", "true")
	p.ok("confirm", health, "--value", "backward-compatible, 외부 모니터링이 호출")
	p.commit("harness")
	p.replace("internal/api/api.go", `	s.mux.HandleFunc("GET /health", s.handleHealth)`, `	s.mux.HandleFunc("GET /stats", s.handleStats)`)
	p.replace("internal/api/api.go", "func (s *Server) handleHealth(", "func (s *Server) handleStats(")
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", test[:strings.Index(test, "func TestHealth")])
	p.commit("external: health -> stats")
	p.ok("reconcile")
	stats := p.ok("inspect", p.findContract("GET /stats"))
	if str(stats, "contract", "id") == health || get(stats, "contract", "external_consumers") == true {
		t.Fatalf("a new route inherited a confirmed contract on one anchor: %v", get(stats, "contract"))
	}
	pending := p.ok("confirm", "--list")
	var cand string
	for _, it := range list(pending, "list") {
		if str(it, "kind") == "candidate" && strings.Contains(str(it, "value"), health) {
			cand = str(it, "id")
		}
	}
	if cand == "" {
		t.Fatalf("the one-anchor match must be presented as a merge candidate: %v", pending)
	}
	p.ok("confirm", "--merge", cand)
	merged := p.ok("inspect", health)
	if str(merged, "contract", "selector") != "GET /stats" || get(merged, "contract", "external_consumers") != true {
		t.Fatalf("merge keeps the old ID and its confirmed attributes: %v", get(merged, "contract"))
	}
	if chk := p.ok("check"); get(chk, "ok") != true {
		t.Fatalf("check after merge: %v", get(chk, "errors"))
	}
}

func TestNestedGoModuleCanBeVerified(t *testing.T) {
	p := newProject(t, "")
	p.write("services/svc/go.mod", "module example.com/svc\n\ngo 1.22\n")
	p.write("services/svc/pkg/pkg.go", "package pkg\n\nfunc Add(a, b int) int { return a + b }\n")
	p.write("services/svc/pkg/pkg_test.go", "package pkg\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal()\n\t}\n}\n")
	p.commit("v0")
	p.ok("init")
	p.ok("run", "pkg 버그 고쳐줘")
	p.replace("services/svc/pkg/pkg.go", "a + b", "b + a")
	if ev := p.ok("verify"); str(ev, "overall") != "PASSED" || len(strs(ev, "could_not_run")) > 0 {
		t.Fatalf("nested module verification: %v / %v", get(ev, "ran"), get(ev, "could_not_run"))
	}
}

func TestConfirmedMeaningIsNotOverwrittenByTheAgent(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	feat := p.featureFor("POST /messages")
	p.ok("run", "POST /messages 응답 형식 변경")
	p.ok("confirm", post, "--value", "backward-compatible")
	p.ok("confirm", feat, "--value", "메시지를 받아 발행한다")
	if _, code, _ := p.run("intent", "contract", "--id", post, "--compatibility", "breaking allowed"); code == 0 {
		t.Fatal("an INFERRED write must not replace a CONFIRMED compatibility")
	}
	if _, code, _ := p.run("intent", "feature", "--id", feat, "--purpose", "다른 목적"); code == 0 {
		t.Fatal("an INFERRED write must not replace a purpose the user confirmed in this same turn")
	}
	if _, code, _ := p.run("intent", "contract", "--id", post, "--external-consumers", "true", "--origin", "EXTRACTED"); code == 0 {
		t.Fatal("contract attributes cannot be EXTRACTED")
	}
}

func TestOpenDecisionIsAnsweredByID(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "POST /messages 응답 형식 변경")
	d := p.ok("decide", "--question", "응답 코드를 바꿀까?", "--option", "keep=유지", "--option", "change=201로")
	adr := str(d, "decision", "id")
	p.ok("turn", "abort")
	a := p.ok("decide", "--adr", adr, "--answer", "change")
	if str(a, "decision", "chosen") != "change" || str(a, "decision", "confirmation") != "CONFIRMED" {
		t.Fatalf("answer by ID outside a turn: %v", get(a, "decision"))
	}
	if _, code, _ := p.run("decide", "--adr", adr, "--answer", "bogus"); code == 0 {
		t.Fatal("an answer that is not an option is refused")
	}
}

func TestLegacyDirectoryIsMovedOnceWithANotice(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	if err := os.Rename(filepath.Join(p.dir, ".cccv"), filepath.Join(p.dir, ".myharness")); err != nil {
		t.Fatal(err)
	}
	_, code, stderr := p.run("status")
	if code != 0 || !strings.Contains(stderr, "moved .myharness/ to .cccv/") {
		t.Fatalf("first run moves and notices: exit %d %q", code, stderr)
	}
	if _, _, stderr = p.run("status"); strings.Contains(stderr, "moved") {
		t.Fatal("the notice is shown once")
	}
	p.assertAllCurrent("after migration")
}
