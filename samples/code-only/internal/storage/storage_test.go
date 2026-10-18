package storage

import (
	"context"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
)

func TestWriterSaveIsIdempotent(t *testing.T) {
	w := NewWriter("memory://")
	h := heartbeat.Heartbeat{ID: "a", Source: "node-1", At: time.Unix(1, 0)}
	for i := 0; i < 2; i++ {
		if err := w.Save(context.Background(), h); err != nil {
			t.Fatal(err)
		}
	}
	if w.Count() != 1 {
		t.Fatalf("count = %d, want 1", w.Count())
	}
}
