package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Snapshotting a subdirectory of the repository keeps the paths of a
// submodule and of a nested repository with no commit under that
// subdirectory, as git add keeps every other file's.
func TestASubdirectorySnapshotKeepsItsPrefix(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, filepath.Join(root, "sub", "f.go"), "package sub\n")
	write(t, filepath.Join(root, "sub", "n", "a.go"), "package n\n")
	git(t, filepath.Join(root, "sub", "n"), "init", "-q")
	write(t, filepath.Join(root, "sub", "m", "b.go"), "package m\n")
	m := filepath.Join(root, "sub", "m")
	git(t, m, "init", "-q")
	git(t, m, "add", "-A")
	git(t, m, "commit", "-qm", "m")
	git(t, root, "add", "sub/m")

	tree, err := Snapshot(filepath.Join(root, "sub"), ".abbd")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(git(t, root, "ls-tree", "-r", "--name-only", tree))
	for _, want := range []string{"sub/f.go", "sub/n/a.go", "sub/m"} {
		if !slices.Contains(got, want) {
			t.Errorf("snapshot lacks %s: %v", want, got)
		}
	}
}
