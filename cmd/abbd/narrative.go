package main

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/whyjp/abbd/internal/narrative"
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
