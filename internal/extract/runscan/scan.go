package runscan

import (
	"bytes"
	"cmp"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/whyjp/abbd/internal/gitx"
)

var langs = map[string]Lang{
	".c": C, ".h": C, ".cc": C, ".cpp": C, ".cxx": C, ".hpp": C, ".hh": C, ".hxx": C, ".ipp": C,
	".rs": Rust,
	".go": Go,
}

// testDirs are the directory names whose code only tests run.
var testDirs = map[string]bool{
	"test": true, "tests": true, "testing": true, "testdata": true,
	"test_util": true, "test_utils": true, "test-utils": true,
	"bench": true, "benches": true, "fuzz": true,
}

// LangOf is the catalog language of a file, or "" for one the catalog does
// not read: another language, or code only tests run. A Rust file named
// tests.rs or test.rs is the tests module its parent declares.
func LangOf(p string) Lang {
	p = filepath.ToSlash(p)
	dirs := strings.Split(path.Dir(p), "/")
	if slices.ContainsFunc(dirs, func(d string) bool {
		return testDirs[d] || strings.HasSuffix(d, "_tests") || strings.HasSuffix(d, "-tests")
	}) {
		return ""
	}
	base := path.Base(p)
	if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_test.rs") || strings.HasSuffix(base, "_tests.rs") || base == "tests.rs" || base == "test.rs" {
		return ""
	}
	return langs[strings.ToLower(path.Ext(base))]
}

// Scan reads every file under root the catalog knows and returns its
// records, sorted. In a git work tree it reads the files git lists, tracked
// or not but not ignored; elsewhere it walks the tree and skips dot
// directories. It reads the code twice: first for the project's own spawn
// wrappers and aliases, then for the sites, so a call through a wrapper
// defined in another file counts.
func Scan(root string) ([]Record, error) {
	files := gitx.WorkingFiles(root)
	if files == nil {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				rel, _ := filepath.Rel(root, p)
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var fs []file
	for _, p := range files {
		p = filepath.ToSlash(p)
		if LangOf(p) == "" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		fs = append(fs, newFile(p, src))
	}
	return scanFiles(fs), nil
}

// ScanFile returns the records of one file, its path given with slashes,
// with only the wrappers and aliases that file defines.
func ScanFile(p string, src []byte) []Record {
	if LangOf(p) == "" {
		return nil
	}
	return scanFiles([]file{newFile(p, src)})
}

func scanFiles(fs []file) []Record {
	slices.SortFunc(fs, func(a, b file) int { return cmp.Compare(a.path, b.path) })
	ix := newIndex(fs)
	var out []Record
	for _, f := range fs {
		out = append(out, f.records(ix)...)
	}
	for i := range out {
		out[i].To.Def = ix.def(LangOf(out[i].Evidence.Path), out[i].To.Name)
	}
	Sort(out)
	return out
}

// Sort orders records by site, then kind, API, channel and function, so
// the same code always prints the same lines.
func Sort(rs []Record) {
	slices.SortFunc(rs, func(a, b Record) int {
		return cmp.Or(
			cmp.Compare(a.Evidence.Path, b.Evidence.Path),
			cmp.Compare(a.Evidence.Line, b.Evidence.Line),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.API, b.API),
			cmp.Compare(a.Channel, b.Channel),
			cmp.Compare(a.To.Name, b.To.Name),
		)
	})
}

func newFile(p string, src []byte) file {
	lang := LangOf(p)
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	code := blank(src, lang)
	f := file{path: p, lang: lang, src: src, code: code, lower: asciiLower(code), starts: lineStarts(src)}
	f.skip = f.skipped(lang)
	f.defs = f.findDefs()
	return f
}

// hit is one catalog match: where its site starts, where the pattern
// ends, and the call's arguments when it opens one.
type hit struct {
	e       Entry
	at, end int
	args    []string
	opened  bool
	group   func(string) string
}

