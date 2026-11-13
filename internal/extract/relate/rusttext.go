package relate

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/whyjp/abbd/internal/extract/lsp"
)

// span is what a byte of Rust source sits in, as far as reading a
// reference needs: code, an attribute, a private use declaration, a pub use
// that re-exports, or text the compiler never resolves (a comment, a
// literal, or code cfg leaves out), where a name uses nothing.
type span byte

const (
	inCode span = iota
	inAttr
	inUse
	inExport
	inText
)

// rustSource is one file, split so a server's line and byte column find
// what the reference sits in.
type rustSource struct {
	lines []string
	// starts is the byte offset each line starts at.
	starts []int
	spans  []span
	// exports are where each pub use or pub extern crate names the crate
	// or module it re-exports from: its first path segment.
	exports []lsp.Position
}

func readRust(src []byte) rustSource {
	spans, exports := scanRust(src)
	s := rustSource{spans: spans, starts: []int{0}}
	for i, b := range src {
		if b == '\n' {
			s.starts = append(s.starts, i+1)
		}
	}
	for i, st := range s.starts {
		end := len(src)
		if i+1 < len(s.starts) {
			end = s.starts[i+1]
		}
		s.lines = append(s.lines, string(src[st:end]))
	}
	for _, at := range exports {
		line, _ := slices.BinarySearch(s.starts, at+1)
		s.exports = append(s.exports, lsp.Position{Line: line - 1, Character: at - s.starts[line-1]})
	}
	return s
}

// text is the source a single-line range covers.
func (s rustSource) text(r lsp.Range) string {
	if r.Start.Line != r.End.Line || r.Start.Line < 0 || r.Start.Line >= len(s.lines) {
		return ""
	}
	l := s.lines[r.Start.Line]
	if r.Start.Character < 0 || r.End.Character > len(l) || r.Start.Character > r.End.Character {
		return ""
	}
	return l[r.Start.Character:r.End.Character]
}

// bang is whether a ! follows the position, past spaces, as a macro
// invocation's name has.
func (s rustSource) bang(p lsp.Position) bool {
	if p.Line < 0 || p.Line >= len(s.lines) || p.Character > len(s.lines[p.Line]) {
		return false
	}
	return strings.HasPrefix(strings.TrimLeft(s.lines[p.Line][p.Character:], " "), "!")
}

// hide marks a range, such as code cfg leaves out, as text, with the
// re-exports in it.
func (s *rustSource) hide(r lsp.Range) {
	off := func(p lsp.Position) int {
		if p.Line >= len(s.starts) {
			return len(s.spans)
		}
		return min(max(s.starts[max(p.Line, 0)]+p.Character, 0), len(s.spans))
	}
	from, to := off(r.Start), off(r.End)
	for i := from; i < to; i++ {
		s.spans[i] = inText
	}
	s.exports = slices.DeleteFunc(s.exports, func(p lsp.Position) bool {
		at := off(p)
		return at >= from && at < to
	})
}

// at is the span the byte at line and column sits in.
func (s rustSource) at(line, col int) span {
	if line < 0 || line >= len(s.starts) {
		return inText
	}
	i := s.starts[line] + col
	if i < 0 || i >= len(s.spans) {
		return inText
	}
	return s.spans[i]
}

