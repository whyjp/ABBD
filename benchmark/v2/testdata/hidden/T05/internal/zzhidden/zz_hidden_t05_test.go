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

func messageOfSize(n int) string {
	const prefix, suffix = `{"key":"k","body":"`, `"}`
	return prefix + strings.Repeat("x", n-len(prefix)-len(suffix)) + suffix
}

func postMessage(body string) *httptest.ResponseRecorder {
	srv := api.NewServer(api.NewKafkaWriter(nil), storage.NewWriter("memory://"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(body)))
	return rec
}

func TestHiddenT05MessagesBodyLimit(t *testing.T) {
	if rec := postMessage(messageOfSize(1024)); rec.Code != http.StatusAccepted {
		t.Fatalf("1024-byte body: status %d, want 202", rec.Code)
	}
	if rec := postMessage(messageOfSize(1025)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("1025-byte body: status %d, want 413", rec.Code)
	}
}

func TestHiddenT05RuleJSONErrors(t *testing.T) {
	assertJSONError(t, postMessage(messageOfSize(4096)), http.StatusRequestEntityTooLarge)
}
