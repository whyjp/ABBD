// Package evidence checks, deterministically, that the code still holds the
// excerpts a reader cited for a semantic unit, relation or parent. A reader
// finds a coupling; this package only decides whether the same tree still
// shows it, so the same tree always gives the same answer.
//
// An item's scope is the file it names. It is live while the file holds the
// quote, drifted while the file exists without it, and dead once the file is
// gone or a sync at a later revision still finds it drifted. A quote never
// follows into another file.
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
	// Gone says the file the item names does not exist.
	Gone bool
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

func (f *file) result(m match) Result {
	h := sha256.Sum256([]byte(strings.Join(f.raw[m.start-1:m.end], "\n")))
	at := fmt.Sprint(m.start)
	if m.end != m.start {
		at += fmt.Sprintf("-%d", m.end)
	}
	return Result{Live: true, At: at, Digest: hex.EncodeToString(h[:8])}
}

// Check finds e's quote in its file, whitespace-insensitively: nearest the
// lines e names within the slack first, then the quote's only occurrence in the
// file. A quote found twice with neither near is not found, so the same tree
// always gives the same answer.
func (c *Checker) Check(e model.Evidence) Result {
	f := c.load(e.Path)
	if f == nil {
		return Result{Gone: true}
	}
	if m, ok := f.find(e); ok {
		return f.result(m)
	}
	return Result{}
}

func (f *file) find(e model.Evidence) (match, bool) {
	q := normalize(e.Quote)
	if q == "" {
		return match{}, false
	}
	first, last, bounded := e.Span()
	ms := f.matches(q)
	if len(ms) > 0 && !bounded {
		return ms[0], true
	}
	// The nearest occurrence wins, so two places of one quote within each
	// other's slack each keep their own.
	best, gap := match{}, Slack+1
	for _, m := range ms {
		if d := max(m.start-last, first-m.end, 0); d < gap {
			best, gap = m, d
		}
	}
	if gap <= Slack {
		return best, true
	}
	if len(ms) == 1 {
		return ms[0], true
	}
	return match{}, false
}

// Around returns the lines that hold e's quote now and n lines on each side,
// or false when Check would not find the quote.
func (c *Checker) Around(e model.Evidence, n int) (string, bool) {
	f := c.load(e.Path)
	if f == nil {
		return "", false
	}
	m, ok := f.find(e)
	if !ok {
		return "", false
	}
	from, to := max(m.start-n, 1), min(m.end+n, len(f.raw))
	return strings.Join(f.raw[from-1:to], "\n"), true
}

// Status checks one item and returns it with its new status. rev is the
// revision of the sync; "" checks without moving anything toward dead.
// drifts is false for the script's evidence, which the script restates on
// every sync, so it is live or dead.
func (c *Checker) Status(e model.Evidence, rev string, drifts bool) (model.Evidence, Result) {
	if e.Status == model.Dead && drifts {
		return e, Result{}
	}
	r := c.Check(e)
	switch {
	case r.Live:
		if e.Lines != "" {
			e.Lines = r.At
		}
		e.Status, e.Since, e.Reason = model.Live, "", ""
	case r.Gone:
		e.Status, e.Since, e.Reason = model.Dead, "", fmt.Sprintf("%s is gone", e.Path)
	case !drifts:
		e.Status, e.Since, e.Reason = model.Dead, "", "the script's quote is not in the file"
	case e.Status == model.Drifted && e.Since != "" && rev != "" && e.Since != rev:
		e.Status, e.Reason = model.Dead, fmt.Sprintf("drifted since %s and not refreshed", short(e.Since))
		e.Since = ""
	case e.Status != model.Drifted:
		e.Status, e.Since = model.Drifted, rev
	case e.Since == "":
		e.Since = rev
	}
	return e, r
}

func short(rev string) string { return rev[:min(len(rev), 12)] }

// all checks a claim's items and returns them with the digest of the live
// ones, in order.
func (c *Checker) all(es []model.Evidence, rev string, drifts bool) ([]model.Evidence, string) {
	out := slices.Clone(es)
	var ds []string
	for i, e := range es {
		var r Result
		out[i], r = c.Status(e, rev, drifts)
		if r.Live {
			ds = append(ds, r.Digest)
		}
	}
	return out, strings.Join(ds, ",")
}

// Apply checks every item of every present unit: a semantic unit's own, its
// parent claims' and its relation claims'. A present semantic unit whose
// claim is dead goes absent. Each live item's lines move to where the quote
// is now, so edits above it never add up to a false removal.
func (c *Checker) Apply(units []model.Unit, rev string) []model.Unit {
	out := make([]model.Unit, len(units))
	for i, u := range units {
		out[i] = u
		if u.Absent {
			continue
		}
		v := &out[i]
		if u.Semantic() {
			v.Evidence, _ = c.all(u.Evidence, rev, true)
			if model.StatusOf(v.Evidence) == model.Dead {
				v.SetAbsent(model.EvidenceDead)
			}
		}
		v.Parents = slices.Clone(u.Parents)
		for j, p := range v.Parents {
			if len(p.Evidence) > 0 {
				v.Parents[j].Evidence, _ = c.all(p.Evidence, rev, true)
			}
		}
		for _, rs := range []*[]model.Relation{&v.Relations, &v.Hidden} {
			*rs = slices.Clone(*rs)
			for j, r := range *rs {
				if len(r.Evidence) > 0 {
					(*rs)[j].Evidence, (*rs)[j].Digest = c.all(r.Evidence, rev, r.Writer != model.Script)
				}
			}
		}
	}
	model.Resolve(out)
	return out
}
