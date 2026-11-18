package relate

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/whyjp/abbd/internal/extract/lsp"
	"github.com/whyjp/abbd/internal/model"
)

// scriptVendored are directory names that hold dependencies or build
// output, which the TypeScript and Python relaters do not walk.
var scriptVendored = map[string]bool{
	"node_modules": true, "dist": true, "build": true, "out": true, "venv": true, "site-packages": true, "__pycache__": true,
	"vendor": true, "vendored": true, "third_party": true, "thirdparty": true, "external": true,
}

// scriptLang is what differs between the relaters that read a scripting
// language through one language server.
type scriptLang struct {
	// exts maps a file extension to the language id the server is told.
	exts map[string]string
	// test is whether a root-relative path is test code, left out by design.
	test func(p string) bool
	// skip is whether a name starts a declaration other units do not name.
	skip func(name string) bool
}

// scriptFiles are the files a unit holds that the relater reads,
// root-relative and sorted. Hidden, vendored and build directories and test
// code are left out.
func scriptFiles(t tree, o owners, l scriptLang) ([]string, error) {
	var out []string
	err := filepath.WalkDir(t.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != t.dir && (strings.HasPrefix(d.Name(), ".") || scriptVendored[strings.ToLower(d.Name())] || strings.HasSuffix(d.Name(), ".egg-info")) {
				return filepath.SkipDir
			}
			return nil
		}
		r, ok := t.rel(p)
		if _, known := l.exts[strings.ToLower(path.Ext(r))]; !ok || !known || l.test(r) || declarationFile(r) {
			return nil
		}
		if _, ok := o.of(r); ok {
			out = append(out, r)
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

// declarationFile is whether a path is a TypeScript declaration file, which
// declares types and holds no code.
func declarationFile(p string) bool {
	p = strings.ToLower(p)
	return strings.HasSuffix(p, ".d.ts") || strings.HasSuffix(p, ".d.mts") || strings.HasSuffix(p, ".d.cts")
}

// scriptServer is the server to run: the environment variable, else the
// name on the PATH. A server that answers --version is asked it, so a broken
// install fails here.
func scriptServer(env, name, install string, version bool) (string, error) {
	bin := os.Getenv(env)
	if bin == "" {
		p, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf("%s is not on the PATH; %s", name, install)
		}
		bin = p
	} else if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("%s=%s: %v; %s", env, bin, err, install)
	}
	if version {
		if out, err := exec.Command(bin, "--version").CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s --version failed (%v: %s); %s", bin, err, strings.TrimSpace(string(out)), install)
		}
	}
	return bin, nil
}

// scriptSymbol is a declaration other units can name.
type scriptSymbol struct {
	path string
	sym  lsp.DocumentSymbol
	unit string
}

// scriptRead is one server session over the files it reads.
type scriptRead struct {
	tree
	owners
	lang scriptLang
	srv  *lsp.Client
	mu   sync.Mutex
	// src holds each read file's lines, decl the name of every declaration
	// as path:line:column, and read the files the server answered for.
	src   map[string][]string
	decl  map[string]bool
	read  map[string]bool
	facts []fact
}

// scripting reads the files of the units with a language server that
// answers references. Every file is opened first, so the server sees them
// all; the declarations other units can name are asked for their references,
// and each reference in another unit becomes a call, an implementation or an
// import of the unit that declares the name. A file the server cannot list
// symbols for is unread.
func scripting(t tree, o owners, l scriptLang, argv []string, init any) (found, error) {
	files, err := scriptFiles(t, o, l)
	if err != nil {
		return found{}, err
	}
	r := &scriptRead{tree: t, owners: o, lang: l, src: map[string][]string{}, decl: map[string]bool{}, read: map[string]bool{}}
	ctx := context.Background()
	r.srv, err = lsp.Start(ctx, lsp.Server{Argv: argv, Root: t.dir, Init: init, Timeout: 5 * time.Minute})
	if err != nil {
		return found{}, err
	}
	defer r.srv.Close()
	for _, p := range files {
		if err := r.srv.Open(r.abs(p), l.exts[strings.ToLower(path.Ext(p))]); err != nil {
			return found{}, err
		}
	}
	symbols := make([][]scriptSymbol, len(files))
	if err := parallel(len(files), func(i int) error { return r.symbols(ctx, files[i], &symbols[i]) }); err != nil {
		return found{}, err
	}
	f := found{covered: map[string]bool{}}
	for _, p := range files {
		if !r.read[p] {
			f.unread = append(f.unread, p)
			continue
		}
		u, _ := o.of(p)
		f.covered[u] = true
	}
	all := slices.Concat(symbols...)
	if err := parallel(len(all), func(i int) error { return r.uses(ctx, all[i]) }); err != nil {
		return found{}, err
	}
	provides := map[link]bool{}
	for _, x := range r.facts {
		if x.kind == model.Provides {
			provides[x.link] = true
		}
	}
	f.facts = slices.DeleteFunc(r.facts, func(x fact) bool {
		return x.kind == model.Consumes && provides[link{x.from, x.to, model.Provides}]
	})
	return f, nil
}

