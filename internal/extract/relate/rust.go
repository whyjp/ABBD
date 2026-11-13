package relate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/whyjp/abbd/internal/extract/lsp"
	"github.com/whyjp/abbd/internal/model"
)

// rustTarget is the one target rust-analyzer evaluates cfg for, the same on
// every host, so a Windows and a Linux run print the same relations. rustc
// knows every built-in target's cfg without its standard library installed.
const rustTarget = "x86_64-unknown-linux-gnu"

// rustConfig is one feature set rust-analyzer reads the workspace under.
type rustConfig struct {
	name     string
	features any
}

// rustConfigs are read one after another and their relations unioned. A
// relation found under only some of them holds under those, its when.
var rustConfigs = []rustConfig{{"default features", []string{}}, {"all features", "all"}}

var rustReads = "default and all features for " + rustTarget

// rustInstall says how to get rust-analyzer on each OS.
const rustInstall = "set ABBD_LSP_RUST to its path, or install it: `rustup component add rust-analyzer` with rustup on any OS, " +
	"the release binary for your OS from https://github.com/rust-lang/rust-analyzer/releases, or `brew install rust-analyzer` on macOS"

// rust reads a Cargo workspace with rust-analyzer, once per feature set.
// Tests, benches, examples, build scripts and proc macros are left out, so
// the server reads what the crates compile to and nothing it would have to
// build first.
func rust(t tree, o owners) (found, error) {
	ra, err := rustAnalyzer()
	if err != nil {
		return found{}, err
	}
	md, err := cargoWorkspace(t.dir)
	if err != nil {
		return found{}, err
	}
	// rust-analyzer's cargo metadata writes a Cargo.lock where there is
	// none; the relater reads only, so it takes away one it caused.
	lock := filepath.Join(md.WorkspaceRoot, "Cargo.lock")
	if _, err := os.Stat(lock); errors.Is(err, fs.ErrNotExist) {
		defer os.Remove(lock)
	}
	files, err := rustFiles(t, o, md)
	if err != nil {
		return found{}, err
	}
	configs := rustConfigs
	if !md.featured() {
		configs = configs[:1]
	}
	f := found{covered: map[string]bool{}, when: map[link]string{}}
	in := map[link][]string{}
	read := map[string]bool{}
	for _, c := range configs {
		r, err := rustRead(t, o, ra, c, files, md.crates(t, o))
		if err != nil {
			return found{}, fmt.Errorf("%s: %w", c.name, err)
		}
		seen := map[link]bool{}
		for _, x := range r.facts {
			if !seen[x.link] {
				seen[x.link] = true
				in[x.link] = append(in[x.link], c.name)
			}
		}
		f.facts = append(f.facts, r.facts...)
		for p := range r.read {
			read[p] = true
			if u, ok := o.of(p); ok {
				f.covered[u] = true
			}
		}
	}
	for l, names := range in {
		if len(names) < len(configs) {
			f.when[l] = strings.Join(names, " | ")
		}
	}
	for _, p := range files {
		if !read[p] && !rustTest(p) {
			f.unread = append(f.unread, p)
		}
	}
	return f, nil
}

// rustTest is whether a path names test code, which cfg(test) gates and
// the relater leaves out by design, so it is no gap worth reporting.
func rustTest(p string) bool {
	dirs := strings.Split(p, "/")
	name := strings.TrimSuffix(dirs[len(dirs)-1], ".rs")
	for _, d := range append(dirs[:len(dirs)-1], name) {
		switch d {
		case "test", "tests", "testing", "test_util", "test_utils", "test_support", "test_helpers":
			return true
		}
	}
	return strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "_tests") || strings.HasPrefix(name, "test_")
}

// rustAnalyzer is the server to run: $ABBD_LSP_RUST, else rust-analyzer on
// the PATH. rustup puts a proxy there that fails until the component is
// added, so it is asked its version first.
func rustAnalyzer() (string, error) {
	ra := os.Getenv("ABBD_LSP_RUST")
	if ra == "" {
		p, err := exec.LookPath("rust-analyzer")
		if err != nil {
			return "", fmt.Errorf("rust-analyzer is not on the PATH; %s", rustInstall)
		}
		ra = p
	}
	if out, err := exec.Command(ra, "--version").CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s --version failed (%v: %s); %s", ra, err, strings.TrimSpace(string(out)), rustInstall)
	}
	return ra, nil
}