// hits runs the catalog's entries of the file's language over the code.
// A hit on a line the scan skips is kept only when keepSkipped is set,
// as the wrapper search reads macro bodies.
func (f file) hits(keepSkipped bool) []hit {
	var out []hit
	seen := map[int]bool{}
	for _, e := range Catalog {
		if e.Lang != f.lang {
			continue
		}
		for _, m := range f.matches(e, f.lower) {
			if h, ok := f.hitAt(e, m, keepSkipped); ok && !seen[h.at] {
				seen[h.at] = true
				out = append(out, h)
			}
		}
	}
	return out
}

// hitAt turns one match of e, its offsets in code, into a hit, or reports
// false for a match that is no site: skipped, a declaration, or a call
// that needs arguments and has none.
func (f file) hitAt(e Entry, m []int, keepSkipped bool) (hit, bool) {
	code := f.code
	at := m[0]
	if i := e.Pattern.SubexpIndex("at"); i > 0 && m[2*i] >= 0 {
		at = m[2*i]
	} else {
		for at < m[1] && isSpace(code[at]) {
			at++
		}
	}
	if (f.skip(at) && !e.Directive && !keepSkipped) || (e.Call && declares(code, at)) {
		return hit{}, false
	}
	args, opened := callArgs(code, m[1])
	if e.Args && (!opened || len(args) == 0) {
		return hit{}, false
	}
	group := func(name string) string {
		if i := e.Pattern.SubexpIndex(name); i > 0 && m[2*i] >= 0 && m[2*i+1] <= len(code) {
			return string(code[m[2*i]:m[2*i+1]])
		}
		return ""
	}
	return hit{e: e, at: at, end: m[1], args: args, opened: opened, group: group}, true
}

// records returns the file's sites: each catalog hit, each call through
// a wrapper the index holds, and each catalog call spelled by an alias.
func (f file) records(ix *index) []Record {
	alias := map[string]string{}
	if f.lang == Rust {
		for _, m := range rustChannel.FindAllSubmatch(f.code, -1) {
			pair := string(m[1]) + "/" + string(m[2])
			alias[string(m[1])], alias[string(m[2])] = pair, pair
		}
	}
	seen := map[int]bool{}
	var out []Record
	add := func(r Record, at int) {
		if seen[at] {
			return
		}
		seen[at] = true
		if a, ok := alias[r.Channel]; ok {
			r.Channel = a
		}
		r.Evidence = f.site(at)
		out = append(out, r)
	}
	for _, h := range f.hits(false) {
		add(h.record(), h.at)
	}
	spawned := map[int]bool{}
	for _, r := range out {
		if r.Kind == Spawn {
			spawned[r.Evidence.Line] = true
		}
	}
	for _, c := range ix.wrapperCalls(f) {
		if !spawned[f.line(c.at)] {
			add(c.r, c.at)
		}
	}
	for _, c := range ix.aliasCalls(f) {
		add(c.r, c.at)
	}
	if f.lang == Go {
		out = append(out, f.goArrows(f.skip, seen)...)
	}
	Sort(out)
	return out
}

func (h hit) record() Record {
	r := Record{Kind: h.e.Kind, API: cmp.Or(h.group("api"), h.e.API), Confidence: "syntactic", Via: "catalog", Configs: []string{}}
	r.To.Name = cmp.Or(name(h.group("fn")), name(nth(h.args, h.e.FnArg)))
	r.Channel = cmp.Or(name(h.group("ch")), name(nth(h.args, h.e.ChanArg)))
	return r
}

type file struct {
	path             string
	lang             Lang
	src, code, lower []byte
	starts           []int
	skip             func(int) bool
	defs             []def
}

func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (f file) line(at int) int {
	i, found := slices.BinarySearch(f.starts, at)
	if found {
		return i + 1
	}
	return i
}

func (f file) site(at int) Site {
	n := f.line(at)
	end := len(f.src)
	if n < len(f.starts) {
		end = f.starts[n] - 1
	}
	return Site{Path: f.path, Line: n, Quote: strings.TrimSpace(string(f.src[f.starts[n-1]:end]))}
}

