package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"

	"github.com/whyjp/abbd/internal/screen"
)

// serve runs the human screen. Every decision the page sends is a CLI command
// run in this process, so the screen holds no gate logic of its own. A POST
// needs the token this launch printed and the screen's own origin. The token
// lives only in this process, the terminal that started it, and the tab's
// sessionStorage: it travels in the link's fragment, which a browser never
// sends, so no file, cookie or request line holds it. The address goes to a
// file so the guard can deny the agent's requests to the screen.
func (e env) serve() error {
	host, _, err := net.SplitHostPort(e.addr)
	if err != nil {
		return fmt.Errorf("--addr %q: %w", e.addr, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--addr %q: the screen listens on a loopback address only, such as 127.0.0.1:7340", e.addr)
	}
	ln, err := net.Listen("tcp", e.addr)
	if err != nil {
		return err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	s := &screenServer{env: e, token: hex.EncodeToString(raw), addr: ln.Addr().String()}
	link := fmt.Sprintf("http://%s/#t=%s", s.authority(), s.token)
	file := e.st.ServeFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(s.addr+"\n"), 0o644); err != nil {
		return err
	}
	defer os.Remove(file)
	fmt.Fprintf(e.stdout, "abbd screen: %s\nopen it in your browser; Ctrl-C stops it\n", link)

	srv := &http.Server{Handler: s}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type screenServer struct {
	env   env
	token string
	addr  string
	// mu runs one decision at a time, and no page reads the store mid-decision.
	mu sync.Mutex
}

// actions turn a form into the CLI command it stands for. A key reaches the
// command line as a positional argument, so it may not look like a flag.
var actions = map[string]func(f url.Values) ([]string, error){
	"approve": func(f url.Values) ([]string, error) { return keyed(f, "approve") },
	"reject": func(f url.Values) ([]string, error) {
		return keyed(f, "reject", "--reason="+f.Get("reason"))
	},
	"confirm": func(f url.Values) ([]string, error) {
		return keyed(f, "confirm", valueFlag(f)...)
	},
	"confirm-reject": func(f url.Values) ([]string, error) {
		return keyed(f, "confirm", append([]string{"--reject"}, valueFlag(f)...)...)
	},
	"align": func(f url.Values) ([]string, error) {
		if len(f["id"]) == 0 {
			return nil, fmt.Errorf("설계 단위를 하나 이상 고르세요")
		}
		return []string{"align", "--confirm=" + strings.Join(f["id"], ",")}, nil
	},
	"realign": func(f url.Values) ([]string, error) {
		if len(f["key"]) == 0 {
			return nil, fmt.Errorf("인정할 REALIGN 행을 하나 이상 고르세요")
		}
		return []string{"align", "--confirm=" + strings.Join(f["key"], ",")}, nil
	},
	"couple": func(f url.Values) ([]string, error) {
		f.Set("key", f.Get("from")+"."+f.Get("kind")+"."+f.Get("to"))
		return keyed(f, "couple")
	},
}

func keyed(f url.Values, cmd string, flags ...string) ([]string, error) {
	key := f.Get("key")
	if key == "" || strings.HasPrefix(key, "-") {
		return nil, fmt.Errorf("%q는 항목 키가 아닙니다", key)
	}
	return append([]string{cmd, key}, flags...), nil
}

func valueFlag(f url.Values) []string {
	if v := strings.TrimSpace(f.Get("value")); v != "" {
		return []string{"--value=" + v}
	}
	return nil
}

func (s *screenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A page another site loads through a rebound DNS name arrives with that
	// name as its Host; only the loopback address this launch printed is served.
	if r.Host != s.addr && r.Host != s.authority() {
		http.Error(w, "알 수 없는 호스트", http.StatusForbidden)
		return
	}
	switch {
	case r.URL.Path == "/" && r.Method == http.MethodGet:
		s.page(w)
	case r.URL.Path == "/act" && r.Method == http.MethodPost:
		s.act(w, r)
	default:
		http.NotFound(w, r)
	}
}

// authority is the address as a browser writes it in Host and Origin, which
// leave out the default port.
func (s *screenServer) authority() string {
	if host, port, err := net.SplitHostPort(s.addr); err == nil && port == "80" {
		if strings.Contains(host, ":") {
			return "[" + host + "]"
		}
		return host
	}
	return s.addr
}

func (s *screenServer) valid(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

// fromScreen says whether a request came from the screen's own page. Another
// local server on any port is a different origin, so it can neither read the
// token from sessionStorage nor pass this check with a token it guessed.
func (s *screenServer) fromScreen(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	return r.Header.Get("Origin") == "http://"+s.authority()
}

func (s *screenServer) page(w http.ResponseWriter) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	o := screen.Options{Nonce: hex.EncodeToString(raw)}
	s.mu.Lock()
	in, err := s.env.screenInput()
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var b bytes.Buffer
	if err := screen.Render(&b, screen.Build(in), o); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'nonce-%s'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'; frame-ancestors 'none'; form-action 'self'", o.Nonce))
	w.Write(b.Bytes())
}

// act runs one decision and answers with what the CLI printed. Only the page
// that sent it shows that, so a read-only viewer never sees or clears it.
func (s *screenServer) act(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.fromScreen(r) || !s.valid(r.PostForm.Get("t")) {
		http.Error(w, "이 결정은 abbd serve가 출력한 링크로 연 화면에서만 보낼 수 있습니다", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	build, ok := actions[r.PostForm.Get("do")]
	if !ok {
		http.Error(w, "알 수 없는 동작", http.StatusBadRequest)
		return
	}
	argv, err := build(r.PostForm)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprintln(w, err)
		return
	}
	s.mu.Lock()
	var out bytes.Buffer
	code := run(append(argv, "--root", s.env.root), strings.NewReader(""), &out, &out)
	s.mu.Unlock()
	if code != exitOK && code != exitNeedHuman {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	fmt.Fprintf(w, "abbd %s\n%s\n", strings.Join(argv, " "), strings.TrimSpace(out.String()))
}
