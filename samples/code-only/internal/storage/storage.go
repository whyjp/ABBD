package storage

import (
	"context"
	"sync"

	"example.com/heartbeat/internal/heartbeat"
)

const insertHeartbeat = "INSERT INTO heartbeats (id, source, at) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING"

// Writer keeps rows in memory. Statements records what a SQL driver would have executed.
type Writer struct {
	DSN        string
	Statements []string
	mu         sync.Mutex
	rows       map[string]heartbeat.Heartbeat
}

func NewWriter(dsn string) *Writer {
	return &Writer{DSN: dsn, rows: map[string]heartbeat.Heartbeat{}}
}

func (w *Writer) Save(ctx context.Context, h heartbeat.Heartbeat) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Statements = append(w.Statements, insertHeartbeat)
	if _, ok := w.rows[h.ID]; !ok {
		w.rows[h.ID] = h
	}
	return nil
}

func (w *Writer) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.rows)
}
