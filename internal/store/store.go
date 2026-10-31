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
	"sort"

	"gopkg.in/yaml.v3"

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

func Open(projectRoot string) Store { return Store{filepath.Join(projectRoot, Dir)} }

func (s Store) path(parts ...string) string {
	return filepath.Join(append([]string{s.root}, parts...)...)
}

func (s Store) unitDirs() []string { return []string{"architecture", "features"} }

func (s Store) LoadUnits() ([]model.Unit, error) {
	var units []model.Unit
	seen := map[model.ID]string{}
	for _, d := range s.unitDirs() {
		files, err := filepath.Glob(s.path(d, "*.md"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			u, err := readUnit(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			if u.Kind.Dir() != d {
				return nil, fmt.Errorf("%s: kind %s belongs in %s/", f, u.Kind, u.Kind.Dir())
			}
			if other, dup := seen[u.ID]; dup {
				return nil, fmt.Errorf("%s: id %s is also used by %s", f, u.ID, other)
			}
			seen[u.ID] = f
			units = append(units, u)
		}
	}
	return units, nil
}

func readUnit(path string) (model.Unit, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.Unit{}, err
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return model.Unit{}, fmt.Errorf("missing frontmatter")
	}
	front, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return model.Unit{}, fmt.Errorf("unterminated frontmatter")
	}
	var u model.Unit
	if err := yaml.Unmarshal(front, &u); err != nil {
		return model.Unit{}, err
	}
	if i := bytes.Index(body, []byte("\n"+notesHeading)); i >= 0 {
		u.Notes = string(body[i+1:])
	}
	if want := string(u.ID) + ".md"; filepath.Base(path) != want {
		return model.Unit{}, fmt.Errorf("file name does not match id %s", u.ID)
	}
	return u, u.Validate()
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

func (s Store) SaveUnits(units []model.Unit) error {
	for _, u := range units {
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
