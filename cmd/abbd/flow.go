package main

import (
	"fmt"
	"maps"
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
	for i := range f.Steps {
		f.Steps[i].Writer, f.Steps[i].Source = e.writer(), e.sourceRef()
	}
	kept, replaced, err := mayReplaceFlow(prev, had, &f, units)
	if err != nil {
		return err
	}
	f = e.st.StampFlow(f)
	if f.Retired, err = retire(prev, f); err != nil {
		return err
	}
	if err := e.st.SaveFlow(f); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "stored flow %s, %d steps, %s, each step stamped with the lines around the quotes it cites\n", k, len(f.Steps), f.Origin())
	if len(kept) > 0 {
		fmt.Fprintf(e.stdout, "kept the human's steps %s\n", kept)
	}
	if len(replaced) > 0 {
		fmt.Fprintf(e.stdout, "replaced the human's steps %s of the stored flow, since their code changed; ask again and store their word with abbd flow %s FILE --source REF\n", replaced, k)
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
			if ev.Symbol == "" {
				continue
			}
			from, to, ok := c.Declared(ev.Path, ev.Symbol)
			if !ok {
				return fmt.Errorf("%s: %s declares no %s that abbd can find; name the symbol whose code holds %q, or none", at, ev.Path, ev.Symbol, ev.Quote)
			}
			if first, last, _ := es[j].Span(); first < from || last > to {
				return fmt.Errorf("%s: %s lies outside %s, lines %d-%d of %s; name the symbol whose code holds it, or none", at, es[j], ev.Symbol, from, to, ev.Path)
			}
		}
		s.Evidence = es
		if gaps := s.Against(units).Gaps; len(gaps) > 0 {
			return fmt.Errorf("%s: %s", at, strings.Join(gaps, "; "))
		}
	}
	return nil
}

// humanStep is a human's step by its place in a flow, from 1, and the
// source the human said it in.
type humanStep struct {
	place  int
	source string
}

type humanSteps []humanStep

func (hs humanSteps) String() string {
	var places, sources []string
	for _, h := range hs {
		places = append(places, fmt.Sprint(h.place))
		if !slices.Contains(sources, h.source) {
			sources = append(sources, h.source)
		}
	}
	return fmt.Sprintf("%s (%s)", strings.Join(places, ", "), strings.Join(sources, "; "))
}

// retire carries the retired steps of the stored flow over to the stamped
// draft f, with each step of the stored flow whose code changed since it was
// written, and drops those whose code is back as they were written. It
// refuses a step of f that says what one of them said, on the same unit,
// and cites a quote whose code changed under those words: it would read
// fresh with no one reading that code again, however many stores ago it
// left the flow and whatever quotes it adds.
func retire(prev store.Flow, f store.Flow) ([]store.FlowStep, error) {
	var out []store.FlowStep
	for _, s := range append(slices.Clone(prev.Retired), prev.Steps...) {
		if s.Stale() && !slices.ContainsFunc(out, func(o store.FlowStep) bool { return said(o, s) && maps.Equal(o.Code, s.Code) }) {
			out = append(out, store.FlowStep{Step: model.Step{Unit: s.Unit, Does: s.Does, Note: s.Note, Fails: s.Fails, Evidence: s.Evidence}, Code: s.Code, Changed: s.Changed})
		}
	}
	for i, n := range f.Steps {
		for _, r := range out {
			for k, h := range n.Code {
				if was, ok := r.Code[k]; ok && was != h && said(r, n) {
					return nil, fmt.Errorf("step %d says what a STALE step said, on code that changed since it was written (%s); read it again, then store what it does now or cite the code it rests on now", i+1, strings.Join(r.Changed, ", "))
				}
			}
		}
	}
	return out, nil
}

// said reports whether two steps say the same thing of the same unit.
func said(x, y store.FlowStep) bool {
	return x.Unit == y.Unit && sameWords(x.Does, y.Does) && sameWords(x.Note, y.Note) && sameWords(x.Fails, y.Fails)
}

// mayReplaceFlow decides, step by step, whether the draft f may replace the
// stored flow. A human may replace any step. A reader may replace only a
// human's STALE steps: each current one must come back as written, in its
// order, and keeps the human's word, as do the question and caption, so CI
// never waits on the human for a step whose code moved and a human's word on
// a step that still holds is never lost.
// kept are the human's steps f keeps and replaced those the reader took
// over, each named by its place.
func mayReplaceFlow(prev store.Flow, had bool, f *store.Flow, units []model.Unit) (kept, replaced humanSteps, err error) {
	if !had {
		return nil, nil, nil
	}
	stale := func(s store.FlowStep) bool { return s.Stale() || len(s.Against(units).Gaps) > 0 }
	human := func(s store.FlowStep) bool { return s.Writer == model.Human }
	if slices.ContainsFunc(f.Steps, human) || !slices.ContainsFunc(prev.Steps, human) {
		return nil, nil, nil
	}
	if !slices.ContainsFunc(prev.Steps, stale) {
		return nil, nil, fmt.Errorf("a human stated steps of flow %s and every step is current; only a human changes it, with --source", f.Key)
	}
	if !sameWords(prev.Question, f.Question) {
		return nil, nil, fmt.Errorf("the question of flow %s is a human's word; keep it as abbd flow %s prints it, since only a human changes it, with --source", f.Key, f.Key)
	}
	if !sameWords(prev.Caption, f.Caption) {
		return nil, nil, fmt.Errorf("the caption of flow %s is a human's word; keep it as abbd flow %s prints it, since only a human changes it, with --source", f.Key, f.Key)
	}
	at := 0
	for i, s := range prev.Steps {
		switch {
		case !human(s):
			continue
		case stale(s):
			replaced = append(replaced, humanStep{i + 1, s.Source})
			continue
		}
		j := slices.IndexFunc(f.Steps[at:], func(n store.FlowStep) bool { return sameStep(s, n) })
		if j < 0 {
			return nil, nil, fmt.Errorf("step %d is a human's word (%s) and current; keep it as abbd flow %s prints it, in its order, since only a human changes it, with --source", i+1, s.Source, f.Key)
		}
		at += j
		f.Steps[at].Writer, f.Steps[at].Source = model.Human, s.Source
		kept = append(kept, humanStep{at + 1, s.Source})
		at++
	}
	return kept, replaced, nil
}

// sameStep reports whether two steps say the same thing in the same words
// on the same quotes.
func sameStep(x, y store.FlowStep) bool {
	if x.Unit != y.Unit || !slices.Equal(x.Calls, y.Calls) || !sameCitations(x.Evidence, y.Evidence) {
		return false
	}
	for _, p := range [][2]string{{x.Does, y.Does}, {x.Note, y.Note}, {x.Fails, y.Fails}} {
		if !sameWords(p[0], p[1]) {
			return false
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
