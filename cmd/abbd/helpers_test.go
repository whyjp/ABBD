package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test binary doubles as the project extraction script: with
// ABBD_FAKE_EXTRACT set it prints that file.
func TestMain(m *testing.M) {
	if p := os.Getenv("ABBD_FAKE_EXTRACT"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// {{RUN}} plays a script whose output changes on every run.
		if bytes.Contains(b, []byte("{{RUN}}")) {
			n, _ := os.ReadFile(p + ".n")
			n = append(n, 'x')
			os.WriteFile(p+".n", n, 0o644)
			b = bytes.ReplaceAll(b, []byte("{{RUN}}"), n)
		}
		b = bytes.ReplaceAll(b, []byte("{{PYTHONHASHSEED}}"), []byte(os.Getenv("PYTHONHASHSEED")))
		os.Stdout.Write(b)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type project struct {
	t          *testing.T
	root       string
	extraction string
}

// bare is a code-only repository with no .abbd at all, and the path of the
// fake extraction script.
func bare(t *testing.T) (*project, string) {
	t.Helper()
	p := &project{t: t, root: newRepo(t), extraction: filepath.Join(t.TempDir(), "extraction.json")}
	t.Setenv("ABBD_FAKE_EXTRACT", p.extraction)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return p, exe
}

// extract sets what the extraction script reports next time it runs.
func (p *project) extract(json string) { write(p.t, p.extraction, json) }

func (p *project) code(rel, content string) { write(p.t, filepath.Join(p.root, rel), content) }

func (p *project) cmd(stdin string, args ...string) (int, string) {
	p.t.Helper()
	var out, errb bytes.Buffer
	code := run(append(args, "--root", p.root), strings.NewReader(stdin), &out, &errb)
	return code, out.String() + errb.String()
}

func (p *project) want(code int, contains string, args ...string) string {
	p.t.Helper()
	got, out := p.cmd("", args...)
	if got != code || !strings.Contains(out, contains) {
		p.t.Fatalf("abbd %v: want exit %d containing %q, got exit %d:\n%s", args, code, contains, got, out)
	}
	return out
}

func (p *project) read(rel string) string {
	p.t.Helper()
	b, err := os.ReadFile(filepath.Join(p.root, ".abbd", rel))
	if err != nil {
		p.t.Fatal(err)
	}
	return string(b)
}

func (p *project) file(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	write(t, path, content)
	return path
}

// neverRead fails the test if a command reads its stdin, which is how a CLI asks.
type neverRead struct{ t *testing.T }

func (r neverRead) Read([]byte) (int, error) {
	r.t.Fatal("setup must never ask: it read stdin")
	return 0, nil
}

func (p *project) setup(want int, contains string, args ...string) string {
	p.t.Helper()
	var out, errb bytes.Buffer
	code := run(append([]string{"setup", "--root", p.root}, args...), neverRead{p.t}, &out, &errb)
	got := out.String() + errb.String()
	if code != want || !strings.Contains(got, contains) {
		p.t.Fatalf("abbd setup %v: want exit %d containing %q, got exit %d:\n%s", args, want, contains, code, got)
	}
	return got
}

// units returns the unit files only.
func units(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range snapshot(t, root) {
		if strings.HasPrefix(k, "architecture/") || strings.HasPrefix(k, "features/") {
			out[k] = v
		}
	}
	return out
}

func equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
