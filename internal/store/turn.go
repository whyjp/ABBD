package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/turn"
)

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

type Requirement struct {
	ID     string       `yaml:"id"`
	Origin model.Origin `yaml:"origin"`
	Turn   string       `yaml:"turn"`
}

func (s Store) WriteRequirement(r Requirement, text string) error {
	b, err := frontmatter(r, strings.TrimRight(text, "\n")+"\n")
	if err != nil {
		return err
	}
	return writeFile(s.path("requirements", r.ID+".md"), b)
}

func (s Store) SaveTurn(t turn.Turn) error {
	b, err := frontmatter(t, renderTurn(t))
	if err != nil {
		return err
	}
	return writeFile(s.path("changes", t.ID+".md"), b)
}

func renderTurn(t turn.Turn) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s\n\nRequest: %s\n\n## Impact\n\n", t.ID, t.State, t.Requirement)
	for _, i := range t.Impact {
		fmt.Fprintf(&b, "- %s: %s (%s)\n", i.Unit, i.Why, i.By)
	}
	if r := t.Report; r != nil {
		list := func(title string, items []string) {
			if len(items) > 0 {
				fmt.Fprintf(&b, "\n## %s\n\n- %s\n", title, strings.Join(items, "\n- "))
			}
		}
		list("Gate", r.Gate)
		list("Declared but not done", r.DeclaredNotDone)
		list("Done but not declared", r.DoneNotDeclared)
		if r.Why != "" {
			fmt.Fprintf(&b, "\n%s\n", r.Why)
		}
	}
	return b.String()
}

// LatestTurn returns the most recent turn, or nil before the first.
func (s Store) LatestTurn() (*turn.Turn, error) {
	files, err := filepath.Glob(s.path("changes", "CHG-*.md"))
	if err != nil || len(files) == 0 {
		return nil, err
	}
	sort.Strings(files)
	var t turn.Turn
	if err := readFrontmatter(files[len(files)-1], &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s Store) SaveRun(r turn.Run, output []byte) error {
	if err := writeFile(s.path(filepath.FromSlash(r.Output)), output); err != nil {
		return err
	}
	return writeYAML(s.path("verification", "runs", r.ID+".yaml"), r)
}

// LoadRun returns nil when id is empty.
func (s Store) LoadRun(id string) (*turn.Run, error) {
	if id == "" {
		return nil, nil
	}
	var r turn.Run
	if err := readYAML(s.path("verification", "runs", id+".yaml"), &r); err != nil {
		return nil, err
	}
	if r.ID != id {
		return nil, fmt.Errorf("verification run %s is missing", id)
	}
	return &r, nil
}

func (s Store) LoadADRs() ([]ADR, error) {
	files, err := filepath.Glob(s.path("decisions", "ADR-*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []ADR
	for _, f := range files {
		var a ADR
		if err := readFrontmatter(f, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
