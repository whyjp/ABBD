// Package ssot owns the Markdown source of truth under .cccv/.
// Each object is one Markdown file: YAML frontmatter is the structured original, an
// auto-generated summary block follows, and anything after it belongs to the user and is
// preserved on rewrite.
package ssot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/whyjp/cccv/internal/model"
	"gopkg.in/yaml.v3"
)

const Dir = ".cccv"

const (
	autoStart = "<!-- fmh:auto:start -->"
	autoEnd   = "<!-- fmh:auto:end -->"
)

// Snapshot is the whole project intelligence loaded in memory.
type Snapshot struct {
	Root         string
	Project      Project
	State        State
	Features     map[string]*model.Feature
	Modules      map[string]*model.Module
	Tests        map[string]*model.Test
	Decisions    map[string]*model.Decision
	Changes      map[string]*model.Change
	Requirements map[string]*model.Requirement
	Candidates   []model.Candidate
}

type Budget struct {
	ModelCalls   int `yaml:"model_calls" json:"model_calls"`
	GraphQueries int `yaml:"graph_queries" json:"graph_queries"`
	TestRuns     int `yaml:"test_runs" json:"test_runs"`
}

type Project struct {
	Name         string            `yaml:"name"`
	Mode         string            `yaml:"mode"`
	CreatedAt    string            `yaml:"created_at"`
	Languages    []string          `yaml:"languages"`
	Verification VerificationCfg   `yaml:"verification"`
	Policy       PolicyCfg         `yaml:"policy"`
	Budgets      map[string]Budget `yaml:"budgets"`
	Preferences  map[string]string `yaml:"preferences"`
}

type VerificationCfg struct {
	TestCommands  []string `yaml:"test_commands"`
	BuildCommands []string `yaml:"build_commands"`
}

type PolicyCfg struct {
	MaxQuestionsPerTurn int               `yaml:"max_questions_per_turn"`
	LocalCommitTier     model.Tier        `yaml:"local_commit_tier"`
	TierBTimeout        string            `yaml:"tier_b_timeout"`
	ActionTiers         map[string]string `yaml:"action_tiers,omitempty"`
}

// State is the sync watermark: what revision and file contents the maps were last synced to.
type State struct {
	Revision    string `yaml:"revision"`
	Fingerprint string `yaml:"fingerprint"`
	SSOTHash    string `yaml:"ssot_hash"`
	SyncedAt    string `yaml:"synced_at"`
	ActiveTurn  string `yaml:"active_turn,omitempty"`
	// TurnGraphQueries counts knowledge queries of the active turn until verify or sync folds it into the CHG.
	TurnGraphQueries int `yaml:"turn_graph_queries,omitempty"`
	// Commits maps a CHG to the local commit `sync --commit` made for it.
	Commits map[string]string `yaml:"commits,omitempty"`
	Files   map[string]string `yaml:"files"`
}

func DefaultProject(name, mode string) Project {
	return Project{
		Name: name,
		Mode: mode,
		Policy: PolicyCfg{
			MaxQuestionsPerTurn: 3,
			LocalCommitTier:     model.TierA,
			TierBTimeout:        "end-of-turn",
		},
		Budgets: map[string]Budget{
			"T0": {ModelCalls: 5, GraphQueries: 2, TestRuns: 1},
			"T1": {ModelCalls: 15, GraphQueries: 4, TestRuns: 3},
			"T2": {ModelCalls: 40, GraphQueries: 10, TestRuns: 6},
			"T3": {ModelCalls: 80, GraphQueries: 20, TestRuns: 12},
		},
		Preferences: map[string]string{"language": "ko", "verification_strength": "conservative", "question_frequency": "minimal"},
	}
}

func Path(root string, parts ...string) string {
	return filepath.Join(append([]string{root, Dir}, parts...)...)
}

func Exists(root string) bool {
	_, err := os.Stat(Path(root, "project.yaml"))
	return err == nil
}

