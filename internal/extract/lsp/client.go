// Package lsp is the one language-server client abbd's relaters share. It
// runs a server over stdio for one batch read: initialize, wait until the
// server has indexed the workspace, ask the questions, shut down. It answers
// nothing interactively and never edits a file.
package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// Server says how to run one language server and what to tell it.
type Server struct {
	// Argv runs the server; Argv[0] is a path or a name on the PATH.
	Argv []string
	// Env is added to the process environment.
	Env []string
	// Root is the absolute workspace directory the server reads.
	Root string
	// Init is the initializationOptions. Settings answers each
	// workspace/configuration item by its section, so a server that pulls
	// its settings reads the same options it was started with.
	Init     any
	Settings map[string]any
	// Timeout bounds one request; Ready bounds the wait for indexing.
	Timeout, Ready time.Duration
	// Until is the title of the progress that ends the server's indexing,
	// such as rust-analyzer's Indexing, for a server that goes idle between
	// loading phases: Wait returns only after one has ended.
	Until string
	// Quiet is how long after initialized a server that has sent no
	// progress or status counts as ready, for a server that reports
	// neither. Zero waits for one.
	Quiet time.Duration
	// TextDocument adds the server's own text document capabilities, such
	// as an extension it sends notifications for only when asked.
	TextDocument map[string]any
	// Note receives every notification the client does not track itself.
	Note func(method string, params json.RawMessage)
}

// Client is a running server.
type Client struct {
	cmd *exec.Cmd
	// exited closes once the process has exited and its stderr is copied.
	exited chan struct{}
	stdin  io.WriteCloser
	stderr *tail
	srv    Server

	wmu sync.Mutex

	mu      sync.Mutex
	next    int64
	pending map[int64]chan response
	dead    error
	// work holds the progress tokens begun and not yet ended, with their
	// titles; indexed is whether a progress titled Until has ended.
	work    map[string]string
	indexed bool
	// quiet is the server's own word that it is idle, for a server that
	// says so; nil until it first does.
	quiet   *bool
	stirred bool
	changed chan struct{}
	started time.Time

	// utf16 is whether the server counts columns in UTF-16 units, the
	// protocol's default. The typed requests then convert every position
	// they send or read, so callers always count bytes. lines caches the
	// files read for that.
	utf16 bool
	lines sync.Map
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

// RPCError is an error the server answered a request with.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("%s (%d)", e.Message, e.Code) }

// The protocol's codes for a request the server dropped because the
// workspace moved under it; the same request asked again succeeds.
const (
	contentModified = -32801
	serverCancelled = -32802
	internalError   = -32603
)

// transient is whether asking again can succeed: the server dropped the
// request because its state moved while it answered.
func transient(err error) bool {
	var e *RPCError
	if !errors.As(err, &e) {
		return false
	}
	return e.Code == contentModified || e.Code == serverCancelled ||
		e.Code == internalError && (strings.Contains(e.Message, "mid-query") || strings.Contains(e.Message, "cancelled"))
}

// Start runs the server and initializes it over the workspace root.
func Start(ctx context.Context, s Server) (*Client, error) {
	cmd := exec.Command(s.Argv[0], s.Argv[1:]...)
	cmd.Dir = s.Root
	cmd.Env = append(os.Environ(), s.Env...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c := &Client{cmd: cmd, exited: make(chan struct{}), stdin: in, stderr: &tail{}, srv: s, pending: map[int64]chan response{}, work: map[string]string{}, changed: make(chan struct{})}
	cmd.Stderr = c.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		cmd.Wait()
		close(c.exited)
	}()
	go c.read(bufio.NewReaderSize(out, 1<<20))
	caps := map[string]any{
		"window": map[string]any{"workDoneProgress": true},
		"textDocument": map[string]any{
			"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
			"references":     map[string]any{},
			"implementation": map[string]any{"linkSupport": false},
			"definition":     map[string]any{"linkSupport": false},
			"diagnostic":     map[string]any{},
		},
		"workspace":    map[string]any{"configuration": true},
		"general":      map[string]any{"positionEncodings": []string{"utf-8", "utf-16"}},
		"experimental": map[string]any{"serverStatusNotification": true},
	}
	td := caps["textDocument"].(map[string]any)
	for k, v := range s.TextDocument {
		td[k] = v
	}
	root := URI(s.Root)
	params := map[string]any{
		"processId": os.Getpid(), "rootUri": root, "capabilities": caps,
		"workspaceFolders":      []any{map[string]any{"uri": root, "name": "root"}},
		"initializationOptions": s.Init,
	}
	var res struct {
		Capabilities struct {
			PositionEncoding string `json:"positionEncoding"`
		} `json:"capabilities"`
	}
	if err := c.Call(ctx, "initialize", params, &res); err != nil {
		c.Close()
		return nil, err
	}
	c.utf16 = res.Capabilities.PositionEncoding != "utf-8"
	c.started = time.Now()
	if err := c.Notify("initialized", map[string]any{}); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// Call sends a request and decodes its result into result, which may be
// nil. A request the server drops while its state moves is asked again.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	var err error
	for try := range 6 {
		if try > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(try) * 500 * time.Millisecond):
			}
		}
		err = c.call(ctx, method, params, result)
		if !transient(err) {
			return err
		}
	}
	return err
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	if c.srv.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.srv.Timeout)
		defer cancel()
	}
	ch := make(chan response, 1)
	c.mu.Lock()
	if c.dead != nil {
		c.mu.Unlock()
		return c.dead
	}
	c.next++
	id := c.next
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return c.deadErr()
		}
		if r.Error != nil {
			return fmt.Errorf("%s: %w", method, r.Error)
		}
		if result == nil || len(r.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(r.Result, result); err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		c.Notify("$/cancelRequest", map[string]any{"id": id})
		return fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// Notify sends a notification.
func (c *Client) Notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *Client) send(msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n%s", len(b), b); err != nil {
		return c.deadErr()
	}
	return nil
}

