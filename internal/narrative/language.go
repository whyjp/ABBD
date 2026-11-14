package narrative

import (
	"regexp"
	"strings"
	"unicode"
)

// Threshold is the least share of a text's prose words that must be in
// the language's script for the text to read in it. Prose in Korean names
// English terms freely, so the bar is low; prose in another language
// scores near zero.
const Threshold = 0.3

// Share is the share of the prose words of text written in script, and how
// many prose words there are. A word with any letter of script is in it, so
// an English term with a Korean particle, such as "store는", counts as
// Korean. With no prose words, or no script to judge by, the share is 1.
func Share(text string, script []*unicode.RangeTable) (float64, int) {
	if len(script) == 0 {
		return 1, 0
	}
	in, all := 0, 0
	for _, w := range strings.Fields(ProseText(text)) {
		if !strings.ContainsFunc(w, unicode.IsLetter) {
			continue
		}
		all++
		if strings.ContainsFunc(w, func(r rune) bool { return unicode.IsOneOf(script, r) }) {
			in++
		}
	}
	if all == 0 {
		return 1, 0
	}
	return float64(in) / float64(all), all
}

// ReadsIn says text holds no prose words, or enough of them in script.
func ReadsIn(text string, script []*unicode.RangeTable) bool {
	s, _ := Share(text, script)
	return s >= Threshold
}

// ReadsAsWord is ReadsIn for a word that stands alone, such as a line's
// word or a stage's name. Text with letters but no prose word, a bare
// identifier like a type or a constant, names a thing in no language, so it
// does not read in script.
func ReadsAsWord(text string, script []*unicode.RangeTable) bool {
	if len(script) == 0 {
		return true
	}
	if _, n := Share(text, script); n == 0 {
		return !strings.ContainsFunc(text, unicode.IsLetter)
	}
	return ReadsIn(text, script)
}

var (
	inlineCode = regexp.MustCompile("`[^`]*`")
	quotation  = regexp.MustCompile(`"[^"]*"|“[^”]*”|「[^」]*」|『[^』]*』|«[^»]*»`)
	camelCase  = regexp.MustCompile(`\p{Ll}\p{Lu}`)
)

// ProseText is text without what stays verbatim whatever language it is
// written in: code blocks and headings as Lines reads them, inline code,
// quotes, and words that read as code, paths or IDs.
func ProseText(text string) string {
	var b strings.Builder
	for _, l := range Lines(text) {
		if l.Kind != Prose {
			continue
		}
		line := quotation.ReplaceAllString(inlineCode.ReplaceAllString(l.Text, " "), " ")
		for _, w := range strings.Fields(line) {
			if !codeWord(w) {
				b.WriteString(w)
				b.WriteByte(' ')
			}
		}
	}
	return b.String()
}

// codeWord says a word reads as an identifier, a path or an ID: it holds a
// path or call mark or an underscore, a digit beside Latin letters or none,
// is camelCase, or is in capitals. A count such as "3개" stays prose.
func codeWord(w string) bool {
	w = strings.Trim(w, ".,;:!?'*()[]")
	switch {
	case w == "":
		return true
	case strings.ContainsAny(w, `/\_.:()[]{}<>=$@#`), strings.ContainsFunc(w, unicode.IsDigit) && (latinLetter(w) || !strings.ContainsFunc(w, unicode.IsLetter)), camelCase.MatchString(w):
		return true
	}
	letters := 0
	for _, r := range w {
		if unicode.IsLetter(r) {
			if !unicode.IsUpper(r) {
				return false
			}
			letters++
		}
	}
	return letters > 1
}

func latinLetter(w string) bool {
	return strings.ContainsFunc(w, func(r rune) bool { return unicode.Is(unicode.Latin, r) })
}