func (r *scriptRead) abs(p string) string { return filepath.Join(r.root, filepath.FromSlash(p)) }

// symbols lists the file's top-level declarations. A method is never asked:
// the server matches a member name on a receiver whose type it cannot
// resolve, so a common name such as toString or input names another unit.
func (r *scriptRead) symbols(ctx context.Context, p string, out *[]scriptSymbol) error {
	b, err := os.ReadFile(r.abs(p))
	if err != nil {
		return err
	}
	syms, err := r.srv.DocumentSymbols(ctx, r.abs(p))
	if err != nil {
		return nil
	}
	unit, _ := r.of(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.read[p] = true
	r.src[p] = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	for _, s := range syms {
		r.decl[site3(p, s.SelectionRange.Start)] = true
		if !r.lang.skip(s.Name) && s.Kind != lsp.KindTypeParameter && !destructured(r.src[p], s) {
			*out = append(*out, scriptSymbol{p, s, unit})
		}
	}
	return nil
}

var (
	destructuring = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s*[{\[]`)
	literalExport = regexp.MustCompile(`^\s*export\s+default\s+(?:null|undefined|true|false|-?\d|['"` + "`" + `])`)
)

// destructured is whether a variable is bound by destructuring, as in
// const { isFunction } = utils, or is a default export of a literal such as
// null. The server lists the references of the property it reads among the
// variable's own, from any receiver, and every use of the literal among a
// literal's.
func destructured(lines []string, s lsp.DocumentSymbol) bool {
	n := s.Range.Start.Line
	if n >= len(lines) {
		return false
	}
	return (s.Kind == lsp.KindVariable || s.Kind == lsp.KindConstant) && destructuring.MatchString(lines[n]) || literalExport.MatchString(lines[n])
}

// uses adds a fact for every place another unit names s: a trait where it
// extends or implements s, a call
// where it calls s, else an import.
func (r *scriptRead) uses(ctx context.Context, s scriptSymbol) error {
	refs, err := r.srv.References(ctx, r.abs(s.path), s.sym.SelectionRange.Start)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", s.path, s.sym.Name, err)
	}
	ifc, contract := r.owners.contract(s.path, s.sym.Name)
	for _, l := range refs {
		abs, err := lsp.Path(l.URI)
		if err != nil {
			continue
		}
		p, ok := r.rel(abs)
		r.mu.Lock()
		lines, read := r.src[p], r.read[p]
		declared := r.decl[site3(p, l.Range.Start)]
		r.mu.Unlock()
		u, owned := r.of(p)
		if !ok || !read || !owned || u == s.unit || declared || l.Range.Start.Line >= len(lines) {
			continue
		}
		via := refVia(lines, l.Range.Start.Line, l.Range.Start.Character, l.Range.End.Character, p)
		at := site{path: p, line: l.Range.Start.Line + 1}
		r.mu.Lock()
		r.facts = append(r.facts, fact{link{u, s.unit, model.DependsOn}, via, at})
		switch {
		case !contract || ifc == u:
		case via == "trait":
			r.facts = append(r.facts, fact{link{u, ifc, model.Provides}, via, at})
		default:
			r.facts = append(r.facts, fact{link{u, ifc, model.Consumes}, via, at})
		}
		r.mu.Unlock()
	}
	return nil
}

var (
	callAfter   = regexp.MustCompile(`^\s*(<[^<>()]*>\s*)?\(`)
	newBefore   = regexp.MustCompile(`\bnew\s+$`)
	decorBefore = regexp.MustCompile(`^\s*@[\w.]*$`)
	jsxBefore   = regexp.MustCompile(`(^|[^\w$.])</?$`)
	tsHeader    = regexp.MustCompile(`\b(class|interface)\s+[\w$]`)
	pyHeader    = regexp.MustCompile(`^\s*class\s+\w`)
	keywordArg  = regexp.MustCompile(`^\s*=[^=]`)
)

// headerLines is how far above a reference its class header is looked for.
const headerLines = 40

// header is the text from the nearest class header above the reference,
// up to the reference, with comments cut; empty when there is none.
func header(lines []string, row, start int, python bool) string {
	re := tsHeader
	if python {
		re = pyHeader
	}
	for i := row; i >= 0 && i > row-headerLines; i-- {
		text := lines[i]
		if i == row {
			text = text[:start]
		}
		if python {
			text, _, _ = strings.Cut(text, "#")
		} else {
			text, _, _ = strings.Cut(text, "//")
		}
		if loc := re.FindStringIndex(text); loc != nil {
			var b strings.Builder
			b.WriteString(text[loc[0]:])
			if i < row {
				for _, l := range lines[i+1 : row] {
					b.WriteString(" ")
					b.WriteString(l)
				}
				b.WriteString(" ")
				b.WriteString(lines[row][:start])
			}
			return b.String()
		}
	}
	return ""
}

// heritage is whether a name that follows the class header text stands
// directly in the class's base list: after extends or implements at depth 0
// of a TypeScript header, or as a positional argument of a Python class's
// parentheses. A type argument (Base<Foo>, Generic[T]), a keyword argument
// (metaclass=Meta) and a name past the body's opening are not.
func heritage(h string, python bool) bool {
	var stack []byte
	inBase, arg := false, 0
	for i := 0; i < len(h); i++ {
		c := h[i]
		switch c {
		case '(', '[', '{', '<':
			if c == '<' && python {
				continue
			}
			if c == '{' && !python && len(stack) == 0 {
				return false
			}
			stack = append(stack, c)
		case ')', ']', '}':
			if len(stack) == 0 {
				return false
			}
			stack = stack[:len(stack)-1]
			if python && len(stack) == 0 {
				return false
			}
		case ':':
			if python && len(stack) == 0 {
				return false
			}
		case '>':
			if i > 0 && h[i-1] == '=' {
				continue
			}
			if len(stack) > 0 && stack[len(stack)-1] == '<' {
				stack = stack[:len(stack)-1]
			}
		case ',':
			if python && len(stack) == 1 {
				arg = i + 1
			}
		default:
			if !python && len(stack) == 0 && (c == 'e' || c == 'i') {
				for _, k := range []string{"extends", "implements"} {
					if strings.HasPrefix(h[i:], k) && (i == 0 || !isWord(h[i-1])) && (i+len(k) == len(h) || !isWord(h[i+len(k)])) {
						inBase = true
					}
				}
			}
		}
	}
	if python {
		return len(stack) == 1 && stack[0] == '(' && !strings.Contains(strings.ReplaceAll(h[arg:], "==", ""), "=")
	}
	return inBase && len(stack) == 0
}

func isWord(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// refVia is what the reference at byte columns start to end of line row of
// the file p is: trait where it names a base class or an implemented
// interface, call where it is called, instantiated with new, written as a
// JSX tag or applied as a decorator, else import.
func refVia(lines []string, row, start, end int, p string) model.Via {
	if row < 0 || row >= len(lines) {
		return "import"
	}
	line := strings.TrimSuffix(lines[row], "\r")
	if start < 0 || end < start || end > len(line) {
		return "import"
	}
	python := strings.ToLower(path.Ext(p)) == ".py"
	before, after := line[:start], line[end:]
	switch {
	case !keywordArg.MatchString(after) && heritage(header(lines, row, start, python), python):
		return "trait"
	case callAfter.MatchString(after) || newBefore.MatchString(before) || jsxFile(p) && jsxBefore.MatchString(before) || decorBefore.MatchString(before) && !strings.HasPrefix(after, "."):
		return "call"
	}
	return "import"
}

func jsxFile(p string) bool {
	e := strings.ToLower(path.Ext(p))
	return e == ".tsx" || e == ".jsx"
}
