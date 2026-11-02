package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type xunit struct {
	Ref, Kind, Name, Signature, Digest string
}
type xrel struct{ From, To, Kind string }
type xresult struct {
	Units     []xunit
	Relations []xrel
}

// repoCopy copies the files the project extractor reads into a temp dir.
func repoCopy(t *testing.T) string {
	t.Helper()
	src, _ := filepath.Abs(filepath.Join("..", ".."))
	dst := t.TempDir()
	for _, rel := range []string{"go.mod", "go.sum", "cmd", "internal", "plugins", ".claude-plugin", ".abbd/extractor"} {
		from := filepath.Join(src, rel)
		filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			r, _ := filepath.Rel(src, p)
			b, _ := os.ReadFile(p)
			write(t, filepath.Join(dst, r), string(b))
			return nil
		})
	}
	return dst
}

func runExtractor(t *testing.T, dir string) (string, xresult) {
	t.Helper()
	cmd := exec.Command("go", "run", ".abbd/extractor/extract.go")
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("extractor: %v\n%s", err, errb.String())
	}
	var r xresult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return out.String(), r
}

func editFile(t *testing.T, path, old, new string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), old) {
		t.Fatalf("%s: %q not found (%v)", path, old, err)
	}
	write(t, path, strings.Replace(string(b), old, new, 1))
}

// A new command changes no existing contract: it is one new contract the
// CLI provides, so the repository's own diff shows exactly that.
func TestANewCommandIsOneNewContract(t *testing.T) {
	dir := repoCopy(t)
	_, r1 := runExtractor(t, dir)
	editFile(t, filepath.Join(dir, "cmd", "abbd", "main.go"), "\t\"status\":",
		"\t\"ok\": {flags: []string{\"root\"}, pos: []int{0}, run: func(e env, _ []string) error { return e.ok() }},\n\t\"status\":")
	write(t, filepath.Join(dir, "cmd", "abbd", "ok.go"), "package main\n\nfunc (e env) ok() error { return e.status() }\n")
	_, r2 := runExtractor(t, dir)
	old := map[string]xunit{}
	for _, u := range r1.Units {
		old[u.Ref] = u
	}
	var added []string
	for _, u := range r2.Units {
		o, ok := old[u.Ref]
		switch {
		case !ok:
			added = append(added, u.Ref)
		case o.Signature != u.Signature || o.Digest != u.Digest:
			t.Fatalf("%s changed: %+v -> %+v", u.Ref, o, u)
		}
	}
	if len(added) != 1 || added[0] != "if-cli-ok" {
		t.Fatalf("want one new contract if-cli-ok, got %v", added)
	}
	provided := false
	for _, r := range r2.Relations {
		provided = provided || r == (xrel{"mod-cmd-abbd", "if-cli-ok", "provides"})
	}
	if !provided {
		t.Fatalf("the CLI should provide the new command")
	}
}

// A two-word command is read within one line, so the next line's command
// still counts.
func TestSkillCommandsOnConsecutiveLinesAreEachConsumed(t *testing.T) {
	dir := repoCopy(t)
	write(t, filepath.Join(dir, "plugins", "abbd", "skills", "zz", "SKILL.md"),
		"---\nname: zz\n---\n\n```bash\nabbd status\nabbd record unit MOD x --evidence a.go:1:x\n```\n")
	_, r := runExtractor(t, dir)
	got := map[string]bool{}
	for _, rel := range r.Relations {
		if rel.From == "feat-zz" {
			got[rel.To] = true
		}
	}
	t.Logf("feat-zz consumes: %v", got)
	if !got["if-cli-status"] {
		t.Fatalf("status not found at all")
	}
	if !got["if-cli-record-unit"] {
		t.Errorf("feat-zz runs `abbd record unit` but is not its consumer")
	}
}

func TestExtractionIsStableAcrossRunsAndLineEndings(t *testing.T) {
	dir := repoCopy(t)
	a, _ := runExtractor(t, dir)
	b, _ := runExtractor(t, dir)
	if a != b {
		t.Fatalf("two runs differ")
	}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.Contains(filepath.ToSlash(p), ".abbd/extractor") &&
			(strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".md")) {
			c, _ := os.ReadFile(p)
			os.WriteFile(p, bytes.ReplaceAll(c, []byte("\n"), []byte("\r\n")), 0o644)
		}
		return nil
	})
	_, r1 := runExtractor(t, dir)
	var r0 xresult
	json.Unmarshal([]byte(a), &r0)
	old := map[string]xunit{}
	for _, u := range r0.Units {
		old[u.Ref] = u
	}
	for _, u := range r1.Units {
		if o := old[u.Ref]; o.Digest != u.Digest || o.Signature != u.Signature {
			t.Errorf("CRLF changed %s: digest %s -> %s, sig %q -> %q", u.Ref, o.Digest, u.Digest, o.Signature, u.Signature)
		}
	}
}
