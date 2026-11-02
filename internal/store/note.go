package store

import (
	"errors"
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
// stated it; a reader storing the decision again on the same quotes keeps it.
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
	// File is where the note sits, from the project root. Ref is the
	// note's slot. Known is false when the slot is no longer one abbd knows
	// or the file is not where the note of its slot belongs, Misfiled true
	// in the second case; either way nothing reads it as its slot's note.
	// Changed lists the cited places whose code moved since the note was
	// written. LoadNotes fills them all.
	File     string        `yaml:"-"`
	Ref      model.NoteRef `yaml:"-"`
	Known    bool          `yaml:"-"`
	Misfiled bool          `yaml:"-"`
	Changed  []string      `yaml:"-"`
}

// NoteFile is where the note of a unit's slot belongs, from the project root.
func NoteFile(unit model.ID, slot string) string {
	return Dir + "/understanding/" + string(unit) + "/" + slot + ".md"
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
// checked against the code it cites now. A markdown file there that does not
// open with frontmatter, such as a README, is no note: LoadNotes returns its
// path among the strays, from the project root. A file that opens
// frontmatter is a note, and one that is malformed fails.
func (s Store) LoadNotes() (map[model.ID][]Note, []string, error) {
	files, err := filepath.Glob(s.path("understanding", "*", "*.md"))
	if err != nil {
		return nil, nil, err
	}
	out := map[model.ID][]Note{}
	var strays []string
	for _, f := range files {
		rel, err := filepath.Rel(s.root, f)
		if err != nil {
			return nil, nil, err
		}
		file := Dir + "/" + filepath.ToSlash(rel)
		var n Note
		if err := readFrontmatter(f, &n); errors.Is(err, errNoFrontmatter) {
			strays = append(strays, file)
			continue
		} else if err != nil {
			return nil, nil, err
		}
		if n.Text, err = readBody(f); err != nil {
			return nil, nil, err
		}
		r, err := model.ParseNoteRef(string(n.Unit) + "." + n.Slot)
		n.File, n.Misfiled = file, file != NoteFile(n.Unit, n.Slot)
		n.Ref, n.Known = r, err == nil && !n.Misfiled
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
	return out, strays, nil
}

// LoadNote reads one note, false when there is none.
func (s Store) LoadNote(r model.NoteRef) (Note, bool, error) {
	notes, _, err := s.LoadNotes()
	if err != nil {
		return Note{}, false, err
	}
	i := slices.IndexFunc(notes[r.Unit], func(n Note) bool { return n.Known && n.Slot == r.Name() })
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
	if digest, _ := s.NoteStamp(n.Evidence); digest == n.Digest {
		return nil
	}
	if out := s.changed(n.Evidence, n.Code); out != nil {
		return out
	}
	return []string{"its stored code does not match its evidence"}
}
