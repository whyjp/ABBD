package zzhidden_test

import (
	"strings"
	"testing"
	"time"

	"example.com/heartbeat/internal/heartbeat"
)

func TestHiddenT06RejectsFarFutureTimestamps(t *testing.T) {
	now := time.Now()
	err := heartbeat.Validate(heartbeat.Heartbeat{ID: "f", Source: "node-1", At: now.Add(10 * time.Minute)})
	if err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("At = now+10m: err %v, want an error mentioning \"future\"", err)
	}
	for _, at := range []time.Time{now.Add(time.Minute), now.Add(-time.Hour), time.Unix(1, 0)} {
		if err := heartbeat.Validate(heartbeat.Heartbeat{ID: "f", Source: "node-1", At: at}); err != nil {
			t.Fatalf("At = %v rejected: %v", at, err)
		}
	}
}
