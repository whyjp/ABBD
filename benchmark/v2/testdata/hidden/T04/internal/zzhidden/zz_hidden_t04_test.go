package zzhidden_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/heartbeat/internal/api"
	"example.com/heartbeat/internal/kafka"
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

type downWriter struct{}

func (downWriter) WriteMessages(context.Context, ...kafka.Message) error { return errors.New("broker down") }
func (downWriter) Close() error                                          { return nil }

func TestHiddenT04MessageValidationErrorsAreJSON(t *testing.T) {
	srv := api.NewServer(api.NewKafkaWriter(nil), storage.NewWriter("memory://"))
	for _, body := range []string{`{}`, `not json`} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(body)))
		assertJSONError(t, rec, http.StatusBadRequest)
	}
}

func TestHiddenT04PublishFailureIsJSON(t *testing.T) {
	srv := api.NewServer(downWriter{}, storage.NewWriter("memory://"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{"key":"k","body":"hi"}`)))
	assertJSONError(t, rec, http.StatusServiceUnavailable)
}
