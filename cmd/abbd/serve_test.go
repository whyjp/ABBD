package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

// view renders the page as anyone on the machine sees it. The token never
// reaches the server on a GET, so every viewer gets the same page.
func (s *screenServer) view(t *testing.T) string {
	t.Helper()
	rec := s.do(httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: %d %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// send posts a decision the way the page's script does: same origin, token in the body.
func (s *screenServer) send(t *testing.T, header http.Header, token string, kv ...string) *httptest.ResponseRecorder {
	t.Helper()
	f := url.Values{"t": {token}}
	for i := 0; i < len(kv); i += 2 {
		f.Add(kv[i], kv[i+1])
	}
	req := httptest.NewRequest(http.MethodPost, "/act", strings.NewReader(f.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range header {
		req.Header[k] = v
	}
	return s.do(req)
}

func (s *screenServer) sameOrigin() http.Header {
	return http.Header{"Origin": {"http://" + s.addr}, "Sec-Fetch-Site": {"same-origin"}}
}

func (s *screenServer) post(t *testing.T, token string, kv ...string) *httptest.ResponseRecorder {
	t.Helper()
	return s.send(t, s.sameOrigin(), token, kv...)
}

// decide returns what the CLI said, then the page as it reloads.
func (s *screenServer) decide(t *testing.T, kv ...string) string {
	t.Helper()
	rec := s.post(t, s.token, kv...)
	if rec.Code != http.StatusOK && rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("POST %v: %d %s", kv, rec.Code, rec.Body.String())
	}
	return rec.Body.String() + "\n" + s.view(t)
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
	html := s.view(t)
	waiting, thisTurn, drift, mapAt := strings.Index(html, "결정 대기"), strings.Index(html, "이번 턴 CHG-0001"), strings.Index(html, "의도 대 실제"), strings.Index(html, "전체 지도")
	if !(waiting < strings.Index(html, edge) && strings.Index(html, edge) < thisTurn && thisTurn < drift && drift < mapAt) {
		t.Fatalf("the blocked change comes first, then the turn, drift and the map")
	}
	if !strings.Contains(html, "make the change") || !strings.Contains(html, "했지만 선언하지 않음: <span class=\"key\">MOD-0001") {
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

func TestScreenDecisionsNeedTheTokenFromTheScreensOwnOrigin(t *testing.T) {
	p, edge := blockedTurn(t)
	s := p.screen()
	for _, tok := range []string{"", "wrong", testToken + "x"} {
		if code := s.post(t, tok, "do", "approve", "key", edge).Code; code != http.StatusForbidden {
			t.Fatalf("a POST with token %q should be refused, got %d", tok, code)
		}
	}
	for name, h := range map[string]http.Header{
		"no origin":         {},
		"another site":      {"Origin": {"http://evil.example"}},
		"another port":      {"Origin": {"http://127.0.0.1:9999"}, "Sec-Fetch-Site": {"same-site"}},
		"a cross-site page": {"Origin": {"http://" + s.addr}, "Sec-Fetch-Site": {"cross-site"}},
	} {
		if code := s.send(t, h, s.token, "do", "approve", "key", edge).Code; code != http.StatusForbidden {
			t.Fatalf("a POST from %s with the right token should be refused, got %d", name, code)
		}
	}
	if _, err := os.Stat(filepath.Join(p.root, ".abbd", "decisions")); err == nil {
		t.Fatalf("a refused POST must not record a decision")
	}

	for _, target := range []string{"/", "/?t=" + testToken} {
		rec := s.do(httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Set-Cookie") != "" || strings.Contains(rec.Body.String(), testToken) {
			t.Fatalf("GET %s must not set a cookie or carry the token, got %d %v", target, rec.Code, rec.Header())
		}
		if !strings.Contains(rec.Body.String(), `id="readonly"`) {
			t.Fatalf("the page should say it is read only until the script finds the token")
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "evil.example:7340"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a request for another host name should be refused, got %d", rec.Code)
	}
	if rec := s.post(t, testToken, "do", "approve", "key", "--root=/elsewhere"); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "항목 키가 아닙니다") {
		t.Fatalf("a key that looks like a flag must not reach the command line, got %d %s", rec.Code, rec.Body.String())
	}

	out := s.decide(t, "do", "approve", "key", edge)
	if !strings.Contains(out, "approved "+edge) || strings.Contains(s.view(t), "approved "+edge+" for") {
		t.Fatalf("what the CLI said goes back to the page that sent the decision only:\n%s", out)
	}
}

func TestScreenRefusesToBeFramed(t *testing.T) {
	p := aligned(t)
	rec := p.screen().do(httptest.NewRequest(http.MethodGet, "/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	if rec.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "form-action 'self'") {
		t.Fatalf("the screen must refuse framing, got %v", rec.Header())
	}
	nonce := regexp.MustCompile(`script-src 'nonce-([^']+)'`).FindStringSubmatch(csp)
	if nonce == nil || !strings.Contains(rec.Body.String(), `<script nonce="`+nonce[1]+`">`) {
		t.Fatalf("only the page's own script may run, by the nonce the policy names:\n%s", csp)
	}
	if again := p.screen().do(httptest.NewRequest(http.MethodGet, "/", nil)).Header().Get("Content-Security-Policy"); again == csp {
		t.Fatalf("each page needs a fresh nonce")
	}
}

func TestScreenOnPort80AcceptsTheBrowsersHostAndOrigin(t *testing.T) {
	p := aligned(t)
	s := p.screen()
	s.addr = "127.0.0.1:80"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("a browser omits :80 from Host, got %d", rec.Code)
	}
	h := http.Header{"Origin": {"http://127.0.0.1"}, "Sec-Fetch-Site": {"same-origin"}}
	if code := s.send(t, h, s.token, "do", "approve", "key", "-x").Code; code != http.StatusUnprocessableEntity {
		t.Fatalf("a browser omits :80 from Origin, got %d", code)
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
	if !strings.Contains(html, "ADR-0001로 반려했습니다: billing must not read the ledger") {
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
	html := s.view(t)
	if !strings.Contains(html, "첫 정렬") || !strings.Contains(html, `value="MOD-0002" checked`) {
		t.Fatalf("a bootstrapped project should offer the first alignment")
	}
	if !strings.Contains(html, `<span class="pill inferred">추정</span> <span class="key">MOD-0002.purpose`) {
		t.Fatalf("an inferred purpose should be marked on the page")
	}
	s.decide(t, "do", "align", "id", "MOD-0001", "id", "IF-0001")
	if in := p.intended(); !strings.Contains(in, "MOD-0001") || strings.Contains(in, "id: MOD-0002") {
		t.Fatalf("alignment should take the chosen units only:\n%s", in)
	}

	s.decide(t, "do", "confirm", "key", "MOD-0001.responsibility")
	s.decide(t, "do", "confirm", "key", "MOD-0002.purpose", "value", "posts journal entries")
	html = s.decide(t, "do", "confirm-reject", "key", "MOD-0002.consumes.IF-0001")
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
	html := s.decide(t, "do", "couple", "from", "MOD-0002", "kind", "depends_on", "to", "MOD-0001")
	if !strings.Contains(p.intended(), "kind: depends_on") || !strings.Contains(html, "사람이 등록한 결합 1개") {
		t.Fatalf("a coupling registered on the screen between design units should be intended")
	}
	p.editCode("ledger/book.go")
	p.extract(`{"units":[
 {"key":"billing","kind":"MOD","name":"billing","anchors":[{"path":"billing/charge.go","symbol":"Charge"}]},
 {"key":"api","kind":"IF","name":"http api","signature":"GET /charges","anchors":[{"path":"api/http.go","symbol":"Serve"}]}
],"relations":[{"from":"billing","to":"api","kind":"provides"}]}`)
	p.want(2, "REALIGN unit-removed:MOD-0002", "mode", "on", "--session", "s1")
	if html := s.view(t); !strings.Contains(html, "REALIGN 행 인정") || !strings.Contains(html, `class="removed"`) {
		t.Fatalf("a drifted project should offer the short realignment and draw the removed unit")
	}
	s.decide(t, "do", "realign", "key", "unit-removed:MOD-0002")
	p.want(0, "phase: aligned", "status")
}

func TestGeneratedFilesCarryTheWatermarkAndStatusSaysStale(t *testing.T) {
	p := aligned(t)
	p.want(0, "generated: CURRENT", "status")
	p.want(0, "actual topology: CURRENT", "status")
	for _, f := range []string{"graph.json", "explorer.html", "confirm-queue.html"} {
		if !strings.Contains(p.read("generated/"+f), "inputs_hash") {
			t.Fatalf("%s should carry the watermark", f)
		}
	}
	before := p.read("generated/explorer.html")
	e := p.screen().env
	if err := e.publish(); err != nil || p.read("generated/explorer.html") != before {
		t.Fatalf("regenerating unchanged inputs should write the same bytes: %v", err)
	}

	md := filepath.Join(p.root, ".abbd", "architecture", "MOD-0001.md")
	b, _ := os.ReadFile(md)
	write(t, md, strings.Replace(string(b), "name:\n    value: billing", "name:\n    value: payments", 1))
	p.want(0, "generated: STALE", "status")
	p.want(0, "", "confirm", "--list")
	p.want(0, "", "turn", "show")
	p.want(1, "", "approve", "new-relation:MOD-0001.depends_on.MOD-0002")
	p.want(0, "generated: STALE", "status")
	p.want(0, "", "couple", "MOD-0002.consumes.IF-0001")
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

func TestWatermarkCoversEveryInputThePagesRender(t *testing.T) {
	p, edge := blockedTurn(t)
	p.want(0, "as ADR-0001", "approve", edge)
	p.want(0, "generated: CURRENT", "status")
	for _, rel := range []string{"project.yaml", "state.yaml", "intended.yaml", "decisions/ADR-0001.md", "changes/CHG-0001.md", "requirements/REQ-0001.md"} {
		f := filepath.Join(p.root, ".abbd", filepath.FromSlash(rel))
		before := p.read(rel)
		write(t, f, before+"# edited by hand")
		p.want(0, "generated: STALE", "status")
		write(t, f, before)
		p.want(0, "generated: CURRENT", "status")
	}
	write(t, filepath.Join(p.root, ".abbd", "generated", "notes.txt"), "x")
	p.want(0, "generated: CURRENT", "status")
}
