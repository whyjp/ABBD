package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/store"
)

const testToken = "tok3n"

func (p *project) screen() *screenServer {
	e := env{opts: opts{root: p.root}, stdout: io.Discard, stderr: io.Discard, st: store.Open(p.root)}
	return &screenServer{env: e, token: testToken, addr: "127.0.0.1:7340"}
}

func (s *screenServer) do(req *http.Request) *httptest.ResponseRecorder {
	req.Host = s.addr
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// page renders the screen as the human sees it, signed in by the link's cookie.
func (s *screenServer) view(t *testing.T, signedIn bool) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if signedIn {
		req.AddCookie(&http.Cookie{Name: tokenCookie, Value: s.token})
	}
	rec := s.do(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: %d %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func (s *screenServer) post(t *testing.T, token string, kv ...string) int {
	t.Helper()
	f := url.Values{"t": {token}}
	for i := 0; i < len(kv); i += 2 {
		f.Add(kv[i], kv[i+1])
	}
	req := httptest.NewRequest(http.MethodPost, "/act", strings.NewReader(f.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.do(req).Code
}

func (s *screenServer) decide(t *testing.T, kv ...string) string {
	t.Helper()
	if code := s.post(t, s.token, kv...); code != http.StatusSeeOther {
		t.Fatalf("POST %v: want a redirect back to the page, got %d", kv, code)
	}
	return s.view(t, true)
}

func blockedTurn(t *testing.T) (*turnProject, string) {
	p := activeTurnProject(t, topoV1)
	p.start("MOD-0001")
	p.editCode("billing/charge.go")
	p.extract(dependsOnLedger)
	p.verify("pass", 0, "PASSED")
	p.want(2, "CHG-0001 BLOCKED", "turn", "end", "--session", "s1")
	return p, "new-relation:MOD-0001.depends_on.MOD-0002"
}

func TestScreenPutsTheBlockedChangeFirstAndApprovesIt(t *testing.T) {
	p, edge := blockedTurn(t)
	s := p.screen()
	html := s.view(t, true)
	waiting, thisTurn, drift, mapAt := strings.Index(html, "Waiting for you"), strings.Index(html, "This turn: CHG-0001"), strings.Index(html, "Intended vs actual"), strings.Index(html, "Full map")
	if !(waiting < strings.Index(html, edge) && strings.Index(html, edge) < thisTurn && thisTurn < drift && drift < mapAt) {
		t.Fatalf("the blocked change comes first, then the turn, drift and the map")
	}
	if !strings.Contains(html, "make the change") || !strings.Contains(html, "done, not declared: <span class=\"key\">MOD-0001") {
		t.Fatalf("the turn should show its request and the declared-versus-actual report")
	}

	html = s.decide(t, "do", "approve", "key", edge)
	if !strings.Contains(html, "approved "+edge+" for CHG-0001 as ADR-0001") {
		t.Fatalf("the page should show what the CLI said")
	}
	if strings.Contains(p.intended(), "depends_on") {
		t.Fatalf("a turn approval from the screen still waits for the turn to sync")
	}
	if strings.Contains(html, `id="card-`+edge) {
		t.Fatalf("an approved change no longer waits")
	}
	p.want(0, "CHG-0001 SYNCED", "turn", "end", "--session", "s1")
	if !strings.Contains(p.intended(), "kind: depends_on") {
		t.Fatalf("the approval should enter the intended topology when the turn syncs")
	}
}

func TestScreenDecisionsNeedTheLaunchToken(t *testing.T) {
	p, edge := blockedTurn(t)
	s := p.screen()
	for _, tok := range []string{"", "wrong", testToken + "x"} {
		if code := s.post(t, tok, "do", "approve", "key", edge); code != http.StatusForbidden {
			t.Fatalf("a POST with token %q should be refused, got %d", tok, code)
		}
	}
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "decisions")); err == nil {
		t.Fatalf("a refused POST must not record a decision")
	}
	if html := s.view(t, false); strings.Contains(html, testToken) || !strings.Contains(html, "Read only") {
		t.Fatalf("a page opened without the link must not carry the token")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: tokenCookie, Value: "guessed"})
	if rec := s.do(req); strings.Contains(rec.Body.String(), testToken) {
		t.Fatalf("a cookie with the wrong token must not sign the page in")
	}
	req = httptest.NewRequest(http.MethodGet, "/?t="+testToken, nil)
	if rec := s.do(req); rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), "SameSite=Strict") {
		t.Fatalf("the printed link should sign the browser in with a strict cookie, got %d %v", rec.Code, rec.Header())
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "evil.example:7340"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a request for another host name should be refused, got %d", rec.Code)
	}
	if code := s.post(t, testToken, "do", "approve", "key", "--root=/elsewhere"); code != http.StatusSeeOther || !strings.Contains(s.view(t, true), "is not an item key") {
		t.Fatalf("a key that looks like a flag must not reach the command line")
	}
}

