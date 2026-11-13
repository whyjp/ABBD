package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/whyjp/abbd/internal/evidence"
	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/store"
)

// overviewDraft prints a draft a reader writes the overview from: the
// stored zones and lines, if any, under comments that give what the state
// offers. Those are the boxes the overview draws, each with the stage of
// the primary flow it stands at, the stages, the groupings the overview's
// bands draw as zone candidates, and every pair of boxes a live relation
// joins as a line candidate, strongest first, with a quote to start from.
// abbd reads past the comments, so the reader picks, words and stores.
func (e env) overviewDraft() error {
	prev, err := e.st.LoadOverview()
	if err != nil {
		return err
	}
	if prev.File != "" && !prev.Known {
		return fmt.Errorf("%s %s, so it is no overview; fix its frontmatter or store one with abbd overview FILE", prev.File, prev.Why)
	}
	units, err := e.st.LoadUnits()
	if err != nil {
		return err
	}
	figs, err := e.st.LoadFigures()
	if err != nil {
		return err
	}
	for _, l := range overviewStale(prev, nil) {
		fmt.Fprintln(e.stderr, "abbd:", l.text)
	}
	b, err := store.OverviewDraft(prev)
	if err != nil {
		return err
	}
	front, caption, _ := strings.Cut(strings.TrimPrefix(string(b), "---\n"), "---\n")
	if len(prev.Zones)+len(prev.Lines) == 0 {
		front = "zones: []\nlines: []\n"
	}
	var w strings.Builder
	w.WriteString("---\n")
	writeOffers(&w, units, figs, prev, newQuoteFinder(e, units).find)
	w.WriteString(front + "---\n" + caption)
	_, err = io.WriteString(e.stdout, w.String())
	return err
}

// quoteFinder looks for a line to quote under a line candidate no relation
// under it quotes, reading each box's files once.
type quoteFinder struct {
	e     env
	byID  map[model.ID]model.Unit
	units []model.Unit
	check *evidence.Checker
	rests func([]model.ID, model.Evidence) bool
	files map[model.ID][]string
	text  map[string][]string
}

func newQuoteFinder(e env, units []model.Unit) quoteFinder {
	q := quoteFinder{e: e, byID: map[model.ID]model.Unit{}, units: units, check: evidence.New(e.root), rests: model.Rests(units), files: map[model.ID][]string{}, text: map[string][]string{}}
	for _, u := range units {
		q.byID[u.ID] = u
	}
	return q
}

// find returns a line of the candidate's from box that names a unit a
// relation under it runs to, and that its file holds once, with what it
// matched. It looks for a line that calls a name, then an import line that
// names one, then any line of code that names one, and tries the relations
// strongest first, each by the needles in order. It finds none when no such
// line exists.
func (q quoteFinder) find(c model.LineCandidate) (model.Evidence, string, bool) {
	files, ok := q.files[c.From]
	if !ok {
		files, _ = q.e.st.Read(q.units, c.From)
		files = quotable(slices.Sorted(slices.Values(files)))
		q.files[c.From] = files
	}
	for _, u := range []use{useCall, useImport, useName} {
		for _, end := range c.Ends {
			for _, n := range needles(q.byID[end[0]], q.byID[end[1]]) {
				for _, f := range files {
					if ev, ok := q.line(f, n, u, []model.ID{c.From, c.To}); ok {
						return ev, n.name, true
					}
				}
			}
		}
	}
	return model.Evidence{}, "", false
}

// quotable are the files of a box a quote may come from: its code. Tests,
// test fixtures and prose are no code of the box, and a data file such as
// JSON or YAML counts only for a box that has no code of a language.
func quotable(files []string) []string {
	files = slices.DeleteFunc(files, noQuoteFile)
	if slices.ContainsFunc(files, func(f string) bool { return !dataFile(f) }) {
		files = slices.DeleteFunc(files, dataFile)
	}
	return files
}

// use is how a quoted line uses what it names, in the order find prefers.
type use int

const (
	useCall use = iota
	useImport
	useName
)

