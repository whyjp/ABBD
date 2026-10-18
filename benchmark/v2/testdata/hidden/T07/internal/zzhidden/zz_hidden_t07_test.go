package zzhidden_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/heartbeat/internal/api"
	"example.com/heartbeat/internal/storage"
)

func newServer() (*api.Server, *storage.Writer) {
	store := storage.NewWriter("memory://")
	return api.NewServer(api.NewKafkaWriter(nil), store), store
}

func do(srv http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	ct := rec.Header().Get("Content-Type")
	if rec.Code != status || !strings.HasPrefix(ct, "application/json") || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Error == "" {
		t.Fatalf("got %d %q %q, want %d application/json {\"error\": \"...\"}", rec.Code, ct, rec.Body.String(), status)
	}
}

func assertRouteListed(t *testing.T, srv http.Handler, route string) {
	t.Helper()
	var body struct {
		Routes []string `json:"routes"`
	}
	rec := do(srv, http.MethodGet, "/routes", "")
	if json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("GET /routes = %d %q", rec.Code, rec.Body.String())
	}
	for _, r := range body.Routes {
		if r == route {
			return
		}
	}
	t.Fatalf("GET /routes lacks %q: %v", route, body.Routes)
}

func hb(id, source string) string {
	return `{"id":"` + id + `","source":"` + source + `","at":"2026-01-01T00:00:00Z"}`
}

func paddedHB(id string, size int) string {
	base := `{"id":"` + id + `","source":"node-1","at":"2026-01-01T00:00:00Z","note":"`
	return base + strings.Repeat("x", size-len(base)-2) + `"}`
}

func TestHiddenT07ImportStoresValidHeartbeats(t *testing.T) {
	srv, store := newServer()
	rec := do(srv, http.MethodPost, "/heartbeats/import", `[`+hb("i1", "node-1")+`,`+hb("i2", "node-2")+`,{"id":"i3","at":"2026-01-01T00:00:00Z"}]`)
	var body struct {
		Imported *int `json:"imported"`
		Rejected *int `json:"rejected"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Imported == nil || body.Rejected == nil || *body.Imported != 2 || *body.Rejected != 1 {
		t.Fatalf("import = %d %q, want 200 {\"imported\": 2, \"rejected\": 1}", rec.Code, rec.Body.String())
	}
	if store.Count() != 2 {
		t.Fatalf("stored %d, want 2", store.Count())
	}
}

func TestHiddenT07ImportRejectsNonArrayBody(t *testing.T) {
	srv, store := newServer()
	for _, body := range []string{hb("i1", "node-1"), `not json`} {
		if rec := do(srv, http.MethodPost, "/heartbeats/import", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("import %q = %d, want 400", body, rec.Code)
		}
	}
	if store.Count() != 0 {
		t.Fatalf("stored %d, want 0", store.Count())
	}
}

func TestHiddenT07RuleNoTestSource(t *testing.T) {
	srv, store := newServer()
	if rec := do(srv, http.MethodPost, "/heartbeats/import", `[`+hb("i1", "node-1")+`,`+hb("i2", "test")+`]`); rec.Code != http.StatusOK {
		t.Fatalf("import = %d %q, want 200", rec.Code, rec.Body.String())
	}
	if store.Count() != 1 {
		t.Fatalf("stored %d, want 1: heartbeats with source \"test\" are never stored", store.Count())
	}
}

func TestHiddenT07RuleCaseInsensitiveIDs(t *testing.T) {
	srv, store := newServer()
	if rec := do(srv, http.MethodPost, "/heartbeats/import", `[`+hb("IMP", "node-1")+`,`+hb("imp", "node-1")+`]`); rec.Code != http.StatusOK {
		t.Fatalf("import = %d %q, want 200", rec.Code, rec.Body.String())
	}
	if store.Count() != 1 {
		t.Fatalf("stored %d for IMP/imp, want 1", store.Count())
	}
}

func TestHiddenT07RuleBodyLimit(t *testing.T) {
	srv, store := newServer()
	if rec := do(srv, http.MethodPost, "/heartbeats/import", `[`+paddedHB("big", 2000)+`]`); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("2 KB import = %d, want 413", rec.Code)
	}
	if store.Count() != 0 {
		t.Fatalf("stored %d from an oversized body, want 0", store.Count())
	}
}

func TestHiddenT07RuleJSONErrors(t *testing.T) {
	srv, _ := newServer()
	assertJSONError(t, do(srv, http.MethodPost, "/heartbeats/import", `not json`), http.StatusBadRequest)
}

func TestHiddenT07RuleRoutesListed(t *testing.T) {
	srv, _ := newServer()
	assertRouteListed(t, srv, "POST /heartbeats/import")
}
