package store

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
)

// Note is what a reader wrote into one slot of a module or a contract, and
// the code it cites. Unlike a module's text, which describes all of the
// module's code, a note describes only the places it cites: Code holds one
// hash per cited place, of the lines around the quote wherever it sits now,
// and Digest is the hash of Code. So an edit near one note's quotes stales
// that note alone. Why is a human's reason for a decision, with where they
// stated it; a reader storing the note again keeps it.
type Note struct {
	Unit     model.ID          `yaml:"unit"`
	Slot     string            `yaml:"slot"`
	Origin   model.Origin      `yaml:"origin"`
	Digest   string            `yaml:"digest"`
	Format   int               `yaml:"format"`
	Evidence []model.Evidence  `yaml:"evidence"`
	Code     map[string]string `yaml:"code"`
	Why      *model.Fact       `yaml:"why,omitempty"`
	Text     string            `yaml:"-"`
	// Ref is the note's slot, ok false when the slot is no longer one
	// abbd knows. Changed lists the cited places whose code moved since the
	// note was written; Load fills both.
	Ref     model.NoteRef `yaml:"-"`
	Known   bool          `yaml:"-"`
	Changed []string      `yaml:"-"`
}

// Stale reports whether the code a note cites changed since it was written.
func (n Note) Stale() bool { return len(n.Changed) > 0 }

// NoteStamp is the digest and Code of the places a note cites, as the code
// is now. Sync never moves a note's lines, so a quote its file no longer
// holds exactly once counts as missing, rather than as whichever copy now
// sits near the lines the note was written at.
func (s Store) NoteStamp(cited []model.Evidence) (string, map[string]string) {
	_, code := s.stamp(moduleCode{anchors: map[string]anchorCode{}, cited: cited})
	for _, e := range cited {
		if s.code.ev.Occurrences(e) == 1 {
			continue
		}
		for place := 1; ; place++ {
			k := quoteKey(e, place)
			if _, ok := code[k]; !ok {
				break
			}
			code[k] = missing
		}
	}
	return digestOf(code), code
}

// SaveNote writes a note. It refuses one that cites no code, which would
// read current whatever the code did.
func (s Store) SaveNote(n Note) error {
	if n.Digest == "" || len(n.Evidence) == 0 {
		return fmt.Errorf("%s.%s cites no code to check it against", n.Unit, n.Slot)
	}
	for i := range n.Evidence {
		n.Evidence[i] = model.Evidence{Path: n.Evidence[i].Path, Lines: n.Evidence[i].Lines, Symbol: n.Evidence[i].Symbol, Quote: n.Evidence[i].Quote}
	}
	b, err := frontmatter(n, CleanText(n.Text)+"\n")
	if err != nil {
		return err
	}
	return writeFile(s.path("understanding", string(n.Unit), n.Slot+".md"), b)
}

// LoadNotes reads every note, by unit, in slot order and then by key, each
// checked against the code it cites now.
func (s Store) LoadNotes() (map[model.ID][]Note, error) {
	files, err := filepath.Glob(s.path("understanding", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	out := map[model.ID][]Note{}
	for _, f := range files {
		var n Note
		if err := readFrontmatter(f, &n); err != nil {
			return nil, err
		}
		if n.Text, err = readBody(f); err != nil {
			return nil, err
		}
		r, err := model.ParseNoteRef(string(n.Unit) + "." + n.Slot)
		n.Ref, n.Known = r, err == nil
		n.Changed = s.noteChanged(n)
		out[n.Unit] = append(out[n.Unit], n)
	}
	for id := range out {
		slices.SortFunc(out[id], func(a, b Note) int {
			if c := rank(a) - rank(b); c != 0 {
				return c
			}
			return strings.Compare(a.Slot, b.Slot)
		})
	}
	return out, nil
}

// LoadNote reads one note, false when there is none.
func (s Store) LoadNote(r model.NoteRef) (Note, bool, error) {
	notes, err := s.LoadNotes()
	if err != nil {
		return Note{}, false, err
	}
	i := slices.IndexFunc(notes[r.Unit], func(n Note) bool { return n.Slot == r.Name() })
	if i < 0 {
		return Note{}, false, nil
	}
	return notes[r.Unit][i], true, nil
}

func rank(n Note) int {
	if !n.Known {
		return len(model.Slots)
	}
	return slices.IndexFunc(model.Slots, func(s model.Slot) bool { return s.Name == n.Ref.Slot.Name })
}

// noteChanged lists each cited place whose code differs from what the note
// was written against: its lines changed, or its quote is gone.
func (s Store) noteChanged(n Note) []string {
	digest, now := s.NoteStamp(n.Evidence)
	if digest == n.Digest {
		return nil
	}
	var out []string
	for k, h := range now {
		if n.Code[k] != h {
			reason := "the lines around it changed"
			if h == missing {
				reason = "the quote is gone, or no longer once in the file"
			}
			out = append(out, fmt.Sprintf("%s (%s)", keyPath(k), reason))
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) == 0 {
		out = []string{"its stored code does not match its evidence"}
	}
	return out
}
