package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
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

// A long registration table does not crowd out the load beside it.
func TestBindingSitesCapEachKind(t *testing.T) {
	root := t.TempDir()
	text := strings.Repeat("obs_register_source(&info);\n", 12) + "void *h = os_dlopen(path);\nunregister_all();\n"
	if err := os.WriteFile(filepath.Join(root, "obs.c"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib.so"), []byte("\x00ELF dlopen"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := bindingSites(root, []string{"obs.c", "lib.so"})
	all := strings.Join(got, "\n")
	if strings.Count(all, "(registry)") != bindingShown || !strings.Contains(all, "`obs.c:13`") || got[len(got)-1] != "and 7 more" {
		t.Errorf("want %d registry sites, the load at line 13 and 7 more:\n%s", bindingShown, all)
	}
	if strings.Contains(all, "unregister") || strings.Contains(all, "lib.so") {
		t.Errorf("listed an unregister call or a binary file:\n%s", all)
	}
}

func TestLoadAndSpawnAreVias(t *testing.T) {
	for v, word := range map[model.Via]string{"load": "로드", "spawn": "실행"} {
		if err := v.Validate(); err != nil || v.Word() != word {
			t.Errorf("via %s: %v, word %q", v, err, v.Word())
		}
	}
}
