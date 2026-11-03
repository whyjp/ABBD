package evidence

import (
	"regexp"
	"slices"
	"strings"
)

// declaring are the words that may stand before a name a line declares, in
// the languages abbd meets most.
var declaring = []string{
	"func", "def", "class", "type", "struct", "enum", "interface", "trait", "impl", "fn", "function",
	"const", "var", "let", "val", "module", "object", "record", "macro", "sub",
}

// prefix is what may stand before a declared name: words, and parenthesised
// groups such as a Go method's receiver.
var prefix = regexp.MustCompile(`^\s*(?:(?:\([^()]*\)|[A-Za-z_]\w*)\s*)*$`)

// Declared finds the lines of the file that hold symbol's code. The
// declaration is the first line naming the symbol after declaring words only,
// such as func, type, def or class and a Go receiver, else the first line
// that opens with the symbol and assigns it, as in a const block. The span
// takes in the comment lines right above it and runs on through the lines
// indented deeper, to a closing brace or end at its own depth. False when the
// file is gone or no line declares the symbol so.
func (c *Checker) Declared(path, symbol string) (first, last int, ok bool) {
	f := c.load(path)
	if f == nil || strings.TrimSpace(symbol) == "" {
		return 0, 0, false
	}
	name := regexp.MustCompile(`(^|[^\w])` + regexp.QuoteMeta(symbol) + `($|[^\w])`)
	assigned := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(symbol) + `(\s+[\w.\[\]*]+)?\s*:?=`)
	at := -1
	for i, l := range f.raw {
		m := name.FindStringSubmatchIndex(l)
		if m == nil {
			continue
		}
		before := l[:m[3]]
		if prefix.MatchString(before) && slices.ContainsFunc(strings.FieldsFunc(before, func(r rune) bool { return r == ' ' || r == '\t' || r == '(' || r == ')' }), func(w string) bool { return slices.Contains(declaring, w) }) {
			at = i
			break
		}
	}
	if at < 0 {
		at = slices.IndexFunc(f.raw, assigned.MatchString)
	}
	if at < 0 {
		return 0, 0, false
	}
	depth := indent(f.raw[at])
	first, last = at, at
	for first > 0 && comment(f.raw[first-1]) {
		first--
	}
	for i := at + 1; i < len(f.raw); i++ {
		l := strings.TrimSpace(f.raw[i])
		if l == "" {
			continue
		}
		d := indent(f.raw[i])
		closes := l[0] == '}' || l == "end" || strings.HasPrefix(l, "end ")
		if d < depth || d == depth && !closes && !strings.ContainsAny(l[:1], ")]{") {
			break
		}
		last = i
		if d == depth && closes {
			break
		}
	}
	return first + 1, last + 1, true
}

func indent(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }

func comment(l string) bool {
	l = strings.TrimSpace(l)
	for _, p := range []string{"//", "#", "/*", "*", "--", ";", "@"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}
