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
// needs the token this launch printed. The token lives only in this process
// and the terminal that started it; no file holds it, so an agent cannot read
// it from disk. The address goes to a file so the guard can deny the agent's
// requests to the screen.
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
	link := fmt.Sprintf("http://%s/?t=%s", s.addr, s.token)
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
	// mu runs one decision at a time and guards the message the next page shows.
	mu      sync.Mutex
	flash   string
	failure bool
}

const tokenCookie = "abbd_t"

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
			return nil, fmt.Errorf("choose at least one design unit")
		}
		return []string{"align", "--confirm=" + strings.Join(f["id"], ",")}, nil
	},
	"realign": func(url.Values) ([]string, error) { return []string{"align"}, nil },
	"couple": func(f url.Values) ([]string, error) {
		f.Set("key", f.Get("from")+"."+f.Get("kind")+"."+f.Get("to"))
		return keyed(f, "couple")
	},
}

func keyed(f url.Values, cmd string, flags ...string) ([]string, error) {
	key := f.Get("key")
	if key == "" || strings.HasPrefix(key, "-") {
		return nil, fmt.Errorf("%q is not an item key", key)
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
	if r.Host != s.addr {
		http.Error(w, "unknown host", http.StatusForbidden)
		return
	}
	switch {
	case r.URL.Path == "/" && r.Method == http.MethodGet:
		s.page(w, r)
	case r.URL.Path == "/act" && r.Method == http.MethodPost:
		s.act(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *screenServer) valid(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

func (s *screenServer) page(w http.ResponseWriter, r *http.Request) {
	if t := r.URL.Query().Get("t"); t != "" {
		if s.valid(t) {
			http.SetCookie(w, &http.Cookie{Name: tokenCookie, Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	o := screen.Options{}
	if c, err := r.Cookie(tokenCookie); err == nil && s.valid(c.Value) {
		o.Token = s.token
	}
	s.mu.Lock()
	o.Flash, o.Failure = s.flash, s.failure
	s.flash, s.failure = "", false
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b.Bytes())
}

func (s *screenServer) act(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.valid(r.PostForm.Get("t")) {
		http.Error(w, "this decision needs the token abbd serve printed", http.StatusForbidden)
		return
	}
	build, ok := actions[r.PostForm.Get("do")]
	if !ok {
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	argv, err := build(r.PostForm)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.flash, s.failure = err.Error(), true
	} else {
		var out bytes.Buffer
		code := run(append(argv, "--root", s.env.root), strings.NewReader(""), &out, &out)
		s.flash = "abbd " + strings.Join(argv, " ") + "\n" + strings.TrimSpace(out.String())
		s.failure = code != exitOK && code != exitNeedHuman
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
