package relate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/whyjp/abbd/internal/extract/frontend"
	"github.com/whyjp/abbd/internal/extract/lsp"
)

// cManifests are what a C relater's directory holds: a compile database,
// read as it is, or a CMake project, configured where the cmake front-end
// configures it and its compile commands read from the file API.
var cManifests = []string{"compile_commands.json", "CMakeLists.txt"}

// cInstall says how to get clangd on each OS.
const cInstall = "set ABBD_LSP_C to its path, or install it: the release for your OS from https://github.com/clangd/clangd/releases, " +
	"`apt install clangd` on Debian or Ubuntu, `brew install llvm` on macOS (clangd is in $(brew --prefix llvm)/bin), or `winget install LLVM.LLVM` on Windows"

// cSources and cHeaders are the file names the C relater reads, by
// extension. A source compiles on its own and must have an entry in the
// compile database; a header is read through the sources that include it.
var (
	cSources = []string{".c", ".cc", ".cpp", ".cxx", ".c++", ".m", ".mm"}
	cHeaders = []string{".h", ".hh", ".hpp", ".hxx", ".h++", ".inl", ".ipp", ".tcc"}
)

func cExt(p string, exts []string) bool { return slices.Contains(exts, strings.ToLower(path.Ext(p))) }

// cTest is whether a path names test, benchmark or fuzz code, which the
// relater leaves out by design, so it is no gap worth reporting.
func cTest(p string) bool {
	dirs := strings.Split(strings.ToLower(p), "/")
	name := strings.TrimSuffix(dirs[len(dirs)-1], path.Ext(p))
	for _, d := range dirs[:len(dirs)-1] {
		switch d {
		case "test", "tests", "testing", "testdata", "unittest", "unittests", "bench", "benches", "benchmark", "benchmarks", "fuzz", "fuzzing":
			return true
		}
	}
	for _, s := range []string{"_test", "_tests", "_unittest", "_bench", "_benchmark", "_fuzzer", "-test", "-tests"} {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return strings.HasPrefix(name, "test_") || name == "test" || name == "tests"
}

// clangdPath is the server to run: $ABBD_LSP_C, else clangd on the PATH,
// asked its version first so a broken install fails here.
func clangdPath() (string, error) {
	c := os.Getenv("ABBD_LSP_C")
	if c == "" {
		p, err := exec.LookPath("clangd")
		if err != nil {
			return "", fmt.Errorf("clangd is not on the PATH; %s", cInstall)
		}
		c = p
	}
	if out, err := exec.Command(c, "--version").CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s --version failed (%v: %s); %s", c, err, strings.TrimSpace(string(out)), cInstall)
	}
	return c, nil
}

// cEntry is one compile database entry: the file it compiles, absolute,
// and the entry as clangd reads it.
type cEntry struct {
	file string
	raw  map[string]any
}

// cCompiles reads how the build compiles each source: from the directory's
// compile_commands.json when it holds one, else from its CMake project.
func cCompiles(t tree) ([]cEntry, error) {
	db := filepath.Join(t.dir, "compile_commands.json")
	if _, err := os.Stat(db); err != nil {
		cs, err := frontend.CMakeCompiles(t.root, t.dir, t.cache)
		if err != nil {
			return nil, err
		}
		var out []cEntry
		for _, c := range cs {
			out = append(out, cEntry{c.File, map[string]any{"directory": c.Dir, "file": c.File, "arguments": c.Args}})
		}
		return out, nil
	}
	b, err := os.ReadFile(db)
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.ToSlash(filepath.Join(t.dir, "compile_commands.json")), err)
	}
	var out []cEntry
	for _, e := range raw {
		dir, _ := e["directory"].(string)
		file, _ := e["file"].(string)
		if file == "" {
			continue
		}
		f := filepath.FromSlash(file)
		if !filepath.IsAbs(f) {
			f = filepath.Join(filepath.FromSlash(dir), f)
		}
		out = append(out, cEntry{filepath.Clean(f), e})
	}
	return out, nil
}

// cDatabase writes the compile database clangd reads into the cache: the
// entries for sources under the root that a unit holds and that are no
// test, one per file, the first the build lists. clangd keeps its index
// beside the database, so it lands in the cache too, never in the
// repository. It returns the database's directory and the root-relative
// sources it compiles.
func cDatabase(t tree, o owners, entries []cEntry) (string, map[string]bool, error) {
	compiled := map[string]bool{}
	var keep []map[string]any
	for _, e := range entries {
		r, ok := t.rel(e.file)
		if !ok || compiled[r] || cTest(r) || !cExt(r, cSources) && !cExt(r, cHeaders) {
			continue
		}
		if _, ok := o.of(r); !ok {
			continue
		}
		compiled[r] = true
		keep = append(keep, e.raw)
	}
	h := sha256.Sum256([]byte(strings.ToLower(filepath.ToSlash(t.root + "\n" + t.dir))))
	dir := filepath.Join(t.cache, "clangd", hex.EncodeToString(h[:6]))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	b, err := json.MarshalIndent(keep, "", " ")
	if err != nil {
		return "", nil, err
	}
	return dir, compiled, os.WriteFile(filepath.Join(dir, "compile_commands.json"), b, 0o644)
}

