package runscan

import (
	"bytes"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// def is a function, method or function-like macro the code defines.
type def struct {
	name string
	// qual is what a qualified call names it by: a Rust free function's
	// module, a Go function's package, a C++ method's class. "" for a C
	// function, whose name is global.
	qual   string
	at     int
	params []string
	// body is the span a call inside the definition lies in; [0 0] for a
	// declaration with no body.
	body   [2]int
	method bool
	// local is a C static function, which only its own file can call.
	local bool
}

// wrapper is a project function that starts an execution context with
// work its caller hands in: its body passes one of its own parameters to a
// catalog spawn call. A call to it is a spawn site.
type wrapper struct {
	def
	lang  Lang
	file  string
	param int
}

// api names the wrapper the way a call qualifies it.
func (w wrapper) api() string {
	if w.qual == "" || w.method && w.lang != C {
		return w.name
	}
	if w.lang == Go {
		return w.qual + "." + w.name
	}
	return w.qual + "::" + w.name
}

// alias is a name the code gives a catalog call or type: use X as Y,
// type Y = X, using Y = X or #define Y X.
type alias struct {
	lang         Lang
	name, target string
	file         string
	at           int
	// global marks an alias every file sees: a type alias or a macro. A
	// Rust use is seen in its own file only.
	global bool
	use    *regexp.Regexp
}

// index holds what the first reading found across all files.
type index struct {
	wrappers map[Lang][]wrapper
	pattern  map[string]*regexp.Regexp
	aliases  []alias
	// count is how many definitions carry each name per language.
	count map[Lang]map[string]int
	// wrapped is how many of those definitions are wrappers. A name every
	// definition of which is a wrapper needs no qualifier either.
	wrapped map[Lang]map[string]int
	defs    map[Lang]map[string][]string
}

func newIndex(fs []file) *index {
	ix := &index{wrappers: map[Lang][]wrapper{}, pattern: map[string]*regexp.Regexp{}, count: map[Lang]map[string]int{}, wrapped: map[Lang]map[string]int{}, defs: map[Lang]map[string][]string{}}
	for _, f := range fs {
		if ix.count[f.lang] == nil {
			ix.count[f.lang] = map[string]int{}
			ix.defs[f.lang] = map[string][]string{}
		}
		for _, d := range f.defs {
			ix.count[f.lang][d.name]++
			if d.body[1] > 0 {
				ix.defs[f.lang][d.name] = append(ix.defs[f.lang][d.name], f.path+":"+strconv.Itoa(f.line(d.at)))
			}
		}
		ix.wrappers[f.lang] = append(ix.wrappers[f.lang], f.wrappers()...)
		ix.aliases = append(ix.aliases, f.aliases()...)
	}
	ix.aliases = slices.DeleteFunc(ix.aliases, func(a alias) bool { return !ix.spawnName(a.lang, a.target) })
	for i := range ix.aliases {
		ix.aliases[i].use = regexp.MustCompile(`\b` + regexp.QuoteMeta(ix.aliases[i].name) + `\b`)
	}
	for l, ws := range ix.wrappers {
		ix.wrapped[l] = map[string]int{}
		for _, w := range ws {
			ix.wrapped[l][w.name]++
			if ix.pattern[w.name] == nil {
				ix.pattern[w.name] = regexp.MustCompile(`(?:(?:\b(?P<q>[A-Za-z_]\w*))?\s*(?P<sep>::|\.|->)\s*)?\b` + regexp.QuoteMeta(w.name) + `\s*\(`)
			}
		}
	}
	return ix
}

// spawnName reports whether a name could start a spawn call: its last
// part is a word of a catalog spawn entry's needles, as Runtime is of
// runtime::new, or a wrapper's name. Only such an alias is read again.
func (ix *index) spawnName(l Lang, target string) bool {
	last := target[strings.LastIndex(target, ":")+1:]
	low := strings.ToLower(last)
	for _, e := range Catalog {
		if e.Lang == l && e.Kind == Spawn && slices.ContainsFunc(e.Needles, func(n string) bool {
			return slices.Contains(strings.FieldsFunc(n, func(r rune) bool { return !isWord(byte(r)) }), low)
		}) {
			return true
		}
	}
	return slices.ContainsFunc(ix.wrappers[l], func(w wrapper) bool { return w.name == last })
}

// def is where the one definition of a name stands, as path:line, or ""
// when no definition or more than one does.
func (ix *index) def(l Lang, name string) string {
	if ds := ix.defs[l][name]; len(slices.Compact(slices.Clone(ds))) == 1 {
		return ds[0]
	}
	return ""
}

// wrappers returns the file's definitions whose body hands one of their
// parameters to a catalog spawn call as the work it runs.
func (f file) wrappers() []wrapper {
	var out []wrapper
	found := map[int]bool{}
	for _, h := range f.hits(true) {
		if h.e.Kind != Spawn || (f.lang == Rust && f.skip(h.at)) {
			continue
		}
		d, ok := f.enclosing(h.at)
		if !ok || found[d.at] {
			continue
		}
		work := f.work(h)
		for i, p := range d.params {
			if p != "" && slices.ContainsFunc(work, func(w string) bool { return usesParam(w, p) }) {
				out = append(out, wrapper{def: d, lang: f.lang, file: f.path, param: i + 1})
				found[d.at] = true
				break
			}
		}
	}
	return out
}

// work is the text of what a spawn call runs: its function argument, or
// the function a go statement calls or its literal. It is nil when the
// catalog names no such position, as a pool's size or a runtime builder
// hands no work in.
func (f file) work(h hit) []string {
	if fn := h.group("fn"); fn != "" {
		return []string{fn}
	}
	if h.e.FnArg > 0 {
		return []string{nth(h.args, h.e.FnArg)}
	}
	if f.lang == Go {
		return []string{string(f.code[h.end:itemEnd(f.code, h.end)])}
	}
	return nil
}

// enclosing is the innermost definition whose body holds offset at.
func (f file) enclosing(at int) (def, bool) {
	best, found := def{}, false
	for _, d := range f.defs {
		if d.body[0] <= at && at < d.body[1] && (!found || d.body[1]-d.body[0] < best.body[1]-best.body[0]) {
			best, found = d, true
		}
	}
	return best, found
}

var (
	selfRef = regexp.MustCompile(`^(?:&\s*)?(?:mut\s+)?(?:'\w+\s+)?(?:mut\s+)?self\b`)
	boxed   = regexp.MustCompile(`^Box::(?:pin|new)\s*\(\s*([A-Za-z_]\w*)\s*\)$`)
)

// usesParam reports whether a spawn call's work is parameter p itself:
// p heads the argument, as in task or fut.instrument(span), Box::pin
// wraps it, or a closure, async block or func literal does nothing but
// call or await it. A closure that calls p among other work, such as a
// handler called on each event, does not hand p in.
func usesParam(work, p string) bool {
	if head(work) == p {
		return true
	}
	if m := boxed.FindStringSubmatch(strings.TrimSpace(work)); m != nil && m[1] == p {
		return true
	}
	body := strings.TrimSpace(closureHead.ReplaceAllString(strings.TrimSpace(work), ""))
	if strings.HasPrefix(body, "{") && closeOf(body, 0) == len(body)-1 {
		body = strings.TrimSpace(body[1 : len(body)-1])
	}
	body = strings.TrimSpace(strings.TrimSuffix(body, ";"))
	body = strings.TrimSpace(awaited.ReplaceAllString(body, ""))
	rest, ok := strings.CutPrefix(body, p)
	rest = strings.TrimSpace(rest)
	return ok && (rest == "" || rest[0] == '(' && closeOf(rest, 0) == len(rest)-1)
}

var (
	closureHead = regexp.MustCompile(`^(?:async\s+)?(?:move\s*)?(?:\|[^|]*\||\([^)]*\))?\s*(?:->\s*[\w:<>]+\s*)?(?:async\s+)?(?:move\s*)?`)
	awaited     = regexp.MustCompile(`\.\s*await$`)
)

// head is the first name of an argument's path, after any &, *, move,
// cast or enclosing parentheses: ctx in &ctx->f, f in (f).
func head(arg string) string {
	arg = strings.TrimSpace(arg)
	for {
		before := arg
		for _, w := range []string{"&", "*", "move ", "async ", "unsafe "} {
			arg = strings.TrimSpace(strings.TrimPrefix(arg, w))
		}
		if strings.HasPrefix(arg, "(") && closeOf(arg, 0) == len(arg)-1 {
			arg = strings.TrimSpace(arg[1 : len(arg)-1])
		} else if c := castPrefix.FindString(arg); c != "" && c != arg {
			arg = strings.TrimSpace(arg[len(c):])
		}
		if arg == before {
			break
		}
	}
	p := pathName.FindString(arg)
	return pathSep.Split(p, -1)[0]
}

// closeOf is the offset of the bracket that closes the one at open, or -1.
func closeOf[T string | []byte](s T, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

type call struct {
	r  Record
	at int
}

// wrapperCalls finds the file's calls through a wrapper. A call matches by
// its bare name when the language defines that name once, by the
// wrapper's qualifier anywhere, and unqualified or through self in the
// wrapper's own file, or its own package in Go. A C function's name is
// global unless it is static.
func (ix *index) wrapperCalls(f file) []call {
	defAt := map[int]bool{}
	for _, d := range f.defs {
		defAt[d.at] = true
	}
	var out []call
	done := map[string]bool{}
	for _, w := range ix.wrappers[f.lang] {
		if done[w.name] {
			continue
		}
		done[w.name] = true
		ws := slices.DeleteFunc(slices.Clone(ix.wrappers[f.lang]), func(o wrapper) bool { return o.name != w.name })
		if !bytes.Contains(f.code, []byte(w.name)) {
			continue
		}
		pat := ix.pattern[w.name]
		for _, m := range pat.FindAllSubmatchIndex(f.code, -1) {
			at := m[1] - 1
			for at > 0 && isSpace(f.code[at-1]) {
				at--
			}
			at -= len(w.name)
			if defAt[at] || f.skip(at) || (f.lang == C && declares(f.code, at)) {
				continue
			}
			q, sep := qualifier(f.code, m)
			args, _ := callArgs(f.code, m[1])
			for _, w := range ws {
				if len(args) >= w.param && ix.calls(w, f, q, sep) {
					r := Record{Kind: Spawn, API: w.api(), Confidence: "syntactic", Via: "catalog", Configs: []string{}}
					r.To.Name = name(nth(args, w.param))
					out = append(out, call{r, at})
					break
				}
			}
		}
	}
	return out
}

// qualifier is what stands before a wrapper's name in a call match: the
// name q, if one, and the separator, :: . or ->, if any.
func qualifier(text []byte, m []int) (q, sep string) {
	if m[2] >= 0 {
		q = string(text[m[2]:m[3]])
	}
	if m[4] >= 0 {
		sep = string(text[m[4]:m[5]])
	}
	return q, sep
}

// calls reports whether a call by the wrapper's name, qualified by q and
// sep, in file f, reads as a call of that wrapper. A qualifier must name
// the wrapper's own module, package or class, a method call through .
// or -> must reach a method, and a bare call reaches a C function, a name
// every definition of which in the language is a wrapper, or a function of
// the same file, or the same package in Go. A C static function is
// reached from its own file only.
func (ix *index) calls(w wrapper, f file, q, sep string) bool {
	same := f.path == w.file
	if w.lang == Go {
		same = path.Dir(f.path) == path.Dir(w.file)
	}
	only := ix.count[w.lang][w.name] == ix.wrapped[w.lang][w.name]
	switch {
	case w.local:
		return same && sep == ""
	case q != "" && q == w.qual:
		return true
	case sep == "." || sep == "->":
		return w.method && (only || same)
	case sep == "::":
		return q == "Self" && same || (q == "crate" || q == "super") && only
	case w.lang == C && !w.method:
		return true
	}
	return only || same
}

// aliasCalls finds the file's catalog calls spelled through an alias: each
// use of the alias's name is read again with the aliased name in its place,
// and a catalog entry or wrapper that matches right there makes a site.
func (ix *index) aliasCalls(f file) []call {
	var out []call
	for _, a := range ix.aliases {
		if a.lang != f.lang || !a.global && a.file != f.path || !bytes.Contains(f.code, []byte(a.name)) {
			continue
		}
		for _, m := range a.use.FindAllIndex(f.code, -1) {
			at := m[0]
			if a.file == f.path && at == a.at || f.skip(at) || at > 0 && f.code[at-1] == '.' {
				continue
			}
			rest := f.code[m[1]:min(m[1]+256, len(f.code))]
			probe := []byte(a.target + string(rest))
			if r, ok := f.probe(probe, len(a.target), at, m[1], ix); ok {
				out = append(out, call{r, at})
			}
		}
	}
	return out
}

// probe matches the catalog's spawn entries, and the wrappers, at the start
// of text that spells an alias's target where the code spells the alias.
// shift is the target's length, and end the alias's end in the code, so an
// offset in the probe past the target maps back onto the code.
func (f file) probe(text []byte, shift, at, end int, ix *index) (Record, bool) {
	back := func(i int) int { return end + i - shift }
	for _, e := range Catalog {
		if e.Lang != f.lang || e.Kind != Spawn {
			continue
		}
		m := e.Pattern.FindSubmatchIndex(text)
		if m == nil || m[0] != 0 || m[1] <= shift {
			continue
		}
		if e.Call && declares(f.code, at) {
			continue
		}
		args, opened := callArgs(f.code, back(m[1]))
		if e.Args && (!opened || len(args) == 0) {
			continue
		}
		r := Record{Kind: Spawn, API: e.API, Confidence: "syntactic", Via: "catalog", Configs: []string{}}
		if i := e.Pattern.SubexpIndex("api"); i > 0 && m[2*i] >= 0 {
			r.API = string(text[m[2*i]:m[2*i+1]])
		}
		r.To.Name = name(nth(args, e.FnArg))
		return r, true
	}
	for _, w := range ix.wrappers[f.lang] {
		m := ix.pattern[w.name].FindSubmatchIndex(text)
		if m == nil || m[0] != 0 || m[1] <= shift {
			continue
		}
		q, sep := qualifier(text, m)
		args, _ := callArgs(f.code, back(m[1]))
		if len(args) >= w.param && ix.calls(w, f, q, sep) {
			r := Record{Kind: Spawn, API: w.api(), Confidence: "syntactic", Via: "catalog", Configs: []string{}}
			r.To.Name = name(nth(args, w.param))
			return r, true
		}
	}
	return Record{}, false
}

var (
	rustFn      = regexp.MustCompile(`\bfn\s+([A-Za-z_]\w*)`)
	goFunc      = regexp.MustCompile(`(?m)^func\s*(\([^)]*\)\s*)?([A-Za-z_]\w*)\s*(?:\[[^\]]*\]\s*)?\(`)
	cName       = regexp.MustCompile(`\b((?:[A-Za-z_]\w*\s*::\s*)*~?[A-Za-z_]\w*)\s*\(`)
	cMacro      = regexp.MustCompile(`(?m)^[ \t]*#[ \t]*define[ \t]+([A-Za-z_]\w*)\(([^)]*)\)`)
	cAlias      = regexp.MustCompile(`(?m)^[ \t]*#[ \t]*define[ \t]+([A-Za-z_]\w*)[ \t]+([A-Za-z_][\w:]*)[ \t]*$`)
	cppUsing    = regexp.MustCompile(`\busing\s+([A-Za-z_]\w*)\s*=\s*([A-Za-z_][\w:]*)\s*;`)
	rustUse     = regexp.MustCompile(`\buse\s+[^;]*;`)
	rustUseAs   = regexp.MustCompile(`([A-Za-z_][\w:]*)\s+as\s+([A-Za-z_]\w*)`)
	rustType    = regexp.MustCompile(`\btype\s+([A-Za-z_]\w*)\s*=\s*([A-Za-z_][\w:]*)\s*;`)
	cDeclSuffix = regexp.MustCompile(`^(?:\s*(?:const|noexcept|override|final|volatile)\b)*\s*\{`)
	cStatic     = regexp.MustCompile(`\bstatic\b`)
)

// findDefs lists the functions, methods and function-like macros the file
// defines, read from the code alone.
func (f file) findDefs() []def {
	code := f.code
	var out []def
	params := func(open int) ([]string, int) {
		end := closeOf(code, open)
		if end < 0 {
			return nil, -1
		}
		args, _ := callArgs(code, open+1)
		return args, end
	}
	switch f.lang {
	case Rust:
		mod := rustModule(f.path)
		for _, m := range rustFn.FindAllSubmatchIndex(code, -1) {
			i := skipGenerics(code, m[1])
			if i >= len(code) || code[i] != '(' {
				continue
			}
			ps, end := params(i)
			if end < 0 {
				continue
			}
			d := def{name: string(code[m[2]:m[3]]), at: m[2], qual: mod}
			for k, p := range ps {
				if k == 0 && selfRef.MatchString(p) {
					d.method, d.qual = true, ""
					continue
				}
				d.params = append(d.params, rustParam(p))
			}
			if b := itemEnd(code, end+1); b > 0 && code[b-1] == '}' {
				d.body = [2]int{end + 1, b}
			}
			out = append(out, d)
		}
	case Go:
		pkg := path.Base(path.Dir(f.path))
		for _, m := range goFunc.FindAllSubmatchIndex(code, -1) {
			ps, end := params(m[1] - 1)
			if end < 0 {
				continue
			}
			d := def{name: string(code[m[4]:m[5]]), at: m[4], qual: pkg, method: m[2] >= 0}
			for _, p := range ps {
				d.params = append(d.params, firstWord(p))
			}
			if nl := strings.IndexAny(string(code[end:]), "{\n"); nl >= 0 && code[end+nl] == '{' {
				d.body = [2]int{end + nl, end + nl + closeOf(code[end+nl:], 0) + 1}
			}
			out = append(out, d)
		}
	case C:
		for _, m := range cName.FindAllSubmatchIndex(code, -1) {
			full := strings.Join(strings.Fields(string(code[m[2]:m[3]])), "")
			parts := strings.Split(full, "::")
			at := m[3] - len(parts[len(parts)-1])
			if f.skip(m[2]) || !declares(code, m[2]) && len(parts) == 1 {
				continue
			}
			ps, end := params(m[1] - 1)
			if end < 0 {
				continue
			}
			s := cDeclSuffix.FindIndex(code[end+1:])
			if s == nil {
				continue
			}
			open := end + s[1]
			d := def{name: parts[len(parts)-1], at: at, body: [2]int{open, open + closeOf(code[open:], 0) + 1}}
			if len(parts) > 1 {
				d.qual, d.method = parts[len(parts)-2], true
			}
			start := max(strings.LastIndexAny(string(code[:m[2]]), ";}{"), 0)
			d.local = cStatic.Match(code[start:m[2]])
			for _, p := range ps {
				d.params = append(d.params, cParam(p))
			}
			out = append(out, d)
		}
		for _, m := range cMacro.FindAllSubmatchIndex(code, -1) {
			end := m[1]
			for n := f.line(end) - 1; n < len(f.starts); n++ {
				end = f.lineEnd(n)
				if !strings.HasSuffix(strings.TrimRight(f.lineText(n), " \t"), "\\") {
					break
				}
			}
			d := def{name: string(code[m[2]:m[3]]), at: m[2], body: [2]int{m[1], end}}
			for _, p := range strings.Split(string(code[m[4]:m[5]]), ",") {
				d.params = append(d.params, strings.TrimSpace(p))
			}
			out = append(out, d)
		}
	}
	return out
}

// aliases lists the names the file gives a call or a type.
func (f file) aliases() []alias {
	var out []alias
	add := func(name, target string, at int, global bool) {
		for _, p := range []string{"crate::", "self::", "super::", "::"} {
			target = strings.TrimPrefix(target, p)
		}
		if name != target && !strings.HasSuffix(target, "::"+name) {
			out = append(out, alias{lang: f.lang, name: name, target: target, file: f.path, at: at, global: global})
		}
	}
	switch f.lang {
	case Rust:
		for _, u := range rustUse.FindAllIndex(f.code, -1) {
			for _, m := range rustUseAs.FindAllSubmatchIndex(f.code[u[0]:u[1]], -1) {
				add(string(f.code[u[0]+m[4]:u[0]+m[5]]), string(f.code[u[0]+m[2]:u[0]+m[3]]), u[0]+m[4], false)
			}
		}
		for _, m := range rustType.FindAllSubmatchIndex(f.code, -1) {
			add(string(f.code[m[2]:m[3]]), string(f.code[m[4]:m[5]]), m[2], true)
		}
	case C:
		for _, m := range cAlias.FindAllSubmatchIndex(f.code, -1) {
			add(string(f.code[m[2]:m[3]]), string(f.code[m[4]:m[5]]), m[2], true)
		}
		for _, m := range cppUsing.FindAllSubmatchIndex(f.code, -1) {
			add(string(f.code[m[2]:m[3]]), string(f.code[m[4]:m[5]]), m[2], true)
		}
	}
	return out
}

// rustModule is the module a Rust file's free functions are called
// through: the file's stem, its directory's name for mod.rs, and the
// crate's name, with - read as _, for lib.rs and main.rs.
func rustModule(p string) string {
	stem := strings.TrimSuffix(path.Base(p), ".rs")
	dir := path.Dir(p)
	switch stem {
	case "mod":
		return path.Base(dir)
	case "lib", "main":
		if path.Base(dir) == "src" {
			dir = path.Dir(dir)
		}
		return strings.ReplaceAll(path.Base(dir), "-", "_")
	}
	return stem
}

// skipGenerics returns the offset after a <...> list that starts at i,
// past spaces, or i's first non-space when no list starts there. An arrow
// in a bound, as in F: Fn() -> T, closes nothing.
func skipGenerics(code []byte, i int) int {
	for i < len(code) && isSpace(code[i]) {
		i++
	}
	if i >= len(code) || code[i] != '<' {
		return i
	}
	depth := 0
	for ; i < len(code); i++ {
		switch {
		case code[i] == '<':
			depth++
		case code[i] == '>' && code[i-1] != '-':
			depth--
			if depth == 0 {
				i++
				for i < len(code) && isSpace(code[i]) {
					i++
				}
				return i
			}
		}
	}
	return i
}

// rustParam is a Rust parameter's name: the binding before its type.
func rustParam(p string) string {
	pat, _, ok := strings.Cut(p, ":")
	if !ok {
		return ""
	}
	pat = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(pat), "mut "))
	if pathName.FindString(pat) != pat || strings.Contains(pat, "::") {
		return ""
	}
	return pat
}

var cFnPtr = regexp.MustCompile(`\(\s*\*\s*([A-Za-z_]\w*)\s*\)`)

// cParam is a C parameter's name: inside (*name) for a function pointer,
// else its last word.
func cParam(p string) string {
	if m := cFnPtr.FindStringSubmatch(p); m != nil {
		return m[1]
	}
	p = strings.TrimSpace(p)
	if i := strings.IndexAny(p, "=["); i >= 0 {
		p = strings.TrimSpace(p[:i])
	}
	i := len(p)
	for i > 0 && isWord(p[i-1]) {
		i--
	}
	if i == 0 || p[i:] == "void" {
		return ""
	}
	return p[i:]
}

func firstWord(p string) string {
	p = strings.TrimSpace(p)
	i := 0
	for i < len(p) && isWord(p[i]) {
		i++
	}
	return p[:i]
}
