package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A false the human chose stays in project.yaml through a save, so setup
// does not ask again; a project that never chose writes no line.
func TestAChosenOverviewCutSurvivesASave(t *testing.T) {
	for text, want := range map[string]string{"overview_cut: false\n": "overview_cut: false", "overview_cut: true\n": "overview_cut: true", "name: x\n": ""} {
		root := t.TempDir()
		st := Open(root)
		if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, Dir, "project.yaml"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := st.LoadProject()
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SaveProject(p); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(root, Dir, "project.yaml"))
		if got := string(b); want == "" && strings.Contains(got, "overview_cut") || want != "" && !strings.Contains(got, want) {
			t.Errorf("%q saved as %q, want %q", text, got, want)
		}
	}
}
