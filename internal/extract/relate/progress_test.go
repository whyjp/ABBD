package relate

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A long stage says what it will do at once and, every interval, how many of
// its items are done of the total.
func TestAReadSaysHowFarItHasGot(t *testing.T) {
	old := progressEvery
	progressEvery = 5 * time.Millisecond
	defer func() { progressEvery = old }()
	var out lockedBuffer
	p := startProgress(&out, "tsserver: opening", "files", 7)
	for range 3 {
		p.tick()
	}
	time.Sleep(60 * time.Millisecond)
	p.end()
	got := out.String()
	if !strings.HasPrefix(got, "relate: tsserver: opening 7 files\n") {
		t.Errorf("no opening line in %q", got)
	}
	if !strings.Contains(got, "relate: tsserver: opening 3/7 files, ") || !strings.Contains(got, " elapsed\n") {
		t.Errorf("no progress line in %q", got)
	}
}