func NewSnapshot(root string) *Snapshot {
	return &Snapshot{
		Root:         root,
		Features:     map[string]*model.Feature{},
		Modules:      map[string]*model.Module{},
		Tests:        map[string]*model.Test{},
		Decisions:    map[string]*model.Decision{},
		Changes:      map[string]*model.Change{},
		Requirements: map[string]*model.Requirement{},
	}
}

// collection describes one directory of objects. The table keeps load/save free of per-kind branches.
type collection struct {
	dir   string
	load  func(s *Snapshot, data []byte) error
	items func(s *Snapshot) map[string]any
}

func decodeInto[T any](m map[string]*T, id func(*T) string) func(*Snapshot, []byte) error {
	return func(_ *Snapshot, data []byte) error {
		v := new(T)
		if err := yaml.Unmarshal(data, v); err != nil {
			return err
		}
		m[id(v)] = v
		return nil
	}
}

func asAny[T any](m map[string]*T) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *Snapshot) collections() []collection {
	return []collection{
		{"features", decodeInto(s.Features, func(v *model.Feature) string { return v.ID }), func(s *Snapshot) map[string]any { return asAny(s.Features) }},
		{"architecture", decodeInto(s.Modules, func(v *model.Module) string { return v.ID }), func(s *Snapshot) map[string]any { return asAny(s.Modules) }},
		{"verification", decodeInto(s.Tests, func(v *model.Test) string { return v.ID }), func(s *Snapshot) map[string]any { return asAny(s.Tests) }},
		{"decisions", decodeInto(s.Decisions, func(v *model.Decision) string { return v.ID }), func(s *Snapshot) map[string]any { return asAny(s.Decisions) }},
		{"changes", decodeInto(s.Changes, func(v *model.Change) string { return v.ID }), func(s *Snapshot) map[string]any { return asAny(s.Changes) }},
		{"requirements", decodeInto(s.Requirements, func(v *model.Requirement) string { return v.ID }), func(s *Snapshot) map[string]any { return asAny(s.Requirements) }},
	}
}