type cargoMeta struct {
	WorkspaceRoot string        `json:"workspace_root"`
	Packages      []cargoMember `json:"packages"`
}

type cargoMember struct {
	ManifestPath string              `json:"manifest_path"`
	Features     map[string][]string `json:"features"`
	Targets      []struct {
		Name    string   `json:"name"`
		Kind    []string `json:"kind"`
		SrcPath string   `json:"src_path"`
	} `json:"targets"`
}

// featured is whether any member declares a feature, so that all features
// can read code the default ones leave out.
func (m cargoMeta) featured() bool {
	return slices.ContainsFunc(m.Packages, func(p cargoMember) bool { return len(p.Features) > 0 })
}

// aside are the paths of the targets that are not what a crate compiles
// to: tests, benches, examples and build scripts. rust-analyzer reads them
// all, whatever cargo.allTargets says. A target's own directory, such as
// tests/ or benches/, goes aside with the modules it holds, unless it is
// the package root or holds the crate's lib or bin root too.
func (m cargoMeta) aside() (dirs, files []string) {
	for _, p := range m.Packages {
		pkg := filepath.Dir(p.ManifestPath)
		roots := map[string]bool{}
		for _, t := range p.Targets {
			if !slices.ContainsFunc(t.Kind, sideTarget) {
				roots[filepath.Dir(t.SrcPath)] = true
			}
		}
		for _, t := range p.Targets {
			if !slices.ContainsFunc(t.Kind, sideTarget) {
				continue
			}
			files = append(files, t.SrcPath)
			if d := filepath.Dir(t.SrcPath); d != pkg && !roots[d] && !slices.Contains(t.Kind, "custom-build") {
				dirs = append(dirs, d)
			}
		}
	}
	return dirs, files
}

// rustCrate is a workspace member's library as code names it: by its
// crate name, with the unit that holds its root.
type rustCrate struct{ name, unit string }

// crates are the members' libraries that a unit holds.
func (m cargoMeta) crates(t tree, o owners) []rustCrate {
	var out []rustCrate
	for _, p := range m.Packages {
		for _, tg := range p.Targets {
			if !slices.ContainsFunc(tg.Kind, func(k string) bool { return k == "lib" || k == "rlib" || k == "proc-macro" }) {
				continue
			}
			if r, ok := t.rel(tg.SrcPath); ok {
				if u, ok := o.of(r); ok {
					out = append(out, rustCrate{strings.ReplaceAll(tg.Name, "-", "_"), u})
				}
			}
		}
	}
	return out
}

func sideTarget(kind string) bool {
	return kind == "test" || kind == "bench" || kind == "example" || kind == "custom-build"
}

func cargoWorkspace(dir string) (cargoMeta, error) {
	if _, err := exec.LookPath("cargo"); err != nil {
		return cargoMeta{}, errors.New("cargo is not on the PATH, and rust-analyzer runs it to read the workspace; install Rust with rustup from https://rustup.rs (on Windows, rustup-init.exe; on Linux or macOS, the curl line there), or your OS's rust package")
	}
	cmd := exec.Command("cargo", "metadata", "--format-version", "1", "--no-deps", "--offline")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return cargoMeta{}, fmt.Errorf("cargo metadata failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return cargoMeta{}, err
	}
	var m cargoMeta
	if err := json.Unmarshal(out, &m); err != nil {
		return cargoMeta{}, fmt.Errorf("cargo metadata output: %w", err)
	}
	return m, nil
}