func (c *Client) deadErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead != nil {
		return c.dead
	}
	return errors.New("the language server closed its pipe")
}

// read dispatches each message the server sends until its stdout closes.
func (c *Client) read(r *bufio.Reader) {
	for {
		b, err := frame(r)
		if err != nil {
			c.die(err)
			return
		}
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			response
		}
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		switch {
		case m.Method != "" && m.ID != nil:
			c.answer(m.ID, m.Method, m.Params)
		case m.Method != "":
			c.note(m.Method, m.Params)
		default:
			id, err := strconv.ParseInt(string(m.ID), 10, 64)
			if err != nil {
				continue
			}
			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- m.response
			}
		}
	}
}

// frame reads one Content-Length framed message body.
func frame(r *bufio.Reader) ([]byte, error) {
	n := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Content-Length") {
			if n, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
				return nil, fmt.Errorf("bad Content-Length %q", v)
			}
		}
	}
	if n < 0 {
		return nil, errors.New("a message without Content-Length")
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}

func (c *Client) die(err error) {
	select {
	case <-c.exited:
	case <-time.After(5 * time.Second):
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead == nil {
		c.dead = fmt.Errorf("the language server stopped (%v): %s", err, c.stderr.String())
	}
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.poke()
}

// answer replies to a request from the server: its settings for
// workspace/configuration, and an empty success for anything else, such as
// window/workDoneProgress/create.
func (c *Client) answer(id json.RawMessage, method string, params json.RawMessage) {
	var result any
	if method == "workspace/configuration" {
		var p struct {
			Items []struct {
				Section string `json:"section"`
			} `json:"items"`
		}
		json.Unmarshal(params, &p)
		out := make([]any, len(p.Items))
		for i, it := range p.Items {
			out[i] = c.srv.Settings[it.Section]
		}
		result = out
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

// note tracks the server's progress reports, which say when it is busy,
// and hands any other notification to Server.Note.
func (c *Client) note(method string, params json.RawMessage) {
	if method != "$/progress" && method != "experimental/serverStatus" {
		if c.srv.Note != nil {
			c.srv.Note(method, params)
		}
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch method {
	case "$/progress":
		var p struct {
			Token json.RawMessage `json:"token"`
			Value struct {
				Kind  string `json:"kind"`
				Title string `json:"title"`
			} `json:"value"`
		}
		if json.Unmarshal(params, &p) != nil {
			return
		}
		switch p.Value.Kind {
		case "begin":
			c.work[string(p.Token)] = p.Value.Title
			c.stirred = true
		case "end":
			if t, ok := c.work[string(p.Token)]; ok && t == c.srv.Until {
				c.indexed = true
			}
			delete(c.work, string(p.Token))
		}
	case "experimental/serverStatus":
		var p struct {
			Quiescent bool `json:"quiescent"`
		}
		if json.Unmarshal(params, &p) != nil {
			return
		}
		c.quiet = &p.Quiescent
		c.stirred = true
	default:
		return
	}
	c.poke()
}

// poke wakes Wait; the caller holds mu.
func (c *Client) poke() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// Wait blocks until the server has begun work and finished it: no progress
// is open, the Until progress has ended, and a server that reports its
// status says it is quiescent. A server silent for Server.Quiet counts as
// having begun and finished. The state must hold for settle, since a server
// opens its next phase just after closing one.
func (c *Client) Wait(ctx context.Context, settle time.Duration) error {
	if c.srv.Ready > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.srv.Ready)
		defer cancel()
	}
	for {
		c.mu.Lock()
		if c.dead != nil {
			c.mu.Unlock()
			return c.dead
		}
		silent := c.srv.Quiet - time.Since(c.started)
		begun := c.stirred || c.srv.Quiet > 0 && silent <= 0
		idle := begun && len(c.work) == 0 && (c.quiet == nil || *c.quiet) && (c.srv.Until == "" || c.indexed)
		changed := c.changed
		c.mu.Unlock()
		var settled, hushed <-chan time.Time
		if idle {
			settled = time.After(settle)
		} else if !begun && c.srv.Quiet > 0 {
			hushed = time.After(silent)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the language server did not finish indexing: %w", ctx.Err())
		case <-changed:
		case <-hushed:
		case <-settled:
			return nil
		}
	}
}

// Close shuts the server down, and kills it if it does not exit in time.
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c.call(ctx, "shutdown", nil, nil)
	c.Notify("exit", nil)
	c.stdin.Close()
	select {
	case <-c.exited:
	case <-time.After(10 * time.Second):
		c.cmd.Process.Kill()
		<-c.exited
	}
	return nil
}

// Pid is the server process's id, for a caller that measures it.
func (c *Client) Pid() int { return c.cmd.Process.Pid }

// Open tells the server a file is open with its text on disk, for a server
// that answers only about open files.
func (c *Client) Open(abs, language string) error {
	b, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	return c.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": URI(abs), "languageId": language, "version": 1, "text": string(b),
	}})
}

