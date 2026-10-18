package zzhidden_test

import (
	"context"
	"testing"

	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/kafka"
	"example.com/heartbeat/internal/storage"
)

type sliceReader struct{ msgs []kafka.Message }

func (r *sliceReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	if len(r.msgs) == 0 {
		return kafka.Message{}, kafka.ErrNoMessages
	}
	m := r.msgs[0]
	r.msgs = r.msgs[1:]
	return m, nil
}

func (r *sliceReader) Close() error { return nil }

func payloads(lines ...string) *sliceReader {
	r := &sliceReader{}
	for _, l := range lines {
		r.msgs = append(r.msgs, kafka.Message{Value: []byte(l)})
	}
	return r
}

func TestHiddenT02ConsumerDropsTestSource(t *testing.T) {
	store := storage.NewWriter("memory://")
	r := payloads(
		`{"id":"x1","source":"test","at":"2026-01-01T00:00:00Z"}`,
		`{"id":"n1","source":"node-1","at":"2026-01-01T00:00:00Z"}`,
		`{"id":"x2","source":"test","at":"2026-01-01T00:00:00Z"}`,
	)
	if err := heartbeat.NewConsumer(r, store).Run(context.Background()); err != nil {
		t.Fatalf("Run = %v, want nil: load-test heartbeats are not errors", err)
	}
	if store.Count() != 1 {
		t.Fatalf("stored %d heartbeats, want 1 (only node-1)", store.Count())
	}
}
