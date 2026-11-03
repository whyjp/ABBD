package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/extract"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// flow stores a flow from a draft, or prints the stored one, as figure does
// for every kind.
func (e env) flow(key string, args []string) error { return figure[model.Step](e, key, args) }

// figure stores a figure of kind C from a draft, or prints the stored one as
// a draft when no FILE is given. Every check of a draft happens here, so the
// store and the screen trust what they read.
func figure[C model.Claim[C]](e env, key string, args []string) error {
	kind := model.KindOf[C]()
	k, err := model.ParseFigureKey[C](key)
	if err != nil {
		return err
	}
	prev, had, err := store.LoadFigure[C](e.st, k)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		if !had {
			return fmt.Errorf("there is no %s %s; store one with abbd %s %s FILE", kind.Noun, k, kind.Noun, k)
		}
		return printFigure(e, prev)
	}
	text, err := e.readInput(args[0], fmt.Sprintf("pass a %s draft: frontmatter with a question and %s, and a caption", kind.Noun, kind.Parts))
	if err != nil {
		return err
	}
	f, err := store.ParseFigureDraft[C]([]byte(text))
	if err != nil {
		return err
	}
	f.Key = k
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	if err := checkFigure(e, &f, units); err != nil {
		return err
	}
	for i := range f.Parts {
		f.Parts[i].Writer, f.Parts[i].Source = e.writer(), e.sourceRef()
	}
	kept, replaced, err := mayReplace(prev, had, &f, units)
	if err != nil {
		return err
	}
	if f.Retired, err = retire(e, prev, f, units); err != nil {
		return err
	}
	if err := store.SaveFigure(e.st, f); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "stored %s %s, %d %s, %s, each %s stamped with the lines around the quotes it cites\n", kind.Noun, k, len(f.Parts), kind.Parts, f.Origin(), kind.Part)
	if len(kept) > 0 {
		fmt.Fprintf(e.stdout, "kept the human's %s %s\n", kind.Parts, kept)
	}
	if len(replaced) > 0 {
		fmt.Fprintf(e.stdout, "replaced the human's %s %s of the stored %s, since their code changed; ask again and store their word with abbd %s %s FILE --source REF\n", kind.Parts, replaced, kind.Noun, kind.Noun, k)
	}
	for i, p := range f.Parts {
		for _, w := range p.Claim.Words() {
			if n := len([]rune(w.Text)); n > 80 {
				fmt.Fprintf(e.stdout, "long: %s %s %s %d %s holds %d characters; a %s reads best under 80\n", kind.Noun, k, kind.Part, i+1, w.Field, n, kind.Part)
			}
		}
	}
	return nil
}

// checkFigure refuses a draft that is no figure of its kind true of the
// state: no question, too few or too many parts, a part that is not well
// formed, a quote the code does not hold once, a part the state does not
// bear out, or parts the state does not bear out taken together.
func checkFigure[C model.Claim[C]](e env, f *store.Figure[C], units []model.Unit) error {
	kind := model.KindOf[C]()
	switch {
	case f.Question == "" || strings.ContainsAny(f.Question, "\r\n"):
		return fmt.Errorf("a %s answers one question, on one line, in its question field", kind.Noun)
	case len(f.Parts) < kind.Min:
		return fmt.Errorf("a %s has %d %s or more, and this one has %d", kind.Noun, kind.Min, kind.Parts, len(f.Parts))
	case len(f.Parts) > kind.Max:
		return fmt.Errorf("a %s has at most %d %s, and this one has %d; split the question in two", kind.Noun, kind.Max, kind.Parts, len(f.Parts))
	}
	c := evidence.New(e.root)
	for i := range f.Parts {
		p := &f.Parts[i]
		at := fmt.Sprintf("%s %d", kind.Part, i+1)
		if err := p.Claim.Validate(); err != nil {
			return fmt.Errorf("%s: %w", at, err)
		}
		es, err := extract.NormalizeEvidence(p.Claim.Quotes())
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
		p.Claim = p.Claim.Requote(es)
		if gaps := p.Claim.Gaps(units); len(gaps) > 0 {
			return fmt.Errorf("%s: %s", at, strings.Join(gaps, "; "))
		}
	}
	if gaps := f.Gaps(units); len(gaps) > 0 {
		return errors.New(strings.Join(gaps, "; "))
	}
	return nil
}

// humanPart is a human's part by its place in a figure, from 1, and the
// source the human said it in.
type humanPart struct {
	place  int
	source string
}

type humanParts []humanPart

