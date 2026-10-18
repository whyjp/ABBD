package zzhidden_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/heartbeat/internal/replay"
	"example.com/heartbeat/internal/storage"
)

func dump(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dump.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func hb(id, source string) string {
	return `{"id":"` + id + `","source":"` + source + `","at":"2026-01-01T00:00:00Z"}`
}

func TestHiddenT09ReplaySavesValidLines(t *testing.T) {
	store := storage.NewWriter("memory://")
	path := dump(t, hb("r1", "node-1"), `garbage`, `{"id":"r2","at":"2026-01-01T00:00:00Z"}`, "", hb("r3", "node-2"))
	n, err := replay.File(context.Background(), path, store)
	if err != nil || n != 2 {
		t.Fatalf("File = %d, %v; want 2, nil", n, err)
	}
	if store.Count() != 2 {
		t.Fatalf("stored %d, want 2", store.Count())
	}
}

func TestHiddenT09ReplayMissingFileIsError(t *testing.T) {
	if _, err := replay.File(context.Background(), filepath.Join(t.TempDir(), "nope.jsonl"), storage.NewWriter("memory://")); err == nil {
		t.Fatal("missing file replayed without error")
	}
}

func TestHiddenT09RuleNoTestSource(t *testing.T) {
	store := storage.NewWriter("memory://")
	if _, err := replay.File(context.Background(), dump(t, hb("ra", "node-1"), hb("rb", "test")), store); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("ra"); !ok {
		t.Fatal("node-1 heartbeat not replayed")
	}
	if _, ok := store.Get("rb"); ok || store.Count() != 1 {
		t.Fatalf("heartbeat with source \"test\" was stored (count %d)", store.Count())
	}
}

func TestHiddenT09RuleCaseInsensitiveIDs(t *testing.T) {
	store := storage.NewWriter("memory://")
	if _, err := replay.File(context.Background(), dump(t, hb("RC", "node-1"), hb("rc", "node-1")), store); err != nil {
		t.Fatal(err)
	}
	if store.Count() != 1 {
		t.Fatalf("stored %d for RC/rc, want 1", store.Count())
	}
}