// skipped reports the offsets the catalog does not read: a C preprocessor
// line, whose call is a macro's body and not a site, and a Rust item that
// compiles only for tests.
func (f file) skipped(lang Lang) func(int) bool {
	var spans [][2]int
	switch lang {
	case C:
		for n := 0; n < len(f.starts); n++ {
			text := f.lineText(n)
			if !strings.HasPrefix(strings.TrimSpace(text), "#") {
				continue
			}
			first := n
			for strings.HasSuffix(strings.TrimRight(f.lineText(n), " \t"), "\\") && n+1 < len(f.starts) {
				n++
			}
			spans = append(spans, [2]int{f.starts[first], f.starts[n] + len(f.lineText(n))})
		}
	case Rust:
		for _, m := range rustTest.FindAllIndex(f.code, -1) {
			if bytes.Contains(f.code[m[0]:m[1]], []byte("not(")) {
				continue
			}
			spans = append(spans, [2]int{m[0], itemEnd(f.code, m[1])})
		}
	}
	return func(at int) bool {
		return slices.ContainsFunc(spans, func(s [2]int) bool { return at >= s[0] && at < s[1] })
	}
}

func (f file) lineText(n int) string {
	end := len(f.code)
	if n+1 < len(f.starts) {
		end = f.starts[n+1] - 1
	}
	return string(f.code[f.starts[n]:end])
}

// itemEnd is where the item that starts at i ends: its closing brace, or
// its semicolon when it has no body.
func itemEnd(code []byte, i int) int {
	for ; i < len(code); i++ {
		switch code[i] {
		case ';':
			return i + 1
		case '{':
			depth := 0
			for j := i; j < len(code); j++ {
				switch code[j] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						return j + 1
					}
				}
			}
			return len(code)
		}
	}
	return len(code)
}

// callKeywords may stand right before a call without making it a
// declaration.
var callKeywords = map[string]bool{
	"return": true, "else": true, "case": true, "do": true, "if": true, "while": true, "for": true,
	"match": true, "in": true, "await": true, "move": true, "yield": true, "unsafe": true, "async": true,
	"loop": true, "throw": true, "co_await": true, "defer": true, "go": true, "sizeof": true, "not": true,
	"and": true, "or": true, "then": true,
}

// declares reports whether the name at i reads as declared rather than
// called: a type or a keyword such as fn stands right before it. A
// word on a preprocessor line, such as #endif or #ifdef X, declares nothing.
func declares(code []byte, i int) bool {
	j := i
	for j > 0 && (isSpace(code[j-1]) || code[j-1] == '*' || code[j-1] == '&') {
		j--
	}
	k := j
	for k > 0 && isWord(code[k-1]) {
		k--
	}
	line := bytes.LastIndexByte(code[:k], '\n') + 1
	directive := strings.HasPrefix(strings.TrimLeft(string(code[line:k]), " \t"), "#")
	return k < j && !callKeywords[string(code[k:j])] && !directive
}

// callArgs reads the arguments of the call whose opening bracket ends just
// before i. opened is false when no bracket does.
func callArgs(code []byte, i int) (args []string, opened bool) {
	if i == 0 || (code[i-1] != '(' && code[i-1] != '{') {
		return nil, false
	}
	depth, start := 0, i
	for j := i; j < len(code); j++ {
		switch code[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				if a := strings.TrimSpace(string(code[start:j])); a != "" || len(args) > 0 {
					args = append(args, a)
				}
				return args, true
			}
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(string(code[start:j])))
				start = j + 1
			}
		}
	}
	return args, true
}

func nth(args []string, n int) string {
	if n < 1 || n > len(args) {
		return ""
	}
	return args[n-1]
}

