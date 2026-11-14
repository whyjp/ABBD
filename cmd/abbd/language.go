package main

import (
	"crypto/sha256"
	"encoding/hex"
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

// foreign is a stored text that does not read in the project's language:
// the key lint reports it under and a digest of its words, so the debt
// owes those words and not whatever the slot holds later.
type foreign struct{ Key, Digest string }

func proseKeys(fs []foreign) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Key
	}
	return out
}

// foreignProse finds each stored text an agent wrote that does not read in
// t: once per unit field, phrase, note and figure part, so a new foreign
// text never hides under the key of one the debt carries. A person's own
// words and what the script read from code comments stay as they were said.
func foreignProse(in screen.Input, t model.Language) []foreign {
	var out []foreign
	check := func(key string, texts ...string) {
		text := strings.Join(texts, "\n")
		if !narrative.ReadsIn(text, t.Script()) {
			sum := sha256.Sum256([]byte(text))
			out = append(out, foreign{key, hex.EncodeToString(sum[:4])})
		}
	}
	word := func(key, text string) {
		if !narrative.ReadsAsWord(text, t.Script()) {
			sum := sha256.Sum256([]byte(text))
			out = append(out, foreign{key, hex.EncodeToString(sum[:4])})
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
	if o := in.Overview; o.Known {
		check("overview caption", o.Caption)
		for _, z := range o.Zones {
			word("overview zone "+z.Claim.Name, z.Claim.Name)
		}
		for _, l := range o.Lines {
			word("overview line "+l.Claim.Label(), l.Claim.Word)
		}
		if f, ok := in.Figures.Primary(); ok {
			for _, p := range f.Parts {
				word("overview stage "+p.Claim.Name, p.Claim.Name)
			}
		}
	}
	for _, f := range in.Figures.Tables {
		var cols []string
		for _, c := range f.Columns {
			cols = append(cols, c.Name)
		}
		check("table "+string(f.Key)+" columns", cols...)
		figureProse(check, f, func(r model.Row) []string {
			return slices.Sorted(maps.Values(r.Cells))
		})
	}
	return out
}

// figureProse checks a figure's question and caption, and each part an
// agent wrote by its label, the unit or layer name it stands for, so adding
// or removing a part leaves the keys of the others alone. A label that
// repeats in the figure takes its count: "step MOD-0002 #2".
func figureProse[C model.Claim[C]](check func(string, ...string), f store.Figure[C], prose func(C) []string) {
	k := model.KindOf[C]()
	key := k.Noun + " " + string(f.Key)
	check(key, f.Question, f.Caption)
	seen := map[string]int{}
	for _, p := range f.Parts {
		l := p.Claim.Label()
		seen[l]++
		if n := seen[l]; n > 1 {
			l = fmt.Sprintf("%s #%d", l, n)
		}
		if p.Writer.Origin() == model.Inferred {
			check(fmt.Sprintf("%s %s %s", key, k.Part, l), prose(p.Claim)...)
		}
	}
}

func agentWrote(f *model.Fact) bool { return f != nil && f.Writer.Origin() == model.Inferred }

// owes reports whether d carries f under t: the same key with the same
// words. A text written again under an owed key is not owed.
func owes(d *store.ProseDebt, t model.Language, f foreign) bool {
	return d != nil && d.Language == t && d.Owed[f.Key] == f.Digest
}

// proseGate lists each foreign text the debt does not carry, and each the
// debt carries whose slot no longer holds a foreign text and may leave it.
func proseGate(found []foreign, t model.Language, d *store.ProseDebt) (fails, lowerable []string) {
	var owed map[string]string
	if d != nil && d.Language == t {
		owed = d.Owed
	}
	for _, f := range found {
		switch digest, ok := owed[f.Key]; {
		case !ok:
			fails = append(fails, fmt.Sprintf("prose: %s does not read as %s; write it in %s, keeping identifiers, paths and quotes verbatim", f.Key, t, t))
		case digest != f.Digest:
			fails = append(fails, fmt.Sprintf("prose: %s was written again since the debt took it and does not read as %s; write it in %s, keeping identifiers, paths and quotes verbatim", f.Key, t, t))
		}
	}
	keys := proseKeys(found)
	for _, k := range slices.Sorted(maps.Keys(owed)) {
		if !slices.Contains(keys, k) {
			lowerable = append(lowerable, fmt.Sprintf("prose: %s reads as %s now; drop it from the debt", k, t))
		}
	}
	return fails, lowerable
}

// proseDebt is the prose debt lint --record writes. Under the language the
// prior debt names it only shrinks: it keeps each owed text still found
// with the same words, so a foreign text written since never joins it. On
// the first record, or once project.yaml names another language, it takes
// every foreign text found.
func proseDebt(found []foreign, t model.Language, prior *store.ProseDebt) *store.ProseDebt {
	reason := fmt.Sprintf("written before project.yaml named %s; each text is written in %s when it is next read again, stale or touched", t, t)
	same := prior != nil && prior.Language == t
	if same && prior.Reason != "" {
		reason = prior.Reason
	}
	owed := map[string]string{}
	for _, f := range found {
		if !same || owes(prior, t, f) {
			owed[f.Key] = f.Digest
		}
	}
	if len(owed) == 0 {
		return nil
	}
	return &store.ProseDebt{Language: t, Reason: reason, Owed: owed}
}

func printProse(w io.Writer, found []string, t model.Language) {
	if n := len(found); n > 0 {
		fmt.Fprintf(w, "lint: prose: %s not in %s; abbd lint --json lists them\n", plural(n, "text"), t)
	}
}
