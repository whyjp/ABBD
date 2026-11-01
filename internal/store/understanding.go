package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Understanding is what pstack's how wrote about one module for a human to
// read beside its drawing: its Overview and Key Concepts. It is a reader's
// account, so its origin is INFERRED, and Digest is the module's code as it
// was when the text was written.
type Understanding struct {
	Unit   model.ID     `yaml:"unit"`
	Origin model.Origin `yaml:"origin"`
	By     string       `yaml:"by"`
	Digest string       `yaml:"digest"`
	Text   string       `yaml:"-"`
}

func (s Store) understandingPath(id model.ID) string {
	return s.path("understanding", string(id)+".md")
}

func (s Store) SaveUnderstanding(u Understanding) error {
	b, err := frontmatter(u, strings.TrimSpace(u.Text)+"\n")
	if err != nil {
		return err
	}
	return writeFile(s.understandingPath(u.Unit), b)
}

// LoadUnderstandings reads every module's text, by unit.
func (s Store) LoadUnderstandings() (map[model.ID]Understanding, error) {
	files, err := filepath.Glob(s.path("understanding", "*.md"))
	if err != nil {
		return nil, err
	}
	out := map[model.ID]Understanding{}
	for _, f := range files {
		var u Understanding
		if err := readFrontmatter(f, &u); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		if _, body, ok := bytes.Cut(bytes.TrimPrefix(b, []byte("---\n")), []byte("\n---\n")); ok {
			u.Text = strings.TrimSpace(string(body))
		}
		out[u.Unit] = u
	}
	return out, nil
}

// ModuleDigest fingerprints the code a module's text describes: the files its
// anchors and evidence cite, and those of every unit it contains. A file that
// is gone counts as gone, so deleting one changes the digest too.
func (s Store) ModuleDigest(units []model.Unit, id model.ID) string {
	var paths []string
	for _, u := range units {
		if u.Absent || (u.ID != id && u.Parent != id) {
			continue
		}
		for _, a := range u.Anchors {
			paths = append(paths, a.Path)
		}
		for _, e := range u.Evidence {
			paths = append(paths, e.Path)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	if len(paths) == 0 {
		return ""
	}
	project := filepath.Dir(s.root)
	h := sha256.New()
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(p)))
		if err != nil {
			fmt.Fprintf(h, "%s\x00missing\x00", p)
			continue
		}
		b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
