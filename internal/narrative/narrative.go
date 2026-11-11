// Package narrative reads a module's text as a human would: in sections,
// and for what the code is now. It reports what reads as history and never
// refuses it, because such a pattern can be a version number or a name.
package narrative

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxSection is how many characters of prose a section holds before a
// reader loses its thread. Status reports each section over it, and abbd
// understand refuses a text only when its first section, the summary a
// page shows first, runs over it. A large module, which holds more to
// name, has MaxSectionLarge instead.
const (
	MaxSection      = 2000
	MaxSectionLarge = 3000
)

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

// LineKind is what a line of a text is to a reader.
type LineKind int

const (
	Prose LineKind = iota
	Heading
	// Fence opens or closes a code block.
	Fence
	// Code is a line inside a code block.
	Code
)

// Line is one line of a text. Text is the heading's words for a heading,
// the line without trailing blanks for code, and the trimmed line else.
type Line struct {
	Kind LineKind
	Text string
}

// Lines reads a text as the lint and the blueprint both do. A line
// starting with # is a heading. A line starting with three or more
// backticks or tildes opens a code block, which a line of only as many or
// more of the same closes, or the text's end. After backticks the rest of
// the line holds no backtick, as CommonMark has it, so a line that starts
// with an inline code span stays prose.
func Lines(text string) []Line {
	var out []Line
	fence := ""
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		run := ""
		if t != "" {
			run = t[:len(t)-len(strings.TrimLeft(t, t[:1]))]
		}
		switch {
		case fence != "" && strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "":
			out = append(out, Line{Fence, t})
			fence = ""
		case fence != "":
			out = append(out, Line{Code, strings.TrimRight(line, " \t\r")})
		case strings.HasPrefix(t, "~~~"),
			strings.HasPrefix(t, "```") && !strings.Contains(t[len(run):], "`"):
			fence = run
			out = append(out, Line{Fence, t})
		case strings.HasPrefix(t, "#"):
			out = append(out, Line{Heading, strings.TrimSpace(strings.TrimLeft(t, "#"))})
		default:
			out = append(out, Line{Prose, t})
		}
	}
	return out
}

// Sections splits a text at its headings, in order.
func Sections(text string) []Section {
	var out []Section
	cur := -1
	for _, l := range Lines(text) {
		if l.Kind == Heading {
			out = append(out, Section{Heading: l.Text})
			cur = len(out) - 1
			continue
		}
		t := strings.TrimSpace(l.Text)
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

// Long is each section over max characters.
func Long(text string, max int) []Section {
	var out []Section
	for _, s := range Sections(text) {
		if s.Chars > max {
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

var url = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://\S+`)

// quoted marks the bytes of a line's `code` spans and URLs, which quote
// code, examples and pages rather than tell the module's story, so History
// skips them.
func quoted(line string) []bool {
	marked := make([]bool, len(line))
	for _, m := range url.FindAllStringIndex(line, -1) {
		for j := m[0]; j < m[1]; j++ {
			marked[j] = true
		}
	}
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

// History is each phrase that reads as history in prose, outside `code`
// and URLs, by section, line by line.
func History(text string) []Mark {
	var out []Mark
	section := ""
	for _, l := range Lines(text) {
		if l.Kind == Heading {
			section = l.Text
		}
		if l.Kind != Prose {
			continue
		}
		t := l.Text
		taken := quoted(t)
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
