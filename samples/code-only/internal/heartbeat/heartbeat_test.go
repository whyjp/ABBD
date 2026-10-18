package heartbeat

import (
	"context"
	"testing"
	"time"

	"example.com/heartbeat/internal/kafka"
)

func TestValidate(t *testing.T) {
	ok := Heartbeat{ID: "a", Source: "node-1", At: time.Unix(1, 0)}
	if err := Validate(ok); err != nil {
		t.Fatalf("valid heartbeat rejected: %v", err)
	}
	if err := Validate(Heartbeat{Source: "node-1", At: time.Unix(1, 0)}); err == nil {
		t.Fatal("heartbeat without id accepted")
	}
}

type memStore struct{ saved []Heartbeat }

func (m *memStore) Save(_ context.Context, h Heartbeat) error {
	m.saved = append(m.saved, h)
	return nil
}

func TestConsumerSkipsDuplicates(t *testing.T) {
	store := &memStore{}
	w := kafka.NewWriter(kafka.WriterConfig{Topic: "test-heartbeats"})
	payload := []byte(`{"id":"a","source":"node-1","at":"2026-01-01T00:00:00Z"}`)
	if err := w.WriteMessages(context.Background(), kafka.Message{Value: payload}, kafka.Message{Value: payload}); err != nil {
		t.Fatal(err)
	}
	c := NewConsumer(kafka.NewReader(kafka.ReaderConfig{Topic: "test-heartbeats"}), store)
	if err := c.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved %d heartbeats, want 1", len(store.saved))
	}
}