func (hs humanParts) String() string {
	var places, sources []string
	for _, h := range hs {
		places = append(places, fmt.Sprint(h.place))
		if !slices.Contains(sources, h.source) {
			sources = append(sources, h.source)
		}
	}
	return fmt.Sprintf("%s (%s)", strings.Join(places, ", "), strings.Join(sources, "; "))
}

// retire carries the retired parts of the stored figure over to the draft f,
// with each part of the stored figure whose code changed since it was
// written. It forgets those whose code is back as they were written, a unit
// of which left the state, or whose files are all gone. A reader's part of f
// that repeats one of them is refused: it would read fresh with no one
// reading that code again, however many stores ago it left the figure. A
// human's store is the human's word, so it is not refused and clears the
// retired parts it repeats.
func retire[C model.Claim[C]](e env, prev, f store.Figure[C], units []model.Unit) ([]store.Part[C], error) {
	kind := model.KindOf[C]()
	present := map[model.ID]bool{}
	for _, u := range model.Present(units) {
		present[u.ID] = true
	}
	gone := func(p store.Part[C]) bool {
		return slices.ContainsFunc(p.Claim.Names(), func(id model.ID) bool { return !present[id] }) ||
			!slices.ContainsFunc(p.Claim.Quotes(), func(ev model.Evidence) bool {
				_, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(ev.Path)))
				return err == nil
			})
	}
	var out []store.Part[C]
	for _, p := range append(slices.Clone(prev.Retired), prev.Parts...) {
		if p.Stale() && !gone(p) && !slices.ContainsFunc(out, func(o store.Part[C]) bool { return said(o.Claim, p.Claim) && maps.Equal(o.Code, p.Code) }) {
			out = append(out, store.Part[C]{Claim: p.Claim, Stamp: store.Stamp{Code: p.Code, Changed: p.Changed, Moved: p.Moved}})
		}
	}
	if e.writer() == model.Human {
		return slices.DeleteFunc(out, func(r store.Part[C]) bool {
			return slices.ContainsFunc(f.Parts, func(n store.Part[C]) bool { return repeats(n, r) })
		}), nil
	}
	for i, n := range f.Parts {
		for _, r := range out {
			if repeats(n, r) {
				return nil, fmt.Errorf("%s %d says what a STALE %s said, on a file whose code changed since it was written (%s); read it again, then say in new words what it does now", kind.Part, i+1, kind.Part, strings.Join(r.Changed, ", "))
			}
		}
	}
	return out, nil
}

// said reports whether two claims say the same words, in any case and
// spacing, whatever units they name.
func said[C model.Claim[C]](x, y C) bool {
	same := func(a, b model.Word) bool {
		return strings.EqualFold(strings.Join(strings.Fields(a.Text), ""), strings.Join(strings.Fields(b.Text), ""))
	}
	return slices.EqualFunc(x.Words(), y.Words(), same)
}

// repeats reports whether part n says what the retired part r said and
// cites a file whose code changed under r.
func repeats[C model.Claim[C]](n, r store.Part[C]) bool {
	return said(n.Claim, r.Claim) && slices.ContainsFunc(n.Claim.Quotes(), func(ev model.Evidence) bool { return slices.Contains(r.Moved, ev.Path) })
}

// mayReplace decides, part by part, whether the draft f may replace the
// stored figure. A human may replace any part. A reader may replace only a
// human's STALE parts: each current one must come back as written, in its
// order, and keeps the human's word, as do the question and caption, so CI
// never waits on the human for a part whose code moved and a human's word on
// a part that still holds is never lost.
// kept are the human's parts f keeps and replaced those the reader took
// over, each named by its place.
func mayReplace[C model.Claim[C]](prev store.Figure[C], had bool, f *store.Figure[C], units []model.Unit) (kept, replaced humanParts, err error) {
	if !had {
		return nil, nil, nil
	}
	kind := model.KindOf[C]()
	stale := func(p store.Part[C]) bool { return len(p.Why(units)) > 0 }
	human := func(p store.Part[C]) bool { return p.Writer == model.Human }
	if slices.ContainsFunc(f.Parts, human) || !slices.ContainsFunc(prev.Parts, human) {
		return nil, nil, nil
	}
	if !slices.ContainsFunc(prev.Parts, stale) {
		return nil, nil, fmt.Errorf("a human stated %s of %s %s and every %s is current; only a human changes it, with --source", kind.Parts, kind.Noun, f.Key, kind.Part)
	}
	for _, w := range []struct{ field, was, now string }{{"question", prev.Question, f.Question}, {"caption", prev.Caption, f.Caption}} {
		if !sameWords(w.was, w.now) {
			return nil, nil, fmt.Errorf("the %s of %s %s is a human's word; keep it as abbd %s %s prints it, since only a human changes it, with --source", w.field, kind.Noun, f.Key, kind.Noun, f.Key)
		}
	}
	at := 0
	for i, p := range prev.Parts {
		switch {
		case !human(p):
			continue
		case stale(p):
			replaced = append(replaced, humanPart{i + 1, p.Source})
			continue
		}
		j := slices.IndexFunc(f.Parts[at:], func(n store.Part[C]) bool { return samePart(p.Claim, n.Claim) })
		if j < 0 {
			return nil, nil, fmt.Errorf("%s %d is a human's word (%s) and current; keep it as abbd %s %s prints it, in its order, since only a human changes it, with --source", kind.Part, i+1, p.Source, kind.Noun, f.Key)
		}
		at += j
		f.Parts[at].Writer, f.Parts[at].Source = model.Human, p.Source
		kept = append(kept, humanPart{at + 1, p.Source})
		at++
	}
	return kept, replaced, nil
}