// DocumentSymbols are the declarations in a file, nested.
func (c *Client) DocumentSymbols(ctx context.Context, abs string) ([]DocumentSymbol, error) {
	var out []DocumentSymbol
	err := c.Call(ctx, "textDocument/documentSymbol", map[string]any{"textDocument": doc(abs)}, &out)
	uri := URI(abs)
	var conv func(list []DocumentSymbol)
	conv = func(list []DocumentSymbol) {
		for i := range list {
			list[i].Range = c.fromServer(uri, list[i].Range)
			list[i].SelectionRange = c.fromServer(uri, list[i].SelectionRange)
			conv(list[i].Children)
		}
	}
	conv(out)
	return out, err
}

// Diagnostics are what the server finds in a file when asked for them, by
// textDocument/diagnostic.
func (c *Client) Diagnostics(ctx context.Context, abs string) ([]Diagnostic, error) {
	var out struct {
		Items []Diagnostic `json:"items"`
	}
	err := c.Call(ctx, "textDocument/diagnostic", map[string]any{"textDocument": doc(abs)}, &out)
	for i := range out.Items {
		out.Items[i].Range = c.fromServer(URI(abs), out.Items[i].Range)
	}
	return out.Items, err
}

// References are the places that name the symbol at pos, its own
// declaration left out.
func (c *Client) References(ctx context.Context, abs string, pos Position) ([]Location, error) {
	var out []Location
	err := c.Call(ctx, "textDocument/references", map[string]any{"textDocument": doc(abs), "position": c.toServer(URI(abs), pos), "context": map[string]any{"includeDeclaration": false}}, &out)
	c.locate(out)
	return out, err
}

