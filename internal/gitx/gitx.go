// Package gitx reads the few git facts ABBD needs.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

func run(dir string, env []string, args ...string) (string, error) {
	return runInput(dir, env, "", args...)
}

func runInput(dir string, env []string, input string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(input)
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
// one checked out, so every clone of a commit snapshots the same tree. A
// nested repository with no commit yet counts as the untracked directory
// it is, since git add refuses it.
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
	unborn := unbornRepos(dir)
	// git refuses to add when a pathspec names a path .gitignore already
	// ignores, as a whitelist .gitignore such as "/*" does for .abbd; such
	// a path stays out of the snapshot anyway, so it needs no exclude.
	add := []string{"add", "-A", "--", "."}
	if !ignored(dir, exclude) {
		add = append(add, ":(exclude)"+exclude)
	}
	for _, l := range links {
		if !ignored(dir, l.path) {
			add = append(add, ":(exclude,literal)"+l.path)
		}
	}
	for _, r := range unborn {
		if !ignored(dir, r) {
			add = append(add, ":(exclude,literal)"+r)
		}
	}
	if _, err := run(dir, env, add...); err != nil {
		return "", err
	}
	prefix, err := run(dir, nil, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	prefix = strings.TrimSpace(prefix)
	for _, r := range unborn {
		if err := addFiles(dir, prefix, env, r); err != nil {
			return "", err
		}
	}
	for _, l := range links {
		if _, err := run(dir, env, "update-index", "--add", "--cacheinfo", "160000,"+l.commit+","+prefix+l.path); err != nil {
			return "", err
		}
	}
	out, err := run(dir, env, "write-tree")
	return strings.TrimSpace(out), err
}

// unbornRepos lists the untracked repositories under dir that have no
// commit yet, by their paths relative to dir.
func unbornRepos(dir string) []string {
	out, err := run(dir, nil, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil
	}
	var repos []string
	for _, f := range strings.Split(out, "\x00") {
		if r, ok := strings.CutSuffix(f, "/"); ok && Head(filepath.Join(dir, r)) == "" {
			repos = append(repos, r)
		}
	}
	return repos
}

// addFiles adds the files of the repository at repo, relative to dir, to
// the index env names, as git add adds an untracked directory's files: the
// files and symlinks on disk that neither repository ignores, but none below
// a symlink. hash-object and update-index read their paths from the top of
// the work tree, so each takes prefix, where dir sits in it.
func addFiles(dir, prefix string, env []string, repo string) error {
	modes := map[string]string{}
	var paths []string
	for _, f := range WorkingFiles(filepath.Join(dir, repo)) {
		p := repo + "/" + f
		if m := addMode(dir, p); m != "" {
			modes[p] = m
			paths = append(paths, p)
		}
	}
	paths, err := unignored(dir, paths)
	if err != nil || len(paths) == 0 {
		return err
	}
	hashes := map[string]string{}
	var files []string
	var in strings.Builder
	for _, p := range paths {
		if modes[p] != "120000" {
			files = append(files, p)
			in.WriteString(`"` + cQuote.Replace(prefix+p) + "\"\n")
			continue
		}
		target, err := os.Readlink(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return fmt.Errorf("reading the link %s in %s: %w", p, repo, err)
		}
		out, err := runInput(dir, nil, filepath.ToSlash(target), "hash-object", "-w", "--stdin")
		if err != nil {
			return fmt.Errorf("git hash-object in %s: %w", repo, err)
		}
		hashes[p] = strings.TrimSpace(out)
	}
	if len(files) > 0 {
		out, err := runInput(dir, nil, in.String(), "hash-object", "-w", "--stdin-paths")
		if err != nil {
			return fmt.Errorf("git hash-object in %s: %w", repo, err)
		}
		h := strings.Fields(out)
		if len(h) != len(files) {
			return fmt.Errorf("git hash-object in %s: %d hashes for %d files", repo, len(h), len(files))
		}
		for i, p := range files {
			hashes[p] = h[i]
		}
	}
	var info strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&info, "%s %s\t%s%s\x00", modes[p], hashes[p], prefix, p)
	}
	if _, err := runInput(dir, env, info.String(), "update-index", "--add", "-z", "--index-info"); err != nil {
		return fmt.Errorf("git update-index in %s: %w", repo, err)
	}
	return nil
}

