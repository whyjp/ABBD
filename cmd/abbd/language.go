package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/narrative"
	"github.com/whyjp/abbd/internal/screen"
	"github.com/whyjp/abbd/internal/store"
)

// languageLine tells the writer of stored prose which language a person
// reads it in.
func languageLine(t model.Language) string {
	return fmt.Sprintf("Write all prose in %s; keep code identifiers, paths and quotes verbatim.", t)
}

// lang is the language the project's prose is written in.
func (e env) lang() (model.Language, error) {
	p, err := e.st.LoadProject()
	return p.Lang(), err
}

// checkLanguage tells the writer when a text they stored does not read in
// the project's language. Like readText it never refuses.
func (e env) checkLanguage(what, text string) {
	t, err := e.lang()
	if err != nil {
		return
	}
	if share, n := narrative.Share(text, t.Script()); n > 0 && share < narrative.Threshold {
		fmt.Fprintf(e.stdout, "language: %s does not read as %s (%.0f%% of its prose words); %s\n", what, t, share*100, languageLine(t))
	}
}

// foreignProse is the key of each stored text an agent wrote that does not
// read in t: once per unit field, phrase, note and figure part, so a new
// foreign text never hides under the key of one the debt carries. A
// person's own words and what the script read from code comments stay as
// they were said.
func foreignProse(in screen.Input, t model.Language) []string {
	var out []string
	check := func(key string, texts ...string) {
		if !narrative.ReadsIn(strings.Join(texts, "\n"), t.Script()) {
			out = append(out, key)
		}
	}
	check("project purpose", in.Project.Purpose)
	for _, u := range model.Present(in.Units) {
		if f := u.Purpose; agentWrote(f) {
			check(string(u.ID)+" purpose", f.Value)
		}
		if f := u.Responsibility; agentWrote(f) {
			check(string(u.ID)+" responsibility", f.Value)
		}
		for _, p := range u.Phrases {
			if p.Writer.Origin() == model.Inferred {
				check(fmt.Sprintf("%s phrase %s %s", u.ID, p.Kind, p.To), p.Text)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(in.Understandings)) {
		check(string(id)+" text", in.Understandings[id].Text)
	}
	for _, id := range slices.Sorted(maps.Keys(in.Notes)) {
		for _, n := range in.Notes[id] {
			check(string(id)+" note "+n.Slot, n.Text)
		}
	}
	for _, f := range in.Figures.Flows {
		figureProse(check, f, func(s model.Step) []string { return []string{s.Does, s.Note, s.Fails} })
	}
	for _, f := range in.Figures.Stacks {
		figureProse(check, f, func(l model.Layer) []string { return []string{l.Role} })
	}
	for _, f := range in.Figures.Tables {
		var cols []string
		for _, c := range f.Columns {
			cols = append(cols, c.Name)
		}
		check("table "+string(f.Key)+" columns", cols...)
		figureProse(check, f, func(r model.Row) []string { return slices.Collect(maps.Values(r.Cells)) })
	}
	return out
}

// figureProse checks a figure's question and caption, and each part an
// agent wrote by its place, with the words prose picks from it.
func figureProse[C model.Claim[C]](check func(string, ...string), f store.Figure[C], prose func(C) []string) {
	k := model.KindOf[C]()
	key := k.Noun + " " + string(f.Key)
	check(key, f.Question, f.Caption)
	for i, p := range f.Parts {
		if p.Writer.Origin() == model.Inferred {
			check(fmt.Sprintf("%s %s %d", key, k.Part, i+1), prose(p.Claim)...)
		}
	}
}

func agentWrote(f *model.Fact) bool { return f != nil && f.Writer.Origin() == model.Inferred }

// proseGate lists each foreign text the debt does not carry, and each the
// debt carries that now reads in the language and may leave it.
func proseGate(found []string, t model.Language, d *store.ProseDebt) (fails, lowerable []string) {
	var owed []string
	if d != nil && d.Language == t {
		owed = d.Owed
	}
	for _, k := range found {
		if !slices.Contains(owed, k) {
			fails = append(fails, fmt.Sprintf("prose: %s does not read as %s; write it in %s, keeping identifiers, paths and quotes verbatim", k, t, t))
		}
	}
	for _, k := range owed {
		if !slices.Contains(found, k) {
			lowerable = append(lowerable, fmt.Sprintf("prose: %s reads as %s now; drop it from the debt", k, t))
		}
	}
	return fails, lowerable
}

// proseDebt is the prose debt lint --record writes. Under the language the
// prior debt names it only shrinks: it keeps each owed text still found, so
// a foreign text written since never joins it. On the first record, or once
// project.yaml names another language, it takes every foreign text found.
func proseDebt(found []string, t model.Language, prior *store.ProseDebt) *store.ProseDebt {
	reason := fmt.Sprintf("written before project.yaml named %s; each text is written in %s when it is next read again, stale or touched", t, t)
	if prior != nil && prior.Language == t {
		found = slices.DeleteFunc(slices.Clone(found), func(k string) bool { return !slices.Contains(prior.Owed, k) })
		if prior.Reason != "" {
			reason = prior.Reason
		}
	}
	if len(found) == 0 {
		return nil
	}
	return &store.ProseDebt{Language: t, Reason: reason, Owed: found}
}

func printProse(w io.Writer, found []string, t model.Language) {
	if n := len(found); n > 0 {
		fmt.Fprintf(w, "lint: prose: %s not in %s; abbd lint --json lists them\n", plural(n, "text"), t)
	}
}
