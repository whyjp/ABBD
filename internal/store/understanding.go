package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
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
// was when the text was written. Code is the hash of each file that digest
// read then, by path, which lets a restamp prove on any clone that the
// module's code has not changed since. An older ABBD recorded no Code.
type Understanding struct {
	Unit   model.ID          `yaml:"unit"`
	Origin model.Origin      `yaml:"origin"`
	Digest string            `yaml:"digest"`
	Code   map[string]string `yaml:"code,omitempty"`
	Text   string            `yaml:"-"`
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
// standing for the files and submodules under it, and the quotes those units
// cite in files no anchor covers.
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
				working, listed = workingFiles(project), true
			}
			dir := strings.TrimSuffix(a.Path, "/")
			for _, f := range working {
				if (dir == "." || f == dir || strings.HasPrefix(f, dir+"/")) && !strings.HasPrefix(f, Dir+"/") {
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

// workingFiles lists what git lists under the project, a submodule as one
// path. Outside a repository it walks the disk instead, so a directory
// anchor never stands for no code.
func workingFiles(project string) []string {
	if files := gitx.WorkingFiles(project); files != nil {
		return files
	}
	var files []string
	filepath.WalkDir(project, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil || d.Name() == ".git":
			return filepath.SkipDir
		case !d.IsDir():
			rel, _ := filepath.Rel(project, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files
}

// read is a path's content as a digest sees it: a file with LF endings, a
// submodule as the commit the superproject records for it, which every
// clone has. false means there is nothing there.
func (s Store) read(path string) ([]byte, bool) {
	project := filepath.Dir(s.root)
	full := filepath.Join(project, filepath.FromSlash(path))
	if fi, err := os.Stat(full); err == nil && fi.IsDir() {
		if commit := gitx.Gitlink(project, path); commit != "" {
			return []byte("submodule " + commit), true
		}
		return nil, false
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil, false
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), true
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
	for _, p := range paths {
		b, ok := s.read(p)
		if !ok {
			fmt.Fprintf(h, "%s\x00missing\x00", p)
			continue
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
		h.Write(b)
	}
}

// missing is the Code entry of a path with nothing there.
const missing = "missing"

func (s Store) fileHash(path string) string {
	b, ok := s.read(path)
	if !ok {
		return missing
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

// Stamp is what abbd understand records with a new text: the module's
// digest, and the hash of every file the digest reads, a quoted file whole.
func (s Store) Stamp(units []model.Unit, id model.ID) (string, map[string]string) {
	c := s.moduleCode(units, id)
	code := map[string]string{}
	for _, p := range c.files {
		code[p] = s.fileHash(p)
	}
	for _, q := range c.quotes {
		code[q.Path] = s.fileHash(q.Path)
	}
	return s.ModuleDigest(units, id), code
}

// Changed lists the files that keep a text from being restamped: a file of
// the module now that differs from what the text was written against, or
// that the text never saw, and a file the text saw that is gone. A file
// that left the module and still exists is no longer the module's code.
func (s Store) Changed(units []model.Unit, id model.ID, written map[string]string) []string {
	var out []string
	_, now := s.Stamp(units, id)
	for p, h := range now {
		switch was, ok := written[p]; {
		case !ok:
			out = append(out, p+" (new to the module)")
		case was != h:
			out = append(out, p)
		}
	}
	for p, was := range written {
		if _, ok := now[p]; !ok && was != missing && s.fileHash(p) == missing {
			out = append(out, p+" (gone)")
		}
	}
	slices.Sort(out)
	return out
}

// legacyModuleDigest is the digest ABBD 0.6 stamped texts with: the whole
// files cited by a module and its direct children, a directory counting as
// missing. It also returns those paths, so UpgradeUnderstandings can tell
// whether it read all the code the new digest reads.
func (s Store) legacyModuleDigest(units []model.Unit, id model.ID) (string, []string) {
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
		return "", nil
	}
	h := sha256.New()
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
	return hex.EncodeToString(h.Sum(nil))[:16], paths
}

// UpgradeUnderstandings carries over each text an older ABBD stored, which
// records no Code. Such a text is current only when its code is shown
// unchanged since it was written: the 0.6 digest of the code now matches its
// stamp, and that digest read, whole, every file the new one reads. Those
// texts are stamped again and returned in current. A text that was current
// under the 0.6 digest, but whose module holds code that digest never read,
// cannot be shown current: it keeps its stamp, reads STALE, and is returned
// in widened. A text already STALE stays STALE.
func (s Store) UpgradeUnderstandings(units []model.Unit) (current, widened []model.ID, err error) {
	texts, err := s.LoadUnderstandings()
	if err != nil {
		return nil, nil, err
	}
	project := filepath.Dir(s.root)
	for id, u := range texts {
		if u.Code != nil {
			continue
		}
		legacy, read := s.legacyModuleDigest(units, id)
		if legacy == "" || u.Digest != legacy {
			continue
		}
		digest, code := s.Stamp(units, id)
		covered := true
		for p := range code {
			fi, err := os.Stat(filepath.Join(project, filepath.FromSlash(p)))
			if _, ok := slices.BinarySearch(read, p); !ok || (err == nil && fi.IsDir()) {
				covered = false
				break
			}
		}
		if !covered {
			widened = append(widened, id)
			continue
		}
		u.Digest, u.Code = digest, code
		if err := s.SaveUnderstanding(u); err != nil {
			return nil, nil, err
		}
		current = append(current, id)
	}
	slices.Sort(current)
	slices.Sort(widened)
	return current, widened, nil
}
