package heartbeat_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/kafka"
)

type flakyStore struct {
	fails, calls int
	err          error
}

func (f *flakyStore) Save(ctx context.Context, h heartbeat.Heartbeat) error {
	f.calls++
	if f.calls <= f.fails {
		return f.err
	}
	return nil
}

func hiddenMsgT05(id string) kafka.Message {
	return kafka.Message{Value: []byte(`{"id":"` + id + `","source":"s","at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`)}
}

func TestHiddenT05Retry(t *testing.T) {
	boom := errors.New("db down")
	s := &flakyStore{fails: 2, err: boom}
	c := heartbeat.NewConsumer(nil, s)
	c.Backoff = 0
	if err := c.Handle(context.Background(), hiddenMsgT05("a")); err != nil || s.calls != 3 {
		t.Fatalf("two failures then success: err %v calls %d", err, s.calls)
	}
	s2 := &flakyStore{fails: 5, err: boom}
	c2 := heartbeat.NewConsumer(nil, s2)
	c2.Backoff = 0
	err := c2.Handle(context.Background(), hiddenMsgT05("b"))
	if !errors.Is(err, boom) || s2.calls != 3 {
		t.Fatalf("three attempts then wrapped error: err %v calls %d", err, s2.calls)
	}
	s2.fails = 0
	if err := c2.Handle(context.Background(), hiddenMsgT05("b")); err != nil || s2.calls != 4 {
		t.Fatalf("an id whose saves failed is processed again: err %v calls %d", err, s2.calls)
	}
}

func TestHiddenT05DefaultBackoff(t *testing.T) {
	if c := heartbeat.NewConsumer(nil, &flakyStore{}); c.Backoff != 100*time.Millisecond {
		t.Fatalf("default backoff %v", c.Backoff)
	}
}