func Load(root string) (*Snapshot, error) {
	s := NewSnapshot(root)
	if err := readYAML(Path(root, "project.yaml"), &s.Project); err != nil {
		return nil, fmt.Errorf("load project.yaml: %w", err)
	}
	_ = readYAML(Path(root, "state.yaml"), &s.State)
	_ = readYAML(Path(root, "candidates.yaml"), &s.Candidates)
	for _, c := range s.collections() {
		files, _ := filepath.Glob(Path(root, c.dir, "*.md"))
		for _, f := range files {
			front, _, err := readMarkdown(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			if err := c.load(s, front); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
		}
	}
	return s, nil
}

// Save writes every object. Files of objects no longer present are removed so the
// directory mirrors the snapshot exactly (idempotent, NFR-04).
func (s *Snapshot) Save() error {
	if err := writeYAML(Path(s.Root, "project.yaml"), s.Project); err != nil {
		return err
	}
	if err := writeYAML(Path(s.Root, "candidates.yaml"), s.Candidates); err != nil {
		return err
	}
	for _, c := range s.collections() {
		dir := Path(s.Root, c.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		items := c.items(s)
		keep := map[string]bool{}
		for id, v := range items {
			p := filepath.Join(dir, id+".md")
			keep[p] = true
			if err := writeMarkdown(p, v, Summary(v)); err != nil {
				return err
			}
		}
		existing, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		for _, f := range existing {
			if !keep[f] {
				os.Remove(f)
			}
		}
	}
	return nil
}

func (s *Snapshot) SaveState() error {
	return writeYAML(Path(s.Root, "state.yaml"), s.State)
}

// Hash is the md-original revision used to detect a stale HTML (FR-29). It covers the
// structured frontmatter only, so user notes do not make the HTML stale.
func Hash(root string) string {
	h := sha256.New()
	var files []string
	for _, d := range []string{"features", "architecture", "verification", "decisions", "changes", "requirements"} {
		m, _ := filepath.Glob(Path(root, d, "*.md"))
		files = append(files, m...)
	}
	sort.Strings(files)
	for _, f := range files {
		front, _, err := readMarkdown(f)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(Path(root), f)
		h.Write([]byte(filepath.ToSlash(rel) + "\n"))
		h.Write(front)
	}
	b, _ := os.ReadFile(Path(root, "candidates.yaml"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (s *Snapshot) AllIDs() []string {
	var ids []string
	for _, c := range s.collections() {
		for id := range c.items(s) {
			ids = append(ids, id)
		}
	}
	for _, c := range s.Candidates {
		ids = append(ids, c.ID)
	}
	for _, m := range s.Modules {
		for _, c := range m.ExternalContracts {
			ids = append(ids, c.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func (s *Snapshot) NextID(k model.Kind) string { return model.NextID(k, s.AllIDs()) }

// Contract looks a contract up by ID and returns its owning module.
func (s *Snapshot) Contract(id string) (*model.Contract, *model.Module) {
	for _, m := range s.Modules {
		for i := range m.ExternalContracts {
			if m.ExternalContracts[i].ID == id {
				return &m.ExternalContracts[i], m
			}
		}
	}
	return nil, nil
}

func (s *Snapshot) ModuleByPath(p string) *model.Module {
	for _, m := range s.Modules {
		if m.Path == p && m.Status != model.ItemRemoved {
			return m
		}
	}
	return nil
}

func (s *Snapshot) ActiveChange() *model.Change {
	if s.State.ActiveTurn == "" {
		return nil
	}
	return s.Changes[s.State.ActiveTurn]
}

func readYAML(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

func writeYAML(p string, v any) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return writeIfChanged(p, b)
}

// writeIfChanged keeps mtimes and git diffs quiet when content is identical.
func writeIfChanged(p string, b []byte) error {
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, b) {
		return nil
	}
	return os.WriteFile(p, b, 0o644)
}

func readMarkdown(p string) (front []byte, user string, err error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, "", err
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, "", fmt.Errorf("missing frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return nil, "", fmt.Errorf("unterminated frontmatter")
	}
	front = []byte(text[4 : 4+end+1])
	rest := text[4+end+5:]
	if i := strings.Index(rest, autoEnd); i >= 0 {
		rest = rest[i+len(autoEnd):]
	}
	return front, strings.TrimLeft(rest, "\n"), nil
}

func writeMarkdown(p string, v any, summary string) error {
	front, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	user := "## Notes\n\n(사용자 메모 영역. 하네스는 이 아래를 덮어쓰지 않는다.)\n"
	if _, u, err := readMarkdown(p); err == nil && strings.TrimSpace(u) != "" {
		user = u
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(front)
	buf.WriteString("---\n")
	buf.WriteString(autoStart + "\n")
	buf.WriteString(summary)
	buf.WriteString(autoEnd + "\n\n")
	buf.WriteString(user)
	return writeIfChanged(p, buf.Bytes())
}

func WriteText(root string, rel string, text string) error {
	p := Path(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return writeIfChanged(p, []byte(text))
}

func WriteYAMLFile(root, rel string, v any) error { return writeYAML(Path(root, rel), v) }
func ReadYAMLFile(root, rel string, v any) error  { return readYAML(Path(root, rel), v) }

// LegacyDir is the v1.3 directory name. MigrateLegacy moves it to Dir once (PRD v1.4 CR-1)
// and returns a notice to show the user that one time; afterwards it is a no-op.
const LegacyDir = ".myharness"

func MigrateLegacy(root string) (string, error) {
	legacy := filepath.Join(root, LegacyDir)
	if _, err := os.Stat(legacy); err != nil {
		return "", nil
	}
	if _, err := os.Stat(filepath.Join(root, Dir)); err == nil {
		return "", fmt.Errorf("both %s and %s exist in %s; keep one", LegacyDir, Dir, root)
	}
	if err := os.Rename(legacy, filepath.Join(root, Dir)); err != nil {
		return "", fmt.Errorf("move %s to %s: %w", LegacyDir, Dir, err)
	}
	return fmt.Sprintf("notice: moved %s/ to %s/ (cccv v1.4 rename). Generated files keep working; commit the move.", LegacyDir, Dir), nil
}
