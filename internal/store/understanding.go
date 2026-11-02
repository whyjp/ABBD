package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
)

// Understanding is what pstack's how wrote about one module for a human to
// read beside its drawing: its Overview and Key Concepts. It is a reader's
// account, so its origin is INFERRED, and Digest is the module's code as it
// was when the text was written. Tree is the working tree's snapshot then,
// which lets a restamp prove the module's code has not changed since.
type Understanding struct {
	Unit   model.ID     `yaml:"unit"`
	Origin model.Origin `yaml:"origin"`
	Digest string       `yaml:"digest"`
	Tree   string       `yaml:"tree,omitempty"`
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

// quoteContext is how many lines on each side of a quote count toward a
// module's digest when the module only quotes the file.
const quoteContext = 5

// moduleCode is the code a module's text describes: the files anchored by
// the module or any unit under it, at any depth, with a directory anchor
// standing for the files under it, and the quotes those units cite in files
// no anchor covers.
type moduleCode struct {
	files  []string
	quotes []model.Evidence
}

func (s Store) moduleCode(units []model.Unit, id model.ID) moduleCode {
	in := map[model.ID]bool{id: true}
	for grew := true; grew; {
		grew = false
		for _, u := range units {
			if !u.Absent && !in[u.ID] && in[u.Parent] {
				in[u.ID], grew = true, true
			}
		}
	}
	project := filepath.Dir(s.root)
	var working []string
	listed := false
	var c moduleCode
	var cited []model.Evidence
	for _, u := range units {
		if u.Absent || !in[u.ID] {
			continue
		}
		for _, a := range u.Anchors {
			// Extraction anchors a unit at each file its evidence cites;
			// such a file counts by its quotes alone.
			if slices.ContainsFunc(u.Evidence, func(e model.Evidence) bool { return e.Anchor() == a }) {
				continue
			}
			if fi, err := os.Stat(filepath.Join(project, filepath.FromSlash(a.Path))); err != nil || !fi.IsDir() {
				c.files = append(c.files, a.Path)
				continue
			}
			if !listed {
				working, listed = gitx.WorkingFiles(project), true
			}
			dir := strings.TrimSuffix(a.Path, "/") + "/"
			if dir == "./" {
				dir = ""
			}
			for _, f := range working {
				if strings.HasPrefix(f, dir) && !strings.HasPrefix(f, Dir+"/") {
					c.files = append(c.files, f)
				}
			}
		}
		cited = append(cited, u.Evidence...)
	}
	slices.Sort(c.files)
	c.files = slices.Compact(c.files)
	for _, e := range cited {
		if _, covered := slices.BinarySearch(c.files, e.Path); !covered {
			c.quotes = append(c.quotes, model.Evidence{Path: e.Path, Lines: e.Lines, Quote: e.Quote})
		}
	}
	slices.SortFunc(c.quotes, func(a, b model.Evidence) int {
		return strings.Compare(a.Path+"\x00"+a.Quote, b.Path+"\x00"+b.Quote)
	})
	c.quotes = slices.CompactFunc(c.quotes, func(a, b model.Evidence) bool { return a.Path == b.Path && a.Quote == b.Quote })
	return c
}

// ModulePaths lists the files whose content a module's digest reads.
func (s Store) ModulePaths(units []model.Unit, id model.ID) []string {
	c := s.moduleCode(units, id)
	paths := slices.Clone(c.files)
	for _, q := range c.quotes {
		paths = append(paths, q.Path)
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// ModuleDigest fingerprints the code a module's text describes: each file of
// its moduleCode whole, and each quote by the lines around it, wherever in
// the file they sit now. A file or a quote that is gone counts as gone, so
// deleting one changes the digest too.
func (s Store) ModuleDigest(units []model.Unit, id model.ID) string {
	c := s.moduleCode(units, id)
	if len(c.files)+len(c.quotes) == 0 {
		return ""
	}
	h := sha256.New()
	s.hashWhole(h, c.files)
	ev := evidence.New(filepath.Dir(s.root))
	for _, q := range c.quotes {
		lines, ok := ev.Around(q, quoteContext)
		if !ok {
			fmt.Fprintf(h, "%s\x00%s\x00missing\x00", q.Path, q.Quote)
			continue
		}
		fmt.Fprintf(h, "%s\x00%s\x00%d\x00%s", q.Path, q.Quote, len(lines), lines)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (s Store) hashWhole(h io.Writer, paths []string) {
	project := filepath.Dir(s.root)
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
}

// legacyModuleDigest is the digest ABBD 0.6 stamped texts with: the whole
// files cited by a module and its direct children, a directory counting as
// missing. It only lets UpgradeUnderstandings tell which old texts were
// current.
func (s Store) legacyModuleDigest(units []model.Unit, id model.ID) string {
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
	h := sha256.New()
	s.hashWhole(h, paths)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// UpgradeUnderstandings restamps each text still stamped with the 0.6
// digest of the code as it is now. That text was current before the digest
// grew to the module's whole code, so it stays current, at tree. A text the
// old digest called STALE keeps its stamp and stays STALE. It returns the
// modules it restamped.
func (s Store) UpgradeUnderstandings(units []model.Unit, tree string) ([]model.ID, error) {
	texts, err := s.LoadUnderstandings()
	if err != nil {
		return nil, err
	}
	var ids []model.ID
	for id, u := range texts {
		now := s.ModuleDigest(units, id)
		if u.Digest == now || u.Digest != s.legacyModuleDigest(units, id) {
			continue
		}
		u.Digest, u.Tree = now, tree
		if err := s.SaveUnderstanding(u); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}
