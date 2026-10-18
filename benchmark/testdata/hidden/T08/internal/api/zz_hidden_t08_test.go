package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/heartbeat/internal/api"
	"example.com/heartbeat/internal/kafka"
)

type countZeroT08 struct{}

func (countZeroT08) Count() int { return 0 }

func TestHiddenT08Priority(t *testing.T) {
	srv := api.NewServer(api.NewKafkaWriter(nil), countZeroT08{})
	post := func(body string) int {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(body)))
		return rec.Code
	}
	if code := post(`{"key":"k","body":"x","priority":12}`); code != http.StatusBadRequest {
		t.Fatalf("priority 12 must be 400, got %d", code)
	}
	before := len(kafka.Default.Messages(api.TopicMessages))
	if code := post(`{"key":"k","body":"x","priority":3}`); code != http.StatusAccepted {
		t.Fatalf("priority 3 accepted, got %d", code)
	}
	msgs := kafka.Default.Messages(api.TopicMessages)
	if len(msgs) != before+1 || string(msgs[len(msgs)-1].Key) != "3:k" {
		t.Fatalf("key must be 3:k, got %q", string(msgs[len(msgs)-1].Key))
	}
	if code := post(`{"key":"z","body":"x"}`); code != http.StatusAccepted {
		t.Fatalf("absent priority defaults to 0, got %d", code)
	}
	msgs = kafka.Default.Messages(api.TopicMessages)
	if string(msgs[len(msgs)-1].Key) != "0:z" {
		t.Fatalf("default key 0:z, got %q", string(msgs[len(msgs)-1].Key))
	}
}
