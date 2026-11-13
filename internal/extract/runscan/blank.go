package runscan

// blank returns the code with its comments and the contents of its string
// and character literals turned to spaces, newlines kept, so a pattern
// never matches text the program does not run and every offset still
// points at the same line.
func blank(src []byte, lang Lang) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	clear := func(i, j int) {
		for k := i; k < j && k < len(out); k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '/' && i+1 < n && src[i+1] == '/':
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			clear(i, j)
			i = j
		case c == '/' && i+1 < n && src[i+1] == '*':
			depth, j := 1, i+2
			for j < n && depth > 0 {
				switch {
				case lang == Rust && src[j] == '/' && j+1 < n && src[j+1] == '*':
					depth, j = depth+1, j+2
				case src[j] == '*' && j+1 < n && src[j+1] == '/':
					depth, j = depth-1, j+2
				default:
					j++
				}
			}
			clear(i, j)
			i = j
		case lang == Rust && (c == 'r' || c == 'b' && i+1 < n && src[i+1] == 'r') && (i == 0 || !isWord(src[i-1])) && rawStart(src, i) > 0:
			i = rawString(src, i, clear)
		case c == '"':
			j := i + 1
			for j < n && src[j] != '"' && (lang == Rust || src[j] != '\n') {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			clear(i+1, j)
			i = j + 1
		case lang == Go && c == '`':
			j := i + 1
			for j < n && src[j] != '`' {
				j++
			}
			clear(i+1, j)
			i = j + 1
		case c == '\'':
			i = charLiteral(src, i, lang, clear)
		default:
			i++
		}
	}
	return out
}

// rawStart is the offset of the opening quote of a Rust raw string that
// starts at i, such as r"..", r#".."# or br"..", or 0 when none does.
func rawStart(src []byte, i int) int {
	if src[i] == 'b' {
		i++
	}
	j := i + 1
	for j < len(src) && src[j] == '#' {
		j++
	}
	if j < len(src) && src[j] == '"' {
		return j
	}
	return 0
}

func rawString(src []byte, i int, clear func(int, int)) int {
	q := rawStart(src, i)
	hashes := q - i - 1
	if src[i] == 'b' {
		hashes--
	}
	for j := q + 1; j < len(src); j++ {
		if src[j] != '"' {
			continue
		}
		k := j + 1
		for k < len(src) && k-j-1 < hashes && src[k] == '#' {
			k++
		}
		if k-j-1 == hashes {
			clear(q+1, j)
			return k
		}
	}
	clear(q+1, len(src))
	return len(src)
}

// charLiteral blanks a character literal at i and returns where scanning
// goes on. In Rust a quote that no literal closes starts a lifetime.
func charLiteral(src []byte, i int, lang Lang, clear func(int, int)) int {
	n := len(src)
	j := i + 1
	if j < n && src[j] == '\\' {
		j += 2
		for j < n && j < i+12 && src[j] != '\'' && src[j] != '\n' {
			j++
		}
	} else {
		for j < n && j < i+5 && src[j] != '\'' && src[j] != '\n' {
			j++
		}
		if lang == Rust && j > i+2 && src[i+1] < 0x80 {
			return i + 1
		}
	}
	if j < n && src[j] == '\'' {
		clear(i+1, j)
		return j + 1
	}
	return i + 1
}
