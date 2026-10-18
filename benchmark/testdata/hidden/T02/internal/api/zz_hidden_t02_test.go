package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/heartbeat/internal/api"
)

type fixedCount int

func (f fixedCount) Count() int { return int(f) }

func TestHiddenT02CountEndpoint(t *testing.T) {
	srv := api.NewServer(api.NewKafkaWriter(nil), fixedCount(3))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/heartbeats/count", nil))
	var body struct{ Count int }
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Count != 3 {
		t.Fatalf("GET /heartbeats/count = %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != 200 {
		t.Fatalf("health must keep working: %d", rec.Code)
	}
}
