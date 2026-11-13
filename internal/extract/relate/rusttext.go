package relate

import (
	"unicode/utf8"
)

// span is what a byte of Rust source sits in, as far as reading a
// reference needs: code, a use declaration, or text the compiler never
// resolves (a comment or a literal), where a doc link names a symbol
// without using it.
type span byte

const (
	inCode span = iota
	inUse
	inText
)

// rustSource is one file, split so a server's line and byte column find
// what the reference sits in.
type rustSource struct {
	lines []string
	// starts is the byte offset each line starts at.
	starts []int
	spans  []span
}

func readRust(src []byte) rustSource {
	s := rustSource{spans: scanRust(src), starts: []int{0}}
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
	return s
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

// scanRust marks every byte of src. It lexes only as far as telling code
// from comments and literals, and finds each use declaration, which runs
// from the use keyword to its semicolon.
func scanRust(src []byte) []span {
	out := make([]span, len(src))
	mark := func(from, to int, k span) {
		for i := from; i < to && i < len(out); i++ {
			out[i] = k
		}
	}
	use, depth := -1, 0
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
			if !raw && use < 0 && string(src[i:j]) == "use" && !precise(src, j) {
				use, depth = i, 0
			}
			i = j
		default:
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
								out[k] = inUse
							}
						}
						use = -1
					}
				}
			}
			i++
		}
	}
	return out
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
