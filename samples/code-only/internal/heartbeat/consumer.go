package heartbeat

import (
	"context"
	"errors"
	"fmt"

	"example.com/heartbeat/internal/kafka"
)

type Store interface {
	Save(ctx context.Context, h Heartbeat) error
}

type Consumer struct {
	reader kafka.Reader
	store  Store
	seen   *dedupe
}

func NewConsumer(r kafka.Reader, s Store) *Consumer {
	return &Consumer{reader: r, store: s, seen: newDedupe()}
}

// Run drains the reader until it reports no more messages or the context ends.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if errors.Is(err, kafka.ErrNoMessages) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := c.Handle(ctx, msg); err != nil {
			return err
		}
	}
}

func (c *Consumer) Handle(ctx context.Context, msg kafka.Message) error {
	h, err := Parse(msg.Value)
	if err != nil {
		return fmt.Errorf("parse heartbeat: %w", err)
	}
	if err := Validate(h); err != nil {
		return err
	}
	if !c.seen.firstSeen(h.ID) {
		return nil
	}
	return c.store.Save(ctx, h)
}
