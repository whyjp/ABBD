package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
	"github.com/whyjp/abbd/internal/topology"
)

// entryStems name the files a reader opens first: where a package starts or
// says what it is.
var entryStems = []string{"main", "index", "mod", "lib", "__init__", "doc", "readme"}

// brief prints the reading brief for a module's text: the skeleton the text
// fills, then what to read, which the reader deletes before storing. It
// reads only.
func (e env) brief(ref string) error {
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	id := model.ID(ref)
	_, ok := textModule(units, id)
	if !ok {
		return fmt.Errorf("%s is not a module or feature; abbd explain QUERY finds its ID", ref)
	}
	name := names(units)
	ex := topology.Explain(units, id)
	w := e.stdout
	fmt.Fprint(w, "## Overview\n\nTODO write this last, in 3 to 5 sentences from the claim ledger: what the module is responsible for, each claim with its guard, and the neighbour that owns the adjacent work.\n\n## Key Concepts\n\n")
	byID := map[model.ID]model.Unit{}
	for _, v := range units {
		byID[v.ID] = v
	}
	var hints []string
	for _, r := range ex.Inside {
		fmt.Fprintf(w, "- TODO **%s**: %s %s\n", r.Name, r.Kind, r.ID)
		if h := hint(byID[r.ID]); h != "" {
			hints = append(hints, fmt.Sprintf("**%s** %s: %s", r.Name, r.ID, h))
		}
	}
	for _, l := range ex.Out {
		if l.Kind == model.Provides {
			fmt.Fprintf(w, "- TODO **%s**: provided contract %s\n", name[l.To], l.To)
			if h := hint(byID[l.To]); h != "" {
				hints = append(hints, fmt.Sprintf("**%s** %s: %s", name[l.To], l.To, h))
			}
		}
	}

	fmt.Fprint(w, "\n## Read first\n\n")
	p, err := e.st.LoadProject()
	if err != nil {
		return err
	}
	if p.Language != "" {
		fmt.Fprintf(w, "Write all prose in %s; keep code identifiers, paths and quotes verbatim.\n\n", p.Language)
	}
	fmt.Fprintf(w, "Every line below is a hypothesis to confirm or drop, never prose to rewrite, and the text says nothing about this brief. Build the claim ledger the reader describes, write each TODO above from it, check the draft as the reader's Before you store section says, then delete this section and store the text with abbd understand %s FILE --reader WHO.\n\n### Files, entry first\n\n", id)
	files, quotes := e.st.Read(units, id)
	slices.SortStableFunc(files, func(a, b string) int { return entryRank(a) - entryRank(b) })
	for _, f := range files {
		size := "gone"
		if fi, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(f))); err == nil {
			size = fmt.Sprintf("%d bytes", fi.Size())
		}
		fmt.Fprintf(w, "- `%s` (%s)\n", f, size)
	}
	for _, q := range quotes {
		fmt.Fprintf(w, "- the lines around %s\n", q)
	}
	var contracts []string
	for _, l := range ex.Out {
		contracts = append(contracts, fmt.Sprintf("%s %s %s%s", l.Kind, l.To, name[l.To], l.Label()))
	}
	for _, l := range ex.In {
		contracts = append(contracts, fmt.Sprintf("used by %s %s, %s%s", l.From, name[l.From], l.Kind, l.Label()))
	}
	list(w, "Files to start from and stored lines, by unit; a bullet's code span is the file its claim's quote is in, not one named here", hints)
	var used []string
	for _, k := range e.keyUnitsOf(units, id, keyShown) {
		used = append(used, fmt.Sprintf("`%s` in `%s`, named in %d file(s) outside the module", k.Name, k.File, k.Users))
	}
	list(w, "Names other modules use most: candidates for the types and calls a user touches first; cover those the code bears out and skip the rest without a word", used)
	list(w, "Boxes that say only their kind: each draft comes from a Go doc comment or a file's header comment; keep it only where the code bears it out, and store the line in the supplement, or a module's or feature's with abbd understand --sentence", e.drafts(byID, name, drawnWith(id, ex)))
	list(w, "Run-time binding sites: record the tie each makes as a coupling with its quote, via load, spawn, trait, callback or config, and a registry as a contract its members provide", bindingSites(e.root, files))
	list(w, "Contracts and neighbours", contracts)
	var couplings []string
	for _, l := range ex.Inferred {
		s := fmt.Sprintf("%s %s %s %s %s", l.From, name[l.From], l.Kind, l.To, name[l.To])
		if p := l.PhraseLabel(); p != "" {
			s += "; " + p
		}
		for _, ev := range l.Evidence {
			s += "\n  " + ev.String()
		}
		couplings = append(couplings, s)
	}
	list(w, "Couplings readers recorded", couplings)
	cites, err := e.citing(units, id)
	if err != nil {
		return err
	}
	list(w, "What cites it", cites)
	texts, err := e.st.LoadUnderstandings()
	if err != nil {
		return err
	}
	if t, had := texts[id]; had {
		moved := e.st.Changed(units, id, t.Code)
		if len(moved) == 0 {
			moved = []string{"nothing: the stored text is CURRENT"}
		}
		list(w, "Moved since the stored text", moved)
	}
	return nil
}

