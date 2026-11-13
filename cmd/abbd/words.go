package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/narrative"
	"github.com/whyjp/abbd/internal/store"
)

// wordsShown bounds the sentences the text of a diff lists of one change
// each way, so a rewritten text does not bury the rest. --json lists all.
const wordsShown = 4

// A wording is one thing written about a unit, read as its sentences.
type wording struct {
	unit model.ID
	name string
	said []string
}

// written is what readers and people wrote: each module's text and each
// slot note by key, and each flow's steps in order by the flow's key.
// Texts are keyed by unit ID, so a renamed module keeps its text.
type written struct {
	once  map[string]wording
	flows map[string][]wording
}

// A wordChange is one text, note or flow step that came, went or now says
// something else, with every sentence it lost and gained.
type wordChange struct {
	Key    string   `json:"key"`
	Kind   string   `json:"kind"`
	Unit   model.ID `json:"unit"`
	Name   string   `json:"name,omitempty"`
	Change string   `json:"change"`
	Lost   []string `json:"lost"`
	Gained []string `json:"gained"`
}

// writtenIn reads what st holds in words.
func writtenIn(st store.Store, units []model.Unit) (written, error) {
	w := written{once: map[string]wording{}, flows: map[string][]wording{}}
	name := names(units)
	texts, err := st.LoadUnderstandings()
	if err != nil {
		return w, err
	}
	for id, t := range texts {
		w.once["text "+string(id)] = wording{id, name[id], sentences(t.Text)}
	}
	notes, _, err := st.LoadNotes()
	if err != nil {
		return w, err
	}
	for id, ns := range notes {
		for _, n := range ns {
			w.once["note "+n.Ref.String()] = wording{unit: id, said: sentences(n.Text)}
		}
	}
	fs, err := st.LoadFigures()
	if err != nil {
		return w, err
	}
	for _, f := range fs.Flows {
		for _, p := range f.Parts {
			s := p.Claim
			w.flows[string(f.Key)] = append(w.flows[string(f.Key)], wording{unit: s.Unit, said: slices.Concat(sentences(s.Does), sentences(s.Fails), sentences(s.Note))})
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
	at := filepath.Join(e.st.CompareDir(), "at")
	st, err := e.st.CheckoutAt(base, at)
	return st, func() { os.RemoveAll(at) }, err
}

// wordsDiff says what readers and people wrote differently since base:
// flow steps first, aligned so a step put in or taken out is one change,
// then notes and texts by key. in, when set, keeps only what is about the
// units it holds.
func wordsDiff(was, now written, in func(model.ID) bool) []wordChange {
	var out []wordChange
	for _, k := range slices.Sorted(maps.Keys(union(was.flows, now.flows))) {
		for _, s := range aligned(was.flows[k], now.flows[k]) {
			out = append(out, changed(fmt.Sprintf("flow %s step %d", k, s.at+1), "step", s.was, s.now)...)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(union(was.once, now.once))) {
		before, had := was.once[k]
		after, has := now.once[k]
		out = append(out, changed(k, strings.Fields(k)[0], opt(before, had), opt(after, has))...)
	}
	return slices.DeleteFunc(out, func(c wordChange) bool { return in != nil && !in(c.Unit) })
}

func union[V any](a, b map[string]V) map[string]bool {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	return keys
}

func opt(w wording, ok bool) *wording {
	if !ok {
		return nil
	}
	return &w
}

// changed is the change from was to now, either absent, or none when both
// say the same sentences.
func changed(key, kind string, was, now *wording) []wordChange {
	c := wordChange{Key: key, Kind: kind, Lost: []string{}, Gained: []string{}}
	switch {
	case was == nil:
		c.Unit, c.Name, c.Change, c.Gained = now.unit, now.name, "added", now.said
	case now == nil:
		c.Unit, c.Name, c.Change, c.Lost = was.unit, was.name, "removed", was.said
	default:
		c.Unit, c.Name, c.Change = now.unit, now.name, "changed"
		c.Lost, c.Gained = only(was.said, now.said), only(now.said, was.said)
		if len(c.Lost)+len(c.Gained) == 0 {
			return nil
		}
	}
	return []wordChange{c}
}

// only is the sentences of a that b does not hold, in order.
func only(a, b []string) []string {
	return slices.DeleteFunc(slices.Clone(a), func(s string) bool { return slices.Contains(b, s) })
}

// A step is one place in a flow's aligned steps: the step then, now, or
// both, and its number, now's when it has one.
type step struct {
	was, now *wording
	at       int
}

// aligned pairs a flow's steps then and now, most alike first: a step
// saying the same weighs two, one on the same unit one. A step put in or
// taken out stays unpaired, and the steps after it keep their pairs.
func aligned(was, now []wording) []step {
	weight := func(i, j int) int {
		switch {
		case was[i].unit != now[j].unit:
			return 0
		case slices.Equal(was[i].said, now[j].said):
			return 2
		}
		return 1
	}
	best := make([][]int, len(was)+1)
	for i := range best {
		best[i] = make([]int, len(now)+1)
	}
	for i := len(was) - 1; i >= 0; i-- {
		for j := len(now) - 1; j >= 0; j-- {
			best[i][j] = max(best[i+1][j], best[i][j+1])
			if w := weight(i, j); w > 0 {
				best[i][j] = max(best[i][j], w+best[i+1][j+1])
			}
		}
	}
	var out []step
	i, j := 0, 0
	for i < len(was) || j < len(now) {
		switch {
		case i < len(was) && j < len(now) && weight(i, j) > 0 && best[i][j] == weight(i, j)+best[i+1][j+1]:
			out = append(out, step{&was[i], &now[j], j})
			i, j = i+1, j+1
		case i < len(was) && (j == len(now) || best[i][j] == best[i+1][j]):
			out = append(out, step{&was[i], nil, i})
			i++
		default:
			out = append(out, step{nil, &now[j], j})
			j++
		}
	}
	return out
}

// lines is how the text of a diff and the compare page show words: a
// line per change, then up to wordsShown sentences each way.
func lines(cs []wordChange) []string {
	var out []string
	for _, c := range cs {
		head := c.Key
		switch {
		case c.Name != "":
			head += " " + c.Name
		case c.Kind == "step":
			head += " (" + string(c.Unit) + ")"
		}
		switch c.Change {
		case "added":
			out = append(out, fmt.Sprintf("+ %s (%d sentences)", head, len(c.Gained)))
		case "removed":
			out = append(out, fmt.Sprintf("- %s (%d sentences)", head, len(c.Lost)))
		default:
			out = append(out, "~ "+head)
			out = append(out, listed("-", c.Lost)...)
			out = append(out, listed("+", c.Gained)...)
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

// listMark is a bullet or a number that opens a list item.
var listMark = regexp.MustCompile(`^(?:[-*+]|\d+[.)])\s+`)

// sentences splits a text into what it says: each prose line's sentences
// past its list mark, and each code line whole. Headings, rules and table
// rulings say nothing, so one that only moved is no change.
func sentences(text string) []string {
	var out []string
	for _, l := range narrative.Lines(text) {
		switch l.Kind {
		case narrative.Code:
			if t := strings.TrimSpace(l.Text); t != "" {
				out = append(out, t)
			}
		case narrative.Prose:
			if strings.Trim(l.Text, "-*_=:| ") == "" {
				continue
			}
			t := listMark.ReplaceAllString(l.Text, "")
			for t != "" {
				i := sentenceEnd(t)
				if s := strings.TrimSpace(t[:i]); s != "" {
					out = append(out, s)
				}
				t = strings.TrimSpace(t[i:])
			}
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
