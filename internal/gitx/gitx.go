// Package gitx reads the few git facts ABBD needs.
package gitx

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func run(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

// Head returns the current commit, or "" outside a repository or before the first commit.
func Head(dir string) string {
	out, err := run(dir, nil, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Snapshot records the working tree as a git tree object, including untracked
// files, without touching the user's index. It returns "" outside a repository.
func Snapshot(dir, exclude string) (string, error) {
	if _, err := run(dir, nil, "rev-parse", "--git-dir"); err != nil {
		return "", nil
	}
	tmp, err := os.MkdirTemp("", "abbd-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := run(dir, env, "add", "-A", "--", ".", ":(exclude)"+exclude); err != nil {
		return "", err
	}
	out, err := run(dir, env, "write-tree")
	return strings.TrimSpace(out), err
}

// Renames maps old to new paths, relative to dir, between two snapshots.
// ok is false when the old snapshot no longer exists, for example after gc.
func Renames(dir, oldTree, newTree string) (renames map[string]string, ok bool, err error) {
	renames = map[string]string{}
	if oldTree == "" || newTree == "" || oldTree == newTree {
		return renames, true, nil
	}
	if _, err := run(dir, nil, "cat-file", "-e", oldTree+"^{tree}"); err != nil {
		return renames, false, nil
	}
	out, err := run(dir, nil, "diff-tree", "-r", "-M", "-l0", "--no-color", "--name-status", "-z", "--relative", oldTree, newTree)
	if err != nil {
		return nil, false, err
	}
	f := strings.Split(out, "\x00")
	for i := 0; i < len(f); i++ {
		switch {
		case strings.HasPrefix(f[i], "R") && i+2 < len(f):
			renames[f[i+1]] = f[i+2]
			i += 2
		case strings.HasPrefix(f[i], "C"):
			i += 2
		case f[i] != "":
			i++
		}
	}
	return renames, true, nil
}
