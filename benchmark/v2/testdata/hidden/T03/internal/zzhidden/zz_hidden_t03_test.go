package zzhidden_test

import (
	"context"
	"testing"
	"time"

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

type countingStore struct{ saves int }

func (c *countingStore) Save(context.Context, heartbeat.Heartbeat) error {
	c.saves++
	return nil
}

func TestHiddenT03ConsumerTreatsIDsCaseInsensitively(t *testing.T) {
	r := &sliceReader{}
	for _, id := range []string{"ABC", "abc", "AbC"} {
		r.msgs = append(r.msgs, kafka.Message{Value: []byte(`{"id":"` + id + `","source":"node-1","at":"2026-01-01T00:00:00Z"}`)})
	}
	store := &countingStore{}
	if err := heartbeat.NewConsumer(r, store).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.saves != 1 {
		t.Fatalf("consumer saved %d times for ABC/abc/AbC, want 1", store.saves)
	}
}

func TestHiddenT03StorageTreatsIDsCaseInsensitively(t *testing.T) {
	store := storage.NewWriter("memory://")
	for _, id := range []string{"XyZ", "xyz", "XYZ"} {
		if err := store.Save(context.Background(), heartbeat.Heartbeat{ID: id, Source: "node-1", At: time.Unix(1, 0)}); err != nil {
			t.Fatal(err)
		}
	}
	if store.Count() != 1 {
		t.Fatalf("storage holds %d heartbeats for XyZ/xyz/XYZ, want 1", store.Count())
	}
}
