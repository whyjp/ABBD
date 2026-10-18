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

func list(t *testing.T, srv http.Handler, query string) []string {
	t.Helper()
	rec := do(srv, http.MethodGet, "/heartbeats?"+query, "")
	var body struct {
		Heartbeats []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"heartbeats"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || !strings.Contains(rec.Body.String(), `"heartbeats"`) {
		t.Fatalf("GET /heartbeats?%s = %d %q", query, rec.Code, rec.Body.String())
	}
	ids := []string{}
	for _, h := range body.Heartbeats {
		ids = append(ids, h.ID+"@"+h.Source)
	}
	return ids
}

func TestHiddenT12ListBySource(t *testing.T) {
	srv, store := newServer()
	for _, h := range [][2]string{{"lb", "node-1"}, {"lc", "node-2"}, {"la", "node-1"}} {
		if err := store.Save(context.Background(), heartbeat.Heartbeat{ID: h[0], Source: h[1], At: time.Unix(1, 0)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(list(t, srv, "source=node-1"), ","); got != "la@node-1,lb@node-1" {
		t.Fatalf("source=node-1 listed %q, want la@node-1,lb@node-1", got)
	}
	if got := list(t, srv, "source=nobody"); len(got) != 0 {
		t.Fatalf("source=nobody listed %v, want none", got)
	}
}

func TestHiddenT12ListRequiresSource(t *testing.T) {
	srv, _ := newServer()
	if rec := do(srv, http.MethodGet, "/heartbeats", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /heartbeats without source = %d, want 400", rec.Code)
	}
}

func TestHiddenT12RuleJSONErrors(t *testing.T) {
	srv, _ := newServer()
	assertJSONError(t, do(srv, http.MethodGet, "/heartbeats", ""), http.StatusBadRequest)
}

func TestHiddenT12RuleRoutesListed(t *testing.T) {
	srv, _ := newServer()
	assertRouteListed(t, srv, "GET /heartbeats")
}
