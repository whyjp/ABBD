package gitx

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here (on Windows, turn on developer mode): %v", err)
	}
}

// snapshot maps each path of tree, relative to the top of root's work tree,
// to its mode and the content of its blob.
func snapshot(t *testing.T, root, tree string) map[string]string {
	t.Helper()
	got := map[string]string{}
	for _, e := range strings.Split(strings.TrimSuffix(git(t, root, "ls-tree", "-r", "-z", tree), "\x00"), "\x00") {
		meta, path, _ := strings.Cut(e, "\t")
		f := strings.Fields(meta)
		got[path] = f[0] + " " + git(t, root, "cat-file", "blob", f[2])
	}
	return got
}

// A nested repository with no commit snapshots as git add would add its
// directory: a path its index records under a directory that is now a
// symlink is left out, and each symlink counts as a link to its target.
func TestANestedRepositorySnapshotKeepsSymlinksAsLinks(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	n := filepath.Join(root, "n")
	write(t, filepath.Join(n, "link", "f.go"), "package link\n")
	git(t, n, "init", "-q")
	git(t, n, "add", "link/f.go")
	if err := os.RemoveAll(filepath.Join(n, "link")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(n, "real", "f.go"), "package real\n")
	symlink(t, "real", filepath.Join(n, "link"))
	symlink(t, "real/f.go", filepath.Join(n, "alias.go"))

	tree, err := Snapshot(root, ".abbd")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"n/real/f.go": "100644 package real\n",
		"n/link":      "120000 real",
		"n/alias.go":  "120000 real/f.go",
	}
	if got := snapshot(t, root, tree); !maps.Equal(got, want) {
		t.Errorf("snapshot = %q, want %q", got, want)
	}
}

// A Windows junction in a nested repository is a directory to git, so the
// files below it are snapshotted as git add would record them.
func TestANestedRepositorySnapshotWalksAJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows only")
	}
	root := t.TempDir()
	git(t, root, "init", "-q")
	n := filepath.Join(root, "n")
	write(t, filepath.Join(n, "real", "f.go"), "package real\n")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(n, "junc"), filepath.Join(n, "real")).CombinedOutput(); err != nil {
		t.Skipf("cannot make a junction here: %v %s", err, out)
	}
	git(t, n, "init", "-q")

	tree, err := Snapshot(root, ".abbd")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"n/real/f.go": "100644 package real\n",
		"n/junc/f.go": "100644 package real\n",
	}
	if got := snapshot(t, root, tree); !maps.Equal(got, want) {
		t.Errorf("snapshot = %q, want %q", got, want)
	}
}

// A nested repository's file whose name holds a newline is snapshotted too.
func TestANestedRepositorySnapshotKeepsANewlineName(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	n := filepath.Join(root, "n")
	write(t, filepath.Join(n, "a.go"), "package a\n")
	if err := os.WriteFile(filepath.Join(n, "b\nc.go"), []byte("package b\n"), 0o644); err != nil {
		t.Skipf("cannot name a file with a newline here: %v", err)
	}
	git(t, n, "init", "-q")

	tree, err := Snapshot(root, ".abbd")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"n/a.go":    "100644 package a\n",
		"n/b\nc.go": "100644 package b\n",
	}
	if got := snapshot(t, root, tree); !maps.Equal(got, want) {
		t.Errorf("snapshot = %q, want %q", got, want)
	}
}

// Recorded names the pull request whose merge brought a quote in, and none
// for a quote committed on the main line before an unrelated merge.
func TestRecordedNamesThePullRequestThatBroughtAQuoteIn(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	write(t, filepath.Join(root, "u.md"), "start\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "start")

	git(t, root, "checkout", "-qb", "pr")
	write(t, filepath.Join(root, "u.md"), "start\nbranch quote\n")
	git(t, root, "commit", "-qam", "Record the branch quote")
	git(t, root, "checkout", "-q", "main")
	git(t, root, "merge", "-q", "--no-ff", "pr", "-m", "Merge pull request #7 from x/pr")

	write(t, filepath.Join(root, "u.md"), "start\nbranch quote\nmain quote\n")
	git(t, root, "commit", "-qam", "Record the main quote")
	git(t, root, "checkout", "-qb", "other")
	write(t, filepath.Join(root, "v.md"), "other\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "Other work")
	git(t, root, "checkout", "-q", "main")
	git(t, root, "merge", "-q", "--no-ff", "other", "-m", "Merge pull request #8 from x/other")

	if got := Recorded(root, "branch quote", "u.md"); !strings.HasSuffix(got, "Record the branch quote (#7)") {
		t.Errorf("the branch quote came in by #7: %q", got)
	}
	if got := Recorded(root, "main quote", "u.md"); !strings.HasSuffix(got, "Record the main quote") {
		t.Errorf("the main quote came in by no merge: %q", got)
	}
}

// A whitelist .gitignore such as "/*" ignores .abbd too, and the snapshot
// still takes the tree instead of failing on the exclude.
func TestASnapshotTakesAWhitelistGitignore(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, filepath.Join(root, ".gitignore"), "/*\n!/.gitignore\n!/src/\n")
	write(t, filepath.Join(root, "src", "a.go"), "package src\n")
	write(t, filepath.Join(root, ".abbd", "state.yaml"), "x: 1\n")
	tree, err := Snapshot(root, ".abbd")
	if err != nil || tree == "" {
		t.Fatalf("the snapshot failed on a whitelist .gitignore: %v", err)
	}
	if got := git(t, root, "ls-tree", "-r", "--name-only", tree); !strings.Contains(got, "src/a.go") || strings.Contains(got, ".abbd") {
		t.Errorf("the snapshot should hold src and leave .abbd out:\n%s", got)
	}
}
