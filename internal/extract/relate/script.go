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
		if _, known := l.exts[strings.ToLower(path.Ext(r))]; !ok || !known || l.test(r) || strings.HasSuffix(r, ".d.ts") {
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
	r.srv, err = lsp.Start(ctx, lsp.Server{Argv: argv, Root: t.dir, Init: init, Timeout: 5 * time.Minute, Ready: 2 * time.Hour})
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
	f.facts = slices.DeleteFunc(r.facts, func(x fact) bool {
		return x.kind == model.Consumes && slices.ContainsFunc(r.facts, func(y fact) bool { return y.link == link{x.from, x.to, model.Provides} })
	})
	return f, nil
}

func (r *scriptRead) abs(p string) string { return filepath.Join(r.root, filepath.FromSlash(p)) }

// symbols lists the file's declarations: its top-level ones and the methods
// of its classes and interfaces.
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
	var walk func(list []lsp.DocumentSymbol, top bool)
	walk = func(list []lsp.DocumentSymbol, top bool) {
		for _, s := range list {
			r.decl[site3(p, s.SelectionRange.Start)] = true
			if r.lang.skip(s.Name) || s.Kind == lsp.KindTypeParameter {
				continue
			}
			if top || s.Kind == lsp.KindMethod {
				*out = append(*out, scriptSymbol{p, s, unit})
			}
			if s.Kind == lsp.KindClass || s.Kind == lsp.KindInterface {
				walk(s.Children, false)
			}
		}
	}
	walk(syms, true)
	return nil
}

// uses adds a fact for every place another unit names s: a trait where it
// extends or implements s, or declares the member that overrides it, a call
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
		// The server lists a method's overrides and an interface's members
		// among its references; a class that implements is a trait of its
		// own reference.
		if !ok || !read || !owned || u == s.unit || declared || l.Range.Start.Line >= len(lines) {
			continue
		}
		via := refVia(lines[l.Range.Start.Line], l.Range.Start.Character, l.Range.End.Character)
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
	// heritage is a TypeScript class or interface header with an extends or
	// implements clause before the reference; a Python class header with an
	// open parenthesis.
	tsHeritage  = regexp.MustCompile(`\b(class|interface)\b[^{=]*\b(extends|implements)\b[^{(]*$`)
	pyBases     = regexp.MustCompile(`^\s*class\s+\w+\s*\([^)]*$`)
	callAfter   = regexp.MustCompile(`^\s*(<[^<>()]*>\s*)?\(`)
	newBefore   = regexp.MustCompile(`\bnew\s+$`)
	decorBefore = regexp.MustCompile(`^\s*@[\w.]*$`)
)

// refVia is what the reference at byte columns start to end of a line is:
// trait where it names a base class or an implemented interface, call where
// it is called, instantiated with new or applied as a decorator, else
// import.
func refVia(line string, start, end int) model.Via {
	line = strings.TrimSuffix(line, "\r")
	if start < 0 || end < start || end > len(line) {
		return "import"
	}
	before, after := line[:start], line[end:]
	switch {
	case tsHeritage.MatchString(before) || pyBases.MatchString(before):
		return "trait"
	case callAfter.MatchString(after) || newBefore.MatchString(before) || decorBefore.MatchString(before) && !strings.HasPrefix(after, "."):
		return "call"
	}
	return "import"
}