var (
	castPrefix = regexp.MustCompile(`^(?:\(\s*[\w\s*]+(?:\(\s*\*\s*\)\s*\([\w\s*,]*\))?\s*\)|[&*\s])+`)
	pathName   = regexp.MustCompile(`^[A-Za-z_]\w*(?:(?:::|\.|->)[A-Za-z_]\w*)*`)
	pathSep    = regexp.MustCompile(`::|\.|->`)
)

// name is the bare name an argument starts with, the last part of a path
// such as ctx->queue, self.tx or pump in pump(tx), or "" for anything else,
// such as a closure.
func name(arg string) string {
	arg = strings.TrimSpace(arg)
	arg = arg[len(castPrefix.FindString(arg)):]
	p := pathName.FindString(arg)
	if rest := strings.TrimSpace(arg[len(p):]); p == "" || callKeywords[p] || p == "mut" || rest != "" && rest[0] != '(' && rest[0] != '[' {
		return ""
	}
	parts := pathSep.Split(p, -1)
	return parts[len(parts)-1]
}

// goArrows finds Go channel sends and receives. A send has its channel
// before the arrow; a receive has it after, with an operator, a bracket or
// a keyword before. An arrow beside chan is a type.
func (f file) goArrows(skip func(int) bool, seen map[int]bool) []Record {
	var out []Record
	code := f.code
	for i := 0; i+1 < len(code); i++ {
		if code[i] != '<' || code[i+1] != '-' || skip(i) {
			continue
		}
		j := i
		for j > 0 && (code[j-1] == ' ' || code[j-1] == '	') {
			j--
		}
		k := j
		for k > 0 && (isWord(code[k-1]) || code[k-1] == '.') {
			k--
		}
		before := string(code[k:j])
		after := strings.TrimLeft(string(code[i+2:min(i+64, len(code))]), " \t")
		if before == "chan" || strings.HasPrefix(after, "chan") {
			continue
		}
		r := Record{Confidence: "syntactic", Via: "catalog", Configs: []string{}}
		if k < j && !callKeywords[before] && before != "range" {
			r.Kind, r.API, r.Channel = Send, "<-", name(before)
			if seen[k] {
				continue
			}
			seen[k] = true
			r.Evidence = f.site(k)
		} else if k == j && j > 0 && (code[j-1] == ')' || code[j-1] == ']') {
			continue
		} else {
			m := goRecvChan.FindString(after)
			r.Kind, r.API, r.Channel = Recv, "<-", name(m)
			r.Evidence = f.site(i)
		}
		out = append(out, r)
	}
	return out
}

var goRecvChan = regexp.MustCompile(`^[A-Za-z_][\w.]*`)

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// matches runs the entry's pattern only around the lines that hold one of
// its needles, a line either side so a call split over lines still matches,
// and returns the match offsets in the whole file.
func (f file) matches(e Entry, lower []byte) [][]int {
	var windows [][2]int
	for n := range f.starts {
		if !slices.ContainsFunc(e.Needles, func(s string) bool { return bytes.Contains(lower[f.starts[n]:f.lineEnd(n)], []byte(s)) }) {
			continue
		}
		w := [2]int{f.starts[max(n-1, 0)], f.lineEnd(min(n+1, len(f.starts)-1))}
		if k := len(windows) - 1; k >= 0 && w[0] <= windows[k][1] {
			windows[k][1] = w[1]
			continue
		}
		windows = append(windows, w)
	}
	var out [][]int
	for _, w := range windows {
		for _, m := range e.Pattern.FindAllSubmatchIndex(f.code[w[0]:w[1]], -1) {
			for i := range m {
				if m[i] >= 0 {
					m[i] += w[0]
				}
			}
			out = append(out, m)
		}
	}
	return out
}

// lineEnd is the offset just past line n's newline, counted from 0.
func (f file) lineEnd(n int) int {
	if n+1 < len(f.starts) {
		return f.starts[n+1]
	}
	return len(f.code)
}

// asciiLower lowers ASCII letters only, byte for byte, so every offset in
// the result is the same offset in the code, whatever runes it holds.
func asciiLower(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}
