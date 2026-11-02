// Package gitx reads the few git facts ABBD needs.
package gitx

import (
	"bytes"
	"fmt"
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

// Files lists the files under paths at rev, relative to dir.
func Files(dir, rev string, paths ...string) ([]string, error) {
	out, err := run(dir, nil, append([]string{"ls-tree", "-r", "--name-only", rev, "--"}, paths...)...)
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s: %w", rev, err)
	}
	return strings.Fields(out), nil
}

// WorkingFiles lists the files of the working tree, tracked or not but not
// ignored, relative to dir. It returns nothing outside a repository.
func WorkingFiles(dir string) []string {
	out, err := run(dir, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

// Show returns a file as it was at rev; path is relative to dir.
func Show(dir, rev, path string) ([]byte, error) {
	out, err := run(dir, nil, "show", rev+":./"+filepath.ToSlash(path))
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", rev, path, err)
	}
	return []byte(out), nil
}

// MergeBase returns where HEAD left the default branch, trying origin's HEAD,
// the upstream, origin/main, origin/master, main and master in turn. A CI
// checkout often has origin/main and nothing else.
func MergeBase(dir string) (string, error) {
	var candidates []string
	if out, err := run(dir, nil, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"); err == nil {
		candidates = append(candidates, strings.TrimSpace(out))
	}
	for _, b := range append(candidates, "@{upstream}", "origin/main", "origin/master", "main", "master") {
		if out, err := run(dir, nil, "merge-base", "HEAD", b); err == nil {
			return strings.TrimSpace(out), nil
		}
	}
	return "", fmt.Errorf("no default branch to diff against; pass --base REV")
}

// Snapshot records the working tree as a git tree object, including untracked
// files, without touching the user's index. It returns "" outside a repository.
// A submodule counts as the commit the superproject's index records, not the
// one checked out, so every clone of a commit snapshots the same tree.
func Snapshot(dir, exclude string) (string, error) {
	if _, err := run(dir, nil, "rev-parse", "--git-dir"); err != nil {
		return "", nil
	}
	links, err := gitlinks(dir)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "abbd-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	add := []string{"add", "-A", "--", ".", ":(exclude)" + exclude}
	for _, l := range links {
		add = append(add, ":(exclude,literal)"+l.path)
	}
	if _, err := run(dir, env, add...); err != nil {
		return "", err
	}
	for _, l := range links {
		if _, err := run(dir, env, "update-index", "--add", "--cacheinfo", "160000,"+l.commit+","+l.path); err != nil {
			return "", err
		}
	}
	out, err := run(dir, env, "write-tree")
	return strings.TrimSpace(out), err
}

type gitlink struct{ path, commit string }

// gitlinks lists the submodules the index records under dir, with their
// paths relative to dir.
func gitlinks(dir string) ([]gitlink, error) {
	out, err := run(dir, nil, "ls-files", "-s", "-z")
	if err != nil {
		return nil, err
	}
	var links []gitlink
	for _, e := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(e, "\t")
		if f := strings.Fields(meta); ok && len(f) == 3 && f[0] == "160000" && f[2] == "0" {
			links = append(links, gitlink{path, f[1]})
		}
	}
	return links, nil
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

// Changed lists the paths, relative to dir, that differ between two snapshots.
// A moved file appears under both names. ok is false when the old snapshot no
// longer exists, for example after gc.
func Changed(dir, oldTree, newTree string) (paths []string, ok bool, err error) {
	if oldTree == newTree {
		return nil, true, nil
	}
	if _, err := run(dir, nil, "cat-file", "-e", oldTree+"^{tree}"); err != nil {
		return nil, false, nil
	}
	out, err := run(dir, nil, "diff-tree", "-r", "--no-renames", "--name-only", "-z", "--relative", oldTree, newTree)
	if err != nil {
		return nil, false, err
	}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, true, nil
}

// Gitlinks maps each submodule the index records under dir, by its path
// relative to dir, to the commit recorded for it. It is empty outside a
// repository.
func Gitlinks(dir string) map[string]string {
	links, _ := gitlinks(dir)
	out := map[string]string{}
	for _, l := range links {
		out[l.path] = l.commit
	}
	return out
}
