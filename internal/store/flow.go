package store

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/model"
)

// Flow is one ordered walk through the units and the question it answers.
// Each step keeps its own Code, one hash per quote of the lines around it,
// so an edit near one step's quotes stales that step alone, and its own
// writer, so a reader who replaces a stale step keeps a human's word on the
// others. The caption is the file's body. InputsHash covers .abbd/flows/, so
// storing a flow stales the blueprint.
type Flow struct {
	Question string     `yaml:"question"`
	Format   int        `yaml:"format"`
	Steps    []FlowStep `yaml:"steps"`
	Caption  string     `yaml:"-"`
	// Key is the file's name and File where it sits, from the project root.
	// Known is false when the name is no flow key; status reports it.
	Key   model.FlowKey `yaml:"-"`
	File  string        `yaml:"-"`
	Known bool          `yaml:"-"`
}

// FlowStep is a step, who wrote it and the code it was written against: a
// human's step carries the Source they said it in. Changed lists the cited
// places whose code moved since; LoadFlows fills it.
type FlowStep struct {
	model.Step `yaml:",inline"`
	Writer     model.Writer      `yaml:"writer"`
	Source     string            `yaml:"source,omitempty"`
	Code       map[string]string `yaml:"code"`
	Changed    []string          `yaml:"-"`
}

// Origin is the weakest origin of the flow's steps: USER_STATED only when a
// human wrote every step.
func (f Flow) Origin() model.Origin {
	for _, s := range f.Steps {
		if s.Writer != model.Human {
			return model.Inferred
		}
	}
	return model.UserStated
}

// Stale reports whether the code the step cites changed since it was written.
func (s FlowStep) Stale() bool { return len(s.Changed) > 0 }

// FlowFormat is the version of a flow file.
const FlowFormat = 1

// FlowFile is where a flow belongs, from the project root.
func FlowFile(k model.FlowKey) string { return Dir + "/flows/" + string(k) + ".md" }

// ParseFlowDraft reads the file a reader or a human writes: the stored
// format, of which it keeps the question, the steps and the caption. abbd
// sets the rest when it stores the flow.
func ParseFlowDraft(b []byte) (Flow, error) {
	front, body, err := splitFrontmatter(b)
	if err != nil {
		return Flow{}, err
	}
	var f Flow
	if err := yaml.Unmarshal(front, &f); err != nil {
		return Flow{}, fmt.Errorf("the draft's frontmatter: %w", err)
	}
	out := Flow{Question: strings.TrimSpace(f.Question), Caption: CleanText(string(body))}
	for _, s := range f.Steps {
		out.Steps = append(out.Steps, FlowStep{Step: s.Step})
	}
	return out, nil
}

// StampFlow sets each step's Code to the lines around the quotes it cites,
// as the code is now.
func (s Store) StampFlow(f Flow) Flow {
	f.Steps = slices.Clone(f.Steps)
	for i := range f.Steps {
		_, f.Steps[i].Code = s.NoteStamp(f.Steps[i].Evidence)
		f.Steps[i].Changed = nil
	}
	return f
}

// SaveFlow writes a stamped flow. The same flow on the same code writes the
// same bytes.
func (s Store) SaveFlow(f Flow) error {
	for i, st := range f.Steps {
		if len(st.Code) == 0 {
			return fmt.Errorf("flow %s step %d cites no code to check it against", f.Key, i+1)
		}
		ev := make([]model.Evidence, len(st.Evidence))
		for j, e := range st.Evidence {
			ev[j] = model.Evidence{Path: e.Path, Lines: e.Lines, Symbol: e.Symbol, Quote: e.Quote}
		}
		f.Steps[i].Evidence = ev
	}
	f.Format = FlowFormat
	b, err := frontmatter(f, CleanText(f.Caption)+"\n")
	if err != nil {
		return err
	}
	return writeFile(s.path("flows", string(f.Key)+".md"), b)
}

// Stray is a markdown file under .abbd/flows that is no flow, from the
// project root, and why abbd reads past it.
type Stray struct{ File, Why string }

// LoadFlows reads every flow in key order, each step checked against the
// code it cites now. A markdown file there without frontmatter, or whose
// frontmatter abbd cannot read, is no flow: LoadFlows returns it among the
// strays, so one bad file stops no reader of the others.
func (s Store) LoadFlows() ([]Flow, []Stray, error) {
	files, err := filepath.Glob(s.path("flows", "*.md"))
	if err != nil {
		return nil, nil, err
	}
	slices.Sort(files)
	var out []Flow
	var strays []Stray
	for _, f := range files {
		file := Dir + "/flows/" + filepath.Base(f)
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		front, body, err := splitFrontmatter(b)
		if errors.Is(err, errNoFrontmatter) {
			strays = append(strays, Stray{file, "has no frontmatter"})
			continue
		} else if err != nil {
			strays = append(strays, Stray{file, "has frontmatter abbd cannot read (" + err.Error() + ")"})
			continue
		}
		var fl Flow
		if err := yaml.Unmarshal(front, &fl); err != nil {
			strays = append(strays, Stray{file, "has frontmatter abbd cannot read (" + err.Error() + ")"})
			continue
		}
		k, err := model.ParseFlowKey(strings.TrimSuffix(filepath.Base(f), ".md"))
		fl.Key, fl.File, fl.Known, fl.Caption = k, file, err == nil, CleanText(string(body))
		if !fl.Known {
			fl.Key = model.FlowKey(strings.TrimSuffix(filepath.Base(f), ".md"))
		}
		for i, st := range fl.Steps {
			if st.Writer == "" {
				fl.Steps[i].Writer = model.Reader
			}
			fl.Steps[i].Changed = s.changed(st.Evidence, st.Code)
			if len(st.Evidence) == 0 || len(st.Code) == 0 {
				fl.Steps[i].Changed = []string{"the step cites no code abbd stamped, so nothing shows it is still true"}
			}
		}
		out = append(out, fl)
	}
	return out, strays, nil
}

// LoadFlow reads one flow, false when there is none.
func (s Store) LoadFlow(k model.FlowKey) (Flow, bool, error) {
	flows, _, err := s.LoadFlows()
	if err != nil {
		return Flow{}, false, err
	}
	i := slices.IndexFunc(flows, func(f Flow) bool { return f.Known && f.Key == k })
	if i < 0 {
		return Flow{}, false, nil
	}
	return flows[i], true, nil
}

// changed lists each cited place whose code differs from what was written
// against it: its lines changed, or its quote is gone.
func (s Store) changed(cited []model.Evidence, code map[string]string) []string {
	_, now := s.NoteStamp(cited)
	if maps.Equal(now, code) {
		return nil
	}
	var out []string
	for k, h := range now {
		if code[k] != h {
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

// splitFrontmatter cuts a file into its frontmatter and its body.
func splitFrontmatter(b []byte) (front, body []byte, err error) {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return nil, nil, errNoFrontmatter
	}
	front, body, ok = bytes.Cut(append(rest, '\n'), []byte("\n---\n"))
	if !ok {
		return nil, nil, fmt.Errorf("unterminated frontmatter")
	}
	return front, body, nil
}