// Implementations are the places that implement the trait, interface or
// method at pos.
func (c *Client) Implementations(ctx context.Context, abs string, pos Position) ([]Location, error) {
	return c.locations(ctx, "textDocument/implementation", abs, pos)
}

// Definitions are where the name at pos is declared.
func (c *Client) Definitions(ctx context.Context, abs string, pos Position) ([]Location, error) {
	return c.locations(ctx, "textDocument/definition", abs, pos)
}

// locations reads a result that is one Location, a list of them, or null.
func (c *Client) locations(ctx context.Context, method, abs string, pos Position) ([]Location, error) {
	var raw json.RawMessage
	if err := c.Call(ctx, method, map[string]any{"textDocument": doc(abs), "position": c.toServer(URI(abs), pos)}, &raw); err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '{' {
		raw = append(append([]byte{'['}, raw...), ']')
	}
	var out []Location
	err := json.Unmarshal(raw, &out)
	c.locate(out)
	return out, err
}

// locate turns the server's columns in each location into bytes.
func (c *Client) locate(out []Location) {
	for i := range out {
		out[i].Range = c.fromServer(out[i].URI, out[i].Range)
	}
}

func (c *Client) toServer(uri string, p Position) Position {
	if l, ok := c.line(uri, p.Line); ok {
		p.Character = utf16Column(l, p.Character)
	}
	return p
}

func (c *Client) fromServer(uri string, r Range) Range {
	for _, p := range []*Position{&r.Start, &r.End} {
		if l, ok := c.line(uri, p.Line); ok {
			p.Character = byteColumn(l, p.Character)
		}
	}
	return r
}

// line is a line of the file a URI names, for a UTF-16 server only. Each
// file is read once; one that cannot be read keeps the server's columns.
func (c *Client) line(uri string, n int) (string, bool) {
	if !c.utf16 {
		return "", false
	}
	v, ok := c.lines.Load(uri)
	if !ok {
		var lines []string
		if abs, err := Path(uri); err == nil {
			if b, err := os.ReadFile(abs); err == nil {
				lines = strings.Split(string(b), "\n")
			}
		}
		v, _ = c.lines.LoadOrStore(uri, lines)
	}
	lines := v.([]string)
	if n < 0 || n >= len(lines) {
		return "", false
	}
	return lines[n], true
}

// utf16Column counts the UTF-16 units before a byte column of a line.
func utf16Column(line string, col int) int {
	n := 0
	for i, r := range line {
		if i >= col {
			break
		}
		n += utf16.RuneLen(r)
	}
	return n
}

// byteColumn is the byte column after the first units UTF-16 units of a
// line.
func byteColumn(line string, units int) int {
	n := 0
	for i, r := range line {
		if n >= units {
			return i
		}
		n += utf16.RuneLen(r)
	}
	return len(line)
}

func doc(abs string) map[string]any { return map[string]any{"uri": URI(abs)} }

// tail keeps the last few KB a server wrote to stderr, for the error that
// says why it stopped.
type tail struct {
	mu sync.Mutex
	b  []byte
}

const tailMax = 4 << 10

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > tailMax {
		t.b = t.b[len(t.b)-tailMax:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.b))
}