// cFiles are the C and C++ files a unit holds, root-relative and sorted,
// tests left out.
func cFiles(t tree, o owners) ([]string, error) {
	var out []string
	err := filepath.WalkDir(t.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != t.root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		r, ok := t.rel(p)
		if !ok || cTest(r) || !cExt(r, cSources) && !cExt(r, cHeaders) {
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

// clang reads C and C++ code with clangd over one compile database. Every
// file a unit holds that the database compiles, or a header, is opened;
// the declarations other files can name are asked for their references,
// and each reference becomes a call, a callback, a macro or an import of
// the unit that defines the name. A source the database does not compile
// is unread, never guessed at.
func clang(t tree, o owners) (found, error) {
	if inside(t.root, t.cache) {
		return found{}, fmt.Errorf("the cache %s lies inside the repository; set ABBD_CACHE_DIR to a directory outside it", t.cache)
	}
	bin, err := clangdPath()
	if err != nil {
		return found{}, err
	}
	entries, err := cCompiles(t)
	if err != nil {
		return found{}, err
	}
	db, compiled, err := cDatabase(t, o, entries)
	if err != nil {
		return found{}, err
	}
	files, err := cFiles(t, o)
	if err != nil {
		return found{}, err
	}
	f := found{covered: map[string]bool{}, when: map[link]string{}}
	var read []string
	for _, p := range files {
		if !compiled[p] && !cExt(p, cHeaders) {
			f.unread = append(f.unread, p)
			continue
		}
		read = append(read, p)
		u, _ := o.of(p)
		f.covered[u] = true
	}
	if len(compiled) == 0 {
		return found{}, fmt.Errorf("the compile database compiles no source a unit holds (%d entries)", len(entries))
	}
	r := &cReader{tree: t, owners: o, read: map[string]bool{}, src: map[string][]string{}, starts: map[string][]int{}, classes: map[string][]string{}, syms: map[string][]lsp.DocumentSymbol{}, gated: map[string][]lsp.Range{}}
	for _, p := range read {
		r.read[p] = true
	}
	r.order = read
	ctx := context.Background()
	r.srv, err = lsp.Start(ctx, lsp.Server{
		Argv: []string{bin, "--compile-commands-dir=" + db, "--background-index", "--enable-config=false",
			"--limit-references=0", "--limit-results=0", "--header-insertion=never", "--clang-tidy=false", "--log=error",
			fmt.Sprintf("-j=%d", max(2, runtime.NumCPU()))},
		Root: t.root, Timeout: 5 * time.Minute, Ready: 2 * time.Hour, Quiet: 20 * time.Second,
		TextDocument: map[string]any{"inactiveRegionsCapabilities": map[string]any{"inactiveRegions": true}},
		Note:         r.note,
	})
	if err != nil {
		return found{}, err
	}
	defer r.srv.Close()
	facts, err := r.relate(ctx, read)
	if err != nil {
		return found{}, err
	}
	f.facts = facts
	f.gated = r.gatedLines()
	return f, nil
}

func inside(root, p string) bool {
	r, err := filepath.Rel(root, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

// cReader is one clangd session over the files it reads.
type cReader struct {
	tree
	owners
	srv  *lsp.Client
	read map[string]bool
	// order is the files read, sorted.
	order []string
	mu    sync.Mutex
	src   map[string][]string
	// starts holds the byte offset each line of a file starts at.
	starts map[string][]int
	// classes maps a type's name to the headers that declare it.
	classes map[string][]string
	syms    map[string][]lsp.DocumentSymbol
	// gated holds the ranges clangd reports its configuration leaves out of
	// each open file, by #if.
	gated map[string][]lsp.Range
	facts []fact
}

func (r *cReader) abs(p string) string { return filepath.Join(r.root, filepath.FromSlash(p)) }

// note keeps clangd's inactive regions of each file it opened.
func (r *cReader) note(method string, params json.RawMessage) {
	if method != "textDocument/inactiveRegions" {
		return
	}
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Regions []lsp.Range `json:"regions"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	abs, err := lsp.Path(p.TextDocument.URI)
	if err != nil {
		return
	}
	if rel, ok := r.rel(abs); ok {
		r.mu.Lock()
		r.gated[rel] = p.Regions
		r.mu.Unlock()
	}
}

// gatedLines names each read file with code its configuration leaves out,
// with how many lines, as path (N lines).
func (r *cReader) gatedLines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for p, rs := range r.gated {
		n := 0
		for _, g := range rs {
			n += g.End.Line - g.Start.Line + 1
		}
		if n > 0 {
			out = append(out, fmt.Sprintf("%s (%d lines)", p, n))
		}
	}
	slices.Sort(out)
	return out
}

func (r *cReader) lines(p string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.src[p]
}

func (r *cReader) add(x fact) {
	r.mu.Lock()
	r.facts = append(r.facts, x)
	r.mu.Unlock()
}
