package relate

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// progressEvery is how often a long read says how far it has got.
var progressEvery = 30 * time.Second

// progress tells a person waiting on a language server that the read goes on:
// a line on statusOut when a stage starts and one every progressEvery with
// how many of its items are done and how long it has taken. It only writes to
// the status stream, so what the relater finds is the same without it.
type progress struct {
	w     io.Writer
	label string
	unit  string
	total int
	done  atomic.Int64
	start time.Time
	stop  chan struct{}
	ended chan struct{}
}

// startProgress begins reporting a stage of total items, such as "opening"
// files, and says so at once.
func startProgress(w io.Writer, label, unit string, total int) *progress {
	p := &progress{w: w, label: label, unit: unit, total: total, start: time.Now(), stop: make(chan struct{}), ended: make(chan struct{})}
	fmt.Fprintf(w, "relate: %s %d %s\n", label, total, unit)
	t := time.NewTicker(progressEvery)
	go func() {
		defer close(p.ended)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				fmt.Fprintf(w, "relate: %s %d/%d %s, %s elapsed\n", label, p.done.Load(), total, unit, time.Since(p.start).Round(time.Second))
			case <-p.stop:
				return
			}
		}
	}()
	return p
}

// tick counts one item done.
func (p *progress) tick() { p.done.Add(1) }

// end stops the reports.
func (p *progress) end() {
	close(p.stop)
	<-p.ended
}
