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

func TestHiddenT11PostStoresHeartbeat(t *testing.T) {
	srv, store := newServer()
	rec := do(srv, http.MethodPost, "/heartbeats", hb("p1", "node-1"))
	var h struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	}
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &h) != nil || h.ID != "p1" || h.Source != "node-1" {
		t.Fatalf("POST /heartbeats = %d %q, want 201 echoing p1", rec.Code, rec.Body.String())
	}
	if _, ok := store.Get("p1"); !ok {
		t.Fatal("p1 not stored")
	}
}

func TestHiddenT11PostRejectsInvalid(t *testing.T) {
	srv, store := newServer()
	if rec := do(srv, http.MethodPost, "/heartbeats", `{"id":"p2","at":"2026-01-01T00:00:00Z"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("POST without source = %d, want 400", rec.Code)
	}
	if store.Count() != 0 {
		t.Fatalf("stored %d, want 0", store.Count())
	}
}

func TestHiddenT11RuleNoTestSource(t *testing.T) {
	srv, store := newServer()
	do(srv, http.MethodPost, "/heartbeats", hb("qa1", "test"))
	if rec := do(srv, http.MethodPost, "/heartbeats", hb("qa2", "node-1")); rec.Code != http.StatusCreated {
		t.Fatalf("POST node-1 = %d, want 201", rec.Code)
	}
	if _, ok := store.Get("qa1"); ok || store.Count() != 1 {
		t.Fatalf("heartbeat with source \"test\" was stored (count %d)", store.Count())
	}
}

func TestHiddenT11RuleCaseInsensitiveIDs(t *testing.T) {
	srv, store := newServer()
	if rec := do(srv, http.MethodPost, "/heartbeats", hb("CI", "node-1")); rec.Code != http.StatusCreated {
		t.Fatalf("POST CI = %d, want 201", rec.Code)
	}
	do(srv, http.MethodPost, "/heartbeats", hb("ci", "node-1"))
	if store.Count() != 1 {
		t.Fatalf("stored %d for CI/ci, want 1", store.Count())
	}
}

func TestHiddenT11RuleBodyLimit(t *testing.T) {
	srv, store := newServer()
	if rec := do(srv, http.MethodPost, "/heartbeats", paddedHB("big", 2000)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("2 KB POST = %d, want 413", rec.Code)
	}
	if store.Count() != 0 {
		t.Fatalf("stored %d from an oversized body, want 0", store.Count())
	}
}

func TestHiddenT11RuleJSONErrors(t *testing.T) {
	srv, _ := newServer()
	assertJSONError(t, do(srv, http.MethodPost, "/heartbeats", `{"id":"p2","at":"2026-01-01T00:00:00Z"}`), http.StatusBadRequest)
}

func TestHiddenT11RuleRoutesListed(t *testing.T) {
	srv, _ := newServer()
	assertRouteListed(t, srv, "POST /heartbeats")
}
