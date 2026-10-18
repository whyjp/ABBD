package heartbeat_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/kafka"
)

type countingStoreT06 struct{ n atomic.Int64 }

func (c *countingStoreT06) Save(ctx context.Context, h heartbeat.Heartbeat) error {
	time.Sleep(time.Millisecond)
	c.n.Add(1)
	return nil
}

func TestHiddenT06ConcurrentSameID(t *testing.T) {
	s := &countingStoreT06{}
	c := heartbeat.NewConsumer(nil, s)
	m := kafka.Message{Value: []byte(`{"id":"same","source":"s","at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`)}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = c.Handle(context.Background(), m) }()
	}
	wg.Wait()
	if got := s.n.Load(); got != 1 {
		t.Fatalf("same id from 100 goroutines saved %d times, want 1", got)
	}
}
