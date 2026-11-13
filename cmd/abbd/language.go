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
		fmt.Fprintf(e.stdout, "language: %s does not read as %s (%.0f%% of its prose letters); %s\n", what, t, share*100, languageLine(t))
	}
}

// foreignProse is the key of each stored text an agent wrote that does not
// read in t, once per unit field, note and figure. A person's own words and
// what the script read from code comments stay as they were said.
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
		var phrases []string
		for _, p := range u.Phrases {
			if p.Writer.Origin() == model.Inferred {
				phrases = append(phrases, p.Text)
			}
		}
		check(string(u.ID)+" phrases", phrases...)
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
		texts := []string{f.Question, f.Caption}
		for _, p := range f.Parts {
			texts = append(texts, p.Claim.Does, p.Claim.Note, p.Claim.Fails)
		}
		check("flow "+string(f.Key), texts...)
	}
	for _, f := range in.Figures.Stacks {
		texts := []string{f.Question, f.Caption}
		for _, p := range f.Parts {
			texts = append(texts, p.Claim.Role)
		}
		check("stack "+string(f.Key), texts...)
	}
	for _, f := range in.Figures.Tables {
		check("table "+string(f.Key), f.Question, f.Caption)
	}
	return out
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

// proseDebt is the prose debt lint --record writes: every foreign text
// found now, with the reason prior gave for the same language.
func proseDebt(found []string, t model.Language, prior *store.ProseDebt) *store.ProseDebt {
	if len(found) == 0 {
		return nil
	}
	reason := fmt.Sprintf("written before project.yaml named %s; each text is written in %s when it is next read again, stale or touched", t, t)
	if prior != nil && prior.Language == t && prior.Reason != "" {
		reason = prior.Reason
	}
	return &store.ProseDebt{Language: t, Reason: reason, Owed: found}
}

func printProse(w io.Writer, found []string, t model.Language) {
	if n := len(found); n > 0 {
		fmt.Fprintf(w, "lint: prose: %s not in %s; abbd lint --json lists them\n", plural(n, "text"), t)
	}
}
