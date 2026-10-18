package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/serve"
	"github.com/whyjp/abbd/internal/workflow"
)

func deltaKinds(v any) []string {
	var out []string
	for _, d := range list(v, "receipt", "structural_deltas") {
		out = append(out, str(d, "kind"))
	}
	return out
}

func deltaOf(v any, kind string) map[string]any {
	for _, d := range list(v, "receipt", "structural_deltas") {
		if str(d, "kind") == kind {
			return d.(map[string]any)
		}
	}
	return nil
}

func layoutCells(t *testing.T, p *project, view string) map[string]any {
	t.Helper()
	var l map[string]any
	if err := json.Unmarshal([]byte(p.read(".abbd/generated/layout.json")), &l); err != nil {
		t.Fatal(err)
	}
	m, _ := get(l, "views", view, "cells").(map[string]any)
	return m
}

// AC-REV-01: right after a code-only bootstrap the topology opens, and inferred items are
// told apart by dashed outline and badge.
func TestACREV01TopologyAfterBootstrap(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init", "--mode", "code-only")
	page := p.read(".abbd/generated/review.html")
	for _, want := range []string{`data-view="system"`, `data-view="module"`, `data-view="feature"`, "<svg", "구조 변경 0건"} {
		if !strings.Contains(page, want) {
			t.Fatalf("review.html lacks %q", want)
		}
	}
	inferred := regexp.MustCompile(`<g class="node module inf[^"]*" data-node="MOD-\d+"`)
	if !inferred.MatchString(page) || !strings.Contains(page, ">추정</text>") {
		t.Fatal("inferred modules must be dashed (class inf) with a 추정 badge")
	}
	if len(layoutCells(t, p, "module")) == 0 {
		t.Fatal("bootstrap must persist the layout")
	}
}

