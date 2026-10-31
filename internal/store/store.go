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
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/identity"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/topology"
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

// LoadIntended returns nil before the first alignment.
func (s Store) LoadIntended() (*topology.Intended, error) {
	if _, err := os.Stat(s.path("intended.yaml")); os.IsNotExist(err) {
		return nil, nil
	}
	var in topology.Intended
	if err := readYAML(s.path("intended.yaml"), &in); err != nil {
		return nil, err
	}
	return &in, nil
}

func (s Store) SaveIntended(in topology.Intended) error {
	return writeYAML(s.path("intended.yaml"), in)
}

type ADR struct {
	ID       string       `yaml:"id"`
	Grade    string       `yaml:"grade"`
	Subject  string       `yaml:"subject"`
	Decision string       `yaml:"decision"`
	Origin   model.Origin `yaml:"origin"`
}

func (s Store) WriteADR(a ADR, body string) error {
	front, err := yaml.Marshal(a)
	if err != nil {
		return err
	}
	return writeFile(s.path("decisions", a.ID+".md"), []byte("---\n"+string(front)+"---\n\n"+body+"\n"))
}

var sessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (s Store) sessionPath(id string) (string, error) {
	if !sessionID.MatchString(id) {
		return "", fmt.Errorf("invalid session id %q", id)
	}
	return s.path("sessions", id+".on"), nil
}

// Session state is local to this machine, so sessions/ is kept out of git.
func (s Store) SetMode(id string, on bool) error {
	p, err := s.sessionPath(id)
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := writeFile(s.path(".gitignore"), []byte("sessions/\n")); err != nil {
		return err
	}
	return writeFile(p, nil)
}

func (s Store) ModeOn(id string) bool {
	p, err := s.sessionPath(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func (s Store) AppendJournal(id, line string) error {
	p, err := s.sessionPath(id)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(strings.TrimSuffix(p, ".on")+".journal", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}
