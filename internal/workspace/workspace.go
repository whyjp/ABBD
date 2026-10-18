// Package workspace observes the target repository: git revision and a per-file content
// fingerprint. Freshness compares fingerprints so uncommitted edits and git-less greenfield
// projects are both covered (PRD ch.07 CURRENT = same revision or a valid worktree snapshot).
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var skipDirs = map[string]bool{".git": true, ".abbd": true, "node_modules": true, "vendor": true}

type Snapshot struct {
	Revision    string            `json:"revision"`
	Dirty       bool              `json:"dirty"`
	Fingerprint string            `json:"fingerprint"`
	Files       map[string]string `json:"files"`
}

func Observe(root string) (Snapshot, error) {
	files, err := hashFiles(root)
	if err != nil {
		return Snapshot{}, err
	}
	rev, dirty := gitRevision(root)
	return Snapshot{Revision: rev, Dirty: dirty, Fingerprint: Fingerprint(files), Files: files}, nil
}

func hashFiles(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (skipDirs[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// Line endings are normalized so a Windows checkout and a Linux checkout agree (NFR-02).
		b = []byte(strings.ReplaceAll(string(b), "\r\n", "\n"))
		sum := sha256.Sum256(b)
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:8])
		return nil
	})
	return out, err
}

func Fingerprint(files map[string]string) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k + "=" + files[k] + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Diff lists added, modified and removed paths between two file maps.
func Diff(base, next map[string]string) (changed []string) {
	for p, h := range next {
		if base[p] != h {
			changed = append(changed, p)
		}
	}
	for p := range base {
		if _, ok := next[p]; !ok {
			changed = append(changed, p)
		}
	}
	sort.Strings(changed)
	return changed
}

func gitRevision(root string) (string, bool) {
	out, err := Git(root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "nogit", false
	}
	status, _ := Git(root, "status", "--porcelain", "--", ".", ":(exclude).abbd")
	return out, status != ""
}

func Git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

// Renames maps old path -> new path using git rename detection between rev and the worktree.
func Renames(root, rev string) map[string]string {
	out := map[string]string{}
	if rev == "" || rev == "nogit" {
		return out
	}
	s, err := Git(root, "diff", "--name-status", "-M", rev, "--")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 3 && strings.HasPrefix(f[0], "R") {
			out[f[1]] = f[2]
		}
	}
	return out
}
