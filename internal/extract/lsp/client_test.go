package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for a language server, so the
// client is tested over a real pipe and process.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_LSP") != "" {
		fakeServer()
		return
	}
	os.Exit(m.Run())
}

// fakeServer pulls its settings, reports indexing as a progress, drops the
// first references request as content-modified, and dies on "crash".
func fakeServer() {
	r := bufio.NewReader(os.Stdin)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Printf("Content-Length: %d\r\n\r\n%s", len(b), b)
	}
	dropped := false
	var settings json.RawMessage
	for {
		b, err := frame(r)
		if err != nil {
			return
		}
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}
		json.Unmarshal(b, &m)
		switch m.Method {
		case "initialize":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"capabilities": map[string]any{}}})
		case "initialized":
			send(map[string]any{"jsonrpc": "2.0", "id": 900, "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]any{"section": "fake"}}}})
			send(map[string]any{"jsonrpc": "2.0", "method": "$/progress", "params": map[string]any{"token": "idx", "value": map[string]any{"kind": "begin", "title": "Indexing"}}})
			time.Sleep(200 * time.Millisecond)
			send(map[string]any{"jsonrpc": "2.0", "method": "$/progress", "params": map[string]any{"token": "idx", "value": map[string]any{"kind": "end"}}})
		case "":
			settings = m.Result
		case "textDocument/references":
			if !dropped {
				dropped = true
				send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32801, "message": "content modified"}})
				continue
			}
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": []any{map[string]any{"uri": "file:///x/a.rs", "range": map[string]any{}}}})
		case "workspace/settings":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": settings})
		case "crash":
			os.Stderr.WriteString("fatal: out of cheese\n")
			os.Exit(3)
		case "shutdown":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": nil})
		case "exit":
			return
		}
	}
}

func startFake(t *testing.T) *Client {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, err := Start(context.Background(), Server{
		Argv: []string{exe}, Env: []string{"FAKE_LSP=1"}, Root: t.TempDir(),
		Settings: map[string]any{"fake": map[string]any{"depth": 3}}, Timeout: 10 * time.Second, Ready: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestClientWaitsForIndexingAndAsksAgainWhenTheServerDropsARequest(t *testing.T) {
	c := startFake(t)
	start := time.Now()
	if err := c.Wait(context.Background(), 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Errorf("Wait returned after %s, before indexing ended", time.Since(start))
	}
	var s []map[string]int
	if err := c.Call(context.Background(), "workspace/settings", nil, &s); err != nil || len(s) != 1 || s[0]["depth"] != 3 {
		t.Errorf("the server pulled settings %v (%v), want depth 3", s, err)
	}
	refs, err := c.References(context.Background(), "a.rs", Position{})
	if err != nil || len(refs) != 1 {
		t.Errorf("references = %v, %v; want one after asking again", refs, err)
	}
}

func TestClientSaysWhyTheServerStopped(t *testing.T) {
	c := startFake(t)
	err := c.Call(context.Background(), "crash", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "out of cheese") {
		t.Errorf("err = %v, want the server's last stderr", err)
	}
}

func TestURIRoundTripsAnAbsolutePath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "a dir", "b#c.rs")
	u := URI(abs)
	if !strings.HasPrefix(u, "file:///") || strings.Contains(u, `\`) || strings.Contains(u, " ") {
		t.Errorf("URI(%q) = %q", abs, u)
	}
	back, err := Path(u)
	if err != nil || back != abs {
		t.Errorf("Path(%q) = %q, %v; want %q", u, back, err, abs)
	}
	if runtime.GOOS == "windows" {
		if p, err := Path("file:///c%3A/x/y.rs"); err != nil || p != `c:\x\y.rs` {
			t.Errorf("an escaped drive letter reads as %q, %v", p, err)
		}
	}
}
