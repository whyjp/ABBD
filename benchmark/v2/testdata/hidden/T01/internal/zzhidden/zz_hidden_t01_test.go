package zzhidden_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/heartbeat/internal/api"
	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/storage"
)

func TestHiddenT01CountEndpoint(t *testing.T) {
	store := storage.NewWriter("memory://")
	for _, id := range []string{"a", "b", "c"} {
		if err := store.Save(context.Background(), heartbeat.Heartbeat{ID: id, Source: "node-1", At: time.Unix(1, 0)}); err != nil {
			t.Fatal(err)
		}
	}
	srv := api.NewServer(api.NewKafkaWriter(nil), store)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/heartbeats/count", nil))
	var body struct {
		Count *int `json:"count"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Count == nil || *body.Count != 3 {
		t.Fatalf("GET /heartbeats/count = %d %q, want 200 {\"count\": 3}", rec.Code, rec.Body.String())
	}
}

func TestHiddenT01RoutesEndpoint(t *testing.T) {
	srv := api.NewServer(api.NewKafkaWriter(nil), storage.NewWriter("memory://"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/routes", nil))
	var body struct {
		Routes []string `json:"routes"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("GET /routes = %d %q", rec.Code, rec.Body.String())
	}
	have := map[string]bool{}
	for _, r := range body.Routes {
		have[r] = true
	}
	for _, want := range []string{"POST /messages", "GET /health", "GET /heartbeats/count", "GET /routes"} {
		if !have[want] {
			t.Errorf("GET /routes lacks %q: %v", want, body.Routes)
		}
	}
}