// line is the first line of file f that n names, used as u says, that the
// file holds once and that lies in the code of ends, as abbd overview FILE
// takes a line's quote. A comment, a docstring and the definition of a
// function are no use of what they name.
func (q quoteFinder) line(f string, n needle, u use, ends []model.ID) (model.Evidence, bool) {
	re := n.word
	if u == useCall {
		re = n.call
	}
	if re == nil {
		return model.Evidence{}, false
	}
	lines, ok := q.text[f]
	if !ok {
		b, _ := os.ReadFile(filepath.Join(q.e.root, filepath.FromSlash(f)))
		lines = blankProse(strings.Split(string(b), "\n"))
		q.text[f] = lines
	}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 160 || !re.MatchString(line) || commentary.MatchString(line) || definition.MatchString(line) || u == useImport && !importLine.MatchString(line) {
			continue
		}
		ev := model.Evidence{Path: f, Lines: strconv.Itoa(i + 1), Quote: line}
		if q.check.Check(ev).Live && q.check.Occurrences(ev) == 1 && q.rests(ends, ev) {
			return ev, true
		}
	}
	return model.Evidence{}, false
}

// commentary is a line of a comment in most languages, definition the line
// that defines a function, which names a function without using it, and
// importLine a line that imports, includes or requires what it names.
var (
	importLine = regexp.MustCompile(`^(?:import|from\s+\S+\s+import|use|using|#\s*include|#import|require|extern\s+crate)\b|\b(?:require|import)\s*\(`)
	commentary = regexp.MustCompile(`^(?://|/\*|\*|--|#(?:\s|$|[^a-z]|(?:todo|fixme|note|comment|xxx))|"""|;;)`)
	definition = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:func|fn|def|function)\b`)
)

// blankProse blanks the lines inside a block comment or a triple-quoted
// string, such as a docstring, whose words name what the code does without
// using it. The line that opens or closes one keeps its code.
func blankProse(lines []string) []string {
	out := slices.Clone(lines)
	closer := ""
	for i, l := range lines {
		if closer != "" {
			out[i] = ""
			if strings.Contains(l, closer) {
				closer = ""
			}
			continue
		}
		for _, d := range []string{`"""`, "'''"} {
			if strings.Count(l, d)%2 == 1 {
				closer = d
			}
		}
		if j := strings.LastIndex(l, "/*"); closer == "" && j >= 0 && !strings.Contains(l[j:], "*/") {
			closer = "*/"
		}
	}
	return out
}

// needle is one way the code of a box spells a unit it runs to: word
// matches a line that spells it and call a line that calls it, nil where
// the spelling is no call.
type needle struct {
	name       string
	word, call *regexp.Regexp
}

// needles are what the code of from spells to by, most precise first: the
// names identNames gives, which a line may call or only name, then, for any
// language, the unit's place in the tree. Its anchor directories are what an
// import line spells, as a path (Go, JS, C), dotted (Python, Java) or with
// :: (Rust), and its last directory is what a qualified use spells, as
// gitx.Head or gitx::head, even where that directory is a plain word.
func needles(from, to model.Unit) []needle {
	var out []needle
	for _, name := range identNames(from, to) {
		q := regexp.QuoteMeta(name)
		out = append(out, needle{name, regexp.MustCompile(`\b` + q + `\b`), regexp.MustCompile(`\b` + q + `\(`)})
	}
	dirs := anchorDirs(to)
	for _, d := range dirs {
		segs := strings.Split(d, "/")
		segs = segs[max(0, len(segs)-2):]
		if len(segs) == 1 && oneSegment.MatchString(segs[0]) && !generic[segs[0]] {
			// A top-level directory is found by an include or import path under it.
			out = append(out, needle{segs[0] + "/", regexp.MustCompile(`["'<]` + regexp.QuoteMeta(segs[0]) + `/`), nil})
		}
		if len(segs) == 2 && !generic[segs[1]] {
			out = append(out, needle{strings.Join(segs, "/"), regexp.MustCompile(`\b` + regexp.QuoteMeta(segs[0]) + `(?:/|\.|::)` + regexp.QuoteMeta(segs[1]) + `\b`), nil})
		}
	}
	for _, d := range dirs {
		last := path.Base(d)
		if identName.MatchString(last) && !generic[last] {
			use := `\b` + regexp.QuoteMeta(last) + `(?:\.|::)\w+`
			out = append(out, needle{last + ".", regexp.MustCompile(use), regexp.MustCompile(use + `\s*\(`)})
		}
	}
	return out
}

// anchorDirs are the directories the anchors of u lie in, each once: the
// anchor itself for a directory, its directory for a file.
func anchorDirs(u model.Unit) []string {
	var out []string
	for _, a := range u.Anchors {
		d := path.Clean(a.Path)
		if path.Ext(d) != "" {
			d = path.Dir(d)
		}
		if d != "." && d != "/" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// identNames are the names the code of from spells to, most precise
// first: the entries of to that from's relations to it use, the symbols to
// anchors, its name as an identifier once dashes, dots and spaces become
// underscores, as a package or crate is imported, then the files it anchors,
// as an include or an import names a file. A plain lower-case word, such as
// validate, names many unrelated things and is left out.
func identNames(from, to model.Unit) []string {
	var out []string
	add := func(name string) {
		if identName.MatchString(name) && !plainWord(name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	for _, r := range from.Relations {
		if r.To == to.ID {
			for _, key := range r.Entries {
				if i := slices.IndexFunc(to.Entries, func(e model.Entry) bool { return e.Key == key }); i >= 0 {
					add(to.Entries[i].Name)
				}
			}
		}
	}
	for _, a := range to.Anchors {
		add(a.Symbol)
	}
	add(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(to.Name.Value))
	for _, a := range to.Anchors {
		if base := path.Base(a.Path); a.Symbol == "" && path.Ext(base) != "" && !generic[strings.TrimSuffix(base, path.Ext(base))] && !slices.Contains(out, base) {
			out = append(out, base)
		}
	}
	return out
}

var identName = regexp.MustCompile(`^[A-Za-z_]\w{2,}$`)

// generic are file names many packages share, which name no one unit.
var generic = map[string]bool{"main": true, "index": true, "mod": true, "lib": true, "init": true, "__init__": true, "util": true, "utils": true, "common": true, "types": true, "config": true, "app": true}

// writeOffers writes what the state offers an overview, as YAML comments.
// find gives a quote for a candidate no relation under it quotes.
func writeOffers(w io.Writer, units []model.Unit, figs store.Figures, o store.Overview, find func(model.LineCandidate) (model.Evidence, string, bool)) {
	p, primary := figs.Primary()
	layers := p.Claims()
	byID := map[model.ID]model.Unit{}
	for _, u := range units {
		byID[u.ID] = u
	}
	boxes := model.Boxes(units)
	held := model.Holders(layers, units)
	stage := func(id model.ID) int {
		if n, ok := held[id]; ok {
			return n
		}
		n, _ := model.StandsAt(layers, units, id)
		return n
	}
	fmt.Fprintln(w, "# A draft of the overview. Choose from what follows, write the zones and")
	fmt.Fprintln(w, "# lines below, and store it with abbd overview FILE. abbd reads past # lines.")
	fmt.Fprintln(w, "# This whole output is FILE: save it as it is, keeping the --- line above and")
	fmt.Fprintln(w, "# the one that closes the zones and lines, and abbd overview FILE stores it")
	fmt.Fprintln(w, "# unchanged. The caption goes under the second ---.")
	fmt.Fprintln(w, "#")
	fmt.Fprintln(w, "# A zone names boxes from this list. A line runs between two of these boxes; an")
	fmt.Fprintln(w, "# end may also be a unit inside one, and the line is drawn from that unit's box.")
	fmt.Fprintf(w, "#\n# The boxes the overview draws (%d), and the stage of the primary flow each stands at:\n", len(boxes))
	for _, id := range boxes {
		at := "-"
		if n := stage(id); n > 0 {
			at = fmt.Sprintf("stage %d %s", n, layers[n-1].Name)
		}
		fmt.Fprintf(w, "#   %-10s %-32s %s\n", id, byID[id].Name.Value, at)
	}
	fmt.Fprintln(w, "#")
	if primary {
		fmt.Fprintf(w, "# The primary flow is stack %s (%s), its layers the stages:\n", p.Key, p.Question)
		for i, l := range layers {
			fmt.Fprintf(w, "#   %d %s: %s\n", i+1, l.Name, joinIDs(l.Units))
		}
	} else {
		fmt.Fprintln(w, "# No stack is marked the primary flow. Store the stages a person would narrate")
		fmt.Fprintln(w, "# as a stack with primary: true, with abbd stack KEY FILE.")
	}
	fmt.Fprintln(w, "#\n# Zone candidates, the groupings the overview's bands draw where their boxes sit together:")
	zones := zoneOffers(units, boxes, byID, figs)
	for _, z := range zones {
		fmt.Fprintf(w, "#   - {name: %s, units: [%s]}  # %s\n", strconv.Quote(z.name), joinIDs(z.units), z.from)
	}
	if len(zones) == 0 {
		fmt.Fprintln(w, "#   none")
	}
	chosen := map[[2]model.ID]bool{}
	for _, l := range store.ClaimsOf(o.Lines) {
		chosen[[2]model.ID{l.From, l.To}] = true
	}
	cands := model.LineCandidates(units)
	fmt.Fprintf(w, "#\n# Line candidates (%d), one per pair of boxes a live relation joins in its direction,\n", len(cands))
	fmt.Fprintln(w, "# strongest first: runs (posts_to, call, data, trait, callback), then wires (config,")
	fmt.Fprintln(w, "# convention, load, spawn), then imports. Each gives its relations and a quote under it:")
	fmt.Fprintln(w, "# a relation's own quote, else a line of the from box that names the unit it runs to,")
	fmt.Fprintln(w, "# found by name, which you read before you keep.")
	for _, c := range cands {
		ev, note := "[]", ""
		q := c.Quote
		if q.Path == "" {
			var name string
			var ok bool
			if q, name, ok = find(c); ok {
				note = fmt.Sprintf("\n#     no relation under it quotes code; the quote is a line of %s that names %s", byID[c.From].Name.Value, name)
			} else {
				note = fmt.Sprintf("\n#     no relation under it quotes code, and no line of %s names what it runs to; quote the line where it crosses, such as the call, the include or the file it writes", byID[c.From].Name.Value)
			}
		}
		if q.Path != "" {
			ev = fmt.Sprintf("[{path: %s, lines: %s, quote: %s}]", strconv.Quote(q.Path), strconv.Quote(q.Lines), strconv.Quote(q.Quote))
		}
		mark := ""
		if chosen[[2]model.ID{c.From, c.To}] {
			mark = "  # chosen below"
		}
		fmt.Fprintf(w, "#   - {from: %s, to: %s, word: \"\", evidence: %s}%s\n", c.From, c.To, ev, mark)
		fmt.Fprintf(w, "#     %s -> %s: %s%s\n", byID[c.From].Name.Value, byID[c.To].Name.Value, tiesText(c), note)
	}
	fmt.Fprintln(w, "#")
}

// tiesText says what relations run under a line candidate, by carry.
func tiesText(c model.LineCandidate) string {
	var parts []string
	for _, k := range []model.Carry{model.Runs, model.Wires, model.Imports} {
		var ts []string
		for _, t := range c.Ties {
			if model.CarryOf(t.Kind, t.Via) == k {
				via := string(t.Via)
				if via == "" {
					via = "no via"
				}
				ts = append(ts, fmt.Sprintf("%s %s %d", t.Kind, via, t.N))
			}
		}
		if len(ts) > 0 {
			parts = append(parts, fmt.Sprintf("%s %d (%s)", k, c.Relations(k), strings.Join(ts, ", ")))
		}
	}
	return strings.Join(parts, "; ")
}

// zoneOffer is a grouping a zone could take: its title, the boxes it
// holds and where it comes from.
type zoneOffer struct {
	name  string
	units []model.ID
	from  string
}

// zoneOffers are the groupings the overview's bands draw (screen's
// zone.go), read from the state: each stored stack's layer that holds two
// boxes or more, and each folder that holds two boxes or more but not all.
func zoneOffers(units []model.Unit, boxes []model.ID, byID map[model.ID]model.Unit, figs store.Figures) []zoneOffer {
	isBox := map[model.ID]bool{}
	for _, b := range boxes {
		isBox[b] = true
	}
	boxOf := func(id model.ID) model.ID {
		for seen := map[model.ID]bool{}; !isBox[id] && !seen[id]; id = byID[id].Parent {
			seen[id] = true
		}
		return id
	}
	var out []zoneOffer
	for _, s := range figs.Stacks {
		if !s.Known || s.Primary {
			continue
		}
		for i, l := range s.Claims() {
			var ids []model.ID
			for _, u := range l.Units {
				if b := boxOf(u); isBox[b] && !slices.Contains(ids, b) {
					ids = append(ids, b)
				}
			}
			if len(ids) >= 2 {
				out = append(out, zoneOffer{l.Name, ids, fmt.Sprintf("stack %s layer %d", s.Key, i+1)})
			}
		}
	}
	at := map[string][]model.ID{}
	var dirs []string
	for _, b := range boxes {
		dir := path.Dir(model.Home(byID[b]))
		if dir == "." || dir == "/" || dir == "" {
			continue
		}
		if _, ok := at[dir]; !ok {
			dirs = append(dirs, dir)
		}
		at[dir] = append(at[dir], b)
	}
	slices.Sort(dirs)
	for _, d := range dirs {
		if n := len(at[d]); n >= 2 && n < len(boxes) {
			out = append(out, zoneOffer{d + "/", at[d], "folder " + d})
		}
	}
	return out
}

// oneSegment is a top-level directory name an include path can start with.
var oneSegment = regexp.MustCompile(`^[A-Za-z_]\w+$`)
