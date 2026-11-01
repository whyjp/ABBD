// Package evidence checks, deterministically, that the code still holds the
// excerpts a reader cited for a semantic unit or relation. A reader finds a
// coupling; this package only decides whether the same tree still shows it,
// so the same tree always gives the same answer.
//
// A quote follows its code within its file, and into another file only when
// that file is one git would commit (tracked, or untracked and not ignored,
// as the state's fingerprint counts them), is not a test, and lies under an
// anchor of the unit that owns the evidence. So a build output, a vendored
// copy or a test never keeps a dead coupling alive.
package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/model"
)

// Slack is how many lines a quote may sit outside the lines its evidence names.
const Slack = 3

// Result is one evidence item checked against the tree.
type Result struct {
	Live bool
	// At is where the quote was found, such as "120-121".
	At string
	// Digest fingerprints the lines that hold the quote, so a diff sees an
	// edit to them even while the quote survives.
	Digest string
	// Path is where the quote lives now when it moved to another file.
	Path string
}

// MinMoveQuote is how long, without whitespace, a quote must be before it is
// followed into another file: a short one could match unrelated code.
const MinMoveQuote = 24

type file struct {
	raw []string
	// norm is the file with all whitespace removed, so formatting never kills
	// a quote. line holds the line number of each byte of norm, and gap says
	// whether whitespace stood before it, which is where a word may end.
	norm string
	line []int
	gap  []bool
}

// Checker reads each file once.
type Checker struct {
	root  string
	files map[string]*file
	// paths lists the project's committable files once a quote must be
	// looked for outside its own file.
	paths []string
}

func New(root string) *Checker { return &Checker{root: root, files: map[string]*file{}} }

func normalize(s string) string { return strings.Join(strings.Fields(s), "") }

func word(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

func (c *Checker) load(path string) *file {
	if f, ok := c.files[path]; ok {
		return f
	}
	b, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(path)))
	var f *file
	if err == nil {
		f = &file{raw: strings.Split(string(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))), "\n")}
		var norm strings.Builder
		for i, l := range f.raw {
			for _, w := range strings.Fields(l) {
				norm.WriteString(w)
				for k := range len(w) {
					f.line = append(f.line, i+1)
					f.gap = append(f.gap, k == 0)
				}
			}
		}
		f.norm = norm.String()
	}
	c.files[path] = f
	return f
}

// match is one whole-word occurrence of a quote: its first and last line.
type match struct{ start, end int }

func (f *file) matches(q string) []match {
	var out []match
	for from := 0; from <= len(f.norm); {
		i := strings.Index(f.norm[from:], q)
		if i < 0 {
			break
		}
		at, after := from+i, from+i+len(q)
		// A quote ending in a word must end where the code's word ends, so
		// "charges" does not live on in "charges_archive".
		if (at == 0 || f.gap[at] || !word(f.norm[at-1]) || !word(q[0])) &&
			(after == len(f.norm) || f.gap[after] || !word(f.norm[after]) || !word(q[len(q)-1])) {
			out = append(out, match{f.line[at], f.line[after-1]})
		}
		from += i + 1
	}
	return out
}

func (f *file) result(m match, moved string) Result {
	h := sha256.Sum256([]byte(strings.Join(f.raw[m.start-1:m.end], "\n")))
	at := fmt.Sprint(m.start)
	if m.end != m.start {
		at += fmt.Sprintf("-%d", m.end)
	}
	return Result{Live: true, At: at, Digest: hex.EncodeToString(h[:8]), Path: moved}
}

