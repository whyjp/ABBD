package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
)

// CodeFormat is the shape of Code that abbd records now: one hash per anchor
// and per quote. Format 1 hashed every file the digest read; sync upgrades a
// text stored then.
const CodeFormat = 2

// Understanding is what pstack's how wrote about one module for a human to
// read beside its drawing: its Overview and Key Concepts. It is a reader's
// account, so its origin is INFERRED, and Digest is the module's code as it
// was when the text was written. Code is that code by what read it, an
// anchor or a quote, which lets a restamp prove on any clone that the
// module's code has not changed since. An older ABBD recorded no Code.
type Understanding struct {
	Unit   model.ID          `yaml:"unit"`
	Origin model.Origin      `yaml:"origin"`
	Digest string            `yaml:"digest"`
	Format int               `yaml:"format,omitempty"`
	Code   map[string]string `yaml:"code,omitempty"`
	Text   string            `yaml:"-"`
}

func (s Store) understandingPath(id model.ID) string {
	return s.path("understanding", string(id)+".md")
}

// CleanText is a text as abbd keeps it: no byte order mark, LF line endings,
// no trailing blanks on a line, none around the whole. Two saves of the same
// words compare equal. Trailing blanks carry no meaning here because the
// screen joins a paragraph's lines with a space and draws no hard breaks.
func CleanText(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// SaveUnderstanding refuses a text without a digest: it would read current
// whatever its module's code did.
func (s Store) SaveUnderstanding(u Understanding) error {
	if u.Digest == "" {
		return fmt.Errorf("%s has no code to check a text against", u.Unit)
	}
	b, err := frontmatter(u, CleanText(u.Text)+"\n")
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
			u.Text = CleanText(string(body))
		}
		out[u.Unit] = u
	}
	return out, nil
}

// quoteContext is how many lines on each side of a quote count toward a
// module's digest when the module only quotes the file.
const quoteContext = 5

// missing is the hash of a path with nothing there.
const missing = "missing"

// codebase is the project's code as one command reads it. It lists the
// files and submodules once and hashes each path once, however many modules
// the command digests. abbd never writes code, so nothing it caches goes
// stale while a command runs.
type codebase struct {
	project string
	listed  bool
	files   []string
	links   map[string]string
	hashes  map[string]string
	ev      *evidence.Checker
}

func newCodebase(project string) *codebase {
	return &codebase{project: project, hashes: map[string]string{}, ev: evidence.New(project)}
}

func (c *codebase) list() {
	if c.listed {
		return
	}
	c.listed = true
	c.links = gitx.Gitlinks(c.project)
	c.files = gitx.WorkingFiles(c.project)
	if c.files == nil {
		c.files = c.walk(".")
	}
	c.files = slices.DeleteFunc(c.files, func(f string) bool { return strings.HasPrefix(f, Dir+"/") })
	slices.Sort(c.files)
}

// walk lists the files on disk under dir, which is how a directory anchor
// sees code git lists nothing for: outside a repository, or ignored.
func (c *codebase) walk(dir string) []string {
	var files []string
	filepath.WalkDir(filepath.Join(c.project, filepath.FromSlash(dir)), func(p string, d fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(c.project, p)
		switch rel = filepath.ToSlash(rel); {
		case err != nil || d.Name() == ".git" || rel == Dir:
			return filepath.SkipDir
		case !d.IsDir():
			files = append(files, rel)
		}
		return nil
	})
	return files
}

// under is the code a directory anchor stands for: the paths git lists
// under it, a submodule as one path. When git lists nothing there, the
// directory is inside a submodule and stands for that submodule's pin, or
// it is ignored and stands for the files on disk; fallback says so.
func (c *codebase) under(dir string) (paths []string, fallback bool) {
	c.list()
	if dir = strings.TrimSuffix(dir, "/"); dir == "." {
		return c.files, false
	}
	if _, ok := slices.BinarySearch(c.files, dir); ok {
		paths = append(paths, dir)
	}
	i, _ := slices.BinarySearch(c.files, dir+"/")
	for ; i < len(c.files) && strings.HasPrefix(c.files[i], dir+"/"); i++ {
		paths = append(paths, c.files[i])
	}
	if len(paths) > 0 {
		return paths, false
	}
	for sub := range c.links {
		if strings.HasPrefix(dir, sub+"/") {
			return []string{sub}, true
		}
	}
	return c.walk(dir), true
}

// read is a path's content as a digest sees it: a file with LF endings, a
// submodule as the commit the superproject's index records for it, which
// every clone has. Edits inside a submodule that are not committed and
// staged in the superproject do not show. false means nothing is there.
func (c *codebase) read(path string) ([]byte, bool) {
	c.list()
	if commit, ok := c.links[path]; ok {
		return []byte("submodule " + commit), true
	}
	b, err := os.ReadFile(filepath.Join(c.project, filepath.FromSlash(path)))
	if err != nil {
		return nil, false
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), true
}

