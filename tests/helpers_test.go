package tests

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/cccv/internal/cli"
)

// project is a scratch target repository driven through the real CLI entry point.
type project struct {
	t   *testing.T
	dir string
}

func newProject(t *testing.T, sample string) *project {
	t.Helper()
	dir := t.TempDir()
	if sample != "" {
		src := filepath.Join("..", "samples", sample)
		err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			if d.IsDir() {
				return os.MkdirAll(filepath.Join(dir, rel), 0o755)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	p := &project{t: t, dir: dir}
	p.git("init", "-q")
	if sample != "" {
		p.commit("v0")
	}
	return p
}

func (p *project) git(args ...string) string {
	p.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=e2e@test", "-c", "user.name=e2e", "-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = p.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		p.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (p *project) commit(msg string) {
	p.git("add", "-A", "--", ".", ":(exclude).cccv")
	p.git("commit", "-q", "--allow-empty", "-m", msg)
}

// run executes `cccv --root DIR --json ARGS...` in-process and decodes the result.
func (p *project) run(args ...string) (map[string]any, int, string) {
	p.t.Helper()
	var out, errb bytes.Buffer
	code := cli.Main(append([]string{"--root", p.dir, "--json"}, args...), &out, &errb)
	var v map[string]any
	if out.Len() > 0 {
		var decoded any
		if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
			p.t.Fatalf("cccv %v: output is not JSON: %v\n%s", args, err, out.String())
		}
		if m, ok := decoded.(map[string]any); ok {
			v = m
		} else {
			v = map[string]any{"list": decoded}
		}
	}
	return v, code, errb.String()
}

// ok runs a command that must exit 0.
func (p *project) ok(args ...string) map[string]any {
	p.t.Helper()
	v, code, stderr := p.run(args...)
	if code != 0 {
		p.t.Fatalf("cccv %v exited %d: %s\n%v", args, code, stderr, v)
	}
	return v
}

func (p *project) write(rel, content string) {
	p.t.Helper()
	path := filepath.Join(p.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func (p *project) read(rel string) string {
	p.t.Helper()
	b, err := os.ReadFile(filepath.Join(p.dir, filepath.FromSlash(rel)))
	if err != nil {
		p.t.Fatal(err)
	}
	return string(b)
}

func (p *project) replace(rel, old, new string) {
	p.t.Helper()
	s := p.read(rel)
	if !strings.Contains(s, old) {
		p.t.Fatalf("%s does not contain %q", rel, old)
	}
	p.write(rel, strings.Replace(s, old, new, 1))
}

func get(v any, path ...string) any {
	for _, k := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any, path ...string) string {
	s, _ := get(v, path...).(string)
	return s
}

func list(v any, path ...string) []any {
	l, _ := get(v, path...).([]any)
	return l
}

func strs(v any, path ...string) []string {
	var out []string
	for _, x := range list(v, path...) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func has(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func (p *project) assertAllCurrent(when string) {
	p.t.Helper()
	st := p.ok("status")
	for _, k := range []string{"feature_map", "architecture_map", "graph", "html"} {
		if got := str(st, "freshness", k); got != "CURRENT" {
			p.t.Fatalf("%s: freshness.%s = %s, want CURRENT (%v)", when, k, got, get(st, "freshness"))
		}
	}
	chk := p.ok("check")
	if get(chk, "ok") != true {
		p.t.Fatalf("%s: check failed: %v", when, get(chk, "errors"))
	}
}

// findContract returns the ID of the contract with the given selector.
func (p *project) findContract(selector string) string {
	p.t.Helper()
	for _, c := range list(p.ok("contracts", "list"), "list") {
		if str(c, "selector") == selector && str(c, "status") != "removed" {
			return str(c, "id")
		}
	}
	p.t.Fatalf("no contract with selector %q", selector)
	return ""
}

func (p *project) featureFor(selector string) string {
	p.t.Helper()
	cid := p.findContract(selector)
	ctx := p.ok("inspect", cid)
	used := strs(ctx, "used_by")
	if len(used) == 0 {
		p.t.Fatalf("contract %s (%s) is used by no feature", cid, selector)
	}
	return used[0]
}
