// Package serve is `abbd serve`: a localhost-only server for the review screen that writes
// decisions straight to .abbd/decisions (PRD v1.4 08B section 6, the default channel).
package serve

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/whyjp/abbd/internal/workflow"
)

func NewToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Handler serves the page and the decision API. Every POST must carry the session token that
// only the served page knows, so another site open in the same browser cannot post to it.
func Handler(e *workflow.Engine, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page, err := e.RenderReview(r.URL.Query().Get("chg"), token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, page)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		sum, err := e.Review(r.URL.Query().Get("chg"))
		reply(w, sum, err)
	})
	mux.HandleFunc("POST /api/decide", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-ABBD-Token") != token {
			http.Error(w, "missing or wrong session token", http.StatusForbidden)
			return
		}
		var in workflow.DeltaDecisionInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		in.Channel = "serve"
		d, err := e.DecideDelta(in)
		reply(w, d, err)
	})
	return loopbackHost(mux)
}

// loopbackHost rejects requests whose Host is not a loopback name, which stops DNS rebinding
// from reading the page (and its token) through a foreign domain.
func loopbackHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "abbd serve answers loopback hosts only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(v)
}

// Listen refuses any address that is not loopback: the review server is local by design.
func Listen(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("abbd serve listens on loopback only, not %q", host)
	}
	return net.Listen("tcp", addr)
}

func Run(e *workflow.Engine, addr string, out io.Writer) error {
	ln, err := Listen(addr)
	if err != nil {
		return err
	}
	url := "http://" + strings.Replace(ln.Addr().String(), "127.0.0.1", "localhost", 1) + "/"
	fmt.Fprintf(out, "abbd review: %s (Ctrl+C to stop)\n", url)
	return http.Serve(ln, Handler(e, NewToken()))
}