func short(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

func (c *codebase) hash(path string) string {
	if h, ok := c.hashes[path]; ok {
		return h
	}
	h := missing
	if b, ok := c.read(path); ok {
		h = short(b)
	}
	c.hashes[path] = h
	return h
}

// moduleCode is the code a module's text describes: what each anchor of
// the module or of any unit under it, at any depth, reads whole, and what
// those units cite.
type moduleCode struct {
	anchors map[string]anchorCode
	cited   []model.Evidence
}

// anchorCode is what one anchor reads whole: the file itself, or for a
// directory the code under it. fallback marks a directory git lists
// nothing under.
type anchorCode struct {
	paths         []string
	dir, fallback bool
}

func (c *codebase) moduleCode(units []model.Unit, id model.ID) moduleCode {
	in := map[model.ID]bool{id: true}
	for grew := true; grew; {
		grew = false
		for _, u := range units {
			if !u.Absent && !in[u.ID] && in[u.Parent] {
				in[u.ID], grew = true, true
			}
		}
	}
	m := moduleCode{anchors: map[string]anchorCode{}}
	for _, u := range units {
		if u.Absent || !in[u.ID] {
			continue
		}
		for _, a := range u.Anchors {
			// Extraction anchors a unit at each file its evidence cites;
			// such a file counts by its quotes alone.
			if _, seen := m.anchors[a.Path]; seen || slices.ContainsFunc(u.Evidence, func(e model.Evidence) bool { return e.Anchor() == a }) {
				continue
			}
			if fi, err := os.Stat(filepath.Join(c.project, filepath.FromSlash(a.Path))); err != nil || !fi.IsDir() {
				m.anchors[a.Path] = anchorCode{paths: []string{a.Path}}
				continue
			}
			paths, fallback := c.under(a.Path)
			m.anchors[a.Path] = anchorCode{paths: paths, dir: true, fallback: fallback}
		}
		m.cited = append(m.cited, u.Evidence...)
	}
	return m
}

// files is every path the anchors read whole, sorted, leaving out what a
// fallback directory reads when all is false.
func (m moduleCode) files(all bool) []string {
	var out []string
	for _, a := range m.anchors {
		if all || !a.fallback {
			out = append(out, a.paths...)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// quotes are the cited quotes in files that no path of files covers.
func (m moduleCode) quotes(files []string) []model.Evidence {
	var out []model.Evidence
	for _, e := range m.cited {
		if _, covered := slices.BinarySearch(files, e.Path); !covered {
			out = append(out, model.Evidence{Path: e.Path, Lines: e.Lines, Quote: e.Quote})
		}
	}
	slices.SortFunc(out, func(a, b model.Evidence) int {
		return strings.Compare(a.Path+"\x00"+a.Quote, b.Path+"\x00"+b.Quote)
	})
	return slices.CompactFunc(out, func(a, b model.Evidence) bool { return a.Path == b.Path && a.Quote == b.Quote })
}

// quoteMark ends the Code key of a quote: its path, then the mark, then a
// hash of the quote.
const quoteMark = "#quote-"

func quoteKey(q model.Evidence) string { return q.Path + quoteMark + short([]byte(q.Quote))[:8] }

// keyPath is the path a Code key reads.
func keyPath(k string) string {
	if i := strings.LastIndex(k, quoteMark); i >= 0 && len(k)-i == len(quoteMark)+8 {
		return k[:i]
	}
	return k
}

// Stamp is what abbd understand records with a new text: the module's
// digest, and its code by what read it. A file anchor is the file's hash, a
// directory anchor one hash of every path under it, and a quote the hash of
// the lines around it, wherever in the file they sit now. The digest is the
// hash of all of these, so it reads exactly what Code records, and a path
// or a quote that is gone counts as gone.
func (s Store) Stamp(units []model.Unit, id model.ID) (string, map[string]string) {
	return s.stamp(s.code.moduleCode(units, id))
}

func (s Store) stamp(m moduleCode) (string, map[string]string) {
	c := s.code
	code := map[string]string{}
	for p, a := range m.anchors {
		if !a.dir {
			code[p] = c.hash(p)
			continue
		}
		var b strings.Builder
		for _, f := range a.paths {
			fmt.Fprintf(&b, "%s\x00%s\n", f, c.hash(f))
		}
		code[p] = short([]byte(b.String()))
	}
	for _, q := range m.quotes(m.files(true)) {
		code[quoteKey(q)] = missing
		if lines, ok := c.ev.Around(q, quoteContext); ok {
			code[quoteKey(q)] = short([]byte(lines))
		}
	}
	if len(code) == 0 {
		return "", code
	}
	keys := make([]string, 0, len(code))
	for k := range code {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s\x00%s\n", k, code[k])
	}
	return short([]byte(b.String())), code
}

// ModuleDigest fingerprints the code a module's text describes, as Stamp
// does. It is empty only for a module with no anchor and no quote.
func (s Store) ModuleDigest(units []model.Unit, id model.ID) string {
	d, _ := s.Stamp(units, id)
	return d
}

// Changed lists what keeps a text from being restamped: an anchor or a
// quote of the module now whose code differs from what the text was written
// against, or that the text never saw, and one the text saw whose path is
// gone. One that left the module while its path still exists is no longer
// the module's code.
func (s Store) Changed(units []model.Unit, id model.ID, written map[string]string) []string {
	var out []string
	m := s.code.moduleCode(units, id)
	_, now := s.stamp(m)
	for k, h := range now {
		switch was, ok := written[k]; {
		case !ok:
			out = append(out, k+" (new to the module)")
		case was != h && m.anchors[k].dir:
			out = append(out, k+" (code under it)")
		case was != h:
			out = append(out, k)
		}
	}
	for k, was := range written {
		if _, ok := now[k]; ok || was == missing {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.code.project, filepath.FromSlash(keyPath(k)))); err != nil {
			out = append(out, k+" (gone)")
		}
	}
	slices.Sort(out)
	return out
}

// format1Digest is the digest abbd stamped format 1 texts with: every file
// the anchors read whole, then each quote with the lines around it. A
// directory git listed nothing under read nothing then.
func (s Store) format1Digest(m moduleCode) string {
	c := s.code
	files := m.files(false)
	quotes := m.quotes(files)
	if len(files)+len(quotes) == 0 {
		return ""
	}
	h := sha256.New()
	for _, p := range files {
		b, ok := c.read(p)
		if !ok {
			fmt.Fprintf(h, "%s\x00missing\x00", p)
			continue
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
		h.Write(b)
	}
	for _, q := range quotes {
		lines, ok := c.ev.Around(q, quoteContext)
		if !ok {
			fmt.Fprintf(h, "%s\x00%s\x00missing\x00", q.Path, q.Quote)
			continue
		}
		fmt.Fprintf(h, "%s\x00%s\x00%d\x00%s", q.Path, q.Quote, len(lines), lines)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
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

// UpgradeUnderstandings carries over each text an older ABBD stored. Such a
// text is current only when its code is shown unchanged since it was
// written: its old digest of the code now matches its stamp, and that
// digest read every file the new one reads. Those texts are stamped again
// and returned in current. A text that matches under its old digest, but
// whose module holds code that digest never read, cannot be shown current:
// it keeps its stamp, reads STALE, and is returned in widened. A text
// already STALE stays STALE.
//
// ABBD 0.6 recorded no Code and digested the whole files of a module and
// its direct children. Format 1 digested the same code as now, except that
// a directory git lists nothing under read nothing.
func (s Store) UpgradeUnderstandings(units []model.Unit) (current, widened []model.ID, err error) {
	texts, err := s.LoadUnderstandings()
	if err != nil {
		return nil, nil, err
	}
	for id, u := range texts {
		if u.Format == CodeFormat {
			continue
		}
		m := s.code.moduleCode(units, id)
		var matches, covered bool
		if u.Code == nil {
			legacy, read := s.legacyModuleDigest(units, id)
			matches, covered = legacy != "" && u.Digest == legacy, true
			for _, p := range append(m.files(true), pathsOf(m.quotes(m.files(true)))...) {
				fi, err := os.Stat(filepath.Join(s.code.project, filepath.FromSlash(p)))
				if _, ok := slices.BinarySearch(read, p); !ok || (err == nil && fi.IsDir()) {
					covered = false
				}
			}
		} else {
			matches = u.Digest != "" && u.Digest == s.format1Digest(m)
			covered = true
			for _, a := range m.anchors {
				covered = covered && !a.fallback
			}
		}
		switch {
		case !matches:
			continue
		case !covered:
			widened = append(widened, id)
			continue
		}
		u.Digest, u.Code = s.Stamp(units, id)
		u.Format = CodeFormat
		if err := s.SaveUnderstanding(u); err != nil {
			return nil, nil, err
		}
		current = append(current, id)
	}
	slices.Sort(current)
	slices.Sort(widened)
	return current, widened, nil
}

func pathsOf(es []model.Evidence) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Path)
	}
	return out
}
