package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/heartbeat/internal/kafka"
)

func TestPostMessagePublishes(t *testing.T) {
	srv := NewServer(NewKafkaWriter(nil))
	before := len(kafka.Default.Messages(TopicMessages))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{"key":"k","body":"hi"}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if got := len(kafka.Default.Messages(TopicMessages)) - before; got != 1 {
		t.Fatalf("published %d messages, want 1", got)
	}
}

func TestPostMessageRejectsEmptyBody(t *testing.T) {
	srv := NewServer(NewKafkaWriter(nil))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	NewServer(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("health = %d %q", rec.Code, rec.Body.String())
	}
}
