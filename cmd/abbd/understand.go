package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/narrative"
	"github.com/whyjp/abbd/internal/store"
)

// textModule is the unit abbd understand stores a text for: any present
// module or feature, at any depth, whose text describes its own subtree. A
// stored text whose unit is not one is an orphan: no command can restamp it.
func textModule(units []model.Unit, id model.ID) (model.Unit, bool) {
	u, ok := presentUnit(units, id)
	return u, ok && u.Kind.Holds()
}

// understand stores what a reader wrote about a module, stamped with the
// module's code as it is now, so the screen can show the text beside the
// drawing and mark it STALE once the code moves on. With --restamp it stamps
// the stored text again instead, when only what the module holds moved.
func (e env) understand(ref string, args []string) error {
	e.reader = strings.TrimSpace(e.reader)
	if e.sentence != "" && (strings.Contains(ref, ".") || e.restamp) {
		return fmt.Errorf("--sentence goes with a module's or feature's new text, not with a note or --restamp")
	}
	if strings.Contains(ref, ".") {
		r, err := model.ParseNoteRef(ref)
		if err != nil {
			return err
		}
		return e.note(r, args)
	}
	if len(e.evidence) > 0 {
		return fmt.Errorf("a module's text describes all of its code and cites none; --evidence is for a slot such as %s.semantics", ref)
	}
	if e.rereadBy != "" {
		return fmt.Errorf("--reread-by is for a slot note such as %s.semantics; a module's text reads its whole module, so read it again with the /abbd skill's reader", ref)
	}
	id := model.ID(ref)
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	if e.restamp && e.reader != "" {
		return fmt.Errorf("--restamp keeps the stored text and its reader; pass --reader only with a new text")
	}
	if e.restamp != (len(args) == 0) {
		return fmt.Errorf("pass a FILE with the text, or --restamp without one")
	}
	u, ok := textModule(units, id)
	if !ok {
		return fmt.Errorf("%s is not a module or feature", id)
	}
	old, err := e.st.LoadUnderstandings()
	if err != nil {
		return err
	}
	digest, code := e.st.Stamp(units, id)
	prev, had := old[id]
	if e.restamp {
		if err := e.mayRestamp(units, prev, had, digest); err != nil {
			return err
		}
		// Code stays what the text was written against, so a file that
		// left the module can come back unchanged.
		prev.Digest = digest
		if err := e.st.SaveUnderstanding(prev); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "restamped %s %s against code %s; its code is unchanged since the text was written\n", id, u.Name.Value, digest)
		return nil
	}
	text, err := e.readInput(args[0], "pass the reader's Overview and Key Concepts")
	if err != nil {
		return err
	}
	// The first section is the summary the page shows before it folds the
	// rest, so it has to be short enough to read there.
	if ss := narrative.Sections(text); len(ss) > 0 && ss[0].Chars > narrative.MaxSection {
		return fmt.Errorf("the first section %q holds %d characters, over %d; the page shows it first, so keep it a short summary and move the rest under ### headings", ss[0].Heading, ss[0].Chars, narrative.MaxSection)
	}
	if err := unfinished(text); err != nil {
		return err
	}
	// Restamping the same words onto newer code would mark a stale reading
	// fresh without anyone reading the code again.
	if had && prev.Digest != digest && sameWords(prev.Text, text) {
		return fmt.Errorf("%s's code changed since this text was written; read the module again with the /abbd skill's reader and store the new text", id)
	}
	if err := e.st.SaveUnderstanding(store.Understanding{Unit: id, Origin: model.Inferred, Reader: e.reader, Digest: digest, Format: store.CodeFormat, Code: code, Text: text}); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "understood %s %s, INFERRED, against code %s\n", id, u.Name.Value, digest)
	readText(e.stdout, string(id), text)
	e.checkText(e.stdout, units, id, text, true)
	e.checkLanguage(string(id)+"'s text", text)
	if s := strings.TrimSpace(e.sentence); s != "" {
		// The reader who read the module again says its one line too,
		// stamped with the same code as the text. A person's line outranks
		// a reader's, so it stays.
		if u.Responsibility != nil && u.Responsibility.Writer == model.Human {
			fmt.Fprintf(e.stdout, "responsibility of %s stays as a person stated it; the reader's line is not stored\n", id)
			return nil
		}
		f := model.Said(model.Reader, s)
		f.Digest = digest
		u.Responsibility = &f
		if err := e.st.SaveUnits([]model.Unit{u}); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "responsibility of %s is now %q, INFERRED, against code %s\n", id, s, digest)
		e.checkLanguage(string(id)+"'s responsibility", s)
	}
	return nil
}

