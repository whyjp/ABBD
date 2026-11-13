package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// wordsShown bounds the sentences a diff lists of one text each way, so
// a rewritten text does not bury the rest of the change.
const wordsShown = 4

// written is what readers and people wrote, by what it is about: each
// module's text, each slot note, and each flow step's words, keyed as diff
// names them, with the unit each one is about.
type written struct {
	text map[string]string
	unit map[string]model.ID
}

func (w written) put(key string, id model.ID, text string) {
	w.text[key], w.unit[key] = text, id
}

// writtenIn reads what st holds in words.
func writtenIn(st store.Store, units []model.Unit) (written, error) {
	w := written{text: map[string]string{}, unit: map[string]model.ID{}}
	name := names(units)
	texts, err := st.LoadUnderstandings()
	if err != nil {
		return w, err
	}
	for id, t := range texts {
		w.put(fmt.Sprintf("text %s %s", id, name[id]), id, t.Text)
	}
	notes, _, err := st.LoadNotes()
	if err != nil {
		return w, err
	}
	for id, ns := range notes {
		for _, n := range ns {
			w.put("note "+n.Ref.String(), id, n.Text)
		}
	}
	fs, err := st.LoadFigures()
	if err != nil {
		return w, err
	}
	for _, f := range fs.Flows {
		for i, p := range f.Parts {
			s := p.Claim
			w.put(fmt.Sprintf("flow %s step %d (%s)", f.Key, i+1, s.Unit), s.Unit, strings.Join(slices.DeleteFunc([]string{s.Does, s.Fails, s.Note}, func(x string) bool { return x == "" }), ". "))
		}
	}
	return w, nil
}

// storeAt is the store as it was at base, a commit or the kept previous
// copy, and what removes it when read.
func (e env) storeAt(base string) (store.Store, func(), error) {
	if base == prev {
		return store.Open(e.st.PreviousDir()), func() {}, nil
	}
	at := filepath.Join(e.st.CompareDir(), "words")
	st, err := e.st.CheckoutAt(base, at)
	return st, func() { os.RemoveAll(at) }, err
}

// wordsDiff says, sentence by sentence, what readers and people wrote
// differently since base: a text, note or flow step that came or went, and
// in one that stayed, each sentence it lost (-) and gained (+). in, when
// set, keeps only what is about the units it holds.
func wordsDiff(was, now written, in func(model.ID) bool) []string {
	keys := map[string]bool{}
	for k := range was.text {
		keys[k] = true
	}
	for k := range now.text {
		keys[k] = true
	}
	var out []string
	for _, k := range slices.Sorted(func(yield func(string) bool) {
		for k := range keys {
			if !yield(k) {
				return
			}
		}
	}) {
		id := now.unit[k]
		if id == "" {
			id = was.unit[k]
		}
		if in != nil && !in(id) {
			continue
		}
		before, had := was.text[k]
		after, has := now.text[k]
		switch {
		case !had:
			out = append(out, fmt.Sprintf("+ %s (%d sentences)", k, len(sentences(after))))
		case !has:
			out = append(out, fmt.Sprintf("- %s (%d sentences)", k, len(sentences(before))))
		default:
			lost, gained := changedSentences(sentences(before), sentences(after))
			if len(lost)+len(gained) == 0 {
				continue
			}
			out = append(out, fmt.Sprintf("~ %s", k))
			out = append(out, listed("-", lost)...)
			out = append(out, listed("+", gained)...)
		}
	}
	return out
}

func listed(sign string, ss []string) []string {
	var out []string
	for i, s := range ss {
		if i == wordsShown {
			out = append(out, fmt.Sprintf("    %s and %d more", sign, len(ss)-wordsShown))
			break
		}
		out = append(out, fmt.Sprintf("    %s %q", sign, s))
	}
	return out
}

// changedSentences are the sentences only before holds and only after
// holds, each in its order.
func changedSentences(before, after []string) (lost, gained []string) {
	for _, s := range before {
		if !slices.Contains(after, s) {
			lost = append(lost, s)
		}
	}
	for _, s := range after {
		if !slices.Contains(before, s) {
			gained = append(gained, s)
		}
	}
	return lost, gained
}

// sentences splits prose into its sentences, past headings and list marks,
// so a heading or a bullet that only moved is no change.
func sentences(text string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		t = strings.TrimSpace(strings.TrimLeft(t, "-*"))
		for t != "" {
			i := sentenceEnd(t)
			if s := strings.TrimSpace(t[:i]); s != "" {
				out = append(out, s)
			}
			t = strings.TrimSpace(t[i:])
		}
	}
	return out
}

// sentenceEnd is where the first sentence of t ends: after a period,
// question or exclamation mark that a space follows, past abbreviations
// and code spans, or the end of t.
func sentenceEnd(t string) int {
	code := false
	for i := 0; i < len(t); i++ {
		switch c := t[i]; {
		case c == '`':
			code = !code
		case !code && (c == '.' || c == '?' || c == '!') && i+1 < len(t) && t[i+1] == ' ':
			if !slices.ContainsFunc(abbreviations, func(a string) bool { return strings.HasSuffix(strings.ToLower(t[:i+1]), a) }) {
				return i + 1
			}
		}
	}
	return len(t)
}
