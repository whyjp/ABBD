package dedupe_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"example.com/heartbeat/internal/dedupe"
)

func TestHiddenT07Set(t *testing.T) {
	s := dedupe.New()
	if !s.FirstSeen("a") || s.FirstSeen("a") || !s.FirstSeen("b") {
		t.Fatal("FirstSeen must be true once per id")
	}
	var firsts atomic.Int64
	var wg sync.WaitGroup
	s2 := dedupe.New()
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s2.FirstSeen("x") {
				firsts.Add(1)
			}
		}()
	}
	wg.Wait()
	if firsts.Load() != 1 {
		t.Fatalf("concurrent FirstSeen true %d times", firsts.Load())
	}
}
