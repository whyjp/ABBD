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
// first references request as content-modified, and dies on "crash". It
// counts columns in FAKE_ENCODING, reports nothing with FAKE_QUIET, and
// answers "last" with the last position it was asked at and "opened" with
// the text of the last file opened.
func fakeServer() {
	r := bufio.NewReader(os.Stdin)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Printf("Content-Length: %d\r\n\r\n%s", len(b), b)
	}
	dropped := false
	var settings, last, opened json.RawMessage
	for {
		b, err := frame(r)
		if err != nil {
			return
		}
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Params struct {
				TextDocument struct {
					URI  string          `json:"uri"`
					Text json.RawMessage `json:"text"`
				} `json:"textDocument"`
				Position json.RawMessage `json:"position"`
			} `json:"params"`
		}
		json.Unmarshal(b, &m)
		switch m.Method {
		case "initialize":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"capabilities": map[string]any{"positionEncoding": os.Getenv("FAKE_ENCODING")}}})
		case "initialized":
			if os.Getenv("FAKE_QUIET") != "" {
				continue
			}
			send(map[string]any{"jsonrpc": "2.0", "id": 900, "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]any{"section": "fake"}}}})
			send(map[string]any{"jsonrpc": "2.0", "method": "$/progress", "params": map[string]any{"token": "load", "value": map[string]any{"kind": "begin", "title": "Loading"}}})
			send(map[string]any{"jsonrpc": "2.0", "method": "$/progress", "params": map[string]any{"token": "load", "value": map[string]any{"kind": "end"}}})
			time.Sleep(300 * time.Millisecond)
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
			last = m.Params.Position
			end := map[string]any{"line": 0, "character": 3}
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": []any{map[string]any{"uri": m.Params.TextDocument.URI, "range": map[string]any{"start": last, "end": end}}}})
		case "textDocument/didOpen":
			opened = m.Params.TextDocument.Text
		case "last":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": last})
		case "opened":
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": opened})
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

func startFake(t *testing.T, env ...string) *Client {
	return startServer(t, Server{Env: env, Ready: 10 * time.Second, Until: "Indexing"})
}

func startServer(t *testing.T, s Server) *Client {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s.Argv, s.Root, s.Timeout = []string{exe}, t.TempDir(), 10*time.Second
	s.Env = append(s.Env, "FAKE_LSP=1")
	s.Settings = map[string]any{"fake": map[string]any{"depth": 3}}
	c, err := Start(context.Background(), s)
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
	if time.Since(start) < 500*time.Millisecond {
		t.Errorf("Wait returned after %s, in the idle gap before indexing", time.Since(start))
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

func TestClientCountsBytesWhereTheServerCountsUTF16Units(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.rs")
	if err := os.WriteFile(src, []byte("é🎉 = f();"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, enc := range []string{"utf-16", "utf-8"} {
		c := startFake(t, "FAKE_ENCODING="+enc)
		refs, err := c.References(context.Background(), src, Position{Character: 9})
		if err != nil || len(refs) != 1 {
			t.Fatalf("%s: references = %v, %v", enc, refs, err)
		}
		var last Position
		if err := c.Call(context.Background(), "last", nil, &last); err != nil {
			t.Fatal(err)
		}
		sent, start, end := 6, 9, 6
		if enc == "utf-8" {
			sent, start, end = 9, 9, 3
		}
		if last.Character != sent || refs[0].Range.Start.Character != start || refs[0].Range.End.Character != end {
			t.Errorf("%s: sent column %d, read %d to %d; want %d, then %d to %d", enc, last.Character, refs[0].Range.Start.Character, refs[0].Range.End.Character, sent, start, end)
		}
	}
}

func TestClientOpensAFileWithItsTextOnDisk(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.rs")
	if err := os.WriteFile(src, []byte("fn main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := startFake(t)
	if err := c.Open(src, "rust"); err != nil {
		t.Fatal(err)
	}
	var text string
	if err := c.Call(context.Background(), "opened", nil, &text); err != nil || text != "fn main() {}" {
		t.Errorf("the server holds %q (%v), want the file's text", text, err)
	}
}

func TestClientWaitsOutAServerThatReportsNoProgress(t *testing.T) {
	c := startServer(t, Server{Env: []string{"FAKE_QUIET=1"}, Ready: 5 * time.Second, Quiet: 300 * time.Millisecond})
	start := time.Now()
	if err := c.Wait(context.Background(), 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 250*time.Millisecond || d > 3*time.Second {
		t.Errorf("Wait returned after %s, want just past the quiet period", d)
	}
	deaf := startServer(t, Server{Env: []string{"FAKE_QUIET=1"}, Ready: 500 * time.Millisecond})
	if err := deaf.Wait(context.Background(), 50*time.Millisecond); err == nil {
		t.Error("Wait returned for a silent server with no quiet period")
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
