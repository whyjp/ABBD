// Package narrative reads a module's text as a human would: in sections,
// and for what the code is now. It reports and never refuses, because a
// pattern that reads as history can be a version number or a name.
package narrative

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxSection is how many characters of prose a section holds before a
// reader loses its thread. It is a report threshold, never a gate.
const MaxSection = 2000

// Section is the prose under one heading, up to the next heading of any
// level. Heading is empty for prose before the first heading. Chars counts
// the characters of its non-blank lines, without the heading.
type Section struct {
	Heading string
	Chars   int
}

// Mark is a phrase in a section that reads as history: Kind says what it
// reads as, Match is the phrase.
type Mark struct {
	Section string
	Kind    string
	Match   string
}

// heading is how the blueprint tells a heading: a line starting with #.
func heading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(line, "#")), true
}

// Sections splits a text at its headings, in order.
func Sections(text string) []Section {
	var out []Section
	cur := -1
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if h, ok := heading(t); ok {
			out = append(out, Section{Heading: h})
			cur = len(out) - 1
			continue
		}
		if t == "" {
			continue
		}
		if cur < 0 {
			out = append(out, Section{})
			cur = 0
		}
		out[cur].Chars += utf8.RuneCountInString(t)
	}
	return out
}

// Long is each section over MaxSection.
func Long(text string) []Section {
	var out []Section
	for _, s := range Sections(text) {
		if s.Chars > MaxSection {
			out = append(out, s)
		}
	}
	return out
}

const (
	date    = "a date"
	ref     = "an issue or PR number"
	history = "a history tag"
)

var months = `(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|June?|July?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)`

// patterns are what reads as history, the more specific first: a phrase
// one pattern reports, no later one reports again. The first group, when
// there is one, is the phrase. Version numbers, identifiers, "no longer"
// and "old format" describe code as it is, so nothing here matches them.
var patterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{date, regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)},
	{date, regexp.MustCompile(`\b` + months + `\.? (?:\d{1,2}, )?\d{4}\b`)},
	{history, regexp.MustCompile(`(?i)\b(?:migrated from|formerly|used to be)\b`)},
	{history, regexp.MustCompile(`(?i)\b(?:added|introduced) in (?:v?\d+(?:\.\d+)+|v\d+|PR #?\d+|#\d+)\b`)},
	{ref, regexp.MustCompile(`(?i)\b(?:PRs?|issues?|pull requests?) #?\d+\b`)},
	{ref, regexp.MustCompile(`\b[\w.-]+/[\w.-]+#\d+\b`)},
	{ref, regexp.MustCompile(`(?:^|[^\w&#/])(#\d+)\b`)},
}

// code marks the bytes of a line's `code` spans, which quote code and
// examples rather than tell the module's story, so History skips them.
func code(line string) []bool {
	marked := make([]bool, len(line))
	open := -1
	for i := range len(line) {
		if line[i] != '`' {
			continue
		}
		if open < 0 {
			open = i
			continue
		}
		for j := open; j <= i; j++ {
			marked[j] = true
		}
		open = -1
	}
	return marked
}

// History is each phrase that reads as history outside `code`, by section,
// line by line.
func History(text string) []Mark {
	var out []Mark
	section := ""
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if h, ok := heading(t); ok {
			section = h
			continue
		}
		taken := code(t)
		for _, p := range patterns {
			for _, m := range p.re.FindAllStringSubmatchIndex(t, -1) {
				from, to := m[0], m[1]
				if len(m) > 2 {
					from, to = m[2], m[3]
				}
				if slices.Contains(taken[from:to], true) {
					continue
				}
				for i := from; i < to; i++ {
					taken[i] = true
				}
				out = append(out, Mark{Section: section, Kind: p.kind, Match: t[from:to]})
			}
		}
	}
	return out
}
