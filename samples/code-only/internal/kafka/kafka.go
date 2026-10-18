// Package kafka is a tiny in-memory stand-in for a Kafka client so the sample builds offline.
package kafka

import (
	"context"
	"errors"
	"sync"
)

var ErrNoMessages = errors.New("kafka: no more messages")

type Message struct {
	Key   []byte
	Value []byte
}

type Reader interface {
	ReadMessage(ctx context.Context) (Message, error)
	Close() error
}

type Writer interface {
	WriteMessages(ctx context.Context, msgs ...Message) error
	Close() error
}

type ReaderConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

type WriterConfig struct {
	Brokers []string
	Topic   string
}

// Broker holds topics in memory. Readers and writers built by the constructors share Default.
type Broker struct {
	mu     sync.Mutex
	topics map[string][]Message
}

var Default = NewBroker()

func NewBroker() *Broker { return &Broker{topics: map[string][]Message{}} }

func (b *Broker) Messages(topic string) []Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Message(nil), b.topics[topic]...)
}

func (b *Broker) append(topic string, msgs []Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.topics[topic] = append(b.topics[topic], msgs...)
}

func NewReader(cfg ReaderConfig) Reader { return &reader{broker: Default, topic: cfg.Topic} }

func NewWriter(cfg WriterConfig) Writer { return &writer{broker: Default, topic: cfg.Topic} }

type reader struct {
	broker *Broker
	topic  string
	offset int
}

func (r *reader) ReadMessage(ctx context.Context) (Message, error) {
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	msgs := r.broker.Messages(r.topic)
	if r.offset >= len(msgs) {
		return Message{}, ErrNoMessages
	}
	r.offset++
	return msgs[r.offset-1], nil
}

func (r *reader) Close() error { return nil }

type writer struct {
	broker *Broker
	topic  string
}

func (w *writer) WriteMessages(ctx context.Context, msgs ...Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.broker.append(w.topic, msgs)
	return nil
}

func (w *writer) Close() error { return nil }