// Check finds e's quote, whitespace-insensitively. It looks within the lines
// e names and the slack first. Code moves, so it then takes the quote's only
// occurrence anywhere in the same file, and, when the file no longer holds it,
// the only occurrence of a long enough quote in any other file of the same
// kind under within, the anchors of the unit that owns e. A quote found twice
// is ambiguous and stays dead, so the same tree always gives the same answer.
func (c *Checker) Check(e model.Evidence, within []model.Anchor) Result {
	q := normalize(e.Quote)
	if q == "" {
		return Result{}
	}
	first, last, bounded := e.Span()
	if f := c.load(e.Path); f != nil {
		ms := f.matches(q)
		for _, m := range ms {
			if !bounded || (m.start <= last+Slack && m.end >= first-Slack) {
				return f.result(m, "")
			}
		}
		if len(ms) == 1 {
			return f.result(ms[0], "")
		}
		if len(ms) > 1 {
			return Result{}
		}
	}
	if len(q) < MinMoveQuote {
		return Result{}
	}
	var found Result
	n := 0
	for _, p := range c.candidates(path.Ext(e.Path), within) {
		if p == e.Path {
			continue
		}
		f := c.load(p)
		if f == nil {
			continue
		}
		for _, m := range f.matches(q) {
			n++
			found = f.result(m, p)
		}
		if n > 1 {
			return Result{}
		}
	}
	if n == 1 {
		return found
	}
	return Result{}
}

// candidates lists, in a fixed order, the committable non-test files with
// extension ext under one of the anchors.
func (c *Checker) candidates(ext string, within []model.Anchor) []string {
	if c.paths == nil {
		c.paths = slices.Sorted(slices.Values(gitx.WorkingFiles(c.root)))
		if c.paths == nil {
			c.paths = []string{}
		}
	}
	var out []string
	for _, p := range c.paths {
		if path.Ext(p) == ext && !isTest(p) && covered(p, within) {
			out = append(out, p)
		}
	}
	return out
}

// covered says whether a file anchor names p or a directory anchor holds it.
func covered(p string, within []model.Anchor) bool {
	for _, a := range within {
		d := strings.TrimSuffix(a.Path, "/")
		if d == "" || d == "." || p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

// isTest says whether p is a test file or sits in a test folder.
func isTest(p string) bool {
	dirs := strings.Split(p, "/")
	name := dirs[len(dirs)-1]
	for _, d := range dirs[:len(dirs)-1] {
		switch d {
		case "test", "tests", "__tests__":
			return true
		}
	}
	return strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.")
}

// all checks a fact's evidence: it is live while any item is, and its digest
// covers every live item in order.
func (c *Checker) all(es []model.Evidence, within []model.Anchor) (live bool, digest string, moved []model.Evidence) {
	var ds []string
	moved = slices.Clone(es)
	for i, e := range es {
		if r := c.Check(e, within); r.Live {
			live = true
			ds = append(ds, r.Digest)
			if e.Lines != "" {
				moved[i].Lines = r.At
			}
			if r.Path != "" {
				moved[i].Path = r.Path
			}
		}
	}
	return live, strings.Join(ds, ","), moved
}

// Apply marks each semantic relation dead when none of its evidence is in the
// tree, and each present semantic unit absent the same way. A relation or
// unit without evidence is left as it is. Each live item's lines move to where
// the quote is now, so edits above it never add up to a false removal. A
// relation's quote may move only within the anchors of its source unit, and a
// unit's within its own.
func (c *Checker) Apply(units []model.Unit) []model.Unit {
	out := make([]model.Unit, len(units))
	for i, u := range units {
		if u.Semantic() && !u.Absent {
			var live bool
			live, _, u.Evidence = c.all(u.Evidence, u.Anchors)
			u.Absent = !live
		}
		// A reader's parent lives while the code it cites does.
		if u.ParentOrigin == model.Inferred && len(u.ParentEvidence) > 0 {
			var live bool
			live, _, u.ParentEvidence = c.all(u.ParentEvidence, u.Anchors)
			if !live {
				u.Parent, u.ParentOrigin, u.ParentEvidence = "", "", nil
			}
		}
		u.Relations = slices.Clone(u.Relations)
		for j, r := range u.Relations {
			if r.Semantic() {
				live, digest, es := c.all(r.Evidence, u.Anchors)
				u.Relations[j].Dead, u.Relations[j].Digest, u.Relations[j].Evidence = !live, digest, es
			}
		}
		out[i] = u
	}
	return out
}
