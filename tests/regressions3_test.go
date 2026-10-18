package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Defects the final independent review reproduced.

func breakingSetup(t *testing.T) (*project, string) {
	t.Helper()
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("confirm", post, "--external-consumers", "true", "--user")
	p.commit("harness")
	p.ok("run", "POST /messages 경로 변경")
	p.replace("internal/api/api.go", `"POST /messages"`, `"POST /v2/messages"`)
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", strings.ReplaceAll(test, `"/messages", strings.NewReader`, `"/v2/messages", strings.NewReader`))
	p.ok("verify")
	return p, post
}

func pendingCard(t *testing.T, v map[string]any, subject string) string {
	t.Helper()
	for _, d := range list(v, "review", "deltas") {
		if str(d, "subject") == subject && str(d, "tier") == "C" {
			return str(d, "id")
		}
	}
	t.Fatalf("no C card for %s: %v", subject, get(v, "review"))
	return ""
}

func TestApprovalDoesNotCoverALaterDifferentChange(t *testing.T) {
	p, post := breakingSetup(t)
	v, _, _ := p.run("sync")
	p.ok("review", "--decide", pendingCard(t, v, post), "--decision", "approve", "--user")
	// Same turn, same contract, a different breaking change than the one approved.
	p.replace("internal/api/api.go", `"POST /v2/messages"`, `"DELETE /v3/messages"`)
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", strings.ReplaceAll(strings.ReplaceAll(test, `"/v2/messages"`, `"/v3/messages"`), "http.MethodPost, \"/v3", "http.MethodDelete, \"/v3"))
	p.ok("verify")
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatal("approving v2 must not authorize v3: sync must stop on a new C card")
	}
	if sel := str(p.ok("inspect", post), "contract", "selector"); sel != "POST /messages" {
		t.Fatalf("the blueprint must not take v3 before its own approval, selector %q", sel)
	}
}

func TestRejectedTierCChangeIsRevertedByTheNextTurn(t *testing.T) {
	p, post := breakingSetup(t)
	v, _, _ := p.run("sync")
	p.ok("review", "--decide", pendingCard(t, v, post), "--decision", "reject", "--reason", "v1 소비자가 아직 있음", "--user")
	next := p.ok("run", "거부된 변경 처리")
	if !strings.Contains(strings.Join(strs(next, "rework"), " "), "v1 소비자가 아직 있음") {
		t.Fatalf("the rejection reason must reach the next turn even though the code is still in the tree: %v", get(next, "rework"))
	}
}

func TestReviewPageIsDeterministicWithManyDeployUnits(t *testing.T) {
	p := newProject(t, "code-only")
	for _, u := range []string{"alpha", "beta", "gamma"} {
		p.write("Dockerfile."+u, "FROM golang\nRUN go build ./cmd/heartbeat-worker\n")
	}
	p.commit("units")
	p.ok("init")
	first := p.read(".abbd/generated/review.html")
	if strings.Count(first, `data-node="unit:`) < 4 {
		t.Fatal("setup must produce several deploy units")
	}
	for i := 0; i < 6; i++ {
		p.ok("render")
		if p.read(".abbd/generated/review.html") != first {
			t.Fatal("review.html must be identical across renders of the same blueprint (FR-30)")
		}
	}
}

func TestCraftedEnumValueIsEscapedAndFailsCheck(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "문서 보강")
	p.write("README.md", "# x\n")
	p.ok("sync")
	files, _ := filepath.Glob(filepath.Join(p.dir, ".abbd", "changes", "CHG-*.md"))
	b, _ := os.ReadFile(files[0])
	os.WriteFile(files[0], []byte(strings.Replace(string(b), "state: COMPLETED", "state: COMPLETED<img src=x onerror=alert(1)>", 1)), 0o644)
	if _, code, _ := p.run("check"); code != 2 {
		t.Fatal("an invalid state must fail check")
	}
	p.ok("render")
	if strings.Contains(p.read(".abbd/generated/review.html"), "<img src=x") {
		t.Fatal("SSOT values must be escaped in review.html")
	}
}
