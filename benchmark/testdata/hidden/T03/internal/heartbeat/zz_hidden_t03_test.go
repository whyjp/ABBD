package heartbeat_test

import (
	"testing"

	"example.com/heartbeat/internal/heartbeat"
)

func TestHiddenT03Version(t *testing.T) {
	h, err := heartbeat.Parse([]byte(`{"id":"a","source":"s","at":"2026-01-01T00:00:00Z"}`))
	if err != nil || h.Version != 1 {
		t.Fatalf("absent version defaults to 1: %+v %v", h, err)
	}
	h, err = heartbeat.Parse([]byte(`{"id":"a","source":"s","at":"2026-01-01T00:00:00Z","version":4}`))
	if err != nil || h.Version != 4 {
		t.Fatalf("explicit version kept: %+v %v", h, err)
	}
}
