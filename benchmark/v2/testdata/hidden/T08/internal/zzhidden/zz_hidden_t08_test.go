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

var at = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func save(t *testing.T, store *storage.Writer, id string) {
	t.Helper()
	if err := store.Save(context.Background(), heartbeat.Heartbeat{ID: id, Source: "node-1", At: at}); err != nil {
		t.Fatal(err)
	}
}

func TestHiddenT08StorageGet(t *testing.T) {
	_, store := newServer()
	save(t, store, "g1")
	h, ok := store.Get("g1")
	if !ok || h.ID != "g1" || h.Source != "node-1" || !h.At.Equal(at) {
		t.Fatalf("Get(g1) = %+v %v", h, ok)
	}
	if _, ok := store.Get("missing"); ok {
		t.Fatal("Get(missing) found something")
	}
}

func TestHiddenT08GetEndpoint(t *testing.T) {
	srv, store := newServer()
	save(t, store, "g1")
	rec := do(srv, http.MethodGet, "/heartbeats/g1", "")
	var h struct {
		ID     string    `json:"id"`
		Source string    `json:"source"`
		At     time.Time `json:"at"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &h) != nil || h.ID != "g1" || h.Source != "node-1" || !h.At.Equal(at) {
		t.Fatalf("GET /heartbeats/g1 = %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(srv, http.MethodGet, "/heartbeats/missing", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /heartbeats/missing = %d, want 404", rec.Code)
	}
}

func TestHiddenT08RuleCaseInsensitiveIDs(t *testing.T) {
	srv, store := newServer()
	save(t, store, "MiXeD")
	for _, id := range []string{"mixed", "MIXED"} {
		if rec := do(srv, http.MethodGet, "/heartbeats/"+id, ""); rec.Code != http.StatusOK {
			t.Fatalf("GET /heartbeats/%s = %d, want 200 (stored as MiXeD)", id, rec.Code)
		}
		if _, ok := store.Get(id); !ok {
			t.Fatalf("Get(%q) missed MiXeD", id)
		}
	}
}

func TestHiddenT08RuleJSONErrors(t *testing.T) {
	srv, _ := newServer()
	assertJSONError(t, do(srv, http.MethodGet, "/heartbeats/missing", ""), http.StatusNotFound)
}

func TestHiddenT08RuleRoutesListed(t *testing.T) {
	srv, _ := newServer()
	assertRouteListed(t, srv, "GET /heartbeats/{id}")
}
