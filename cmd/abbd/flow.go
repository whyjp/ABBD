package main

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// flow stores a flow from a draft, or prints the stored one as a draft when
// no FILE is given. Every check of a draft happens here, so the store and
// the screen trust what they read.
func (e env) flow(key string, args []string) error {
	k, err := model.ParseFlowKey(key)
	if err != nil {
		return err
	}
	prev, had, err := e.st.LoadFlow(k)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		if !had {
			return fmt.Errorf("there is no flow %s; store one with abbd flow %s FILE", k, k)
		}
		return e.printFlow(prev)
	}
	text, err := e.readInput(args[0], "pass a flow draft: frontmatter with a question and steps, and a caption")
	if err != nil {
		return err
	}
	f, err := store.ParseFlowDraft([]byte(text))
	if err != nil {
		return err
	}
	f.Key = k
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	if err := e.checkDraft(&f, units); err != nil {
		return err
	}
	f.Writer, f.Source = e.writer(), e.sourceRef()
	f.Origin = f.Writer.Origin()
	dropped, err := mayReplaceFlow(prev, had, f, units)
	if err != nil {
		return err
	}
	f = e.st.StampFlow(f)
	if err := e.st.SaveFlow(f); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "stored flow %s, %d steps, %s, each step stamped with the lines around the quotes it cites\n", k, len(f.Steps), f.Origin)
	if dropped {
		fmt.Fprintf(e.stdout, "dropped the flow the human stated (%s), since its code changed; ask again and store it with abbd flow %s FILE --source REF\n", prev.Source, k)
	}
	for i, s := range f.Steps {
		if n := len([]rune(s.Does)); n > 80 {
			fmt.Fprintf(e.stdout, "long: flow %s step %d does holds %d characters; a step reads best under 80\n", k, i+1, n)
		}
	}
	return nil
}

// checkDraft refuses a draft that is no ordered walk through the state: no
// question, too few or too many steps, a step that is not well formed, a
// quote the code does not hold once, or a unit or call the state lacks.
func (e env) checkDraft(f *store.Flow, units []model.Unit) error {
	switch {
	case f.Question == "" || strings.ContainsAny(f.Question, "\r\n"):
		return fmt.Errorf("a flow answers one question, on one line, in its question field")
	case len(f.Steps) < 2:
		return fmt.Errorf("a flow has two steps or more; %d is a unit's note, not an order", len(f.Steps))
	case len(f.Steps) > model.FlowSteps:
		return fmt.Errorf("a flow has at most %d steps, and this one has %d; split the question in two", model.FlowSteps, len(f.Steps))
	}
	c := evidence.New(e.root)
	for i := range f.Steps {
		s := &f.Steps[i]
		at := fmt.Sprintf("step %d", i+1)
		if err := s.Validate(); err != nil {
			return fmt.Errorf("%s: %w", at, err)
		}
		es, err := extract.NormalizeEvidence(s.Evidence)
		if err != nil {
			return fmt.Errorf("%s: %w", at, err)
		}
		for j, ev := range es {
			r := c.Check(ev)
			switch {
			case !r.Live:
				return fmt.Errorf("%s: the code does not hold %s; quote it verbatim from the working tree", at, ev)
			case c.Occurrences(ev) > 1:
				return fmt.Errorf("%s: %s holds %q more than once; quote enough of the line that it appears once", at, ev.Path, ev.Quote)
			}
			es[j].Lines = r.At
		}
		s.Evidence = es
		if gaps := s.Against(units).Gaps; len(gaps) > 0 {
			return fmt.Errorf("%s: %s", at, strings.Join(gaps, "; "))
		}
	}
	return nil
}

// mayReplaceFlow decides whether f may replace the stored flow. A reader
// replaces a flow a human stated only once a step of it is STALE, and then
// drops the human's word, so CI does not wait on the human. The same steps
// on the same quotes after their code moved are refused: they would mark a
// stale flow fresh without anyone reading the code again.
func mayReplaceFlow(prev store.Flow, had bool, f store.Flow, units []model.Unit) (dropped bool, err error) {
	if !had {
		return false, nil
	}
	stale := slices.ContainsFunc(prev.Steps, func(s store.FlowStep) bool {
		return s.Stale() || len(s.Against(units).Gaps) > 0
	})
	if stale && sameFlow(prev, f) {
		return false, fmt.Errorf("the code flow %s cites changed since it was written; read it again, then store the new steps or cite the code they rest on now", f.Key)
	}
	if prev.Writer == model.Human && f.Writer != model.Human {
		if !stale {
			return false, fmt.Errorf("a human stated flow %s (%s) and it is current; only a human replaces it, with --source", f.Key, prev.Source)
		}
		return true, nil
	}
	return false, nil
}

// sameFlow reports whether two flows say the same steps in the same words
// on the same quotes.
func sameFlow(a, b store.Flow) bool {
	if !sameWords(a.Question, b.Question) || len(a.Steps) != len(b.Steps) {
		return false
	}
	for i := range a.Steps {
		x, y := a.Steps[i], b.Steps[i]
		if x.Unit != y.Unit || !slices.Equal(x.Calls, y.Calls) || !sameCitations(x.Evidence, y.Evidence) {
			return false
		}
		for _, p := range [][2]string{{x.Does, y.Does}, {x.Note, y.Note}, {x.Fails, y.Fails}} {
			if !sameWords(p[0], p[1]) {
				return false
			}
		}
	}
	return true
}

// printFlow writes the stored flow as a draft to edit: its question, steps
// and caption, without what abbd sets.
func (e env) printFlow(f store.Flow) error {
	draft := struct {
		Question string       `yaml:"question"`
		Steps    []model.Step `yaml:"steps"`
	}{Question: f.Question}
	for i, s := range f.Steps {
		if s.Stale() {
			fmt.Fprintf(e.stderr, "abbd: step %d is STALE: %s\n", i+1, strings.Join(s.Changed, ", "))
		}
		draft.Steps = append(draft.Steps, s.Step)
	}
	b, err := yaml.Marshal(draft)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "---\n%s---\n\n%s\n", b, f.Caption)
	return nil
}

// statusFlows reports the flows: how many steps, which are STALE and why,
// which files are no flows. It returns how many steps are STALE.
func (e env) statusFlows(units []model.Unit, flows []store.Flow, strays []store.Stray) int {
	var steps, stale int
	var lines []string
	for _, f := range flows {
		if !f.Known {
			lines = append(lines, fmt.Sprintf("orphaned: %s is named no flow key, so nothing reads it and the check skips it; rename or delete it", f.File))
			continue
		}
		for i, s := range f.Steps {
			steps++
			why := append(slices.Clone(s.Changed), s.Against(units).Gaps...)
			if len(why) == 0 {
				continue
			}
			stale++
			lines = append(lines, fmt.Sprintf("stale: flow %s step %d (%s): %s; read that code again and store the flow with abbd flow %s FILE, or delete %s to drop it", f.Key, i+1, s.Unit, strings.Join(why, ", "), f.Key, f.File))
		}
	}
	fmt.Fprintf(e.stdout, "flows: %d flow(s), %d step(s), %d STALE\n", len(flows), steps, stale)
	for _, f := range strays {
		fmt.Fprintf(e.stdout, "warning: %s %s, so it is no flow and abbd reads past it; fix its frontmatter, or move it out of %s/flows, to clear this\n", f.File, f.Why, store.Dir)
	}
	for _, l := range lines {
		fmt.Fprintln(e.stdout, l)
	}
	return stale
}
