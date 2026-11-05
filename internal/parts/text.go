package parts

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// declaration is one name a file defines at its top level.
type declaration struct {
	name   string
	isType bool
	line   int
}

type source struct {
	path    string
	node    string
	stem    string
	lines   []string
	code    []string
	decls   []declaration
	methods map[string]int
}

var (
	declRe     = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?(?:abstract\s+)?(func|fn|def|class|struct|enum|trait|interface|type|function\*?|const|let|var|static|union|object|record)\s+(?:\([^)]*\)\s*)?([A-Za-z_$][\w$]*)`)
	blockRe    = regexp.MustCompile(`^(?:type|var|const)\s*\($`)
	blockItem  = regexp.MustCompile(`^\s+([A-Za-z_]\w*)`)
	assignRe   = regexp.MustCompile(`^(?:module\.)?(?:exports\.)?([A-Za-z_$][\w$]*)\s*(?::[^=]*)?=[^=>]`)
	goRecv     = regexp.MustCompile(`^func\s*\(\s*\w*\s*\*?\s*([A-Za-z_]\w*)`)
	rustImpl   = regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(?:[\w:]+(?:<[^>]*>)?\s+for\s+)?([A-Za-z_]\w*)`)
	importRe   = regexp.MustCompile(`\b(?:import|from|require|use|mod|include)\b`)
	identRe    = regexp.MustCompile(`[A-Za-z_$][\w$]*`)
	lineNoise  = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|` + "`[^`]*`")
	typeTokens = map[string]bool{"class": true, "struct": true, "enum": true, "trait": true, "interface": true, "type": true, "union": true, "object": true, "record": true}
)

func load(f File) *source {
	s := &source{path: f.Path, methods: map[string]int{}}
	base := path.Base(f.Path)
	s.stem = strings.TrimSuffix(base, path.Ext(base))
	s.lines = strings.Split(strings.ReplaceAll(f.Text, "\r\n", "\n"), "\n")
	s.code = stripComments(s.lines)
	inBlock := false
	for i, l := range s.code {
		switch {
		case inBlock && strings.HasPrefix(l, ")"):
			inBlock = false
		case inBlock:
			if m := blockItem.FindStringSubmatch(l); m != nil && strings.Count(l, "\t")+strings.Count(l, "    ") <= 1 {
				s.decls = append(s.decls, declaration{m[1], strings.HasPrefix(s.code[blockStart(s.code, i)], "type"), i + 1})
			}
		case blockRe.MatchString(strings.TrimSpace(l)) && !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t"):
			inBlock = true
		}
		if inBlock || l == "" || l[0] == ' ' || l[0] == '\t' {
			continue
		}
		if m := goRecv.FindStringSubmatch(l); m != nil {
			// A method is reached through its type, so its own name is no
			// declaration another file could be said to use.
			s.methods[m[1]]++
			continue
		} else if m := rustImpl.FindStringSubmatch(l); m != nil {
			s.methods[m[1]] += 2
			continue
		}
		if m := declRe.FindStringSubmatch(l); m != nil {
			s.decls = append(s.decls, declaration{m[2], typeTokens[m[1]], i + 1})
		} else if m := assignRe.FindStringSubmatch(l); m != nil && !importRe.MatchString(l) {
			s.decls = append(s.decls, declaration{m[1], false, i + 1})
		}
	}
	return s
}

func blockStart(code []string, i int) int {
	for ; i > 0; i-- {
		if blockRe.MatchString(strings.TrimSpace(code[i])) {
			return i
		}
	}
	return 0
}

// stripComments blanks comments and string literals, keeping line numbers,
// so a name in prose or in a message is not a use.
func stripComments(lines []string) []string {
	out := make([]string, len(lines))
	inBlock, inDoc := false, ""
	for i, l := range lines {
		t := l
		if inDoc != "" {
			if j := strings.Index(t, inDoc); j >= 0 {
				t, inDoc = t[j+3:], ""
			} else {
				continue
			}
		}
		if inBlock {
			if j := strings.Index(t, "*/"); j >= 0 {
				t, inBlock = t[j+2:], false
			} else {
				continue
			}
		}
		for _, q := range []string{`"""`, `'''`} {
			if j := strings.Index(t, q); j >= 0 {
				if k := strings.Index(t[j+3:], q); k >= 0 {
					t = t[:j] + t[j+3+k+3:]
				} else {
					t, inDoc = t[:j], q
				}
			}
		}
		if j := strings.Index(t, "/*"); j >= 0 {
			if k := strings.Index(t[j+2:], "*/"); k >= 0 {
				t = t[:j] + t[j+2+k+2:]
			} else {
				t, inBlock = t[:j], true
			}
		}
		t = lineNoise.ReplaceAllString(t, `""`)
		if j := strings.Index(t, "//"); j >= 0 {
			t = t[:j]
		}
		if j := strings.Index(strings.TrimLeft(t, " \t"), "#"); j == 0 {
			t = ""
		} else if j := strings.Index(t, " #"); j >= 0 {
			t = t[:j]
		}
		out[i] = strings.TrimRight(t, " \t")
	}
	return out
}

// linksOf finds, for each ordered pair of files, the names one takes from
// the other: a top-level name only that file declares, a method on a type
// it declares, or an import line that names its stem.
func linksOf(srcs []*source) []Link {
	owner := map[string]int{}
	for i, s := range srcs {
		for _, d := range s.decls {
			if len(d.name) < 3 {
				continue
			}
			if j, ok := owner[d.name]; ok && j != i {
				owner[d.name] = -1
			} else if !ok {
				owner[d.name] = i
			}
		}
	}
	var out []Link
	for i, s := range srcs {
		own := map[string]bool{}
		for _, d := range s.decls {
			own[d.name] = true
		}
		found := map[int]*Link{}
		add := func(j int, name string, line int) {
			l := found[j]
			if l == nil {
				l = &Link{From: s.path, To: srcs[j].path, Line: line, Quote: strings.TrimSpace(s.lines[line-1])}
				found[j] = l
			}
			if !slices.Contains(l.Names, name) {
				l.Names = append(l.Names, name)
			}
		}
		for n, l := range s.code {
			raw := s.lines[n]
			if importRe.MatchString(raw) {
				for j, t := range srcs {
					if j != i && namesStem(raw, t.stem) {
						add(j, t.stem, n+1)
					}
				}
			}
			for _, at := range identRe.FindAllStringIndex(l, -1) {
				// A name after a dot is a member of something else.
				if at[0] > 0 && l[at[0]-1] == '.' {
					continue
				}
				tok := l[at[0]:at[1]]
				if j, ok := owner[tok]; ok && j >= 0 && j != i && !own[tok] {
					add(j, tok, n+1)
				}
			}
		}
		for typ, k := range s.methods {
			if j, ok := owner[typ]; ok && j >= 0 && j != i {
				for range k {
					add(j, typ+"()", lineOfRecv(s, typ))
				}
			}
		}
		for j := range srcs {
			if l := found[j]; l != nil {
				out = append(out, *l)
			}
		}
	}
	return out
}

func lineOfRecv(s *source, typ string) int {
	for i, l := range s.code {
		if m := goRecv.FindStringSubmatch(l); m != nil && m[1] == typ {
			return i + 1
		}
		if m := rustImpl.FindStringSubmatch(l); m != nil && m[1] == typ {
			return i + 1
		}
	}
	return 1
}

// namesStem says whether an import line names a file by its stem, as a path
// or module segment: ./stem, .stem, ::stem, /stem or "mod stem".
func namesStem(line, stem string) bool {
	if stem == "" || stem == "index" || strings.HasPrefix(stem, "__") {
		return false
	}
	for _, p := range []string{"./" + stem, "/" + stem, "." + stem, "::" + stem, "mod " + stem, "{" + stem, " " + stem + ","} {
		for k := 0; ; {
			j := strings.Index(line[k:], p)
			if j < 0 {
				break
			}
			end := k + j + len(p)
			if end == len(line) || !isWord(rune(line[end])) {
				if p[0] != ' ' && p[0] != '{' || strings.Contains(line, "::") || strings.HasPrefix(strings.TrimSpace(line), "use ") {
					return true
				}
			}
			k = end
		}
	}
	return false
}

func isWord(r rune) bool { return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// prefix is a file stem's first word, so context and context_appengine,
// or termui and _termui_impl, read as one family.
func prefix(stem string) string {
	stem = strings.TrimLeft(stem, "_.")
	if i := strings.IndexAny(stem, "_-."); i > 0 {
		return stem[:i]
	}
	return stem
}

// stripBrand drops a mixed-case leading word that three or more of the
// directory's types share, such as a project name like HTTPie, from the part
// names it would otherwise start, so HTTPieCertificate is "certificate"
// rather than "htt pie certificate".
func stripBrand(ps []Part, srcs []*source) {
	count := map[string]int{}
	for _, s := range srcs {
		for _, d := range s.decls {
			if w := lead(d.name); d.isType && w != "" {
				count[w]++
			}
		}
	}
	for i := range ps {
		w := lead(ps[i].Symbol)
		if w != "" && count[w] >= 3 && strings.ToLower(w[1:]) != w[1:] && len(ps[i].Symbol) > len(w) {
			ps[i].Name = words(ps[i].Symbol[len(w):])
		}
	}
}

// lead is a name's first capitalised word when another capital follows it,
// such as HTTPie in HTTPieSession or Gin in GinEngine.
func lead(name string) string {
	r := []rune(name)
	if len(r) < 2 || !unicode.IsUpper(r[0]) {
		return ""
	}
	for i := 1; i < len(r); i++ {
		if unicode.IsUpper(r[i]) && unicode.IsLower(r[i-1]) {
			return string(r[:i])
		}
	}
	return ""
}

// words splits a code name into lower-case words: RouterGroup is
// "router group", HTTPSAdapter "https adapter", outlier_detection
// "outlier detection".
func words(name string) string {
	var out []string
	var cur []rune
	r := []rune(name)
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	for i, c := range r {
		switch {
		case c == '_' || c == '-' || c == '.' || c == '$':
			flush()
			continue
		case unicode.IsUpper(c) && len(cur) > 0:
			prevLower := unicode.IsLower(cur[len(cur)-1]) || unicode.IsDigit(cur[len(cur)-1])
			nextLower := i+1 < len(r) && unicode.IsLower(r[i+1])
			if prevLower || nextLower && unicode.IsUpper(cur[len(cur)-1]) {
				flush()
			}
		}
		cur = append(cur, c)
	}
	flush()
	return strings.Join(out, " ")
}

var (
	goReturns = regexp.MustCompile(`^func\s+[A-Za-z_]\w*\s*(?:\[[^\]]*\])?\(.*\)\s*\*?([A-Za-z_]\w*)\s*\{`)
	rustFor   = regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(?:[\w]+::)*([A-Za-z_]\w*)(?:<[^>]*>)?\s+for\s+`)
	pyBase    = regexp.MustCompile(`^class\s+\w+\s*\(\s*(?:[\w]+\.)*([A-Za-z_]\w*)`)
	jsExtends = regexp.MustCompile(`class\s+[\w$]+\s+extends\s+(?:[\w$]+\.)*([A-Za-z_$][\w$]*)`)
)

// families groups the files that implement, extend or construct one type
// another file of the directory declares, such as every middleware that
// returns the handler type or every exporter that implements one trait.
func families(srcs []*source) map[string][]int {
	owner := map[string]int{}
	for i, s := range srcs {
		for _, d := range s.decls {
			if d.isType {
				if _, ok := owner[d.name]; ok {
					owner[d.name] = -1
				} else {
					owner[d.name] = i
				}
			}
		}
	}
	out := map[string][]int{}
	for i, s := range srcs {
		seen := map[string]bool{}
		for _, l := range s.code {
			for _, re := range []*regexp.Regexp{goReturns, rustFor, pyBase, jsExtends} {
				m := re.FindStringSubmatch(l)
				if m == nil {
					continue
				}
				if j, ok := owner[m[1]]; ok && j >= 0 && j != i && !seen[m[1]] {
					seen[m[1]] = true
					out[m[1]] = append(out[m[1]], i)
				}
			}
		}
	}
	for k, v := range out {
		if len(v) < 2 {
			delete(out, k)
		}
	}
	return out
}

func (s *source) size() int {
	n := 0
	for _, l := range s.code {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}