// rustFiles are the .rs files under the relater's directory that a unit
// holds, root-relative and sorted, with the tests, benches, examples and
// build scripts left out. A target directory is build output.
func rustFiles(t tree, o owners, md cargoMeta) ([]string, error) {
	var out []string
	dirs, files := md.aside()
	err := filepath.WalkDir(t.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != t.dir && (d.Name() == "target" || strings.HasPrefix(d.Name(), ".") || slices.ContainsFunc(dirs, func(a string) bool { return sameFile(a, p) })) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".rs") || slices.ContainsFunc(files, func(a string) bool { return sameFile(a, p) }) {
			return nil
		}
		if r, ok := t.rel(p); ok {
			if _, ok := o.of(r); ok {
				out = append(out, r)
			}
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

// sameFile compares two absolute paths as the file system does: cargo
// prints a Windows drive letter in either case.
func sameFile(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// rustRun is what one configuration read: its facts, and the files the
// server compiled.
type rustRun struct {
	facts []fact
	read  map[string]bool
}

// rustSymbol is a declaration other code can name, in a file the server
// compiled.
type rustSymbol struct {
	path string
	sym  lsp.DocumentSymbol
	unit string
	// trait is the trait that declares a method, or empty.
	trait string
}

// callable is whether naming the symbol outside a use declaration runs it.
func (s rustSymbol) callable() bool {
	return s.sym.Kind == lsp.KindFunction || s.sym.Kind == lsp.KindMethod || s.sym.Kind == lsp.KindConstructor
}

// rustRequests bounds the requests in flight at once.
const rustRequests = 8

func rustRead(t tree, o owners, ra string, c rustConfig, files []string, crates []rustCrate) (rustRun, error) {
	init := map[string]any{
		"cargo": map[string]any{
			"features": c.features, "target": rustTarget, "allTargets": false,
			"buildScripts": map[string]any{"enable": false},
		},
		"procMacro":    map[string]any{"enable": false},
		"cfg":          map[string]any{"setTest": false},
		"checkOnSave":  false,
		"diagnostics":  map[string]any{"enable": false},
		"cachePriming": map[string]any{"enable": true},
	}
	ctx := context.Background()
	srv, err := lsp.Start(ctx, lsp.Server{
		Argv: []string{ra}, Root: t.dir, Init: init, Settings: map[string]any{"rust-analyzer": init},
		Timeout: 5 * time.Minute, Ready: 2 * time.Hour, Until: "Indexing",
	})
	if err != nil {
		return rustRun{}, err
	}
	defer srv.Close()
	if err := srv.Wait(ctx, time.Second); err != nil {
		return rustRun{}, err
	}
	r := rustReader{tree: t, owners: o, srv: srv, run: rustRun{read: map[string]bool{}}, src: map[string]rustSource{}, decl: map[string]bool{}, shown: map[link]bool{}}
	if err := r.read(ctx, files, crates); err != nil {
		return rustRun{}, err
	}
	r.run.facts = slices.DeleteFunc(r.run.facts, func(x fact) bool { return !r.shown[x.link] })
	return r.run, nil
}

type rustReader struct {
	tree
	owners
	srv *lsp.Client
	mu  sync.Mutex
	run rustRun
	src map[string]rustSource
	// decl holds path:line:column of every declaration's name, since the
	// server lists a trait method's implementations among its references.
	decl map[string]bool
	// shown are the links some reference outside a private use declaration
	// shows. The server resolves the use declarations of a cfg-inactive
	// module, such as cfg(test) tests, and nothing else in it, so a link
	// that only private uses show may live in code no build compiles.
	shown map[link]bool
}

func (r *rustReader) abs(p string) string { return filepath.Join(r.root, filepath.FromSlash(p)) }

func (r *rustReader) read(ctx context.Context, files []string, crates []rustCrate) error {
	// A file the server compiled has a parent module, or is a crate root
	// whose parent is its manifest; one no module tree reaches has none.
	err := parallel(len(files), func(i int) error {
		var parents []json.RawMessage
		pos := map[string]any{"textDocument": map[string]any{"uri": lsp.URI(r.abs(files[i]))}, "position": lsp.Position{}}
		if err := r.srv.Call(ctx, "experimental/parentModule", pos, &parents); err != nil {
			return err
		}
		if len(parents) > 0 {
			r.mu.Lock()
			r.run.read[files[i]] = true
			r.mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return err
	}
	var compiled []string
	for _, p := range files {
		if r.run.read[p] {
			compiled = append(compiled, p)
		}
	}
	symbols := make([][]rustSymbol, len(compiled))
	err = parallel(len(compiled), func(i int) error {
		p := compiled[i]
		b, err := os.ReadFile(r.abs(p))
		if err != nil {
			return err
		}
		syms, err := r.srv.DocumentSymbols(ctx, r.abs(p))
		if err != nil {
			return err
		}
		unit, _ := r.of(p)
		r.mu.Lock()
		defer r.mu.Unlock()
		r.src[p] = readRust(b)
		// The items of a trait impl are never asked about: the server files
		// a call to one under the trait's method, and lists every binding of
		// the trait's associated type, any impl's, under each impl's type.
		var walk func(list []lsp.DocumentSymbol, trait string, ask bool)
		walk = func(list []lsp.DocumentSymbol, trait string, ask bool) {
			for _, s := range list {
				r.decl[site3(p, s.SelectionRange.Start)] = true
				switch s.Kind {
				case lsp.KindModule:
					walk(s.Children, "", ask)
					continue
				case lsp.KindObject:
					walk(s.Children, "", ask && !strings.Contains(s.Name, " for "))
					continue
				case lsp.KindField, lsp.KindEnumMember, lsp.KindVariable:
					continue
				case lsp.KindTypeParameter:
					// A trait's associated type lists every Self of
					// every impl among its references.
					if trait != "" {
						continue
					}
				}
				if ask {
					symbols[i] = append(symbols[i], rustSymbol{path: p, sym: s, unit: unit, trait: trait})
				}
				if s.Kind == lsp.KindInterface {
					walk(s.Children, s.Name, ask)
				} else {
					walk(s.Children, "", ask)
				}
			}
		}
		walk(syms, "", true)
		return nil
	})
	if err != nil {
		return err
	}
	all := slices.Concat(symbols...)
	if err := parallel(len(all), func(i int) error { return r.uses(ctx, all[i]) }); err != nil {
		return err
	}
	if err := parallel(len(crates), func(i int) error { return r.named(ctx, crates[i], compiled) }); err != nil {
		return err
	}
	return parallel(len(compiled), func(i int) error { return r.exports(ctx, compiled[i]) })
}

// named adds an import from every unit whose code names the crate in a
// path, as through a facade crate that re-exports others, where the
// symbols' own references lead past it. The server lists a crate's
// references when asked at any path that names it, so it is asked at the
// first few such paths in another unit's code until one answers.
func (r *rustReader) named(ctx context.Context, c rustCrate, compiled []string) error {
	tries := 0
	for _, p := range compiled {
		if u, _ := r.of(p); u == c.unit {
			continue
		}
		r.mu.Lock()
		src := r.src[p]
		r.mu.Unlock()
		for line, text := range src.lines {
			for col := strings.Index(text, c.name+"::"); col >= 0; {
				if (col == 0 || !ident(text[col-1])) && src.at(line, col) != inText {
					refs, err := r.srv.References(ctx, r.abs(p), lsp.Position{Line: line, Character: col})
					if err != nil {
						return err
					}
					if len(refs) > 0 {
						r.crateRefs(c, refs)
						return nil
					}
					if tries++; tries == 3 {
						return nil
					}
				}
				next := strings.Index(text[col+1:], c.name+"::")
				if next < 0 {
					break
				}
				col += 1 + next
			}
		}
	}
	return nil
}

func (r *rustReader) crateRefs(c rustCrate, refs []lsp.Location) {
	for _, l := range refs {
		p, u, line, ok := r.place(l)
		if !ok || u == c.unit {
			continue
		}
		r.mu.Lock()
		k := r.src[p].at(l.Range.Start.Line, l.Range.Start.Character)
		r.mu.Unlock()
		if k != inText {
			r.add(fact{link{u, c.unit, model.DependsOn}, "import", site{path: p, line: line}}, k != inUse)
		}
	}
}

// exports adds an import from the file's unit to the unit whose crate or
// module each of its pub use or pub extern crate declarations re-exports
// from, as the server resolves the first path segment.
func (r *rustReader) exports(ctx context.Context, p string) error {
	r.mu.Lock()
	src := r.src[p]
	r.mu.Unlock()
	from, _ := r.of(p)
	for _, at := range src.exports {
		defs, err := r.srv.Definitions(ctx, r.abs(p), at)
		if err != nil {
			return err
		}
		for _, d := range defs {
			if _, to, _, ok := r.place(d); ok && to != from {
				r.add(fact{link{from, to, model.DependsOn}, "import", site{path: p, line: at.Line + 1}}, true)
			}
		}
	}
	return nil
}

func site3(p string, pos lsp.Position) string {
	return fmt.Sprintf("%s:%d:%d", p, pos.Line, pos.Character)
}

// uses adds a fact for every place another unit names s: a macro where s is
// a macro invoked with a bang or a proc macro named in an attribute, a call
// where s is a function or method named in code, else an import; and for a
// trait, a trait fact at every implementation another unit holds.
func (r *rustReader) uses(ctx context.Context, s rustSymbol) error {
	refs, err := r.srv.References(ctx, r.abs(s.path), s.sym.SelectionRange.Start)
	if err != nil {
		return err
	}
	ifc, contract := r.contract(s.path, s.trait)
	for _, l := range refs {
		p, u, line, ok := r.place(l)
		if !ok || r.decl[site3(p, l.Range.Start)] {
			continue
		}
		r.mu.Lock()
		src := r.src[p]
		r.mu.Unlock()
		k := src.at(l.Range.Start.Line, l.Range.Start.Character)
		// Self names the type the impl is for, and the server lists it
		// under any alias of that type, in any unit.
		if k == inText || src.text(l.Range) == "Self" {
			continue
		}
		via := model.Via("import")
		switch {
		case s.callable() && (k == inAttr || src.bang(l.Range.End)):
			via = "macro"
		case s.callable() && k == inCode:
			via = "call"
		}
		// A call through a contract trait consumes it, from the declaring
		// unit too, as a host calls through its own plug point.
		if via == "call" && contract && ifc != u {
			r.add(fact{link{u, ifc, model.Consumes}, "trait", site{path: p, line: line}}, true)
		}
		if u != s.unit {
			r.add(fact{link{u, s.unit, model.DependsOn}, via, site{path: p, line: line}}, k != inUse)
		}
	}
	if s.sym.Kind != lsp.KindInterface {
		return nil
	}
	impls, err := r.srv.Implementations(ctx, r.abs(s.path), s.sym.SelectionRange.Start)
	if err != nil {
		return err
	}
	ifc, contract = r.contract(s.path, s.sym.Name)
	for _, l := range impls {
		p, u, line, ok := r.place(l)
		if !ok || u == s.unit {
			continue
		}
		if contract {
			r.add(fact{link{u, ifc, model.Provides}, "trait", site{path: p, line: line}}, true)
		} else {
			r.add(fact{link{u, s.unit, model.DependsOn}, "trait", site{path: p, line: line}}, true)
		}
	}
	return nil
}

// place is the compiled file a location sits in, its unit and its 1-based
// line, and false for a place outside the units' compiled code.
func (r *rustReader) place(l lsp.Location) (string, string, int, bool) {
	abs, err := lsp.Path(l.URI)
	if err != nil {
		return "", "", 0, false
	}
	p, ok := r.rel(abs)
	if !ok {
		return "", "", 0, false
	}
	r.mu.Lock()
	compiled := r.run.read[p]
	r.mu.Unlock()
	u, ok := r.of(p)
	return p, u, l.Range.Start.Line + 1, ok && compiled
}

func (r *rustReader) contract(path, trait string) (string, bool) {
	if trait == "" {
		return "", false
	}
	return r.owners.contract(path, trait)
}

func (r *rustReader) add(x fact, shows bool) {
	r.mu.Lock()
	r.run.facts = append(r.run.facts, x)
	if shows {
		r.shown[x.link] = true
	}
	r.mu.Unlock()
}

// parallel runs f over 0..n-1 on rustRequests workers and returns the first
// error.
func parallel(n int, f func(i int) error) error {
	var (
		wg    sync.WaitGroup
		once  sync.Once
		first error
	)
	next := make(chan int)
	for range rustRequests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				if err := f(i); err != nil {
					once.Do(func() { first = err })
				}
			}
		}()
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
	return first
}
