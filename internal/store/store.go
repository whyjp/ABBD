// Package store reads and writes the .abbd directory.
// Markdown with YAML frontmatter is the source; everything else derives from it.
package store

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// State is the sync watermark, and the one file every sync rewrites.
// Fingerprint is a git tree of the working tree at sync time; Blueprint is
// the InputsHash the blueprint was rendered from, and Rendered the HashRender
// of the files that render wrote.
type State struct {
	Revision    string             `yaml:"revision"`
	Fingerprint string             `yaml:"fingerprint"`
	Blueprint   string             `yaml:"blueprint"`
	Rendered    string             `yaml:"rendered"`
	HighWater   map[model.Kind]int `yaml:"high_water,omitempty"`
	// Merged is set when a merge kept two syncs' lines, by union or as
	// conflict markers. The values are the first side's, which match neither
	// the merged tree nor its inputs, so abbd sync writes the state anew.
	Merged bool `yaml:"-"`
}

const notesHeading = "## Notes"

// Store is one command's view of a project's .abbd. It reads the project's
// code at most once, so open one per command.
type Store struct {
	root string
	code *codebase
}

func Open(projectRoot string) Store {
	return Store{root: filepath.Join(projectRoot, Dir), code: newCodebase(projectRoot)}
}

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
	} else if bytes.Contains(front, []byte("supplement_key:")) {
		var key struct {
			Key string `yaml:"supplement_key"`
		}
		if err := yaml.Unmarshal(front, &key); err != nil {
			return u, false, err
		}
		if u.SupplementRef != "" && key.Key != "" && u.SupplementRef != key.Key {
			return u, false, fmt.Errorf("supplement_ref %q and supplement_key %q name the unit twice; supplement_key is the older spelling of supplement_ref, so keep one", u.SupplementRef, key.Key)
		}
		u.SupplementRef = cmp.Or(u.SupplementRef, key.Key)
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

// LoadState reads the state, and a state a merge left with both sides'
// lines: it drops conflict markers, keeps each key's first value and the
// higher of each kind's high water.
func (s Store) LoadState() (State, error) {
	var st State
	b, err := os.ReadFile(s.path("state.yaml"))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if conflictMarker(l) {
			st.Merged = true
			continue
		}
		lines = append(lines, l)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &doc); err != nil {
		return st, fmt.Errorf("reading %s: %w", s.path("state.yaml"), err)
	}
	if len(doc.Content) == 0 {
		return st, nil
	}
	seen := map[string]bool{}
	for k, v := range pairs(doc.Content[0]) {
		if k.Value == "high_water" {
			for kind, n := range pairs(v) {
				var i int
				if err := n.Decode(&i); err != nil {
					return st, fmt.Errorf("reading %s: high_water %s: %w", s.path("state.yaml"), kind.Value, err)
				}
				if st.HighWater == nil {
					st.HighWater = map[model.Kind]int{}
				}
				st.HighWater[model.Kind(kind.Value)] = max(st.HighWater[model.Kind(kind.Value)], i)
			}
			continue
		}
		field := map[string]*string{"revision": &st.Revision, "fingerprint": &st.Fingerprint, "blueprint": &st.Blueprint, "rendered": &st.Rendered}[k.Value]
		switch {
		case field == nil:
		case !seen[k.Value]:
			*field, seen[k.Value] = v.Value, true
		case *field != v.Value:
			st.Merged = true
		}
	}
	return st, nil
}

// conflictMarker is a line git writes around a conflict.
func conflictMarker(l string) bool {
	for _, m := range []string{"<<<<<<<", "|||||||", "=======", ">>>>>>>"} {
		if l == m || strings.HasPrefix(l, m+" ") {
			return true
		}
	}
	return false
}

// pairs walks a YAML mapping's keys and values in order, duplicates included.
func pairs(n *yaml.Node) func(yield func(k, v *yaml.Node) bool) {
	return func(yield func(k, v *yaml.Node) bool) {
		if n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if !yield(n.Content[i], n.Content[i+1]) {
				return
			}
		}
	}
}

func (s Store) SaveState(st State) error { return writeYAML(s.path("state.yaml"), st) }

// InputsHash fingerprints every file under .abbd that a page can render, so
// the blueprint is STALE whenever any of them changed. Neither the blueprint
// nor the state, which records this hash, is an input.
func (s Store) InputsHash() (string, error) {
	var files []string
	err := filepath.WalkDir(s.root, func(p string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && p == s.path("blueprint"):
			return filepath.SkipDir
		case p == s.path("state.yaml"):
			return nil
		case !d.IsDir():
			files = append(files, p)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return s.hashTree(s.root, files)
}

// hashTree hashes files by their path relative to dir.
func (s Store) hashTree(dir string, files []string) (string, error) {
	contents := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(dir, f)
		contents[filepath.ToSlash(rel)] = b
	}
	return HashRender(contents), nil
}

// HashRender hashes files by name. It reads CRLF as LF, so a checkout that
// converts line endings hashes the same as the one that wrote the files.
func HashRender(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		b := bytes.ReplaceAll(files[name], []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(h, "%s\x00%d\x00", name, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
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

// SaveBlueprint writes the rendered files and removes every other one, so the
// blueprint holds exactly what HashRender(files) records.
func (s Store) SaveBlueprint(files map[string][]byte) error {
	for name, b := range files {
		if err := writeFile(s.path("blueprint", name), b); err != nil {
			return err
		}
	}
	old, err := os.ReadDir(s.path("blueprint"))
	if err != nil {
		return err
	}
	for _, e := range old {
		if _, keep := files[e.Name()]; !keep {
			if err := os.RemoveAll(s.path("blueprint", e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// BlueprintHash is the HashRender of the blueprint's files as they are now,
// and "" when there are none.
func (s Store) BlueprintHash() (string, error) {
	dir := s.path("blueprint")
	var files []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if os.IsNotExist(err) || len(files) == 0 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return s.hashTree(dir, files)
}

func frontmatter(v any, body string) ([]byte, error) {
	front, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(front) + "---\n\n" + body), nil
}

var errNoFrontmatter = errors.New("missing frontmatter")

func readFrontmatter(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return fmt.Errorf("%s: %w", path, errNoFrontmatter)
	}
	front, _, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return fmt.Errorf("%s: unterminated frontmatter", path)
	}
	return yaml.Unmarshal(front, v)
}
