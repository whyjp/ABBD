package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The brief lists the lines that bind at run time, which no import shows,
// and leaves out tests and prose.
func TestBindingSitesNameLoadsSpawnsAndRegistries(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"obs/module.c":     "void load(void) {\n\tvoid *h = os_dlopen(path);\n}\n",
		"ffmpeg/mux.c":     "int run(void) { return os_process_pipe_create(cmd, \"w\") != NULL; }\nFILE *p = popen(cmd, \"r\");\n",
		"sources/amqp.rs":  "#[typetag::serde(name = \"amqp\")]\nimpl SourceConfig for AmqpConfig {}\n",
		"app/run.go":       "cmd := exec.Command(\"git\", \"status\")\n",
		"app/run_test.go":  "cmd := exec.Command(\"git\")\n",
		"README.md":        "Call dlopen to load it.\n",
		"plain/nothing.go": "func add(a, b int) int { return a + b }\n",
	}
	var paths []string
	for p, text := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	got := strings.Join(bindingSites(root, paths), "\n")
	for _, want := range []string{"`obs/module.c:2`: `void *h = os_dlopen(path);` (load)", "`ffmpeg/mux.c:2`: `FILE *p = popen(cmd, \"r\");` (spawn)", "`sources/amqp.rs:1`", "(registry)", "`app/run.go:1`"} {
		if !strings.Contains(got, want) {
			t.Errorf("binding sites lack %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"run_test.go", "README.md", "nothing.go"} {
		if strings.Contains(got, not) {
			t.Errorf("binding sites list %s:\n%s", not, got)
		}
	}
}