// readInput reads a text from FILE, or stdin for -, as abbd keeps it.
func (e env) readInput(arg, empty string) (string, error) {
	var b []byte
	var err error
	if arg == "-" {
		b, err = io.ReadAll(e.stdin)
	} else {
		b, err = os.ReadFile(arg)
	}
	if err != nil {
		return "", err
	}
	text := store.CleanText(string(b))
	if text == "" {
		return "", fmt.Errorf("the text is empty; %s", empty)
	}
	return text, nil
}

// note stores what a reader wrote into one slot of a module or a contract,
// stamped with the code it cites, so it goes STALE when that code changes
// and not when other code of the unit does.
func (e env) note(r model.NoteRef, args []string) error {
	switch {
	case e.restamp:
		return fmt.Errorf("a note describes only the code it cites, so moving units never stales it; store it again with abbd understand %s FILE --evidence PATH:LINES:QUOTE", r)
	case e.rereadBy != "" && e.reader != "":
		return fmt.Errorf("--reread-by names who read the cited code again; --reader is for a new note")
	case e.rereadBy != "":
		return e.reread(r, args)
	case len(args) == 0:
		return fmt.Errorf("pass a FILE with the note's text")
	case len(e.evidence) == 0:
		return fmt.Errorf("cite at least one --evidence PATH:LINES:QUOTE that %s rests on", r)
	}
	u, err := e.noteUnit(r)
	if err != nil {
		return err
	}
	cited, err := e.citations()
	if err != nil {
		return err
	}
	// sync never moves a note's lines, so a note follows a quote that moved
	// only when the quote is the one place in its file that holds it.
	c := evidence.New(e.root)
	for _, ev := range cited {
		if c.Occurrences(ev) > 1 {
			return fmt.Errorf("%s holds %q more than once; quote enough of the line that it appears once, so %s can follow it when the code above it moves", ev.Path, ev.Quote, r)
		}
	}
	text, err := e.readInput(args[0], "pass what the reader found for "+r.Slot.Name)
	if err != nil {
		return err
	}
	prev, had, err := e.st.LoadNote(r)
	if err != nil {
		return err
	}
	digest, code := e.st.NoteStamp(cited)
	// The same words on the same quotes, after those quotes' code moved,
	// would mark a stale note fresh without anyone reading the code again.
	if had && prev.Stale() && sameWords(prev.Text, text) && sameCitations(prev.Evidence, cited) {
		return fmt.Errorf("the code %s cites changed since it was written (%s); read it again, then store the new text or cite the code it rests on now", r, strings.Join(prev.Changed, ", "))
	}
	n := store.Note{Unit: r.Unit, Slot: r.Name(), Origin: model.Inferred, Reader: e.reader, Digest: digest, Format: store.CodeFormat, Evidence: cited, Code: code, Text: text}
	// A decision on other quotes is another decision, and the human did not
	// say why about it.
	dropped := prev.Why != nil && !sameCitations(prev.Evidence, cited)
	if !dropped {
		n.Why = prev.Why
	}
	if err := e.st.SaveNote(n); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "understood %s %s, INFERRED, against the code it cites %s\n", r, u.Name.Value, digest)
	if dropped {
		fmt.Fprintf(e.stdout, "dropped the why %s stated, since the decision now cites other code; ask again and store it with abbd record fact %s --value WHY --source WHERE\n", prev.Why.Source, r)
	}
	readText(e.stdout, r.String(), text)
	e.checkLanguage(r.String(), text)
	return nil
}

// noteUnit is the module or contract a note's slot belongs to, refusing a
// unit that takes no notes and an entry its contract does not list.
func (e env) noteUnit(r model.NoteRef) (model.Unit, error) {
	units, err := e.st.LoadUnits()
	if err != nil {
		return model.Unit{}, err
	}
	u, ok := presentUnit(units, r.Unit)
	if !ok || !u.Kind.HoldsNotes() {
		return u, fmt.Errorf("%s is not a module or a contract of the state", r.Unit)
	}
	if r.Unheld(u) {
		return u, fmt.Errorf("%s lists no entry %s; abbd explain %s lists its entries", r.Unit, r.Key, r.Unit)
	}
	return u, nil
}

