// Package store reads and writes the .abbd directory.
// Markdown with YAML frontmatter is the source; everything else derives from it.
package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/gitx"
	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
)

const Dir = ".abbd"

// State is the sync watermark. Fingerprint is a git tree of the working tree at sync time.
type State struct {
	Revision    string             `yaml:"revision"`
	Fingerprint string             `yaml:"fingerprint"`
	MDHash      string             `yaml:"md_hash"`
	HighWater   map[model.Kind]int `yaml:"high_water,omitempty"`
}

const notesHeading = "## Notes"

type Store struct{ root string }

func Open(projectRoot string) Store { return Store{root: filepath.Join(projectRoot, Dir)} }

func (s Store) path(parts ...string) string {
	return filepath.Join(append([]string{s.root}, parts...)...)
}

func (s Store) unitDirs() []string { return []string{"architecture", "features"} }

// ErrOldFormat refuses a write to a state in the old format, which cannot
// tell the supplement's claims from a reader's.
var ErrOldFormat = fmt.Errorf("the state is in the old format; run abbd migrate first")

// LoadUnits reads every unit file, in either format, and resolves the read
// model.
func (s Store) LoadUnits() ([]model.Unit, error) {
	units, old, err := s.loadUnits()
	if err != nil {
		return nil, err
	}
	finish(units, old)
	return units, nil
}

// Writable is LoadUnits for a command that will write: it refuses a state
// with any file in the old format.
func (s Store) Writable() ([]model.Unit, error) {
	units, old, err := s.loadUnits()
	if err != nil {
		return nil, err
	}
	if len(old) > 0 {
		return nil, ErrOldFormat
	}
	finish(units, old)
	return units, nil
}

// Legacy loads the state and names the units still in the old format.
func (s Store) Legacy() ([]model.Unit, map[model.ID]bool, error) {
	units, old, err := s.loadUnits()
	if err != nil {
		return nil, nil, err
	}
	finish(units, old)
	return units, old, nil
}

func finish(units []model.Unit, old map[model.ID]bool) {
	regroup(units, old)
	model.Resolve(units)
}

func (s Store) loadUnits() ([]model.Unit, map[model.ID]bool, error) {
	var units []model.Unit
	old := map[model.ID]bool{}
	seen := map[model.ID]string{}
	for _, d := range s.unitDirs() {
		files, err := filepath.Glob(s.path(d, "*.md"))
		if err != nil {
			return nil, nil, err
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, nil, err
			}
			u, isOld, err := parseUnit(f, b)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", f, err)
			}
			if u.Kind.Dir() != d {
				return nil, nil, fmt.Errorf("%s: kind %s belongs in %s/", f, u.Kind, u.Kind.Dir())
			}
			if other, dup := seen[u.ID]; dup {
				return nil, nil, fmt.Errorf("%s: id %s is also used by %s", f, u.ID, other)
			}
			seen[u.ID] = f
			if isOld {
				old[u.ID] = true
			}
			units = append(units, u)
		}
	}
	return units, old, nil
}

// UnitsAt reads the unit files as they were committed at rev. A revision
// without .abbd/ has no units.
func (s Store) UnitsAt(rev string) ([]model.Unit, error) {
	project := filepath.Dir(s.root)
	var dirs []string
	for _, d := range s.unitDirs() {
		dirs = append(dirs, Dir+"/"+d)
	}
	files, err := gitx.Files(project, rev, dirs...)
	if err != nil {
		return nil, err
	}
	var units []model.Unit
	old := map[model.ID]bool{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		b, err := gitx.Show(project, rev, f)
		if err != nil {
			return nil, err
		}
		u, isOld, err := parseUnit(f, b)
		if err != nil {
			return nil, fmt.Errorf("%s at %s: %w", f, rev, err)
		}
		if isOld {
			old[u.ID] = true
		}
		units = append(units, u)
	}
	finish(units, old)
	return units, nil
}

func parseUnit(path string, b []byte) (u model.Unit, old bool, err error) {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return u, false, fmt.Errorf("missing frontmatter")
	}
	front, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return u, false, fmt.Errorf("unterminated frontmatter")
	}
	if old = legacy(front); old {
		var l legacyUnit
		if err := yaml.Unmarshal(front, &l); err != nil {
			return u, true, err
		}
		u = l.convert()
	} else if err := yaml.Unmarshal(front, &u); err != nil {
		return u, false, err
	}
	if i := bytes.Index(body, []byte("\n"+notesHeading)); i >= 0 {
		u.Notes = string(body[i+1:])
	}
	if want := string(u.ID) + ".md"; filepath.Base(path) != want {
		return u, old, fmt.Errorf("file name does not match id %s", u.ID)
	}
	return u, old, u.Validate()
}

