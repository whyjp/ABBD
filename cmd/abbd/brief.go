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
	fmt.Fprint(w, "## Overview\n\nTODO what the module is responsible for, the contracts it provides and uses by name, and one thing it does not do.\n\n## Key Concepts\n\n")
	byID := map[model.ID]model.Unit{}
	for _, v := range units {
		byID[v.ID] = v
	}
	for _, r := range ex.Inside {
		v := byID[r.ID]
		at := ""
		if len(v.Anchors) > 0 {
			at = " (`" + v.Anchors[0].Path + "`)"
		}
		fmt.Fprintf(w, "- TODO **%s**%s: %s %s%s\n", r.Name, at, r.Kind, r.ID, stored(v))
	}
	for _, l := range ex.Out {
		if l.Kind == model.Provides {
			fmt.Fprintf(w, "- TODO **%s**: provided contract %s%s\n", name[l.To], l.To, stored(byID[l.To]))
		}
	}

	fmt.Fprintf(w, "\n## Read first\n\nRead what this section lists, rewrite every TODO above, delete this section, then store the text with abbd understand %s FILE --reader WHO.\n\n### Files, entry first\n\n", id)
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
	var keys []string
	for _, k := range e.keyUnitsOf(units, id, keyShown) {
		keys = append(keys, fmt.Sprintf("`%s` in `%s`, named %d times elsewhere", k.Name, k.File, k.Uses))
	}
	list(w, "Key units by fan-in: name each in the text, or say why it stays out", keys)
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

// stored is a unit's stored sentence with its origin, for a TODO to rewrite.
func stored(u model.Unit) string {
	for _, f := range []*model.Fact{u.Responsibility, u.Purpose, u.Signature} {
		if f != nil && f.Value != "" {
			return fmt.Sprintf("; stored: %s (%s)", f.Value, f.Origin)
		}
	}
	return ""
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

// keyShown is how many key units the brief lists and the text check holds
// the text to.
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
