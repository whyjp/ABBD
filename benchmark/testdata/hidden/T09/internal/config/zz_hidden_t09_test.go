package config_test

import (
	"os"
	"testing"

	"example.com/heartbeat/internal/config"
)

func TestHiddenT09HTTPAddr(t *testing.T) {
	for _, name := range []string{"HTTP_ADDR", "HEARTBEAT_HTTP_ADDR"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	if got := config.HTTPAddr(); got != ":8080" {
		t.Fatalf("default %q", got)
	}
	t.Setenv("HEARTBEAT_HTTP_ADDR", ":9001")
	if got := config.HTTPAddr(); got != ":9001" {
		t.Fatalf("legacy fallback %q", got)
	}
	t.Setenv("HTTP_ADDR", ":9002")
	if got := config.HTTPAddr(); got != ":9002" {
		t.Fatalf("new variable wins %q", got)
	}
}
