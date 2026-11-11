package main

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/narrative"
	"github.com/whyjp/abbd/internal/topology"
)

// readText prints what makes a module's text hard to read: each section
// over narrative.MaxSection and each phrase that reads as history. It is a
// report for the agent who writes the text, so it never refuses one and
// never fails a check.
func readText(w io.Writer, id, text string) (long, history int) {
	for _, s := range narrative.Long(text) {
		fmt.Fprintf(w, "long: %s %s holds %d characters, over %d; split it under ### headings or into shorter sections\n", id, section(s.Heading), s.Chars, narrative.MaxSection)
		long++
	}
	for _, m := range narrative.History(text) {
		fmt.Fprintf(w, "history: %s %s says %q, which reads as %s; say what the code is now and leave its history to git\n", id, section(m.Section), m.Match, m.Kind)
		history++
	}
	return long, history
}

// textSections are the sections a module's text holds. Order, failure and
// where things sit belong to flows, slots and the drawing.
var textSections = []string{"Overview", "Key Concepts"}

// elsewhere is where a section a module's text does not hold goes instead.
var elsewhere = map[string]string{
	"how it works":      "an ordered flow, abbd flow KEY FILE",
	"where things live": "the drawing, which shows it already",
	"gotchas":           "a slot note, abbd understand ID.failure FILE --evidence PATH:LINES:QUOTE",
	"read first":        "nowhere: it is abbd brief's reading list, so delete it",
}

// unfinished refuses a module's text that holds a section other than
// textSections, or a TODO line abbd brief left in it.
func unfinished(text string) error {
	raw := strings.Split(text, "\n")
	for i, l := range narrative.Lines(text) {
		t := strings.TrimSpace(raw[i])
		switch {
		case l.Kind == narrative.Heading && len(t)-len(strings.TrimLeft(t, "#")) <= 2 && !slices.ContainsFunc(textSections, func(s string) bool { return strings.EqualFold(s, l.Text) }):
			where, ok := elsewhere[strings.ToLower(l.Text)]
			if !ok {
				where = "a ### heading inside Overview or Key Concepts"
			}
			return fmt.Errorf("a module's text holds only ## Overview and ## Key Concepts; %q goes to %s", l.Text, where)
		case l.Kind == narrative.Prose && todo.MatchString(l.Text):
			return fmt.Errorf("line %d still says %q; write what abbd brief left as TODO, or delete the line", i+1, l.Text)
		}
	}
	return nil
}

// checkText reports where a module's text falls short of the reader's
// contract: an inside unit no line names, and a path in a code span that is
// no code the text is stamped against. Like readText it never refuses.
// keys adds the key units by fan-in, which cost a repository search, so
// only storing a text checks them and status, run in CI, does not.
func (e env) checkText(w io.Writer, units []model.Unit, id model.ID, text string, keys bool) {
	lower := strings.ToLower(text)
	for _, r := range topology.Explain(units, id).Inside {
		if !strings.Contains(lower, strings.ToLower(r.Name)) {
			fmt.Fprintf(w, "unmentioned: %s's text never names %s %s, which sits inside it; name it in a Key Concepts bullet, with the units it works with\n", id, r.ID, r.Name)
		}
	}
	var key []keyUnit
	if keys {
		key = e.keyUnitsOf(units, id, keyShown)
	}
	for _, k := range unnamed(text, key) {
		fmt.Fprintf(w, "unmentioned: %s's text never names %s, a key unit of %s named %d times elsewhere; name it, or say in one line why it stays out\n", id, k.Name, k.File, k.Uses)
	}
	files, quotes := e.st.Read(units, id)
	for _, q := range quotes {
		files = append(files, q.Path)
	}
	for _, p := range codePaths(text) {
		if slices.ContainsFunc(files, func(f string) bool {
			return f == p || strings.HasPrefix(f, p+"/") || strings.HasSuffix(f, "/"+p)
		}) {
			continue
		}
		if o, ok := holder(units, e.root, p); ok {
			fmt.Fprintf(w, "names: %s's text names %s, code of %s %s that the text is not stamped against; name %s instead, or drop the path\n", id, p, o.ID, o.Name.Value, o.Name.Value)
		}
	}
}

// holder is the unit whose anchor holds path p most closely, when p is a
// file or directory under that anchor and not itself a unit's name or a
// top-level unit's directory: naming a neighbour module is how a text names
// what it uses, and naming its files is a claim about code the text was
// not stamped against.
func holder(units []model.Unit, root, p string) (model.Unit, bool) {
	if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil && fi.IsDir() {
		for _, u := range model.Present(units) {
			if u.Parent == "" && slices.ContainsFunc(u.Anchors, func(a model.Anchor) bool { return a.Path == p }) {
				return model.Unit{}, false
			}
		}
	}
	var best model.Unit
	depth := -1
	for _, u := range model.Present(units) {
		if u.Name.Value == p {
			return model.Unit{}, false
		}
		for _, a := range u.Anchors {
			if (a.Path == p || strings.HasPrefix(p, a.Path+"/")) && (len(a.Path) > depth || len(a.Path) == depth && u.Kind.Holds() && !best.Kind.Holds()) {
				best, depth = u, len(a.Path)
			}
		}
	}
	return best, depth >= 0
}

// todo is a line abbd brief left: TODO first, after a list marker if any.
var todo = regexp.MustCompile(`^(?:[-*+]\s+)?TODO\b`)

var codeSpan = regexp.MustCompile("`([^`]+)`")

// codePaths are the code spans of a text's prose that read as a path: they
// hold a slash and no space, scheme or wildcard. A trailing :LINE goes.
func codePaths(text string) []string {
	var out []string
	for _, l := range narrative.Lines(text) {
		if l.Kind != narrative.Prose {
			continue
		}
		for _, m := range codeSpan.FindAllStringSubmatch(l.Text, -1) {
			p := m[1]
			if !strings.Contains(p, "/") || strings.ContainsAny(p, " \t*<>{}()") || strings.Contains(p, "://") {
				continue
			}
			p, _, _ = strings.Cut(p, ":")
			p = strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/")
			if p != "" && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func section(heading string) string {
	if heading == "" {
		return "before its first heading"
	}
	return fmt.Sprintf("%q", heading)
}

// readTexts reports every text, by the module or the note it is, after a
// summary line.
func readTexts(w io.Writer, texts map[string]string) {
	var lines bytes.Buffer
	var long, history int
	for _, id := range slices.Sorted(maps.Keys(texts)) {
		l, h := readText(&lines, id, texts[id])
		long, history = long+l, history+h
	}
	fmt.Fprintf(w, "readability: %d section(s) over %d characters, %d phrase(s) that read as history; a report, the check ignores it\n", long, narrative.MaxSection, history)
	lines.WriteTo(w)
}