func TestScreenRejectKeepsTheChangeOutOfTheIntendedTopology(t *testing.T) {
	p, edge := blockedTurn(t)
	s := p.screen()
	before := p.intended()
	html := s.decide(t, "do", "reject", "key", edge, "reason", "billing must not read the ledger")
	if p.intended() != before {
		t.Fatalf("a rejection from the screen must leave the intended topology alone")
	}
	if !strings.Contains(html, "You rejected this as ADR-0001: billing must not read the ledger") {
		t.Fatalf("the card should stay, marked with the rejection:\n%s", html)
	}
	p.want(2, "REJECTED "+edge+" by ADR-0001: billing must not read the ledger", "turn", "end", "--session", "s1")
	if p.intended() != before {
		t.Fatalf("a rejected change must stay out after the turn ends again")
	}
	if !strings.Contains(s.decide(t, "do", "reject", "key", edge, "reason", ""), "needs --reason") {
		t.Fatalf("a rejection without a reason should be refused")
	}
}

func TestScreenAlignsAndWorksTheConfirmQueue(t *testing.T) {
	p, exe := bare(t)
	p.extract(codeOnly)
	p.setup(0, "", "--supplement", p.file(t, "sup.json", supplement), "--", exe)
	s := p.screen()
	html := s.view(t, true)
	if !strings.Contains(html, "First alignment") || !strings.Contains(html, `value="MOD-0002" checked`) {
		t.Fatalf("a bootstrapped project should offer the first alignment")
	}
	if !strings.Contains(html, `<span class="pill inferred">inferred</span> <span class="key">MOD-0002.purpose`) {
		t.Fatalf("an inferred purpose should be marked on the page")
	}
	s.decide(t, "do", "align", "id", "MOD-0001", "id", "IF-0001")
	if in := p.intended(); !strings.Contains(in, "MOD-0001") || strings.Contains(in, "id: MOD-0002") {
		t.Fatalf("alignment should take the chosen units only:\n%s", in)
	}

	s.decide(t, "do", "confirm", "key", "MOD-0001.responsibility")
	s.decide(t, "do", "confirm", "key", "MOD-0002.purpose", "value", "posts journal entries")
	html = s.decide(t, "do", "confirm-reject", "key", "MOD-0002.consumes.MOD-0001")
	md1, md2 := p.read("architecture/MOD-0001.md"), p.read("architecture/MOD-0002.md")
	if !strings.Contains(md1, "confirmation: CONFIRMED") || !strings.Contains(md2, "posts journal entries\n    origin: USER_STATED") || !strings.Contains(md2, "confirmation: REJECTED") {
		t.Fatalf("each queue decision should reach the unit:\n%s\n%s", md1, md2)
	}
	if strings.Contains(p.intended(), "consumes") || !strings.Contains(html, "MOD-0001.purpose") || strings.Contains(html, `id="q-MOD-0002.purpose"`) {
		t.Fatalf("decided items leave the queue and a rejected coupling stays out")
	}
	if !strings.Contains(s.decide(t, "do", "confirm-reject", "key", "MOD-0001.purpose"), "say what it is instead") {
		t.Fatalf("rejecting a purpose needs its replacement")
	}
}

func TestScreenRealignsAndRegistersACoupling(t *testing.T) {
	p := aligned(t)
	s := p.screen()
	html := s.decide(t, "do", "couple", "from", "MOD-0002", "kind", "consumes", "to", "MOD-0001")
	if !strings.Contains(p.intended(), "kind: consumes") || !strings.Contains(html, "1 coupling(s) are registered by a human") {
		t.Fatalf("a coupling registered on the screen between design units should be intended")
	}
	p.editCode("ledger/book.go")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[{"from":"billing","to":"api","kind":"provides"}]}`)
	p.want(2, "REALIGN unit-removed:MOD-0002", "mode", "on", "--session", "s1")
	if html := s.view(t, true); !strings.Contains(html, "Acknowledge the REALIGN rows") || !strings.Contains(html, `class="removed"`) {
		t.Fatalf("a drifted project should offer the short realignment and draw the removed unit")
	}
	s.decide(t, "do", "realign")
	p.want(0, "phase: aligned", "status")
}

func TestGeneratedFilesCarryTheWatermarkAndStatusSaysStale(t *testing.T) {
	p := aligned(t)
	p.want(0, "generated: CURRENT", "status")
	p.want(0, "actual topology: CURRENT", "status")
	for _, f := range []string{"graph.json", "explorer.html", "confirm-queue.html"} {
		if !strings.Contains(p.read("generated/"+f), "md_hash") {
			t.Fatalf("%s should carry the watermark", f)
		}
	}
	before := p.read("generated/explorer.html")
	p.want(0, "", "confirm", "--list")
	if p.read("generated/explorer.html") != before {
		t.Fatalf("regenerating unchanged Markdown should write the same bytes")
	}

	md := filepath.Join(p.root, ".abbd", "architecture", "MOD-0001.md")
	b, _ := os.ReadFile(md)
	write(t, md, strings.Replace(string(b), "name:\n    value: billing", "name:\n    value: payments", 1))
	p.want(0, "generated: STALE", "status")
	p.want(0, "generated: STALE", "status")
	p.want(0, "", "confirm", "--list")
	p.want(0, "generated: CURRENT", "status")
	if !strings.Contains(p.read("generated/graph.json"), "payments") {
		t.Fatalf("the regenerated graph should follow the Markdown")
	}

	p.editCode("billing/charge.go")
	p.want(0, "actual topology: STALE", "status")
}

func TestServeListensOnLoopbackOnly(t *testing.T) {
	p := aligned(t)
	p.want(1, "loopback address only", "serve", "--addr", "0.0.0.0:0")
	p.want(1, "loopback address only", "serve", "--addr", "localhost:0")
}
