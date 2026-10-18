package heartbeat_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
	"example.com/heartbeat/internal/kafka"
)

type endlessReaderT10 struct{ n int }

func (r *endlessReaderT10) ReadMessage(ctx context.Context) (kafka.Message, error) {
	r.n++
	return kafka.Message{Value: []byte(fmt.Sprintf(`{"id":"m%d","source":"s","at":"%s"}`, r.n, time.Now().UTC().Format(time.RFC3339)))}, nil
}

func (r *endlessReaderT10) Close() error { return nil }

type nopStoreT10 struct{}

func (nopStoreT10) Save(ctx context.Context, h heartbeat.Heartbeat) error { return nil }

func TestHiddenT10RunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- heartbeat.NewConsumer(&endlessReaderT10{}, nopStoreT10{}).Run(ctx) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run must return ctx.Err(), got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}