// reread stamps a STALE note against its cited code again, keeping its
// words and quotes, for a reader who read that code again and found the note
// still holds, as after a whitespace or comment edit near a quote. It names
// who read it and records that with the date and what had moved, and it is
// refused while any quote is not in its file exactly once, word for word.
func (e env) reread(r model.NoteRef, args []string) error {
	by := strings.TrimSpace(e.rereadBy)
	switch {
	case len(args) > 0 || len(e.evidence) > 0:
		return fmt.Errorf("--reread-by keeps %s's words and quotes; pass neither a FILE nor --evidence, or store new ones without it", r)
	case by == "":
		return fmt.Errorf("--reread-by names who read the code %s cites again", r)
	}
	u, err := e.noteUnit(r)
	if err != nil {
		return err
	}
	n, had, err := e.st.LoadNote(r)
	switch {
	case err != nil:
		return err
	case !had:
		return fmt.Errorf("there is no note %s to stamp again; store one with abbd understand %s FILE --evidence PATH:LINES:QUOTE", r, r)
	case !n.Stale():
		return fmt.Errorf("%s is current; nothing to stamp again", r)
	}
	c := evidence.New(e.root)
	for i, ev := range n.Evidence {
		if c.Occurrences(ev) != 1 {
			return fmt.Errorf("%s no longer holds %q exactly once, so %s does not rest on the code it quotes; read it again and store the note with abbd understand %s FILE --evidence PATH:LINES:QUOTE", ev.Path, ev.Quote, r, r)
		}
		if ev.Lines != "" {
			n.Evidence[i].Lines = c.Check(ev).At
		}
	}
	// A note whose evidence was edited by hand was stamped against other
	// quotes, so its words and a human's why speak about other code.
	digest, code := e.st.NoteStamp(n.Evidence)
	if !slices.Equal(slices.Sorted(maps.Keys(code)), slices.Sorted(maps.Keys(n.Code))) {
		return fmt.Errorf("%s was stamped against other quotes than its evidence names, so its words were not written about them; read the code it cites and store it again with its words, abbd understand %s FILE --evidence PATH:LINES:QUOTE", r, r)
	}
	n.Digest, n.Code = digest, code
	n.Format = store.CodeFormat
	n.Reread = &store.Reread{By: by, On: time.Now().UTC().Format(time.DateOnly), Moved: n.Changed}
	if err := e.st.SaveNote(n); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "stamped %s %s again against the code it cites %s, reread by %s after %s moved; its words and quotes are unchanged\n", r, u.Name.Value, n.Digest, by, strings.Join(n.Changed, ", "))
	return nil
}

// forget removes the note whose file ref names, with the human's why it
// holds, and says what it was. It goes by the file and not by the slot, so a
// note abbd no longer reads as its slot's, orphaned or misfiled, goes too.
// A bare ID forgets a unit sync lists as unmatched.
func (e env) forget(ref string) error {
	unit, slot, ok := strings.Cut(ref, ".")
	if _, _, err := model.ParseID(ref); !ok && err == nil {
		return e.forgetUnit(model.ID(ref))
	}
	if !ok || unit == "" || slot == "" {
		return fmt.Errorf("name the note as ID.SLOT[.KEY], as abbd status prints it, or an unmatched unit as ID")
	}
	notes, _, err := e.st.LoadNotes()
	if err != nil {
		return err
	}
	// A misfiled note is listed under the unit it says it is, not its file's.
	var n store.Note
	for _, ns := range notes {
		if i := slices.IndexFunc(ns, func(n store.Note) bool { got, ok := n.FileRef(); return ok && got == ref }); i >= 0 {
			n = ns[i]
		}
	}
	if n.File == "" {
		return fmt.Errorf("there is no note at %s", store.NoteFile(model.ID(unit), slot))
	}
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	u, present := presentUnit(units, n.Unit)
	state := "CURRENT"
	switch {
	case n.Misfiled:
		state = fmt.Sprintf("misfiled, it says it is %s.%s", n.Unit, n.Slot)
	case !present || !u.Kind.HoldsNotes() || !n.Known || n.Ref.Unheld(u):
		state = "orphaned"
	case n.Stale():
		state = "STALE"
	}
	if err := e.st.RemoveNote(n); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "forgot %s (%s): removed %s\n", ref, state, n.File)
	if n.Why != nil {
		fmt.Fprintf(e.stdout, "the why stated at %s went with it: %s\n", n.Why.Source, n.Why.Value)
	}
	return nil
}

