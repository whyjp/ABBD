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
	// Digest fingerprints the lines that hold the quote, so a turn sees an
	// edit to them even while the quote survives.
	Digest string
}

type file struct {
	raw []string
	// norm is the file with every run of whitespace folded to one space, and
	// line holds the line number of each byte of norm.
	norm string
	line []int
}

// Checker reads each file once.
type Checker struct {
	root  string
	files map[string]*file
}

func New(root string) *Checker { return &Checker{root: root, files: map[string]*file{}} }

func normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

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
				if norm.Len() > 0 {
					norm.WriteByte(' ')
					f.line = append(f.line, i+1)
				}
				norm.WriteString(w)
				for range len(w) {
					f.line = append(f.line, i+1)
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
		start, end := f.line[from+i], f.line[from+i+len(q)-1]
		if !bounded || (start <= last+Slack && end >= first-Slack) {
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
func (c *Checker) all(es []model.Evidence) (live bool, digest string) {
	var ds []string
	for _, e := range es {
		if r := c.Check(e); r.Live {
			live = true
			ds = append(ds, r.Digest)
		}
	}
	return live, strings.Join(ds, ",")
}

// Apply marks each semantic relation dead when none of its evidence is in the
// tree, and each present semantic unit absent the same way. A relation or
// unit without evidence is left as it is.
func (c *Checker) Apply(units []model.Unit) []model.Unit {
	out := make([]model.Unit, len(units))
	for i, u := range units {
		if u.Semantic() && !u.Absent {
			live, _ := c.all(u.Evidence)
			u.Absent = !live
		}
		u.Relations = slices.Clone(u.Relations)
		for j, r := range u.Relations {
			if r.Semantic() {
				live, digest := c.all(r.Evidence)
				u.Relations[j].Dead, u.Relations[j].Digest = !live, digest
			}
		}
		out[i] = u
	}
	return out
}