// hint is where a unit's reading starts and the sentence stored for it, as
// a hypothesis: a supplement's or a reader's line carries no evidence, so
// the reader confirms it from the code or drops it.
func hint(u model.Unit) string {
	var parts []string
	if len(u.Anchors) > 0 {
		parts = append(parts, "start from `"+u.Anchors[0].Path+"`")
	}
	if f := storedFact(u); f != nil {
		switch f.Writer {
		case model.Supplement:
			parts = append(parts, fmt.Sprintf("stored line, unverified (setup wrote it without evidence): confirm or drop: %q", f.Value))
		case model.Reader:
			parts = append(parts, fmt.Sprintf("stored line, unverified (a reader wrote it without evidence): confirm or drop: %q", f.Value))
		default:
			parts = append(parts, fmt.Sprintf("stored line (%s): %q", f.Origin, f.Value))
		}
	}
	return strings.Join(parts, "; ")
}

func storedFact(u model.Unit) *model.Fact {
	for _, f := range []*model.Fact{u.Responsibility, u.Purpose, u.Signature} {
		if f != nil && f.Value != "" {
			return f
		}
	}
	return nil
}

func entryRank(p string) int {
	stem := strings.ToLower(path.Base(p))
	stem = strings.TrimSuffix(stem, path.Ext(stem))
	if slices.Contains(entryStems, stem) {
		return 0
	}
	return 1
}

func list(w io.Writer, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(w, "\n### %s\n\n", title)
	for _, l := range lines {
		fmt.Fprintf(w, "- %s\n", l)
	}
}

// citing names the notes, flows, stacks and tables that name the module or
// a unit inside it.
func (e env) citing(units []model.Unit, id model.ID) ([]string, error) {
	own := map[model.ID]bool{id: true}
	for grew := true; grew; {
		grew = false
		for _, u := range model.Present(units) {
			if !own[u.ID] && own[u.Parent] {
				own[u.ID], grew = true, true
			}
		}
	}
	var out []string
	notes, _, err := e.st.LoadNotes()
	if err != nil {
		return nil, err
	}
	for _, u := range slices.Sorted(maps.Keys(own)) {
		for _, n := range notes[u] {
			out = append(out, "note "+n.Ref.String())
		}
	}
	fs, err := e.st.LoadFigures()
	if err != nil {
		return nil, err
	}
	out = append(out, cited("flow", fs.Flows, own)...)
	out = append(out, cited("stack", fs.Stacks, own)...)
	return append(out, cited("table", fs.Tables, own)...), nil
}

func cited[C model.Claim[C]](kind string, figs []store.Figure[C], own map[model.ID]bool) []string {
	var out []string
	for _, f := range figs {
		for _, p := range f.Parts {
			if slices.ContainsFunc(p.Claim.Names(), func(id model.ID) bool { return own[id] }) {
				out = append(out, fmt.Sprintf("%s %s: %s", kind, f.Key, p.Claim.Label()))
			}
		}
	}
	return out
}

// keyShown is how many of the names other modules use most the brief lists.
const keyShown = 8

// drawnWith is the module, the units its page draws inside it, and the
// contracts it provides.
func drawnWith(id model.ID, ex topology.Explanation) []model.ID {
	out := []model.ID{id}
	for _, r := range ex.Inside {
		out = append(out, r.ID)
	}
	for _, l := range ex.Out {
		if l.Kind == model.Provides {
			out = append(out, l.To)
		}
	}
	return out
}

// drafts is, for each of ids that records neither a purpose nor a
// responsibility, a draft drawn from its anchors and the file it came from.
func (e env) drafts(byID map[model.ID]model.Unit, name map[model.ID]string, ids []model.ID) []string {
	var out []string
	for _, id := range ids {
		u, ok := byID[id]
		if !ok || hasValue(u.Purpose) || hasValue(u.Responsibility) {
			continue
		}
		s, from := e.draft(u)
		if s == "" {
			at := "its code"
			if len(u.Anchors) > 0 {
				at = "`" + u.Anchors[0].Path + "`"
			}
			out = append(out, fmt.Sprintf("%s %s: no comment at its anchors says what it is for; read %s", id, name[id], at))
			continue
		}
		out = append(out, fmt.Sprintf("%s %s: %q (from `%s`)", id, name[id], s, from))
	}
	return out
}