// samePart reports whether two claims name the same units and say the same
// thing in the same words on the same quotes.
func samePart[C model.Claim[C]](x, y C) bool {
	return slices.Equal(x.Names(), y.Names()) && sameCitations(x.Quotes(), y.Quotes()) &&
		slices.EqualFunc(x.Words(), y.Words(), func(a, b model.Word) bool { return sameWords(a.Text, b.Text) })
}

// printFigure writes the stored figure as a draft to edit, and says on
// stderr which parts are STALE.
func printFigure[C model.Claim[C]](e env, f store.Figure[C]) error {
	for i, p := range f.Parts {
		if p.Stale() {
			fmt.Fprintf(e.stderr, "abbd: %s %d is STALE: %s\n", model.KindOf[C]().Part, i+1, strings.Join(p.Changed, ", "))
		}
	}
	b, err := store.FigureDraft(f)
	if err != nil {
		return err
	}
	_, err = e.stdout.Write(b)
	return err
}

// statusFigures reports the figures of kind C: how many parts, which are
// STALE and why, which files are no figures. It returns what the check fails
// on, or "" when every part is current.
func statusFigures[C model.Claim[C]](e env, units []model.Unit, figs []store.Figure[C], strays []store.Stray) string {
	kind := model.KindOf[C]()
	var parts, stale int
	var lines []string
	for _, f := range figs {
		if !f.Known {
			lines = append(lines, fmt.Sprintf("orphaned: %s is named no %s key, so nothing reads it and the check skips it; rename or delete it", f.File, kind.Noun))
			continue
		}
		for i, p := range f.Parts {
			parts++
			why := p.Why(units)
			if len(why) == 0 {
				continue
			}
			stale++
			lines = append(lines, fmt.Sprintf("stale: %s %s %s %d (%s): %s; read that code again and store the %s with abbd %s %s FILE (deleting %s drops the whole %s and what it remembers of its STALE %s)", kind.Noun, f.Key, kind.Part, i+1, p.Claim.Label(), strings.Join(why, ", "), kind.Noun, kind.Noun, f.Key, f.File, kind.Noun, kind.Parts))
		}
		for _, g := range f.Gaps(units) {
			stale++
			lines = append(lines, fmt.Sprintf("stale: %s %s: %s; store the %s again with abbd %s %s FILE", kind.Noun, f.Key, g, kind.Noun, kind.Noun, f.Key))
		}
	}
	fmt.Fprintf(e.stdout, "%s: %d %s(s), %d %s(s), %d STALE\n", kind.Dir, len(figs), kind.Noun, parts, kind.Part, stale)
	dir := store.FigureDir[C]()
	for _, f := range strays {
		if path.Dir(f.File) == dir {
			fmt.Fprintf(e.stdout, "warning: %s %s, so it is no %s and abbd reads past it; fix its frontmatter, or move it out of %s, to clear this\n", f.File, f.Why, kind.Noun, dir)
		}
	}
	for _, l := range lines {
		fmt.Fprintln(e.stdout, l)
	}
	if stale == 0 {
		return ""
	}
	return fmt.Sprintf("%d %s %s(s) STALE, read the code each cites again and store the %s with abbd %s KEY FILE, or delete the file status names", stale, kind.Noun, kind.Part, kind.Noun, kind.Noun)
}
