package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gfMain = `package main

import (
	"io"
	"net/http"
	"os"
)

var received []string

func handleMessage(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	if len(b) == 0 {
		http.Error(w, "empty", http.StatusBadRequest)
		return
	}
	received = append(received, string(b))
	w.WriteHeader(http.StatusAccepted)
}

func routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /messages", handleMessage)
	return mux
}

func main() {
	http.ListenAndServe(os.Getenv("LISTEN_ADDR"), routes())
}
`

const gfTest = `package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReceiveMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader("hi")))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d", rec.Code)
	}
}
`

// TestGreenfieldE2E is the second E2E scenario of PRD ch.12 and AC-BOOT-01: an empty
// directory and a short request go through SSOT draft -> code -> test -> docs and a next turn.
func TestGreenfieldE2E(t *testing.T) {
	p := newProject(t, "")

	boot := p.ok("init")
	if str(boot, "mode") != "greenfield" || get(boot, "questions_asked") != float64(0) {
		t.Fatalf("empty dir must bootstrap greenfield without questions: %v", boot)
	}
	if _, err := os.Stat(filepath.Join(p.dir, ".myharness", "generated", "explorer.html")); err != nil {
		t.Fatalf("AC-BOOT-04: explorer must exist without any input: %v", err)
	}

	plan := p.ok("run", "메시지 수신 API와 기본 테스트를 만들어줘")
	feat := str(plan, "new_feature")
	f := p.ok("inspect", feat)
	if str(f, "feature", "status") != "draft" || str(f, "feature", "intent", "purpose", "origin") != "USER_STATED" {
		t.Fatalf("the request becomes a USER_STATED draft feature: %v", get(f, "feature"))
	}
	if str(p.ok("inspect", str(plan, "requirement")), "statement", "origin") != "USER_STATED" {
		t.Fatal("the request becomes a USER_STATED requirement")
	}

	p.write("go.mod", "module example.com/inbox\n\ngo 1.22\n")
	p.write("main.go", gfMain)
	p.write("main_test.go", gfTest)
	ev := p.ok("verify")
	if str(ev, "overall") != "PASSED" || len(strs(ev, "ran")) == 0 {
		t.Fatalf("greenfield verify must discover and run the new tests: %v", ev)
	}
	sync := p.ok("sync")
	if get(sync, "completed") != true {
		t.Fatalf("first greenfield turn completes: %v", get(sync, "reason"))
	}
	if got := p.featureFor("POST /messages"); got != feat {
		t.Fatalf("delivered feature %s must own the new route, got %s", feat, got)
	}
	f = p.ok("inspect", feat)
	if str(f, "feature", "status") != "current" || len(list(f, "modules")) == 0 || len(list(f, "tests")) == 0 {
		t.Fatalf("delivered feature links module and test: %v", f)
	}
	p.assertAllCurrent("after first greenfield turn")
	p.commit("turn 1")

	// The next turn builds on the knowledge the first one produced.
	plan = p.ok("run", "GET /health 헬스 체크 엔드포인트 추가")
	p.replace("main.go", `	mux.HandleFunc("POST /messages", handleMessage)`, `	mux.HandleFunc("POST /messages", handleMessage)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })`)
	p.write("health_test.go", strings.Replace(gfTest, "TestReceiveMessage", "TestHealth", 1))
	p.ok("verify")
	if sync = p.ok("sync"); get(sync, "completed") != true {
		t.Fatalf("second turn completes: %v", get(sync, "reason"))
	}
	if p.featureFor("GET /health") != str(plan, "new_feature") || p.featureFor("POST /messages") != feat {
		t.Fatal("second turn must deliver its own feature and keep the first")
	}
	p.assertAllCurrent("after second greenfield turn")
}

// TestAdoptMode covers FR-17 and AC-BOOT-05: a legacy document is reconstructed into the
// standard structure as ADOPTED, and a broken adopt input never blocks the base bootstrap.
func TestAdoptMode(t *testing.T) {
	p := newProject(t, "code-only")
	missing := p.ok("init", "--mode", "adopt", "--from", filepath.Join(p.dir, "nope.html"))
	if len(strs(missing, "adopt", "errors")) == 0 || get(missing, "counts", "modules").(float64) == 0 {
		t.Fatalf("AC-BOOT-05: missing adopt input reported, bootstrap still built: %v", missing)
	}

	q := newProject(t, "code-only")
	q.write("legacy/architecture.md", "| 모듈 | 경로 | 책임 |\n|---|---|---|\n| storage | internal/storage | heartbeat 레코드를 저장한다 |\n| ghost | internal/ghost | 존재하지 않는 모듈 |\n")
	preview := q.ok("init", "--mode", "adopt", "--from", filepath.Join(q.dir, "legacy"), "--preview")
	if _, err := os.Stat(filepath.Join(q.dir, ".myharness")); err == nil {
		t.Fatal("--preview must not write .myharness")
	}
	if len(strs(preview, "adopt", "applied")) != 1 {
		t.Fatalf("preview reports what adopt would apply: %v", get(preview, "adopt"))
	}
	rep := q.ok("init", "--mode", "adopt", "--from", filepath.Join(q.dir, "legacy"))
	if !has(strs(rep, "adopt", "unmatched"), "ghost") {
		t.Fatalf("adopt reports entries it could not match: %v", get(rep, "adopt"))
	}
	m := q.ok("inspect", q.moduleAt("internal/storage"))
	if str(m, "module", "responsibility", "origin") != "ADOPTED" || str(m, "module", "responsibility", "value") != "heartbeat 레코드를 저장한다" {
		t.Fatalf("adopted responsibility: %v", get(m, "module", "responsibility"))
	}
	q.assertAllCurrent("after adopt")
}
