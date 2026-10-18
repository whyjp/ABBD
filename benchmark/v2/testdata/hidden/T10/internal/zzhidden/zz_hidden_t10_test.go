package zzhidden_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/heartbeat/internal/api"
	"example.com/heartbeat/internal/heartbeat"
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

func save(t *testing.T, store *storage.Writer, id string) {
	t.Helper()
	if err := store.Save(context.Background(), heartbeat.Heartbeat{ID: id, Source: "node-1", At: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
}

func TestHiddenT10StorageDelete(t *testing.T) {
	_, store := newServer()
	save(t, store, "d1")
	if !store.Delete("d1") {
		t.Fatal("Delete(d1) = false, want true")
	}
	if store.Delete("d1") || store.Count() != 0 {
		t.Fatalf("second Delete(d1) = true or count %d != 0", store.Count())
	}
}

func TestHiddenT10DeleteEndpoint(t *testing.T) {
	srv, store := newServer()
	save(t, store, "d1")
	save(t, store, "keep")
	if rec := do(srv, http.MethodDelete, "/heartbeats/d1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /heartbeats/d1 = %d, want 204", rec.Code)
	}
	if _, ok := store.Get("d1"); ok || store.Count() != 1 {
		t.Fatalf("d1 still stored or keep lost (count %d)", store.Count())
	}
	if rec := do(srv, http.MethodDelete, "/heartbeats/d1", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second DELETE /heartbeats/d1 = %d, want 404", rec.Code)
	}
}

func TestHiddenT10RuleCaseInsensitiveIDs(t *testing.T) {
	srv, store := newServer()
	save(t, store, "del")
	if rec := do(srv, http.MethodDelete, "/heartbeats/DeL", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /heartbeats/DeL = %d, want 204 (stored as del)", rec.Code)
	}
	if store.Count() != 0 {
		t.Fatalf("count %d after deleting del via DeL, want 0", store.Count())
	}
	save(t, store, "Up")
	if !store.Delete("uP") {
		t.Fatal("Delete(uP) missed Up")
	}
}

func TestHiddenT10RuleJSONErrors(t *testing.T) {
	srv, _ := newServer()
	assertJSONError(t, do(srv, http.MethodDelete, "/heartbeats/missing", ""), http.StatusNotFound)
}

func TestHiddenT10RuleRoutesListed(t *testing.T) {
	srv, _ := newServer()
	assertRouteListed(t, srv, "DELETE /heartbeats/{id}")
}