// cQuote escapes a path for the C-style quotes hash-object --stdin-paths
// reads, so a name may hold a newline.
var cQuote = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)

// addMode returns the mode git add records for p, relative to dir, or ""
// when git add skips it: p is neither a file nor a symlink, or sits below a
// symlink, which git add does not follow. A Windows junction is not a
// symlink to git, which walks into it as a directory, and Lstat reports it
// as irregular, so a parent that is irregular and resolves to a directory
// counts as one.
func addMode(dir, p string) string {
	for d := path.Dir(p); d != "."; d = path.Dir(d) {
		at := filepath.Join(dir, filepath.FromSlash(d))
		fi, err := os.Lstat(at)
		if err != nil {
			return ""
		}
		if fi.Mode()&os.ModeIrregular != 0 {
			fi, err = os.Stat(at)
			if err != nil {
				return ""
			}
		}
		if !fi.IsDir() {
			return ""
		}
	}
	fi, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(p)))
	switch {
	case err != nil:
		return ""
	case fi.Mode()&os.ModeSymlink != 0:
		return "120000"
	case !fi.Mode().IsRegular():
		return ""
	case fi.Mode()&0o111 != 0:
		return "100755"
	}
	return "100644"
}

// unignored drops the paths, relative to dir, that dir's repository ignores.
func unignored(dir string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out, err := runInput(dir, nil, strings.Join(paths, "\x00")+"\x00", "check-ignore", "-z", "--stdin")
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) && exit.ExitCode() == 1 {
		return paths, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git check-ignore: %w", err)
	}
	ignored := strings.Split(out, "\x00")
	return slices.DeleteFunc(paths, func(p string) bool { return slices.Contains(ignored, p) }), nil
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

// ChangedSince lists the tracked files that differ between rev and the
// working tree, and the untracked ones git does not ignore, relative to dir.
// A renamed file counts under its old name as well as its new one.
func ChangedSince(dir, rev string) ([]string, error) {
	out, err := run(dir, nil, "diff", "-z", "--name-only", "--no-renames", "--relative", rev, "--")
	if err != nil {
		return nil, err
	}
	extra, err := run(dir, nil, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(out+"\x00"+extra, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// LastCommitted is the date, as YYYY-MM-DD, of the last commit that touched
// path, or "" when none did.
func LastCommitted(dir, path string) string {
	out, err := run(dir, nil, "log", "-1", "--format=%cs", "--", path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Recorded says which commit first wrote needle in paths, with its date
// and subject, and the pull request whose merge brought it in when there is
// one: who recorded a reader's claim, and when. It is "" when git finds
// none.
func Recorded(dir, needle string, paths ...string) string {
	out, err := run(dir, nil, append([]string{"log", "--reverse", "--format=%h %cs %s", "-S", needle, "--"}, paths...)...)
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	if first == "" {
		return ""
	}
	hash, _, _ := strings.Cut(first, " ")
	merges, err := run(dir, nil, "log", "--merges", "--ancestry-path", "--reverse", "--format=%P%x09%s", hash+"..HEAD")
	if err != nil {
		return first
	}
	// The merge that brought the commit in is the first whose first parent
	// does not hold it yet. A commit made on that line itself came in by no
	// merge, so it names no pull request.
	for _, m := range strings.Split(strings.TrimSpace(merges), "\n") {
		parents, subject, ok := strings.Cut(m, "\t")
		mainline, _, _ := strings.Cut(parents, " ")
		if !ok || mainline == "" {
			continue
		}
		if _, err := run(dir, nil, "merge-base", "--is-ancestor", hash, mainline); err == nil {
			continue
		}
		if pr, ok := strings.CutPrefix(subject, "Merge pull request "); ok {
			pr, _, _ = strings.Cut(pr, " ")
			return first + " (" + pr + ")"
		}
		return first
	}
	return first
}

// ignored reports whether .gitignore ignores path, relative to dir.
func ignored(dir, path string) bool {
	_, err := run(dir, nil, "check-ignore", "-q", "--", path)
	return err == nil
}
