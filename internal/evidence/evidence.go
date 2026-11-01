// Package evidence checks, deterministically, that the code still holds the
// excerpts a reader cited for a semantic unit or relation. A reader finds a
// coupling; this package only decides whether the same tree still shows it,
// so the same tree always gives the same answer.
package evidence

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
}

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

// Check finds e's quote in its file, whitespace-insensitively, within its
// lines and the slack when it names lines, and anywhere in the file otherwise.
func (c *Checker) Check(e model.Evidence) Result {
	f := c.load(e.Path)
	q := normalize(e.Quote)
	if f == nil || q == "" {
		return Result{}
	}
	first, last, bounded := e.Span()
	for from := 0; from <= len(f.norm); {
		i := strings.Index(f.norm[from:], q)
		if i < 0 {
			break
		}
		at, after := from+i, from+i+len(q)
		start, end := f.line[at], f.line[after-1]
		// A quote ending in a word must end where the code's word ends, so
		// "charges" does not live on in "charges_archive".
		whole := (at == 0 || f.gap[at] || !word(f.norm[at-1]) || !word(q[0])) &&
			(after == len(f.norm) || f.gap[after] || !word(f.norm[after]) || !word(q[len(q)-1]))
		if whole && (!bounded || (start <= last+Slack && end >= first-Slack)) {
			h := sha256.Sum256([]byte(strings.Join(f.raw[start-1:end], "\n")))
			at := fmt.Sprint(start)
			if end != start {
				at += fmt.Sprintf("-%d", end)
			}
			return Result{Live: true, At: at, Digest: hex.EncodeToString(h[:8])}
		}
		from += i + 1
	}
	return Result{}
}

// all checks a fact's evidence: it is live while any item is, and its digest
// covers every live item in order.
func (c *Checker) all(es []model.Evidence) (live bool, digest string, moved []model.Evidence) {
	var ds []string
	moved = slices.Clone(es)
	for i, e := range es {
		if r := c.Check(e); r.Live {
			live = true
			ds = append(ds, r.Digest)
			if e.Lines != "" {
				moved[i].Lines = r.At
			}
		}
	}
	return live, strings.Join(ds, ","), moved
}

// Apply marks each semantic relation dead when none of its evidence is in the
// tree, and each present semantic unit absent the same way. A relation or
// unit without evidence is left as it is. Each live item's lines move to where
// the quote is now, so edits above it never add up to a false removal.
func (c *Checker) Apply(units []model.Unit) []model.Unit {
	out := make([]model.Unit, len(units))
	for i, u := range units {
		if u.Semantic() && !u.Absent {
			var live bool
			live, _, u.Evidence = c.all(u.Evidence)
			u.Absent = !live
		}
		// A reader's parent lives while the code it cites does.
		if u.ParentOrigin == model.Inferred && len(u.ParentEvidence) > 0 {
			var live bool
			live, _, u.ParentEvidence = c.all(u.ParentEvidence)
			if !live {
				u.Parent, u.ParentOrigin, u.ParentEvidence = "", "", nil
			}
		}
		u.Relations = slices.Clone(u.Relations)
		for j, r := range u.Relations {
			if r.Semantic() {
				live, digest, es := c.all(r.Evidence)
				u.Relations[j].Dead, u.Relations[j].Digest, u.Relations[j].Evidence = !live, digest, es
			}
		}
		out[i] = u
	}
	return out
}
