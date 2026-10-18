package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const countRoute = `	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /messages/count", s.handleCount)`

const countHandler = `
func (s *Server) handleCount(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(map[string]int{"count": s.count})
}
`

const countTest = `package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMessageCount(t *testing.T) {
	s := NewServer(NewKafkaWriter(nil))
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(` + "`" + `{"body":"x"}` + "`" + `)))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/messages/count", nil))
	if !strings.Contains(rec.Body.String(), ` + "`" + `"count":BROKEN` + "`" + `) {
		t.Fatalf("got %s", rec.Body.String())
	}
}
`

// TestCodeOnlyE2E is the first E2E acceptance scenario of PRD ch.12 (the Phase 1 gate).
func TestCodeOnlyE2E(t *testing.T) {
	p := newProject(t, "code-only")

	// 1-3. init on a repo with code and tests but no maps: candidates with provenance, no questions.
	boot := p.ok("init", "--mode", "code-only", "--verify-available")
	if str(boot, "mode") != "code-only" || get(boot, "questions_asked") != float64(0) {
		t.Fatalf("bootstrap mode/questions: %v %v", get(boot, "mode"), get(boot, "questions_asked"))
	}
	if n := get(boot, "unconfirmed_count").(float64); n == 0 || int(n) != len(list(boot, "unconfirmed")) {
		t.Fatalf("AC-BOOT-06: unconfirmed_count %v vs list %d", n, len(list(boot, "unconfirmed")))
	}
	queue := p.read(".abbd/generated/confirm-queue.html")
	for _, item := range list(boot, "unconfirmed") {
		if !strings.Contains(queue, str(item, "id")) {
			t.Fatalf("AC-BOOT-06: %s missing from the HTML confirm queue", str(item, "id"))
		}
	}
	explorer := p.read(".abbd/generated/explorer.html")
	for _, want := range []string{"fmh-watermark", "확인 대기", "Architecture View", "외부 계약 뷰", "Feature View", "heartbeat-events", "POST /messages"} {
		if !strings.Contains(explorer, want) {
			t.Fatalf("AC-BOOT-04: explorer.html lacks %q", want)
		}
	}
	postFeat := p.featureFor("POST /messages")
	f := p.ok("inspect", postFeat)
	if str(f, "feature", "intent", "purpose", "origin") != "INFERRED" || str(f, "feature", "intent", "purpose", "confirmation") != "UNCONFIRMED" {
		t.Fatalf("AC-BOOT-02: scanned purpose must be INFERRED+UNCONFIRMED, got %v", get(f, "feature", "intent", "purpose"))
	}
	topic := p.ok("inspect", p.findContract("heartbeat-events"))
	if str(topic, "contract", "origin") != "EXTRACTED" || str(topic, "contract", "verification") != "UNVERIFIABLE" || str(topic, "contract", "verification_note") == "" {
		t.Fatalf("08A: a broker topic is EXTRACTED and UNVERIFIABLE with a reason: %v", get(topic, "contract"))
	}
	p.assertAllCurrent("after bootstrap")

	// AC-BOOT-03: a re-run keeps stable IDs and confirmed intent, and creates no duplicates.
	p.ok("confirm", postFeat, "--value", "메시지를 받아 message-events 토픽에 발행한다", "--user")
	again := p.ok("bootstrap", "--mode", "code-only")
	if n := len(list(again, "knowledge", "new")); n != 0 {
		t.Fatalf("AC-BOOT-03: re-run created %d new objects: %v", n, get(again, "knowledge", "new"))
	}
	f = p.ok("inspect", postFeat)
	if str(f, "feature", "intent", "purpose", "origin") != "USER_STATED" || str(f, "feature", "intent", "purpose", "confirmation") != "CONFIRMED" {
		t.Fatalf("AC-BOOT-03: confirmed intent lost on re-run: %v", get(f, "feature", "intent", "purpose"))
	}
	p.commit("harness bootstrap")

	// 4. A development turn: the request becomes intent, the code change is verified, maps follow.
	plan := p.ok("run", "메시지 개수를 돌려주는 GET /messages/count API 추가")
	chg, newFeat := str(plan, "change"), str(plan, "new_feature")
	if chg == "" || newFeat == "" {
		t.Fatalf("run must open a CHG and a draft feature: %v", plan)
	}
	p.replace("internal/api/api.go", "	mux    *http.ServeMux\n", "	mux    *http.ServeMux\n	count  int\n")
	p.replace("internal/api/api.go", `	s.mux.HandleFunc("GET /health", s.handleHealth)`, countRoute)
	p.replace("internal/api/api.go", "	w.WriteHeader(http.StatusAccepted)\n}\n", "	s.count++\n	w.WriteHeader(http.StatusAccepted)\n}\n"+countHandler)
	p.write("internal/api/count_test.go", countTest)

	// A failing verification never completes and never overwrites the confirmed contract.
	ev, code, _ := p.run("verify")
	if code != 2 || str(ev, "overall") != "FAILED" {
		t.Fatalf("broken test must FAIL verification with exit 2: exit %d %v", code, get(ev, "ran"))
	}
	if f := list(ev, "failures"); len(f) != 1 || !strings.Contains(str(f[0], "output"), `got {"count":1}`) {
		t.Fatalf("the report must carry the failing test's own output, not just its name: %v", f)
	}
	if _, code, _ := p.run("sync"); code != 2 {
		t.Fatalf("sync after FAILED verification exited %d, want 2 (gate)", code)
	}
	st := p.ok("status")
	if str(st, "freshness", "feature_map") != "DRAFT" || str(st, "freshness", "html") != "DRAFT" {
		t.Fatalf("a failed turn shows DRAFT, got %v", get(st, "freshness"))
	}
	if !strings.Contains(p.read(".abbd/generated/explorer.html"), "작업 중 초안 DRAFT") {
		t.Fatal("HTML must distinguish the draft from the last confirmed version")
	}
	if str(p.ok("inspect", postFeat), "feature", "intent", "purpose", "value") != "메시지를 받아 message-events 토픽에 발행한다" {
		t.Fatal("failed turn changed a confirmed purpose")
	}

	p.replace("internal/api/count_test.go", `"count":BROKEN`, `"count":1`)
	if ev = p.ok("verify"); str(ev, "overall") != "PASSED" {
		t.Fatalf("fixed change must PASS: %v", get(ev, "ran"))
	}
	// Editing after verification invalidates the evidence: the gate is keyed on the workspace.
	p.write("internal/api/doc.go", "// Package api serves the message API.\npackage api\n")
	if v, code, _ := p.run("sync"); code != 2 || !strings.Contains(str(v, "reason"), "다시") {
		t.Fatalf("sync after an unverified edit exited %d (%v), want 2", code, get(v, "reason"))
	}
	p.ok("verify")
	p.ok("turn", "note", "--model-calls", "4")
	sync := p.ok("sync")
	// v1.4: a new external contract is a tier B structural change, so the turn completes with
	// its default applied and waits for a decision (NEEDS_USER).
	if get(sync, "completed") != true || str(sync, "receipt", "state") != "NEEDS_USER" || get(sync, "review", "by_tier", "B") == nil {
		t.Fatalf("sync must complete with an open B card: %v %v %v", get(sync, "reason"), str(sync, "receipt", "state"), get(sync, "review"))
	}
	receipt := get(sync, "receipt")
	if !has(strs(receipt, "delivers"), newFeat) || get(receipt, "telemetry", "test_runs").(float64) < 2 || get(receipt, "telemetry", "model_calls") != float64(4) {
		t.Fatalf("receipt lacks delivery or telemetry: %v", receipt)
	}
	if get(receipt, "intent_fidelity", "covered_by_passed_tests") != float64(1) {
		t.Fatalf("intent fidelity proxy: %v", get(receipt, "intent_fidelity"))
	}
	if got := p.featureFor("GET /messages/count"); got != newFeat {
		t.Fatalf("delivered feature %s must own GET /messages/count, owned by %s", newFeat, got)
	}
	if p.featureFor("GET /health") == newFeat {
		t.Fatal("delivered feature stole an existing route")
	}
	nf := p.ok("inspect", newFeat)
	if str(nf, "feature", "status") != "current" || str(nf, "feature", "intent", "purpose", "origin") != "USER_STATED" {
		t.Fatalf("delivered feature: %v", get(nf, "feature"))
	}
	p.assertAllCurrent("after the turn")
	trace := p.ok("graph", "trace", "--from", newFeat, "--to", p.findContract("GET /messages/count"))
	if get(trace, "found") != true || str(trace, "freshness") != "CURRENT" {
		t.Fatalf("Feature -> Interface trace: %v", trace)
	}
	p.commit("CHG: count api")

	// 6. Another session re-opens the project and reuses implementation, intent and evidence.
	other := &project{t: t, dir: p.dir}
	ctx := other.ok("inspect", newFeat)
	if len(list(ctx, "feature", "verification")) == 0 || len(list(ctx, "changes")) == 0 {
		t.Fatalf("a new session must see the verification evidence: %v", ctx)
	}

	// 7. An external commit moves a file: reconcile without questions, IDs survive the move.
	storage := p.moduleAt("internal/storage")
	table := p.findContract("heartbeats")
	if err := os.Rename(filepath.Join(p.dir, "internal/storage/storage.go"), filepath.Join(p.dir, "internal/storage/store.go")); err != nil {
		t.Fatal(err)
	}
	p.commit("external: rename storage.go")
	st = p.ok("status")
	if get(st, "external_change") != true || str(st, "freshness", "feature_map") != "STALE" {
		t.Fatalf("external change must be detected as STALE: %v", st)
	}
	rec := p.ok("reconcile")
	if get(rec, "questions_asked") != float64(0) || str(rec, "state") != "COMPLETED" {
		t.Fatalf("reconcile asks nothing and completes: %v", rec)
	}
	if ch := p.ok("inspect", str(rec, "change")); str(ch, "intent_source") != "external" {
		t.Fatalf("reconcile CHG intent_source: %v", get(ch, "intent_source"))
	}
	if p.findContract("heartbeats") != table || p.moduleAt("internal/storage") != storage {
		t.Fatal("stable IDs must survive a file move")
	}
	p.assertAllCurrent("after reconcile")

	// External removal of a route whose purpose the user confirmed: tier B record, no stop.
	health := p.featureFor("GET /health")
	p.ok("confirm", health, "--value", "헬스 체크", "--user")
	p.replace("internal/api/api.go", "	s.mux.HandleFunc(\"GET /health\", s.handleHealth)\n", "")
	test := p.read("internal/api/api_test.go")
	p.write("internal/api/api_test.go", test[:strings.Index(test, "func TestHealth")])
	p.commit("external: drop health")
	rec = p.ok("reconcile")
	if str(rec, "state") != "NEEDS_USER" || len(list(rec, "conflicts")) == 0 {
		t.Fatalf("conflict with a CONFIRMED feature must be NEEDS_USER: %v", rec)
	}
	if n := len(list(p.ok("decisions"), "list")); n == 0 {
		t.Fatal("conflict must be recorded as an ADR")
	}
	p.assertAllCurrent("after conflicting reconcile")

	// Tier C: a breaking change to an interface with external consumers stops until approved.
	post := p.findContract("POST /messages")
	p.ok("confirm", post, "--external-consumers", "true", "--user")
	p.ok("run", "POST /messages 경로를 /v2/messages 로 변경")
	p.replace("internal/api/api.go", `"POST /messages"`, `"POST /v2/messages"`)
	p.replace("internal/api/api_test.go", `"/messages", strings.NewReader(`+"`"+`{"key"`, `"/v2/messages", strings.NewReader(`+"`"+`{"key"`)
	p.replace("internal/api/api_test.go", `"/messages", strings.NewReader(`+"`{}`", `"/v2/messages", strings.NewReader(`+"`{}`")
	p.replace("internal/api/count_test.go", `"/messages", strings`, `"/v2/messages", strings`)
	if ev = p.ok("verify"); str(ev, "overall") != "PASSED" {
		t.Fatalf("v2 change must pass: %v %v", get(ev, "failed_tests"), str(list(ev, "runs")[len(list(ev, "runs"))-1], "output_tail"))
	}
	ci := p.ok("contracts", "impact", "--diff")
	if str(ci, "highest_tier") != "C" {
		t.Fatalf("breaking external contract must be tier C: %v", ci)
	}
	v, code, _ := p.run("sync")
	if code != 2 || !strings.Contains(str(v, "reason"), "등급 C") {
		t.Fatalf("tier C must block sync: exit %d %v", code, get(v, "reason"))
	}
	p.ok("decide", "--tier", "C", "--question", "POST /messages breaking change 승인?", "--subject", post, "--answer", "approve", "--user")
	p.ok("verify")
	if sync = p.ok("sync"); get(sync, "completed") != true {
		t.Fatalf("approved tier C change completes: %v", get(sync, "reason"))
	}
	p.assertAllCurrent("after approved tier C change")
}

// moduleAt returns the live module ID at a path.
func (p *project) moduleAt(path string) string {
	p.t.Helper()
	for i := 1; ; i++ {
		id := fmt.Sprintf("MOD-%03d", i)
		v, code, _ := p.run("inspect", id)
		if code != 0 {
			break
		}
		if str(v, "module", "path") == path && str(v, "module", "status") != "removed" {
			return id
		}
	}
	p.t.Fatalf("no module at %s", path)
	return ""
}