// scanRust marks every byte of src, and returns the byte offset of each
// re-export's first path segment. It lexes only as far as telling code from
// comments and literals, and finds each use declaration or extern crate,
// which runs from its keyword to its semicolon.
func scanRust(src []byte) ([]span, []int) {
	var exports []int
	out := make([]span, len(src))
	mark := func(from, to int, k span) {
		for i := from; i < to && i < len(out); i++ {
			out[i] = k
		}
	}
	use, depth, kind := -1, 0, inUse
	attr, nest := -1, 0
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '/' && at(src, i+1) == '/':
			j := i
			for j < len(src) && src[j] != '\n' {
				j++
			}
			mark(i, j, inText)
			i = j
		case c == '/' && at(src, i+1) == '*':
			j, d := i+2, 1
			for j < len(src) && d > 0 {
				switch {
				case src[j] == '/' && at(src, j+1) == '*':
					d++
					j += 2
				case src[j] == '*' && at(src, j+1) == '/':
					d--
					j += 2
				default:
					j++
				}
			}
			mark(i, j, inText)
			i = j
		case c == '"' || (c == 'b' || c == 'c') && at(src, i+1) == '"':
			j := i + 1
			if c != '"' {
				j++
			}
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			mark(i, j+1, inText)
			i = j + 1
		case rawString(src, i) > 0:
			j := i + rawString(src, i)
			mark(i, j, inText)
			i = j
		case c == '\'':
			j := charLiteral(src, i)
			if j > i {
				mark(i, j, inText)
				i = j
				continue
			}
			i++
			for i < len(src) && ident(src[i]) {
				i++
			}
		case identStart(c):
			j := i
			for j < len(src) && ident(src[j]) {
				j++
			}
			raw := c == 'r' && j == i+1 && at(src, j) == '#'
			if raw {
				j += 2
				for j < len(src) && ident(src[j]) {
					j++
				}
			}
			word := string(src[i:j])
			if !raw && use < 0 && (word == "use" && !precise(src, j) || word == "extern" && next(src, j) == "crate") {
				use, depth, kind = i, 0, inUse
				if exported(src, i) {
					kind = inExport
					k := j
					if word == "extern" {
						k = skip(src, j) + len("crate")
					}
					if k = skip(src, k); identStart(at(src, k)) {
						if n := next(src, k); n != "crate" && n != "self" && n != "super" {
							exports = append(exports, k)
						}
					}
				}
			}
			i = j
		case c == '#' && attr < 0 && (at(src, i+1) == '[' || at(src, i+1) == '!' && at(src, i+2) == '['):
			attr, nest = i, 0
			i++
		default:
			if attr >= 0 {
				switch c {
				case '[':
					nest++
				case ']':
					nest--
					if nest == 0 {
						for k := attr; k <= i; k++ {
							if out[k] == inCode {
								out[k] = inAttr
							}
						}
						attr = -1
					}
				}
			}
			if use >= 0 {
				switch c {
				case '{':
					depth++
				case '}':
					depth--
				case ';':
					if depth <= 0 {
						for k := use; k <= i; k++ {
							if out[k] == inCode {
								out[k] = kind
							}
						}
						use = -1
					}
				}
			}
			i++
		}
	}
	return out, exports
}

// skip is the first offset at or after j that is no space.
func skip(src []byte, j int) int {
	for j < len(src) && space(src[j]) {
		j++
	}
	return j
}

// next is the identifier that starts at or after j, past spaces.
func next(src []byte, j int) string {
	j = skip(src, j)
	k := j
	for k < len(src) && ident(src[k]) {
		k++
	}
	return string(src[j:k])
}

func at(src []byte, i int) byte {
	if i < len(src) {
		return src[i]
	}
	return 0
}

func identStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
func ident(c byte) bool { return identStart(c) || c >= '0' && c <= '9' }

// exported is whether the use keyword at i follows a pub visibility, such
// as pub or pub(crate), so the declaration re-exports what it names.
func exported(src []byte, i int) bool {
	j := i - 1
	for j >= 0 && space(src[j]) {
		j--
	}
	if j >= 0 && src[j] == ')' {
		for j >= 0 && src[j] != '(' {
			j--
		}
		j--
		for j >= 0 && space(src[j]) {
			j--
		}
	}
	return j >= 2 && string(src[j-2:j+1]) == "pub" && (j < 3 || !ident(src[j-3]))
}

func space(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// precise is whether the use keyword ending at j is a precise-capturing
// bound, as in impl Trait + use<'a>, rather than a declaration.
func precise(src []byte, j int) bool {
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	return at(src, j) == '<'
}

// rawString is the length of the raw string literal at i, such as
// r#"..."# or br"...", or 0.
func rawString(src []byte, i int) int {
	j := i
	if at(src, j) == 'b' || at(src, j) == 'c' {
		j++
	}
	if at(src, j) != 'r' {
		return 0
	}
	j++
	hashes := 0
	for at(src, j) == '#' {
		hashes++
		j++
	}
	if at(src, j) != '"' {
		return 0
	}
	for j++; j < len(src); j++ {
		if src[j] != '"' {
			continue
		}
		k := 0
		for k < hashes && at(src, j+1+k) == '#' {
			k++
		}
		if k == hashes {
			return j + 1 + hashes - i
		}
	}
	return len(src) - i
}

// charLiteral is the end of the char literal that opens at i, or i for a
// lifetime or a label.
func charLiteral(src []byte, i int) int {
	if at(src, i+1) == '\\' {
		j := i + 2
		for j < len(src) && src[j] != '\'' && src[j] != '\n' {
			j++
		}
		return j + 1
	}
	_, n := utf8.DecodeRune(src[min(i+1, len(src)):])
	if at(src, i+1+n) == '\'' {
		return i + 2 + n
	}
	return i
}
