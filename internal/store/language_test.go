package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whyjp/abbd/internal/model"
)

func projectWith(t *testing.T, text string) (Store, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, Dir, "project.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return Open(root), filepath.Join(root, Dir, "project.yaml")
}

// project.yaml's language loads as a canonical BCP-47 tag, a project that
// sets none takes the OS's, and a tag that is none stops the load.
func TestProjectLanguageIsAValidatedTag(t *testing.T) {
	st, path := projectWith(t, "name: x\nlanguage: ko_kr\n")
	p, err := st.LoadProject()
	if err != nil || p.Language != "ko-KR" || p.Lang() != "ko-KR" {
		t.Fatalf("language %q, Lang %q, %v; want ko-KR", p.Language, p.Lang(), err)
	}
	if err := st.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "language: ko-KR\n") {
		t.Errorf("saved %q, want language: ko-KR", b)
	}

	st, _ = projectWith(t, "name: x\n")
	if p, err := st.LoadProject(); err != nil || p.Language != "" || p.Lang() != model.DefaultLanguage() {
		t.Errorf("unset language: %q, Lang %q, %v; want none and the OS's %s", p.Language, p.Lang(), err, model.DefaultLanguage())
	}

	st, _ = projectWith(t, "language: korean\n")
	if _, err := st.LoadProject(); err == nil || !strings.Contains(err.Error(), `"korean" is no BCP-47 tag`) {
		t.Errorf("a bad tag loaded: %v", err)
	}
}
