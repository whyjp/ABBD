package heartbeat_test

import (
	"context"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/kafka"
)

type memStoreT07 struct{ n int }

func (m *memStoreT07) Save(ctx context.Context, h heartbeat.Heartbeat) error { m.n++; return nil }

func TestHiddenT07ConsumerStillDedupes(t *testing.T) {
	s := &memStoreT07{}
	c := heartbeat.NewConsumer(nil, s)
	m := kafka.Message{Value: []byte(`{"id":"a","source":"s","at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`)}
	_ = c.Handle(context.Background(), m)
	_ = c.Handle(context.Background(), m)
	if s.n != 1 {
		t.Fatalf("duplicate saved %d times", s.n)
	}
}
