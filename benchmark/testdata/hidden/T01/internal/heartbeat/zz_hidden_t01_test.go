package heartbeat_test

import (
	"strings"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
)

func TestHiddenT01FutureTimestamp(t *testing.T) {
	err := heartbeat.Validate(heartbeat.Heartbeat{ID: "a", Source: "s", At: time.Now().Add(10 * time.Minute)})
	if err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("10 minutes ahead must fail with 'future', got %v", err)
	}
	if err := heartbeat.Validate(heartbeat.Heartbeat{ID: "a", Source: "s", At: time.Now().Add(4 * time.Minute)}); err != nil {
		t.Fatalf("4 minutes ahead is valid, got %v", err)
	}
	if err := heartbeat.Validate(heartbeat.Heartbeat{ID: "a", Source: "s", At: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatalf("past is valid, got %v", err)
	}
}