// AC-REV-02: a turn that adds one module dependency shows only that edge green, and no other
// node moves.
func TestACREV02OneDependencyEdgeGreenLayoutStable(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.commit("harness")
	before := layoutCells(t, p, "module")
	api, storage := p.moduleAt("internal/api"), p.moduleAt("internal/storage")

	p.ok("run", "api 핸들러 정리 리팩터")
	p.replace("internal/api/api.go", `	"example.com/heartbeat/internal/kafka"`, `	"example.com/heartbeat/internal/kafka"
	"example.com/heartbeat/internal/storage"`)
	p.write("internal/api/store.go", "package api\n\nimport \"example.com/heartbeat/internal/storage\"\n\nfunc newStore() *storage.Writer { return storage.NewWriter(\"\") }\n")
	p.replace("internal/api/api.go", `	"example.com/heartbeat/internal/storage"
`, "")
	p.ok("verify")
	sync := p.ok("sync")
	if kinds := deltaKinds(sync); len(kinds) != 1 || kinds[0] != "dependency_added" {
		t.Fatalf("exactly one structural change expected, got %v", kinds)
	}
	if d := deltaOf(sync, "dependency_added"); str(d, "tier") != "A" || str(d, "subject") != api+"->"+storage {
		t.Fatalf("dependency_added is tier A on %s->%s: %v", api, storage, d)
	}
	after := layoutCells(t, p, "module")
	for id, cell := range before {
		if a, _ := json.Marshal(after[id]); string(a) != mustJSON(cell) {
			t.Fatalf("node %s moved: %v -> %v", id, cell, after[id])
		}
	}
	page := p.read(".abbd/generated/review.html")
	green := regexp.MustCompile(`<line class="edge k-depends[^"]* add[^"]*" data-from="` + api + `" data-to="` + storage + `"`)
	if !green.MatchString(page) {
		t.Fatal("the new dependency edge must be drawn green (class add)")
	}
	if n := len(regexp.MustCompile(`<line class="edge k-depends[^"]* add[" ]`).FindAllString(page, -1)); n != 1 {
		t.Fatalf("only one edge may be green, found %d", n)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// AC-REV-03: changing an external contract's compatibility yields a tier B card; rejecting it
// hands the reason to the next turn's IntentDelta.
func TestACREV03CompatChangeBCardRejectFeedsNextTurn(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("run", "POST /messages 응답 형식 변경")
	p.ok("intent", "contract", "--id", post, "--compatibility", "v2 응답 형식만 지원")
	sync := p.ok("sync")
	card := deltaOf(sync, "contract_compat_changed")
	if card == nil || str(card, "tier") != "B" || str(card, "status") != "default_applied" {
		t.Fatalf("compat change must be a B card applied by default: %v", get(sync, "receipt", "structural_deltas"))
	}
	if get(sync, "review") == nil {
		t.Fatal("a B card must produce a review notice")
	}
	if _, code, _ := p.run("review", "--decide", str(card, "id"), "--decision", "reject", "--user"); code == 0 {
		t.Fatal("a rejection without a reason is refused")
	}
	p.ok("review", "--decide", str(card, "id"), "--decision", "reject", "--reason", "구버전 클라이언트 호환 유지", "--user")
	next := p.ok("run", "응답 형식 후속 작업")
	rework := strings.Join(strs(next, "rework"), " ")
	if !strings.Contains(rework, "구버전 클라이언트 호환 유지") || !strings.Contains(rework, str(card, "id")) {
		t.Fatalf("the next turn must receive the rejection reason: %v", get(next, "rework"))
	}
	chg := p.ok("inspect", str(next, "change"))
	if !strings.Contains(strings.Join(strs(chg, "intent_delta", "additions"), " "), "구버전 클라이언트 호환 유지") {
		t.Fatal("the rejection must be in the next turn's IntentDelta")
	}
}

// AC-REV-04: a breaking change to a contract with external consumers appears only as a tier C
// card and is not reflected before approval.
func TestACREV04BreakingConsumerContractIsCOnly(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("confirm", post, "--external-consumers", "true")
	p.commit("harness")
	p.ok("run", "POST /messages 경로를 /v2/messages 로 변경")
	p.replace("internal/api/api.go", `"POST /messages"`, `"POST /v2/messages"`)
	p.replace("internal/api/api_test.go", `"/messages", strings.NewReader(`+"`"+`{"key"`, `"/v2/messages", strings.NewReader(`+"`"+`{"key"`)
	p.replace("internal/api/api_test.go", `"/messages", strings.NewReader(`+"`{}`", `"/v2/messages", strings.NewReader(`+"`{}`")
	p.ok("verify")
	v, code, _ := p.run("sync")
	if code != 2 {
		t.Fatalf("sync must stop on the C card, exit %d", code)
	}
	var cards []string
	for _, d := range list(v, "review", "deltas") {
		if str(d, "subject") == post {
			cards = append(cards, str(d, "tier")+":"+str(d, "status"))
		}
	}
	if len(cards) != 1 || cards[0] != "C:pending" {
		t.Fatalf("the breaking change must be one pending C card, got %v", cards)
	}
	if sel := str(p.ok("inspect", post), "contract", "selector"); sel != "POST /messages" {
		t.Fatalf("before approval the blueprint must not reflect the change, selector %q", sel)
	}
	id := ""
	for _, d := range list(v, "review", "deltas") {
		if str(d, "subject") == post {
			id = str(d, "id")
		}
	}
	p.ok("review", "--decide", id, "--decision", "approve", "--user")
	if s := p.ok("sync"); get(s, "completed") != true {
		t.Fatalf("approved C change completes: %v", get(s, "reason"))
	}
	if sel := str(p.ok("inspect", post), "contract", "selector"); sel != "POST /v2/messages" {
		t.Fatalf("after approval the blueprint follows, selector %q", sel)
	}
}

// AC-REV-05: a turn that only changes code inside a module records zero structural changes and
// raises no review notice.
func TestACREV05InternalChangeHasNoStructuralDelta(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	p.ok("run", "storage Count 버그 고쳐줘")
	src := p.read("internal/storage/storage.go")
	p.write("internal/storage/storage.go", src+"\n// countHint documents an internal detail.\nfunc countHint() int { return 0 }\n")
	p.ok("verify", "--no-repro", "--reason", "내부 정리: 관찰 가능한 동작 변화 없음")
	p.ok("verify")
	sync := p.ok("sync")
	if n := len(list(sync, "receipt", "structural_deltas")); n != 0 {
		t.Fatalf("internal-only change must have 0 structural deltas, got %v", get(sync, "receipt", "structural_deltas"))
	}
	if get(sync, "review") != nil {
		t.Fatal("no review notice for a turn without structural changes")
	}
}

// AC-REV-06: when an item changes again after it was decided, the old decision is marked
// invalid and a new card appears.
func TestACREV06RedecideAfterTheItemChangesAgain(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("run", "POST /messages 호환성 정책 변경")
	p.ok("intent", "contract", "--id", post, "--compatibility", "v2만 지원")
	first := deltaOf(p.ok("sync"), "contract_compat_changed")
	p.ok("review", "--decide", str(first, "id"), "--decision", "approve", "--user")

	p.ok("run", "POST /messages 호환성 정책 다시 변경")
	p.ok("intent", "contract", "--id", post, "--compatibility", "v3만 지원", "--origin", "USER_STATED")
	second := deltaOf(p.ok("sync"), "contract_compat_changed")
	if second == nil || str(second, "id") == str(first, "id") || str(second, "status") != "default_applied" {
		t.Fatalf("a new card must appear: %v", second)
	}
	old := p.ok("review", strings.Split(str(first, "id"), "#")[0])
	for _, d := range list(old, "deltas") {
		if str(d, "id") == str(first, "id") {
			if str(d, "status") != "superseded" || str(d, "decision", "invalidated_by") == "" {
				t.Fatalf("the earlier decision must be invalidated: %v", d)
			}
			return
		}
	}
	t.Fatal("first card not found")
}

// The serve channel: loopback only, and decisions need the page's session token.
func TestServeRequiresTokenAndLoopback(t *testing.T) {
	p := newProject(t, "code-only")
	p.ok("init")
	post := p.findContract("POST /messages")
	p.ok("run", "POST /messages 호환성 변경")
	p.ok("intent", "contract", "--id", post, "--compatibility", "v2만 지원")
	card := deltaOf(p.ok("sync"), "contract_compat_changed")

	if _, err := serve.Listen("0.0.0.0:0"); err == nil {
		t.Fatal("serve must refuse a non-loopback address")
	}
	e, _ := workflow.New(p.dir)
	h := serve.Handler(e, "tok")
	do := func(method, host, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+host+"/api/decide", strings.NewReader(body))
		if method == "GET" {
			r = httptest.NewRequest(method, "http://"+host+"/", nil)
		}
		r.Host = host
		if token != "" {
			r.Header.Set("X-ABBD-Token", token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	body := `{"delta":"` + str(card, "id") + `","decision":"approve"}`
	if rec := do("POST", "localhost:7787", "", body); rec.Code != http.StatusForbidden {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := do("GET", "evil.example:7787", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign Host must be refused (DNS rebinding): %d", rec.Code)
	}
	if rec := do("GET", "localhost:7787", "", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `name="abbd-serve-token" content="tok"`) {
		t.Fatalf("served page carries the token: %d", rec.Code)
	}
	if rec := do("POST", "127.0.0.1:7787", "tok", body); rec.Code != 200 {
		t.Fatalf("decision with token: %d %s", rec.Code, rec.Body.String())
	}
	for _, d := range list(p.ok("review"), "deltas") {
		if str(d, "id") == str(card, "id") && (str(d, "status") != "approved" || str(d, "decision", "channel") != "serve") {
			t.Fatalf("serve decision recorded: %v", d)
		}
	}
	// The decision file channel writes the same record.
	f := filepath.Join(p.dir, "decisions.json")
	os.WriteFile(f, []byte(`{"decisions":[{"delta":"`+str(card, "id")+`","decision":"reject","reason":"file channel"}]}`), 0o644)
	p.ok("review", "--apply", f)
	for _, d := range list(p.ok("review"), "deltas") {
		if str(d, "id") == str(card, "id") && str(d, "decision", "channel") != "file" {
			t.Fatalf("file decision recorded: %v", d)
		}
	}
}
