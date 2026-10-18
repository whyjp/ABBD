package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/storage"
)

func TestHiddenT04GetKeepsVersion(t *testing.T) {
	w := storage.NewWriter("")
	h := heartbeat.Heartbeat{ID: "a", Source: "s", At: time.Unix(0, 0), Version: 2}
	if err := w.Save(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	got, ok := w.Get("a")
	if !ok || got.Version != 2 || got.Source != "s" {
		t.Fatalf("Get = %+v %v", got, ok)
	}
	if _, ok := w.Get("missing"); ok {
		t.Fatal("missing id must report false")
	}
	if len(w.Statements) == 0 || !strings.Contains(w.Statements[0], "version") {
		t.Fatalf("insert must include version: %v", w.Statements)
	}
}