// sameWords reports whether two texts say the same words, however they
// are spaced or broken into lines.
func sameWords(a, b string) bool {
	return slices.Equal(strings.Fields(a), strings.Fields(b))
}

// sameCitations reports whether two notes cite the same set of quotes,
// whatever their order or lines.
func sameCitations(a, b []model.Evidence) bool {
	set := func(es []model.Evidence) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Path+"\x00"+e.Quote)
		}
		slices.Sort(out)
		return slices.Compact(out)
	}
	return slices.Equal(set(a), set(b))
}

// mayRestamp allows a restamp only when every file of the module now is one
// the text was written against, unchanged, and none it was written against
// is gone, so an extraction that moved units is the only thing that moved.
func (e env) mayRestamp(units []model.Unit, prev store.Understanding, had bool, digest string) error {
	id := prev.Unit
	switch {
	case !had:
		return fmt.Errorf("there is no text to restamp; store one with abbd understand MOD-ID FILE")
	case prev.Digest == digest:
		return fmt.Errorf("%s's text is current; nothing to restamp", id)
	case prev.Format != store.CodeFormat:
		return fmt.Errorf("%s's text was stored before abbd recorded the code a text is written against; read the module again with the /abbd skill's reader and store the new text", id)
	}
	changed := e.st.Changed(units, id, prev.Code)
	if len(changed) == 0 {
		return nil
	}
	if len(changed) > 5 {
		changed = append(changed[:5], fmt.Sprintf("and %d more", len(changed)-5))
	}
	return fmt.Errorf("%s's code changed since this text was written (%s); read the module again with the /abbd skill's reader and store the new text", id, strings.Join(changed, ", "))
}

// stampSentences keeps the stamp each purpose and responsibility of a
// module or feature carried while its words stay the same, and stamps a
// sentence no stamp covers yet against its code as it is now. A human's
// sentence is never stamped: a person said it, and no code bears it out.
func stampSentences(st store.Store, stored, units []model.Unit) {
	was := map[model.ID]model.Unit{}
	for _, u := range stored {
		was[u.ID] = u
	}
	for i := range units {
		u := &units[i]
		if u.Absent || !u.Kind.Holds() {
			continue
		}
		old := was[u.ID]
		digest := ""
		for _, f := range []struct{ now, then *model.Fact }{{u.Purpose, old.Purpose}, {u.Responsibility, old.Responsibility}} {
			if f.now == nil || f.now.Value == "" || f.now.Writer == model.Human {
				continue
			}
			if f.then != nil && f.then.Value == f.now.Value && f.then.Digest != "" {
				f.now.Digest = f.then.Digest
				continue
			}
			if digest == "" {
				digest = st.ModuleDigest(units, u.ID)
			}
			f.now.Digest = digest
		}
	}
}

// forgetUnit removes a unit the extraction no longer reports, once nothing
// else in the state names it, so sync stops listing it as unmatched. Its
// number stays taken, so a unit that comes back takes a new ID.
func (e env) forgetUnit(id model.ID) error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(units, func(u model.Unit) bool { return u.ID == id })
	switch {
	case i < 0:
		return fmt.Errorf("there is no unit %s", id)
	case !units[i].Absent:
		return fmt.Errorf("%s is present, and abbd forget removes a unit only once sync lists it as unmatched; name a note as ID.SLOT[.KEY], as abbd status prints it", id)
	}
	naming, err := e.st.Naming(units[i])
	if err != nil {
		return err
	}
	if len(naming) > 0 {
		return fmt.Errorf("%s is still named in %s; remove or retract what names it first", id, strings.Join(naming, ", "))
	}
	if err := e.st.RemoveUnit(units[i]); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "forgot %s %s: sync no longer lists it as unmatched, and its number stays taken\n", id, units[i].Name.Value)
	return nil
}