func renderUnit(u model.Unit) ([]byte, error) {
	front, err := yaml.Marshal(u)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("---\n\n# ")
	b.WriteString(u.Name.Value)
	b.WriteString("\n\n")
	for _, a := range u.Anchors {
		b.WriteString("- `" + a.Path + "`")
		if a.Symbol != "" {
			b.WriteString(" `" + a.Symbol + "`")
		}
		b.WriteString("\n")
	}
	if u.Notes != "" {
		b.WriteString("\n")
		b.WriteString(u.Notes)
	}
	return b.Bytes(), nil
}

// SaveUnits writes each unit in the current format. Each unit's shown and
// hidden claims are resolved again first, so the file always partitions
// them the way a load would.
func (s Store) SaveUnits(units []model.Unit) error {
	for _, u := range units {
		u.Parents = slices.Clone(u.Parents)
		one := []model.Unit{u}
		model.Resolve(one)
		u = one[0]
		if err := u.Validate(); err != nil {
			return err
		}
		b, err := renderUnit(u)
		if err != nil {
			return err
		}
		if err := writeFile(s.path(u.Kind.Dir(), string(u.ID)+".md"), b); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) LoadCandidates() ([]identity.Candidate, error) {
	var cs []identity.Candidate
	return cs, readYAML(s.path("candidates.yaml"), &cs)
}

func (s Store) SaveCandidates(cs []identity.Candidate) error {
	if cs == nil {
		cs = []identity.Candidate{}
	}
	return writeYAML(s.path("candidates.yaml"), cs)
}

func (s Store) LoadState() (State, error) {
	var st State
	return st, readYAML(s.path("state.yaml"), &st)
}

func (s Store) SaveState(st State) error { return writeYAML(s.path("state.yaml"), st) }

// MDHash fingerprints every unit file so derived views can detect staleness.
func (s Store) MDHash() (string, error) {
	var files []string
	for _, d := range s.unitDirs() {
		fs, err := filepath.Glob(s.path(d, "*.md"))
		if err != nil {
			return "", err
		}
		files = append(files, fs...)
	}
	return s.hashFiles(files)
}

// InputsHash fingerprints every file under .abbd that a page can render, so
// the blueprint is STALE whenever any of them changed. The blueprint is not
// an input.
func (s Store) InputsHash() (string, error) {
	var files []string
	err := filepath.WalkDir(s.root, func(p string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && p == s.path("blueprint"):
			return filepath.SkipDir
		case !d.IsDir():
			files = append(files, p)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return s.hashFiles(files)
}

func (s Store) hashFiles(files []string) (string, error) {
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(s.root, f)
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

func writeYAML(path string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return writeFile(path, b)
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Project is the per-project configuration in project.yaml.
type Project struct {
	// Extract is the argv of the project extraction script pstack generated.
	// It prints an extraction result on stdout.
	Extract []string `yaml:"extract,omitempty"`
}

func (s Store) LoadProject() (Project, error) {
	var p Project
	return p, readYAML(s.path("project.yaml"), &p)
}

func (s Store) SaveProject(p Project) error { return writeYAML(s.path("project.yaml"), p) }

// NextID mints the next number of kind and records it, so it is never reused.
func (s Store) NextID(kind model.Kind) (model.ID, error) {
	st, err := s.LoadState()
	if err != nil {
		return "", err
	}
	if st.HighWater == nil {
		st.HighWater = map[model.Kind]int{}
	}
	st.HighWater[kind]++
	return model.FormatID(kind, st.HighWater[kind]), s.SaveState(st)
}

// The blueprint derives from the state and is rewritten freely.
func (s Store) SaveBlueprint(name string, b []byte) error {
	return writeFile(s.path("blueprint", name), b)
}

// ReadBlueprint returns nil when the file was never rendered.
func (s Store) ReadBlueprint(name string) ([]byte, error) {
	b, err := os.ReadFile(s.path("blueprint", name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}

func frontmatter(v any, body string) ([]byte, error) {
	front, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(front) + "---\n\n" + body), nil
}

func readFrontmatter(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return fmt.Errorf("%s: missing frontmatter", path)
	}
	front, _, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return fmt.Errorf("%s: unterminated frontmatter", path)
	}
	return yaml.Unmarshal(front, v)
}
